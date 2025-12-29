// internal/controller/providers/cnpg/applier.go
package cnpg

import (
	"context"
	"fmt"

	cnpgv1 "github.com/cloudnative-pg/cloudnative-pg/api/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// Applier applies configuration to CNPG cluster
type Applier interface {
	ResetDefaults() error
	Metadata() error
	Paused(paused bool) error
	Engine() error
	Resources() error
	Storage() error
	ConfigBackup() error
	Monitoring() error
	DataSource() error
	Expose() error
	Network() error
	CreateUsers() error
	DataImport() error
	PodSchedulingPolicy() error
}

type applier struct {
	*Provider
	ctx context.Context
}

// defaultSpec returns default CNPG cluster spec
func defaultSpec() cnpgv1.ClusterSpec {
	return cnpgv1.ClusterSpec{
		Instances: 1,
		StorageConfiguration: cnpgv1.StorageConfiguration{
			Size: "10Gi",
		},
		PostgresConfiguration: cnpgv1.PostgresConfiguration{
			Parameters: map[string]string{},
		},
	}
}

// ResetDefaults resets cluster spec to defaults
func (a *applier) ResetDefaults() error {
	a.CNPGCluster.Spec = defaultSpec()
	return nil
}

// Metadata applies metadata and finalizers
func (a *applier) Metadata() error {
	// Add finalizer only if not being deleted
	if a.CNPGCluster.GetDeletionTimestamp().IsZero() {
		controllerutil.AddFinalizer(
			a.CNPGCluster,
			"everest.example.com/finalizer",
		)
	}

	// Set labels
	if a.CNPGCluster.Labels == nil {
		a.CNPGCluster.Labels = make(map[string]string)
	}
	a.CNPGCluster.Labels["app.kubernetes.io/managed-by"] = "db-operator"
	a.CNPGCluster.Labels["everest.example.com/cluster"] = a.Cluster.Name
	a.CNPGCluster.Labels["everest.example.com/engine"] = string(a.EngineSpec.EngineType)

	return nil
}

// Paused sets pause state (scales to 0 instances)
func (a *applier) Paused(paused bool) error {
	if paused {
		a.CNPGCluster.Spec.Instances = 0
	}
	return nil
}

// Engine applies engine configuration
func (a *applier) Engine() error {
	engine := a.EngineSpec

	// Set PostgreSQL version
	a.CNPGCluster.Spec.ImageName = fmt.Sprintf(
		"ghcr.io/cloudnative-pg/postgresql:%s",
		engine.Version,
	)

	// Set replicas
	a.CNPGCluster.Spec.Instances = int(engine.Replicas)

	// Set replicas
	if !a.Cluster.Spec.Paused {
		a.CNPGCluster.Spec.Instances = int(engine.Replicas)
	}

	// Set PostgreSQL configuration parameters
	if engine.Config.KV != nil && len(engine.Config.KV) > 0 {
		if a.CNPGCluster.Spec.PostgresConfiguration.Parameters == nil {
			a.CNPGCluster.Spec.PostgresConfiguration.Parameters = make(map[string]string)
		}
		for k, v := range engine.Config.KV {
			a.CNPGCluster.Spec.PostgresConfiguration.Parameters[k] = v
		}
	}

	return nil
}

// Resources applies CPU and memory resources
func (a *applier) Resources() error {
	engine := a.EngineSpec

	// if engine.Resources == nil {
	// 	return nil
	// }

	resources := corev1.ResourceRequirements{
		Requests: corev1.ResourceList{},
		Limits:   corev1.ResourceList{},
	}

	// CPU
	if engine.Resources.CPU != "" {
		cpu, err := resource.ParseQuantity(engine.Resources.CPU)
		if err != nil {
			return fmt.Errorf("invalid CPU quantity: %w", err)
		}
		resources.Requests[corev1.ResourceCPU] = cpu
		resources.Limits[corev1.ResourceCPU] = cpu
	}

	// Memory
	if engine.Resources.Memory != "" {
		memory, err := resource.ParseQuantity(engine.Resources.Memory)
		if err != nil {
			return fmt.Errorf("invalid memory quantity: %w", err)
		}
		resources.Requests[corev1.ResourceMemory] = memory
		resources.Limits[corev1.ResourceMemory] = memory
	}

	if len(resources.Requests) > 0 || len(resources.Limits) > 0 {
		a.CNPGCluster.Spec.Resources = resources
	}

	return nil
}

// Storage applies storage configuration
func (a *applier) Storage() error {
	engine := a.EngineSpec

	if engine.Storage.Size != "" {
		a.CNPGCluster.Spec.StorageConfiguration.Size = engine.Storage.Size
	}

	if engine.Storage.Class != "" {
		a.CNPGCluster.Spec.StorageConfiguration.StorageClass = &engine.Storage.Class
	}

	return nil
}

// Backup configures backup settings
func (a *applier) ConfigBackup() error {
	// engine := a.EngineSpec

	// if engine.Backup == nil || !engine.Backup.Enabled {
	// 	a.CNPGCluster.Spec.Backup = nil
	// 	return nil
	// }

	// backup := engine.Backup

	// // Fetch backup storage secret
	// secret := &corev1.Secret{}
	// err := a.Client.Get(
	// 	a.ctx,
	// 	types.NamespacedName{
	// 		Name:      backup.CredentialsSecret,
	// 		Namespace: a.Cluster.Namespace,
	// 	},
	// 	secret,
	// )
	// if err != nil {
	// 	return fmt.Errorf("failed to get backup credentials: %w", err)
	// }

	// // Configure CNPG backup
	// a.CNPGCluster.Spec.Backup = &cnpgv1.BackupConfiguration{
	// 	BarmanObjectStore: &cnpgv1.BarmanObjectStoreConfiguration{
	// 		DestinationPath: fmt.Sprintf("s3://%s/%s", backup.Bucket, a.EngineSpec.Name),
	// 		EndpointURL:     backup.Endpoint,
	// 		S3Credentials: &cnpgv1.S3Credentials{
	// 			AccessKeyIDReference: &cnpgv1.SecretKeySelector{
	// 				LocalObjectReference: cnpgv1.LocalObjectReference{
	// 					Name: secret.Name,
	// 				},
	// 				Key: "accessKeyId",
	// 			},
	// 			SecretAccessKeyReference: &cnpgv1.SecretKeySelector{
	// 				LocalObjectReference: cnpgv1.LocalObjectReference{
	// 					Name: secret.Name,
	// 				},
	// 				Key: "secretAccessKey",
	// 			},
	// 		},
	// 	},
	// 	RetentionPolicy: fmt.Sprintf("%dd", backup.RetentionDays),
	// }

	// // Schedule backup if specified
	// if backup.Schedule != "" {
	// 	a.CNPGCluster.Spec.Backup.BarmanObjectStore.ServerName = a.EngineSpec.Name
	// }

	return nil
}

// Monitoring enables monitoring configuration
func (a *applier) Monitoring() error {
	engine := a.EngineSpec

	if &engine.Monitoring == nil || !engine.Monitoring.Enabled {
		a.CNPGCluster.Spec.Monitoring = nil
		return nil
	}

	// Enable PodMonitor for Prometheus
	a.CNPGCluster.Spec.Monitoring = &cnpgv1.MonitoringConfiguration{
		EnablePodMonitor: true,
	}

	return nil
}

// DataSource configures data source for initialization
func (a *applier) DataSource() error {
	// engine := a.EngineSpec

	// if engine.DataSource == nil {
	// 	return nil
	// }

	// // Bootstrap from backup
	// if engine.DataSource.BackupName != "" {
	// 	a.CNPGCluster.Spec.Bootstrap = &cnpgv1.BootstrapConfiguration{
	// 		Recovery: &cnpgv1.BootstrapRecovery{
	// 			Backup: &cnpgv1.BackupSource{
	// 				LocalObjectReference: cnpgv1.LocalObjectReference{
	// 					Name: engine.DataSource.BackupName,
	// 				},
	// 			},
	// 		},
	// 	}
	// }

	return nil
}

// Expose creates services for external access
func (a *applier) Expose() error {
	// engine := a.EngineSpec

	// if engine.Expose == nil {
	// 	return nil
	// }

	// expose := engine.Expose

	// // Determine service role (rw = read-write, ro = read-only)
	// role := "rw"
	// if expose.Type == "readonly" {
	// 	role = "ro"
	// }

	// svcName := fmt.Sprintf("%s-%s", a.EngineSpec.Name, role)

	// // Build Service object
	// svc := &corev1.Service{
	// 	ObjectMeta: metav1.ObjectMeta{
	// 		Name:      svcName,
	// 		Namespace: a.CNPGCluster.Namespace,
	// 	},
	// }

	// // Create or Update Service
	// _, err := controllerutil.CreateOrUpdate(a.ctx, a.Client, svc, func() error {
	// 	// Set owner reference
	// 	if err := controllerutil.SetControllerReference(
	// 		a.CNPGCluster, svc, a.Scheme,
	// 	); err != nil {
	// 		return err
	// 	}

	// 	// Set labels
	// 	svc.Labels = map[string]string{
	// 		"cnpg.io/cluster": a.EngineSpec.Name,
	// 		"cnpg.io/role":    role,
	// 	}

	// 	// Configure service
	// 	svc.Spec.Type = corev1.ServiceType(expose.ServiceType)
	// 	svc.Spec.Ports = []corev1.ServicePort{
	// 		{
	// 			Name:       "postgres",
	// 			Port:       5432,
	// 			TargetPort: intstr.FromInt(5432),
	// 			Protocol:   corev1.ProtocolTCP,
	// 		},
	// 	}

	// 	// Selector
	// 	svc.Spec.Selector = map[string]string{
	// 		"cnpg.io/cluster": a.EngineSpec.Name,
	// 		"cnpg.io/role":    role,
	// 	}

	// 	return nil
	// })

	// return err
	return nil
}

func (a *applier) Network() error {
	return nil
}

// CreateUsers creates database users from per-user secrets
func (a *applier) CreateUsers() error {
	engine := a.EngineSpec

	if len(engine.CredentialsSecretNames) == 0 {
		return nil
	}

	if a.CNPGCluster.Spec.Managed == nil {
		a.CNPGCluster.Spec.Managed = &cnpgv1.ManagedConfiguration{}
	}

	// reset roles to avoid stale users
	a.CNPGCluster.Spec.Managed.Roles = []cnpgv1.RoleConfiguration{}

	for _, secretName := range engine.CredentialsSecretNames {
		secret := &corev1.Secret{}
		if err := a.Client.Get(
			a.ctx,
			types.NamespacedName{
				Name:      secretName,
				Namespace: a.Cluster.Namespace,
			},
			secret,
		); err != nil {
			return fmt.Errorf("failed to read user secret %q: %w", secretName, err)
		}

		username, ok := secret.Data["username"]
		if !ok || len(username) == 0 {
			return fmt.Errorf(
				"secret %q must contain non-empty key \"username\"",
				secretName,
			)
		}

		// CNPG requires key "password"
		if _, ok := secret.Data["password"]; !ok {
			return fmt.Errorf(
				"secret %q must contain key \"password\"",
				secretName,
			)
		}

		a.CNPGCluster.Spec.Managed.Roles = append(
			a.CNPGCluster.Spec.Managed.Roles,
			cnpgv1.RoleConfiguration{
				Name:  string(username),
				Login: true,
				PasswordSecret: &cnpgv1.LocalObjectReference{
					// CNPG will read key "password" from this secret
					Name: secretName,
				},
			},
		)
	}

	return nil
}

func (a *applier) DataImport() error {
	// Handle external data import
	return nil
}

func (a *applier) PodSchedulingPolicy() error {
	// Apply pod affinity, tolerations, etc.
	return nil
}
