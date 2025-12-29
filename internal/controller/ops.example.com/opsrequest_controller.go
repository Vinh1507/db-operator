package opsexamplecom

import (
	"context"
	"fmt"
	"time"

	opsv1alpha1 "github.com/Vinh1507/db-operator/api/ops/v1alpha1"
	everestv1alpha1 "github.com/Vinh1507/db-operator/api/v1alpha1"
	"github.com/Vinh1507/db-operator/internal/controller/providers"
	"github.com/Vinh1507/db-operator/internal/controller/providers/cnpg"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	opsRequestFinalizer = "ops.example.com/finalizer"
	defaultTimeout      = 30 * time.Minute
)

type OpsRequestReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	// ProviderFactory *providers.ProviderFactory
}

// +kubebuilder:rbac:groups=ops.example.com,resources=opsrequests,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=ops.example.com,resources=opsrequests/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=ops.example.com,resources=opsrequests/finalizers,verbs=update
// +kubebuilder:rbac:groups=everest.example.com,resources=clusters,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=postgresql.cnpg.io,resources=clusters,verbs=get;list;watch;update;patch

func (r *OpsRequestReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch OpsRequest
	opsRequest := &opsv1alpha1.OpsRequest{}
	if err := r.Get(ctx, req.NamespacedName, opsRequest); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Handle deletion
	if !opsRequest.GetDeletionTimestamp().IsZero() {
		return r.handleDeletion(ctx, opsRequest)
	}

	// Add finalizer
	if !controllerutil.ContainsFinalizer(opsRequest, opsRequestFinalizer) {
		controllerutil.AddFinalizer(opsRequest, opsRequestFinalizer)
		if err := r.Update(ctx, opsRequest); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Skip if already completed
	if opsRequest.IsComplete() {
		logger.Info("OpsRequest already completed", "phase", opsRequest.Status.Phase)
		return ctrl.Result{}, nil
	}

	// Initialize status if needed
	if opsRequest.Status.Phase == "" {
		opsRequest.Status.Phase = opsv1alpha1.OpsRequestPhasePending
		opsRequest.Status.ObservedGeneration = opsRequest.Generation
		opsRequest.Status.StartTime = &metav1.Time{Time: time.Now()}
		if err := r.updateStatus(ctx, opsRequest); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// Check timeout
	if r.isTimedOut(opsRequest) {
		logger.Info("OpsRequest timed out")
		return r.markAsFailed(ctx, opsRequest, "Timeout", "Operation timed out")
	}

	// Get target cluster
	cluster := &everestv1alpha1.Cluster{}
	if err := r.Get(ctx, types.NamespacedName{
		Name:      opsRequest.Spec.ClusterRef.Name,
		Namespace: opsRequest.GetTargetNamespace(),
	}, cluster); err != nil {
		if k8serrors.IsNotFound(err) {
			return r.markAsFailed(ctx, opsRequest, "ClusterNotFound",
				fmt.Sprintf("Cluster %s not found", opsRequest.Spec.ClusterRef.Name))
		}
		return ctrl.Result{}, err
	}

	// Find target engine in cluster
	var targetEngine *everestv1alpha1.EngineSpec
	if opsRequest.Spec.EngineRef != nil {
		for i := range cluster.Spec.Engines {
			if cluster.Spec.Engines[i].Name == opsRequest.Spec.EngineRef.Name {
				targetEngine = &cluster.Spec.Engines[i]
				break
			}
		}
		if targetEngine == nil {
			return r.markAsFailed(ctx, opsRequest, "EngineNotFound",
				fmt.Sprintf("Engine %s not found in cluster", opsRequest.Spec.EngineRef.Name))
		}
	} else {
		// If no engine specified, operations apply to all engines
		// For simplicity, we'll use the first database engine
		for i := range cluster.Spec.Engines {
			if cluster.Spec.Engines[i].Category == everestv1alpha1.CategoryDatabase {
				targetEngine = &cluster.Spec.Engines[i]
				break
			}
		}
		if targetEngine == nil {
			return r.markAsFailed(ctx, opsRequest, "NoEngineFound",
				"No database engine found in cluster")
		}
	}

	// Execute operation
	return r.executeOperation(ctx, opsRequest, cluster, targetEngine)
}

func (r *OpsRequestReconciler) executeOperation(
	ctx context.Context,
	opsRequest *opsv1alpha1.OpsRequest,
	cluster *everestv1alpha1.Cluster,
	engine *everestv1alpha1.EngineSpec,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// // Create provider based on engine type
	// provider, err := r.ProviderFactory.NewProvider(ctx, cluster, *engine)
	// Create provider based on engine type
	var provider interface {
		Apply(ctx context.Context) cnpg.Applier
		RunPreReconcileHook(ctx context.Context) (bool, time.Duration, string, error)
		Status(ctx context.Context) (everestv1alpha1.EngineStatus, error)
		DBObject() client.Object
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

	// Get applier
	applier := provider.Apply(ctx)

	// Mark as processing if not already
	if opsRequest.Status.Phase == opsv1alpha1.OpsRequestPhasePending {
		// Validate action
		if err := applier.ValidateAction(opsRequest.Spec.Type, opsRequest.Spec.Params); err != nil {
			logger.Error(err, "Action validation failed")
			return r.markAsFailed(ctx, opsRequest, "ValidationFailed", err.Error())
		}

		opsRequest.Status.Phase = opsv1alpha1.OpsRequestPhaseProcessing
		opsRequest.Status.Message = "Starting operation"
		opsRequest.Status.Progress = 10

		meta.SetStatusCondition(&opsRequest.Status.Conditions, metav1.Condition{
			Type:               "Processing",
			Status:             metav1.ConditionTrue,
			Reason:             "OperationStarted",
			Message:            fmt.Sprintf("Started %s operation", opsRequest.Spec.Type),
			ObservedGeneration: opsRequest.Generation,
		})

		if err := r.updateStatus(ctx, opsRequest); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Execute action based on type
	var result providers.ActionResult
	var err error

	switch opsRequest.Spec.Type {
	case opsv1alpha1.ActionStart:
		result, err = applier.Start(ctx, opsRequest.Spec.Params)
	case opsv1alpha1.ActionStop:
		result, err = applier.Stop(ctx, opsRequest.Spec.Params)
	case opsv1alpha1.ActionRestart:
		result, err = applier.Restart(ctx, opsRequest.Spec.Params)
	case opsv1alpha1.ActionHorizontalScaling:
		result, err = applier.HorizontalScale(ctx, opsRequest.Spec.Params)
	case opsv1alpha1.ActionVerticalScaling:
		result, err = applier.VerticalScale(ctx, opsRequest.Spec.Params)
	case opsv1alpha1.ActionVolumeExpansion:
		result, err = applier.VolumeExpansion(ctx, opsRequest.Spec.Params)
	case opsv1alpha1.ActionReconfiguring:
		result, err = applier.Reconfigure(ctx, opsRequest.Spec.Params)
	case opsv1alpha1.ActionUpgrade:
		result, err = applier.Upgrade(ctx, opsRequest.Spec.Params)
	case opsv1alpha1.ActionBackup:
		result, err = applier.Backup(ctx, opsRequest.Spec.Params)
	case opsv1alpha1.ActionRestore:
		result, err = applier.Restore(ctx, opsRequest.Spec.Params)
	case opsv1alpha1.ActionExpose:
		result, err = applier.ExposeService(ctx, opsRequest.Spec.Params)
	case opsv1alpha1.ActionSwitchover:
		result, err = applier.Switchover(ctx, opsRequest.Spec.Params)
	case opsv1alpha1.ActionCustom:
		result, err = applier.Custom(ctx, opsRequest.Spec.Params)
	default:
		return r.markAsFailed(ctx, opsRequest, "UnsupportedAction",
			fmt.Sprintf("Action type %s is not supported", opsRequest.Spec.Type))
	}

	if err != nil {
		logger.Error(err, "Operation execution failed")
		return r.markAsFailed(ctx, opsRequest, "ExecutionFailed", err.Error())
	}

	// Update progress
	opsRequest.Status.Progress = result.Progress
	opsRequest.Status.Message = result.Message

	if result.Output != nil {
		opsRequest.Status.Output = result.Output
	}

	// Check if completed
	if result.Completed {
		opsRequest.Status.Phase = opsv1alpha1.OpsRequestPhaseSucceeded
		opsRequest.Status.Progress = 100
		opsRequest.Status.CompletionTime = &metav1.Time{Time: time.Now()}

		meta.SetStatusCondition(&opsRequest.Status.Conditions, metav1.Condition{
			Type:               "Completed",
			Status:             metav1.ConditionTrue,
			Reason:             "OperationSucceeded",
			Message:            "Operation completed successfully",
			ObservedGeneration: opsRequest.Generation,
		})

		logger.Info("Operation completed successfully")
	}

	if err := r.updateStatus(ctx, opsRequest); err != nil {
		return ctrl.Result{}, err
	}

	// Requeue if not completed
	if !result.Completed {
		return ctrl.Result{RequeueAfter: result.RequeueAfter}, nil
	}

	return ctrl.Result{}, nil
}

func (r *OpsRequestReconciler) markAsFailed(
	ctx context.Context,
	opsRequest *opsv1alpha1.OpsRequest,
	reason, message string,
) (ctrl.Result, error) {
	opsRequest.Status.Phase = opsv1alpha1.OpsRequestPhaseFailed
	opsRequest.Status.Reason = reason
	opsRequest.Status.Message = message
	opsRequest.Status.CompletionTime = &metav1.Time{Time: time.Now()}

	meta.SetStatusCondition(&opsRequest.Status.Conditions, metav1.Condition{
		Type:               "Failed",
		Status:             metav1.ConditionTrue,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: opsRequest.Generation,
	})

	if err := r.updateStatus(ctx, opsRequest); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *OpsRequestReconciler) updateStatus(
	ctx context.Context,
	opsRequest *opsv1alpha1.OpsRequest,
) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		latest := &opsv1alpha1.OpsRequest{}
		if err := r.Get(ctx, client.ObjectKeyFromObject(opsRequest), latest); err != nil {
			return err
		}

		latest.Status = opsRequest.Status
		return r.Status().Update(ctx, latest)
	})
}

func (r *OpsRequestReconciler) isTimedOut(opsRequest *opsv1alpha1.OpsRequest) bool {
	if opsRequest.Status.StartTime == nil {
		return false
	}

	timeout := defaultTimeout
	if opsRequest.Spec.Timeout != "" {
		if d, err := time.ParseDuration(opsRequest.Spec.Timeout); err == nil {
			timeout = d
		}
	}

	return time.Since(opsRequest.Status.StartTime.Time) > timeout
}

func (r *OpsRequestReconciler) handleDeletion(
	ctx context.Context,
	opsRequest *opsv1alpha1.OpsRequest,
) (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(opsRequest, opsRequestFinalizer) {
		return ctrl.Result{}, nil
	}

	// Cancel operation if still processing
	if opsRequest.IsProcessing() {
		opsRequest.Status.Phase = opsv1alpha1.OpsRequestPhaseCanceled
		opsRequest.Status.Message = "Operation canceled by deletion"
		opsRequest.Status.CompletionTime = &metav1.Time{Time: time.Now()}

		if err := r.updateStatus(ctx, opsRequest); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Remove finalizer
	controllerutil.RemoveFinalizer(opsRequest, opsRequestFinalizer)
	if err := r.Update(ctx, opsRequest); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *OpsRequestReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&opsv1alpha1.OpsRequest{}).
		Named("opsrequest").
		Complete(r)
}
