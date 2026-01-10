// internal/controller/cluster_controller.go
package controller

import (
	"context"
	"fmt"
	"time"

	everestv1alpha1 "github.com/Vinh1507/db-operator/api/v1alpha1"
	"github.com/Vinh1507/db-operator/internal/controller/providers/cnpg"
	cnpgv1 "github.com/cloudnative-pg/cloudnative-pg/api/v1"
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

type dbProvider interface {
	// metav1.Object
	Apply(ctx context.Context) cnpg.Applier
	RunPreReconcileHook(ctx context.Context, cluster everestv1alpha1.Cluster) (bool, time.Duration, string, error)
	Status(ctx context.Context) (everestv1alpha1.ClusterStatus, error)
}

func (r *ClusterReconciler) newDBProvider(
	ctx context.Context,
	cluster *everestv1alpha1.Cluster,
) (dbProvider, error) {
	switch cluster.Spec.ProviderType {
	case everestv1alpha1.ProviderCNPG:
		p, err := cnpg.New(ctx, r.Client, r.Scheme, cluster)
		return p, err
	default:
		return nil, fmt.Errorf("unsupported provider: %s", cluster.Spec.ProviderType)
	}
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

	provider, err := r.newDBProvider(ctx, cluster)
	if err != nil {
		return ctrl.Result{}, err
	}
	// Run pre-reconcile hook
	shouldProceed, requeueAfter, message, err := provider.RunPreReconcileHook(ctx, *cluster)
	if err != nil {
		return ctrl.Result{}, err
	}
	if !shouldProceed {
		logger.Info("Skipping reconciliation", "reason", message)
		return ctrl.Result{RequeueAfter: requeueAfter}, nil
	}
	applier := provider.Apply(ctx)

	// Create or update underlying CRs
	dbObjectsWithMutate := applier.DBObjects(ctx, r.Scheme)
	for _, item := range dbObjectsWithMutate {
		_, err = controllerutil.CreateOrUpdate(ctx, r.Client, item.Object, item.MutateFunc)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to create or update object: %w", err)
		}
	}
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to create or update engine: %w", err)
	}

	// Update cluster status
	return r.updateClusterStatus(ctx, cluster, provider)
}

func (r *ClusterReconciler) updateClusterStatus(
	ctx context.Context,
	cluster *everestv1alpha1.Cluster,
	p dbProvider,
) (ctrl.Result, error) {
	dbStatus, err := p.Status(ctx)
	if err != nil {
		// TODO
		return ctrl.Result{}, err
	}
	dbStatus.ObservedGeneration = cluster.GetGeneration()
	cluster.Status = dbStatus

	// Update status with retry
	err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
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
	// logger := log.FromContext(ctx)

	if !controllerutil.ContainsFinalizer(cluster, clusterFinalizer) {
		return ctrl.Result{}, nil
	}

	// Clean up each engine
	// for _, engine := range cluster.Spec.Engines {
	// 	switch engine.EngineType {
	// 	case everestv1alpha1.EngineCNPG:
	// 		provider, err := cnpg.New(ctx, r.Client, r.Scheme, cluster, engine)
	// 		if err != nil {
	// 			if k8serrors.IsNotFound(err) {
	// 				continue
	// 			}
	// 			return ctrl.Result{}, err
	// 		}

	// 		done, err := provider.Cleanup(ctx)
	// 		if err != nil {
	// 			logger.Error(err, "Failed to cleanup engine", "engine", engine.Name)
	// 			return ctrl.Result{}, err
	// 		}

	// 		if !done {
	// 			logger.Info("Engine cleanup in progress", "engine", engine.Name)
	// 			return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	// 		}
	// 	}
	// }

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
