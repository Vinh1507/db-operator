package providers

import (
	"context"
	"time"

	dbaasv1alpha1 "github.com/Vinh1507/db-operator/api/dbaas/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ActionResult represents the result of an action execution
type ActionResult struct {
	// Completed indicates if the action is finished
	Completed bool

	// Progress indicates action progress (0-100)
	Progress int32

	// Message provides status information
	Message string

	// Output contains action-specific output
	Output map[string]string

	// RequeueAfter specifies when to requeue (if not completed)
	RequeueAfter time.Duration
}

// BackupProvider defines the interface for backup operations
type BackupProvider interface {
	// CreateBackup creates a new backup
	CreateBackup(ctx context.Context, backup *dbaasv1alpha1.ClusterBackup) error

	// GetBackupStatus retrieves the current backup status
	GetBackupStatus(ctx context.Context, backup *dbaasv1alpha1.ClusterBackup) (dbaasv1alpha1.ClusterBackupStatus, error)

	// DeleteBackup deletes a backup
	DeleteBackup(ctx context.Context, backup *dbaasv1alpha1.ClusterBackup) error

	// GetUpstreamBackupObject returns the underlying backup object
	GetUpstreamBackupObject(ctx context.Context, backup *dbaasv1alpha1.ClusterBackup) (client.Object, error)
}

type BackupConfig struct {
	// S3 Configuration
	S3Endpoint        string
	S3Bucket          string
	S3Region          string
	S3AccessKeyID     string
	S3SecretAccessKey string
	S3Path            string

	// Verification
	VerifyTLS      bool
	ForcePathStyle bool
}

// DefaultBackupConfig returns hardcoded backup configuration
func DefaultBackupConfig() BackupConfig {
	return BackupConfig{
		S3Endpoint:        "s3.amazonaws.com",
		S3Bucket:          "my-backup-bucket",
		S3Region:          "us-east-1",
		S3AccessKeyID:     "YOUR_ACCESS_KEY",
		S3SecretAccessKey: "YOUR_SECRET_KEY",
		S3Path:            "backups",
		VerifyTLS:         true,
		ForcePathStyle:    false,
	}
}
