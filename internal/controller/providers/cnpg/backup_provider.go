// internal/controller/clusterbackup/provider/cnpg/provider.go
package cnpg

import (
	"context"
	"fmt"

	dbaasv1alpha1 "github.com/Vinh1507/db-operator/api/dbaas/v1alpha1"
	provider "github.com/Vinh1507/db-operator/internal/controller/providers"
	cnpgv1 "github.com/cloudnative-pg/cloudnative-pg/api/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	backupFinalizer = "clusterbackup.everest.percona.com/finalizer"
)

// CNPGBackupProvider implements BackupProvider for CloudNativePG
type CNPGBackupProvider struct {
	client       client.Client
	scheme       *runtime.Scheme
	backupConfig provider.BackupConfig
}

// New creates a new CNPG backup provider
func NewBackupProvider(c client.Client, scheme *runtime.Scheme) provider.BackupProvider {
	return &CNPGBackupProvider{
		client:       c,
		scheme:       scheme,
		backupConfig: provider.DefaultBackupConfig(),
	}
}

// CreateBackup creates a CNPG backup
func (p *CNPGBackupProvider) CreateBackup(ctx context.Context, backup *dbaasv1alpha1.ClusterBackup) error {
	log := ctrl.LoggerFrom(ctx)

	// Get the CNPG Cluster
	cnpgCluster := &cnpgv1.Cluster{}
	err := p.client.Get(ctx, types.NamespacedName{
		Name:      backup.Spec.EngineName,
		Namespace: backup.Namespace,
	}, cnpgCluster)
	if err != nil {
		return fmt.Errorf("failed to get CNPG cluster: %w", err)
	}

	// Ensure backup credentials secret exists
	if err := p.ensureBackupSecret(ctx, backup.Namespace); err != nil {
		return fmt.Errorf("failed to ensure backup secret: %w", err)
	}

	// Create CNPG Backup object
	cnpgBackup := &cnpgv1.Backup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      backup.Name,
			Namespace: backup.Namespace,
		},
	}

	_, err = controllerutil.CreateOrUpdate(ctx, p.client, cnpgBackup, func() error {
		cnpgBackup.Spec = cnpgv1.BackupSpec{
			Cluster: cnpgv1.LocalObjectReference{
				Name: backup.Spec.EngineName,
			},
			Method: cnpgv1.BackupMethodBarmanObjectStore,
			Target: cnpgv1.BackupTargetPrimary,
		}

		// Set owner reference
		if err := controllerutil.SetControllerReference(backup, cnpgBackup, p.scheme); err != nil {
			return err
		}

		// Add finalizer
		controllerutil.AddFinalizer(cnpgBackup, backupFinalizer)

		return nil
	})

	if err != nil {
		log.Error(err, "Failed to create/update CNPG backup")
		return err
	}

	log.Info("Successfully created CNPG backup", "backup", cnpgBackup.Name)
	return nil
}

// GetBackupStatus retrieves backup status from CNPG backup
func (p *CNPGBackupProvider) GetBackupStatus(ctx context.Context, backup *dbaasv1alpha1.ClusterBackup) (dbaasv1alpha1.ClusterBackupStatus, error) {
	status := dbaasv1alpha1.ClusterBackupStatus{}

	// Handle deletion
	if !backup.DeletionTimestamp.IsZero() {
		status.State = dbaasv1alpha1.BackupStateDeleting
		return status, nil
	}

	// Get CNPG backup
	cnpgBackup := &cnpgv1.Backup{}
	err := p.client.Get(ctx, types.NamespacedName{
		Name:      backup.Name,
		Namespace: backup.Namespace,
	}, cnpgBackup)

	if err != nil {
		if k8serrors.IsNotFound(err) {
			status.State = dbaasv1alpha1.BackupStateNew
			return status, nil
		}
		return status, err
	}

	// Map CNPG status to our status
	status.State = p.mapCNPGBackupPhase(cnpgBackup.Status.Phase)
	status.CreatedAt = &cnpgBackup.CreationTimestamp

	if cnpgBackup.Status.StoppedAt != nil {
		status.CompletedAt = cnpgBackup.Status.StoppedAt
	}

	// Build destination path
	if cnpgBackup.Status.BackupName != "" {
		status.Destination = fmt.Sprintf("s3://%s/%s/%s/%s",
			p.backupConfig.S3Bucket,
			p.backupConfig.S3Path,
			backup.Spec.ClusterName,
			cnpgBackup.Status.BackupName,
		)
	}

	// Set message from conditions
	if len(cnpgBackup.Status.Error) > 0 {
		status.Message = cnpgBackup.Status.Error
	}

	return status, nil
}

// DeleteBackup deletes a CNPG backup
func (p *CNPGBackupProvider) DeleteBackup(ctx context.Context, backup *dbaasv1alpha1.ClusterBackup) error {
	log := ctrl.LoggerFrom(ctx)

	cnpgBackup := &cnpgv1.Backup{}
	err := p.client.Get(ctx, types.NamespacedName{
		Name:      backup.Name,
		Namespace: backup.Namespace,
	}, cnpgBackup)

	if err != nil {
		if k8serrors.IsNotFound(err) {
			return nil
		}
		return err
	}

	// Remove finalizer to allow deletion
	if controllerutil.RemoveFinalizer(cnpgBackup, backupFinalizer) {
		if err := p.client.Update(ctx, cnpgBackup); err != nil {
			return err
		}
	}

	// Delete the backup
	if err := p.client.Delete(ctx, cnpgBackup); err != nil && !k8serrors.IsNotFound(err) {
		return err
	}

	log.Info("Successfully deleted CNPG backup", "backup", cnpgBackup.Name)
	return nil
}

// GetUpstreamBackupObject returns the CNPG backup object
func (p *CNPGBackupProvider) GetUpstreamBackupObject(ctx context.Context, backup *dbaasv1alpha1.ClusterBackup) (client.Object, error) {
	cnpgBackup := &cnpgv1.Backup{}
	err := p.client.Get(ctx, types.NamespacedName{
		Name:      backup.Name,
		Namespace: backup.Namespace,
	}, cnpgBackup)

	if err != nil {
		return nil, err
	}

	return cnpgBackup, nil
}

// ensureBackupSecret ensures the backup credentials secret exists
func (p *CNPGBackupProvider) ensureBackupSecret(ctx context.Context, namespace string) error {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "backup-credentials",
			Namespace: namespace,
		},
	}

	_, err := controllerutil.CreateOrUpdate(ctx, p.client, secret, func() error {
		if secret.Data == nil {
			secret.Data = make(map[string][]byte)
		}

		secret.Data["ACCESS_KEY_ID"] = []byte(p.backupConfig.S3AccessKeyID)
		secret.Data["ACCESS_SECRET_KEY"] = []byte(p.backupConfig.S3SecretAccessKey)

		return nil
	})

	return err
}

// mapCNPGBackupPhase maps CNPG backup phase to our backup state
func (p *CNPGBackupProvider) mapCNPGBackupPhase(phase cnpgv1.BackupPhase) dbaasv1alpha1.BackupState {
	switch phase {
	case cnpgv1.BackupPhasePending:
		return dbaasv1alpha1.BackupStateStarting
	case cnpgv1.BackupPhaseRunning:
		return dbaasv1alpha1.BackupStateRunning
	case cnpgv1.BackupPhaseCompleted:
		return dbaasv1alpha1.BackupStateSucceeded
	case cnpgv1.BackupPhaseFailed:
		return dbaasv1alpha1.BackupStateFailed
	default:
		return dbaasv1alpha1.BackupStateNew
	}
}
