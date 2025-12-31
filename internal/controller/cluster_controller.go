// internal/controller/cluster_controller.go
package controller

import (
	"context"
	"fmt"
	"time"

	everestv1alpha1 "github.com/Vinh1507/db-operator/api/v1alpha1"
	"github.com/Vinh1507/db-operator/internal/controller/providers/cnpg"
	cnpgv1 "github.com/cloudnative-pg/cloudnative-pg/api/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	clusterFinalizer    = "everest.example.com/cluster-finalizer"
	defaultRequeueAfter = 30 * time.Second
)

// ClusterReconciler reconciles a Cluster object
type ClusterReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=everest.example.com,resources=clusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=everest.example.com,resources=clusters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=everest.example.com,resources=clusters/finalizers,verbs=update
// +kubebuilder:rbac:groups=postgresql.cnpg.io,resources=clusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=postgresql.cnpg.io,resources=clusters/status,verbs=get

func (r *ClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch Cluster
	cluster := &everestv1alpha1.Cluster{}
	if err := r.Get(ctx, req.NamespacedName, cluster); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Handle deletion
	if !cluster.GetDeletionTimestamp().IsZero() {
		return r.handleDeletion(ctx, cluster)
	}

	// Add finalizer
	if !controllerutil.ContainsFinalizer(cluster, clusterFinalizer) {
		controllerutil.AddFinalizer(cluster, clusterFinalizer)
		if err := r.Update(ctx, cluster); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Reconcile each engine
	for i := range cluster.Spec.Engines {
		engine := &cluster.Spec.Engines[i]

		result, err := r.reconcileEngine(ctx, cluster, engine)
		if err != nil {
			logger.Error(err, "Failed to reconcile engine", "engine", engine.Name)
			return result, err
		}

		if result.Requeue || result.RequeueAfter > 0 {
			return result, nil
		}
	}

	// Update cluster status
	return r.updateClusterStatus(ctx, cluster)
}

func (r *ClusterReconciler) reconcileEngine(
	ctx context.Context,
	cluster *everestv1alpha1.Cluster,
	engine *everestv1alpha1.EngineSpec,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Create provider based on engine type
	var provider interface {
		Apply(ctx context.Context) cnpg.Applier
		RunPreReconcileHook(ctx context.Context) (bool, time.Duration, string, error)
		Status(ctx context.Context) (everestv1alpha1.EngineStatus, error)
		DBObjects() []client.Object
		SetName(string)
		SetNamespace(string)
	}

	switch engine.EngineType {
	case everestv1alpha1.EngineCNPG:
		p, err := cnpg.New(ctx, r.Client, r.Scheme, cluster, *engine)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to create CNPG provider: %w", err)
		}
		provider = p

	// case everestv1alpha1.EnginePXC:
	//     provider = pxc.New(...)
	// case everestv1alpha1.EngineProxySQL:
	//     provider = proxysql.New(...)

	default:
		return ctrl.Result{}, fmt.Errorf("unsupported engine type: %s", engine.EngineType)
	}

	// Run pre-reconcile hook
	shouldProceed, requeueAfter, message, err := provider.RunPreReconcileHook(ctx)
	if err != nil {
		return ctrl.Result{}, err
	}
	if !shouldProceed {
		logger.Info("Skipping reconciliation", "reason", message)
		return ctrl.Result{RequeueAfter: requeueAfter}, nil
	}

	// Set name and namespace
	provider.SetName(engine.Name)
	provider.SetNamespace(cluster.GetNamespace())

	// Create or update underlying CRs
	dbObjects := provider.DBObjects()
	for _, dbObject := range dbObjects {
		_, err = controllerutil.CreateOrUpdate(ctx, r.Client, dbObject, func() error {
			// Set owner reference
			if err := controllerutil.SetControllerReference(cluster, dbObject, r.Scheme); err != nil {
				return fmt.Errorf("failed to set controller reference: %w", err)
			}

			// Get applier
			applier := provider.Apply(ctx)

			if err := applier.Convert(dbObject); err != nil {
				return fmt.Errorf("failed to convert object: %w", err)
			}

			return nil
		})
	}
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to create or update engine: %w", err)
	}

	// Update engine status
	status, err := provider.Status(ctx)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get engine status: %w", err)
	}

	for i := range cluster.Status.Engines {
		if cluster.Status.Engines[i].Name == engine.Name {
			cluster.Status.Engines[i] = status
			break
		}
	}

	found := false
	for _, s := range cluster.Status.Engines {
		if s.Name == engine.Name {
			found = true
			break
		}
	}
	if !found {
		status.Name = engine.Name
		status.Type = string(engine.EngineType)
		cluster.Status.Engines = append(cluster.Status.Engines, status)
	}

	return ctrl.Result{}, nil
}

func (r *ClusterReconciler) updateClusterStatus(
	ctx context.Context,
	cluster *everestv1alpha1.Cluster,
) (ctrl.Result, error) {
	// Calculate overall cluster status
	allReady := true
	anyFailed := false

	for _, engine := range cluster.Status.Engines {
		if engine.Phase != "Ready" {
			allReady = false
		}
		if engine.Phase == "Failed" {
			anyFailed = true
		}
	}

	if anyFailed {
		cluster.Status.Phase = "Failed"
	} else if allReady && len(cluster.Status.Engines) == len(cluster.Spec.Engines) {
		cluster.Status.Phase = "Ready"
	} else {
		cluster.Status.Phase = "Initializing"
	}

	// ensure endpoints struct
	if cluster.Status.Endpoints == nil {
		cluster.Status.Endpoints = &everestv1alpha1.EngineEndpoints{}
	}

	// ensure slices
	if cluster.Status.Endpoints.Internal == nil {
		cluster.Status.Endpoints.Internal = []string{}
	}
	if cluster.Status.Endpoints.External == nil {
		cluster.Status.Endpoints.External = []string{}
	}

	// helper: append if missing
	appendIfMissing := func(list []string, v string) []string {
		for _, e := range list {
			if e == v {
				return list
			}
		}
		return append(list, v)
	}

	// internal endpoint
	cluster.Status.Endpoints.Internal = appendIfMissing(
		cluster.Status.Endpoints.Internal,
		"abc.cnpg.svc.cluster.local:3306",
	)

	// external endpoint (ví dụ)
	cluster.Status.Endpoints.External = appendIfMissing(
		cluster.Status.Endpoints.External,
		"abc.example.com:3306",
	)
	// Update status with retry
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		// Get latest version
		latest := &everestv1alpha1.Cluster{}
		if err := r.Get(ctx, client.ObjectKeyFromObject(cluster), latest); err != nil {
			return err
		}

		// Update status
		latest.Status = cluster.Status
		return r.Status().Update(ctx, latest)
	})

	if err != nil {
		return ctrl.Result{}, err
	}

	// Requeue if not ready
	if cluster.Status.Phase != "Ready" {
		return ctrl.Result{RequeueAfter: defaultRequeueAfter}, nil
	}

	return ctrl.Result{}, nil
}

func (r *ClusterReconciler) handleDeletion(
	ctx context.Context,
	cluster *everestv1alpha1.Cluster,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	if !controllerutil.ContainsFinalizer(cluster, clusterFinalizer) {
		return ctrl.Result{}, nil
	}

	// Clean up each engine
	for _, engine := range cluster.Spec.Engines {
		switch engine.EngineType {
		case everestv1alpha1.EngineCNPG:
			provider, err := cnpg.New(ctx, r.Client, r.Scheme, cluster, engine)
			if err != nil {
				if k8serrors.IsNotFound(err) {
					continue
				}
				return ctrl.Result{}, err
			}

			done, err := provider.Cleanup(ctx)
			if err != nil {
				logger.Error(err, "Failed to cleanup engine", "engine", engine.Name)
				return ctrl.Result{}, err
			}

			if !done {
				logger.Info("Engine cleanup in progress", "engine", engine.Name)
				return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
			}
		}
	}

	// Remove finalizer
	controllerutil.RemoveFinalizer(cluster, clusterFinalizer)
	if err := r.Update(ctx, cluster); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager
func (r *ClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&everestv1alpha1.Cluster{}).
		Owns(&cnpgv1.Cluster{}).
		Named("cluster").
		Complete(r)
}
