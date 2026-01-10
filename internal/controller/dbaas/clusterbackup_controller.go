package dbaas

import (
	"context"
	"fmt"
	"time"

	dbaasv1alpha1 "github.com/Vinh1507/db-operator/api/dbaas/v1alpha1"
	v1alpha1 "github.com/Vinh1507/db-operator/api/v1alpha1"
	cnpgv1 "github.com/cloudnative-pg/cloudnative-pg/api/v1"

	provider "github.com/Vinh1507/db-operator/internal/controller/providers"
	cnpgprovider "github.com/Vinh1507/db-operator/internal/controller/providers/cnpg"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
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
	clusterBackupFinalizer = "clusterbackup.everest.percona.com/finalizer"
	requeueAfter           = 10 * time.Second
)

// ClusterBackupReconciler reconciles a ClusterBackup object
type ClusterBackupReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=everest.percona.com,resources=clusterbackups,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=everest.percona.com,resources=clusterbackups/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=everest.percona.com,resources=clusterbackups/finalizers,verbs=update
// +kubebuilder:rbac:groups=postgresql.cnpg.io,resources=backups,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=postgresql.cnpg.io,resources=clusters,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch

// Reconcile reconciles the ClusterBackup
func (r *ClusterBackupReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Reconciling ClusterBackup")
	fmt.Println("Reconciling ClusterBackup")

	// Fetch the ClusterBackup
	backup := &dbaasv1alpha1.ClusterBackup{}
	if err := r.Get(ctx, req.NamespacedName, backup); err != nil {
		if k8serrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get ClusterBackup")
		return ctrl.Result{}, err
	}

	// Fetch the Cluster
	cluster := &v1alpha1.Cluster{}
	if err := r.Get(ctx, types.NamespacedName{
		Name:      backup.Spec.ClusterName,
		Namespace: backup.Namespace,
	}, cluster); err != nil {
		if k8serrors.IsNotFound(err) {
			logger.Error(err, "Cluster not found", "cluster", backup.Spec.ClusterName)
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Get the appropriate backup provider based on engine type
	backupProvider, err := r.getBackupProvider(cluster)
	if err != nil {
		logger.Error(err, "Failed to get backup provider")
		return ctrl.Result{}, err
	}

	// Handle deletion
	if !backup.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, backup, backupProvider)
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(backup, clusterBackupFinalizer) {
		controllerutil.AddFinalizer(backup, clusterBackupFinalizer)
		if err := r.Update(ctx, backup); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// Set owner reference to cluster
	if err := r.ensureOwnerReference(ctx, backup, cluster); err != nil {
		return ctrl.Result{}, err
	}

	// Reconcile the backup
	requeue, err := r.reconcileBackup(ctx, backup, backupProvider)
	if err != nil {
		logger.Error(err, "Failed to reconcile backup")
		return ctrl.Result{}, err
	}

	// Update status
	if err := r.updateBackupStatus(ctx, backup, backupProvider); err != nil {
		logger.Error(err, "Failed to update backup status")
		return ctrl.Result{}, err
	}

	if requeue {
		return ctrl.Result{RequeueAfter: requeueAfter}, nil
	}

	return ctrl.Result{}, nil
}

// getBackupProvider returns the appropriate backup provider
func (r *ClusterBackupReconciler) getBackupProvider(cluster *v1alpha1.Cluster) (provider.BackupProvider, error) {
	// cnpgCluster := &cnpgv1.Cluster{}
	// err := c.Get(
	// 	ctx,
	// 	types.NamespacedName{
	// 		Name:      engineSpec.Name,
	// 		Namespace: cluster.GetNamespace(),
	// 	},
	// 	cnpgCluster,
	// )
	// if err != nil && !k8serrors.IsNotFound(err) {
	// 	return nil, err
	// }
	// Find the engine in cluster
	return cnpgprovider.NewBackupProvider(r.Client, r.Scheme), nil
	// for _, engine := range cluster.Spec.Engines {
	// 	// For now, we only support CNPG
	// 	// In the future, you can add more providers here
	// 	return cnpgprovider.New(r.Client, r.Scheme), nil
	// }

	// return nil, fmt.Errorf("no supported engine found in cluster")
}

// reconcileBackup reconciles the backup resource
func (r *ClusterBackupReconciler) reconcileBackup(
	ctx context.Context,
	backup *dbaasv1alpha1.ClusterBackup,
	backupProvider provider.BackupProvider,
) (bool, error) {
	logger := log.FromContext(ctx)

	// If backup is already completed, nothing to do
	if backup.IsCompleted() {
		return false, nil
	}

	// Create the backup
	if err := backupProvider.CreateBackup(ctx, backup); err != nil {
		logger.Error(err, "Failed to create backup")
		return true, err
	}

	// Check if we need to requeue
	status, err := backupProvider.GetBackupStatus(ctx, backup)
	if err != nil {
		return true, err
	}

	// Requeue if backup is still in progress
	if status.State == dbaasv1alpha1.BackupStateNew ||
		status.State == dbaasv1alpha1.BackupStateStarting ||
		status.State == dbaasv1alpha1.BackupStateRunning {
		return true, nil
	}

	return false, nil
}

// updateBackupStatus updates the backup status
func (r *ClusterBackupReconciler) updateBackupStatus(
	ctx context.Context,
	backup *dbaasv1alpha1.ClusterBackup,
	backupProvider provider.BackupProvider,
) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		// Get latest version
		latest := &dbaasv1alpha1.ClusterBackup{}
		if err := r.Get(ctx, client.ObjectKeyFromObject(backup), latest); err != nil {
			return err
		}

		// Get status from provider
		status, err := backupProvider.GetBackupStatus(ctx, latest)
		if err != nil {
			return err
		}

		latest.Status = status
		return r.Status().Update(ctx, latest)
	})
}

// handleDeletion handles backup deletion
func (r *ClusterBackupReconciler) handleDeletion(
	ctx context.Context,
	backup *dbaasv1alpha1.ClusterBackup,
	backupProvider provider.BackupProvider,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	if controllerutil.ContainsFinalizer(backup, clusterBackupFinalizer) {
		// Delete the upstream backup
		if err := backupProvider.DeleteBackup(ctx, backup); err != nil {
			logger.Error(err, "Failed to delete upstream backup")
			return ctrl.Result{}, err
		}

		// Remove finalizer
		controllerutil.RemoveFinalizer(backup, clusterBackupFinalizer)
		if err := r.Update(ctx, backup); err != nil {
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

// ensureOwnerReference ensures the backup has owner reference to cluster
func (r *ClusterBackupReconciler) ensureOwnerReference(
	ctx context.Context,
	backup *dbaasv1alpha1.ClusterBackup,
	cluster *v1alpha1.Cluster,
) error {
	if metav1.GetControllerOf(backup) != nil {
		return nil
	}

	if err := controllerutil.SetControllerReference(cluster, backup, r.Scheme); err != nil {
		return err
	}

	return r.Update(ctx, backup)
}

// SetupWithManager sets up the controller with the Manager
func (r *ClusterBackupReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&dbaasv1alpha1.ClusterBackup{}).
		Owns(&cnpgv1.Backup{}).
		Complete(r)
}
