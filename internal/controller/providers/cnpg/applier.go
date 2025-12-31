// internal/controller/providers/cnpg/applier.go
package cnpg

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	opsv1alpha1 "github.com/Vinh1507/db-operator/api/ops/v1alpha1"

	providers "github.com/Vinh1507/db-operator/internal/controller/providers"
	cnpgv1 "github.com/cloudnative-pg/cloudnative-pg/api/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// Applier applies configuration to CNPG cluster
type Applier interface {
	Convert(dbObject client.Object) error
	// ResetDefaults() error
	// Metadata() error
	// Paused(paused bool) error
	// Engine() error
	// Resources() error
	// Storage() error
	// ConfigBackup() error
	// Monitoring() error
	// DataSource() error
	// Expose() error
	// Network() error
	// CreateUsers() error
	// DataImport() error
	// PodSchedulingPolicy() error

	// Action methods for OpsRequest
	ValidateAction(action opsv1alpha1.ActionType, params map[string]string) error
	Start(ctx context.Context, params map[string]string) (providers.ActionResult, error)
	Stop(ctx context.Context, params map[string]string) (providers.ActionResult, error)
	Restart(ctx context.Context, params map[string]string) (providers.ActionResult, error)
	HorizontalScale(ctx context.Context, params map[string]string) (providers.ActionResult, error)
	VerticalScale(ctx context.Context, params map[string]string) (providers.ActionResult, error)
	VolumeExpansion(ctx context.Context, params map[string]string) (providers.ActionResult, error)
	Reconfigure(ctx context.Context, params map[string]string) (providers.ActionResult, error)
	Upgrade(ctx context.Context, params map[string]string) (providers.ActionResult, error)
	Backup(ctx context.Context, params map[string]string) (providers.ActionResult, error)
	Restore(ctx context.Context, params map[string]string) (providers.ActionResult, error)
	ExposeService(ctx context.Context, params map[string]string) (providers.ActionResult, error)
	Switchover(ctx context.Context, params map[string]string) (providers.ActionResult, error)
	Custom(ctx context.Context, params map[string]string) (providers.ActionResult, error)
}

type applier struct {
	*Provider
	ctx context.Context
}

func (a *applier) Convert(dbObject client.Object) error {
	switch obj := dbObject.(type) {
	case *cnpgv1.Cluster:
		a.ResetDefaults()
		a.Metadata()
		a.Paused()
		a.Engine()
		a.Resources()
		a.Storage()
		a.ConfigBackup()
		a.Monitoring()
		a.Expose()
		a.CreateUsers()
		a.DataSource()
		a.DataImport()
		a.PodSchedulingPolicy()
		a.GetOpsResult()
	// case *AnotherCRDType:
	default:
		return fmt.Errorf("unsupported object type: %T", obj)
	}
	return nil
}

// defaultSpec returns default CNPG cluster spec with S3 backup enabled
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
func (a *applier) Paused() error {
	paused := a.Cluster.Spec.Paused
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

// ConfigBackup configures CNPG backup settings
func (a *applier) ConfigBackup() error {
	backup := a.EngineSpec.Backup

	if !backup.Enabled {
		a.CNPGCluster.Spec.Backup = nil
		return nil
	}

	if backup.BackupStorageName == "" {
		return fmt.Errorf("backupStorageName must be set when backup is enabled")
	}

	backupStorage := backup.BackupStorages[0]
	// Fetch backup storage secret
	secret := &corev1.Secret{}
	if err := a.Client.Get(
		a.ctx,
		types.NamespacedName{
			Name:      backupStorage.SecretRef,
			Namespace: a.Cluster.Namespace,
		},
		secret,
	); err != nil {
		return fmt.Errorf("failed to get backup storage secret: %w", err)
	}

	// // Required keys
	// requiredKeys := []string{"accessKeyId", "secretAccessKey", "endpoint", "bucket"}
	// for _, key := range requiredKeys {
	// 	if _, ok := secret.Data[key]; !ok {
	// 		return fmt.Errorf("backup secret %q missing key %q", secret.Name, key)
	// 	}
	// }

	bucket := string(secret.Data["bucket"])
	endpoint := string(secret.Data["endpoint"])

	a.CNPGCluster.Spec.Backup = &cnpgv1.BackupConfiguration{
		BarmanObjectStore: &cnpgv1.BarmanObjectStoreConfiguration{
			DestinationPath: fmt.Sprintf(
				"s3://%s/%s",
				bucket,
				a.EngineSpec.Name,
			),
			EndpointURL: endpoint,
			BarmanCredentials: cnpgv1.BarmanCredentials{
				AWS: &cnpgv1.S3Credentials{
					AccessKeyIDReference: &cnpgv1.SecretKeySelector{
						LocalObjectReference: cnpgv1.LocalObjectReference{
							Name: secret.Name,
						},
						Key: "accessKeyId",
					},
					SecretAccessKeyReference: &cnpgv1.SecretKeySelector{
						LocalObjectReference: cnpgv1.LocalObjectReference{
							Name: secret.Name,
						},
						Key: "secretAccessKey",
					},
				},
			},
		},
	}

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
func (a *applier) GetOpsResult() error {
	// Collect pod info as flat logs
	logsMap, err := a.collectPodInfo()
	if err != nil {
		return fmt.Errorf("failed to collect pod information: %w", err)
	}

	opsResult := &opsv1alpha1.OpsResult{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-ops-logs", a.Cluster.Name),
			Namespace: a.Cluster.Namespace,
		},
	}

	_, err = controllerutil.CreateOrUpdate(a.ctx, a.Client, opsResult, func() error {
		// Owner reference
		if err := controllerutil.SetControllerReference(
			a.Cluster, opsResult, a.Scheme,
		); err != nil {
			return err
		}

		// Spec
		opsResult.Spec = opsv1alpha1.OpsResultSpec{
			ClusterRef: opsv1alpha1.ObjectReference{
				Name:       a.Cluster.Name,
				Namespace:  a.Cluster.Namespace,
				Kind:       a.Cluster.Kind,
				APIVersion: a.Cluster.APIVersion,
			},
			EngineRef: opsv1alpha1.ObjectReference{
				Name:       a.CNPGCluster.Name,
				Namespace:  a.CNPGCluster.Namespace,
				Kind:       "Cluster",
				APIVersion: "postgresql.cnpg.io/v1",
			},
			Logs: logsMap,
		}

		// Status
		opsResult.Status.Phase = "Updated"
		opsResult.Status.LastUpdateTime = metav1.Now()

		return nil
	})

	return err
}

func (a *applier) collectPodInfo() (map[string]string, error) {
	result := make(map[string]string)

	// timestamp
	now := metav1.Now()
	result["timestamp"] = now.Format(time.RFC3339)

	// init log
	var logs []string
	logs = append(logs, "collectPodInfo started")

	podList := &corev1.PodList{}
	labelSelector := client.MatchingLabels{
		"cnpg.io/cluster": a.CNPGCluster.Name,
	}

	if err := a.Client.List(
		a.ctx,
		podList,
		client.InNamespace(a.CNPGCluster.Namespace),
		labelSelector,
	); err != nil {
		logs = append(logs, fmt.Sprintf("failed to list pods: %v", err))
		result["log"] = strings.Join(logs, "\n")
		return result, err
	}

	for _, pod := range podList.Items {
		podPrefix := fmt.Sprintf("pods/%s", pod.Name)

		logs = append(logs, fmt.Sprintf("processing pod %s", pod.Name))

		result[podPrefix+"/phase"] = string(pod.Status.Phase)
		result[podPrefix+"/node"] = pod.Spec.NodeName
		result[podPrefix+"/podIP"] = pod.Status.PodIP
		result[podPrefix+"/hostIP"] = pod.Status.HostIP

		for _, cond := range pod.Status.Conditions {
			key := fmt.Sprintf("%s/condition/%s", podPrefix, cond.Type)
			value := fmt.Sprintf(
				"status=%s reason=%s message=%s",
				cond.Status,
				cond.Reason,
				cond.Message,
			)
			result[key] = value
		}

		// Container statuses
		for _, cs := range pod.Status.ContainerStatuses {
			containerPrefix := fmt.Sprintf("%s/container/%s", podPrefix, cs.Name)

			state := "Unknown"
			message := ""

			if cs.State.Running != nil {
				state = "Running"
			} else if cs.State.Waiting != nil {
				state = "Waiting"
				message = cs.State.Waiting.Message
			} else if cs.State.Terminated != nil {
				state = "Terminated"
				message = cs.State.Terminated.Message
			}

			result[containerPrefix+"/state"] = state
			result[containerPrefix+"/ready"] = strconv.FormatBool(cs.Ready)
			result[containerPrefix+"/restartCount"] = strconv.Itoa(int(cs.RestartCount))
			result[containerPrefix+"/image"] = cs.Image

			if message != "" {
				result[containerPrefix+"/message"] = message
			}
		}
	}

	logs = append(logs, "collectPodInfo finished")
	result["log"] = strings.Join(logs, "\n")

	return result, nil
}

/**

 */

// ==================== Action Methods ====================

func (a *applier) ValidateAction(action opsv1alpha1.ActionType, params map[string]string) error {
	switch action {
	case opsv1alpha1.ActionStart:
		return a.validateStart(params)
	case opsv1alpha1.ActionStop:
		return a.validateStop(params)
	case opsv1alpha1.ActionRestart:
		return a.validateRestart(params)
	case opsv1alpha1.ActionHorizontalScaling:
		return a.validateHorizontalScale(params)
	case opsv1alpha1.ActionVerticalScaling:
		return a.validateVerticalScale(params)
	case opsv1alpha1.ActionVolumeExpansion:
		return a.validateVolumeExpansion(params)
	case opsv1alpha1.ActionReconfiguring:
		return a.validateReconfigure(params)
	case opsv1alpha1.ActionUpgrade:
		return a.validateUpgrade(params)
	case opsv1alpha1.ActionSwitchover:
		return a.validateSwitchover(params)
	default:
		return nil
	}
}

// ==================== Start Action ====================

func (a *applier) validateStart(params map[string]string) error {
	if !a.Cluster.Spec.Paused {
		return fmt.Errorf("cluster is already running")
	}
	return nil
}

func (a *applier) Start(ctx context.Context, params map[string]string) (providers.ActionResult, error) {
	// Unpause the cluster
	a.Cluster.Spec.Paused = false

	if err := a.Client.Update(ctx, a.Cluster); err != nil {
		return providers.ActionResult{}, err
	}

	// Update CNPG cluster instances
	a.CNPGCluster.Spec.Instances = int(a.EngineSpec.Replicas)
	if err := a.Client.Update(ctx, a.CNPGCluster); err != nil {
		return providers.ActionResult{}, err
	}

	// Check if started
	if err := a.Client.Get(ctx, client.ObjectKeyFromObject(a.CNPGCluster), a.CNPGCluster); err != nil {
		return providers.ActionResult{}, err
	}

	if a.CNPGCluster.Status.Phase == cnpgv1.PhaseHealthy &&
		a.CNPGCluster.Status.ReadyInstances == a.CNPGCluster.Spec.Instances {
		return providers.ActionResult{
			Completed: true,
			Progress:  100,
			Message:   "Cluster started successfully",
			Output: map[string]string{
				"readyInstances": strconv.Itoa(a.CNPGCluster.Status.ReadyInstances),
			},
		}, nil
	}

	progress := int32(50)
	if a.CNPGCluster.Spec.Instances > 0 {
		progress = int32(float64(a.CNPGCluster.Status.ReadyInstances) / float64(a.CNPGCluster.Spec.Instances) * 80)
	}

	return providers.ActionResult{
		Completed:    false,
		Progress:     progress,
		Message:      fmt.Sprintf("Starting cluster: %d/%d instances ready", a.CNPGCluster.Status.ReadyInstances, a.CNPGCluster.Spec.Instances),
		RequeueAfter: 10 * time.Second,
	}, nil
}

// ==================== Stop Action ====================

func (a *applier) validateStop(params map[string]string) error {
	if a.Cluster.Spec.Paused {
		return fmt.Errorf("cluster is already stopped")
	}
	return nil
}

func (a *applier) Stop(ctx context.Context, params map[string]string) (providers.ActionResult, error) {
	// Pause the cluster
	a.Cluster.Spec.Paused = true

	if err := a.Client.Update(ctx, a.Cluster); err != nil {
		return providers.ActionResult{}, err
	}

	// Scale CNPG cluster to 0
	a.CNPGCluster.Spec.Instances = 0
	if err := a.Client.Update(ctx, a.CNPGCluster); err != nil {
		return providers.ActionResult{}, err
	}

	// Check if stopped
	if err := a.Client.Get(ctx, client.ObjectKeyFromObject(a.CNPGCluster), a.CNPGCluster); err != nil {
		return providers.ActionResult{}, err
	}

	if a.CNPGCluster.Status.ReadyInstances == 0 {
		return providers.ActionResult{
			Completed: true,
			Progress:  100,
			Message:   "Cluster stopped successfully",
		}, nil
	}

	return providers.ActionResult{
		Completed:    false,
		Progress:     50,
		Message:      fmt.Sprintf("Stopping cluster: %d instances still running", a.CNPGCluster.Status.ReadyInstances),
		RequeueAfter: 10 * time.Second,
	}, nil
}

// ==================== Restart Action ====================

func (a *applier) validateRestart(params map[string]string) error {
	if a.Cluster.Spec.Paused {
		return fmt.Errorf("cannot restart a paused cluster")
	}
	return nil
}

func (a *applier) Restart(ctx context.Context, params map[string]string) (providers.ActionResult, error) {
	// CNPG restart is done by triggering a rolling update
	// We can force this by updating an annotation
	if a.CNPGCluster.Annotations == nil {
		a.CNPGCluster.Annotations = make(map[string]string)
	}

	// Set restart timestamp
	restartKey := "ops.example.com/restart-timestamp"
	previousRestart := a.CNPGCluster.Annotations[restartKey]
	currentRestart := time.Now().Format(time.RFC3339)

	a.CNPGCluster.Annotations[restartKey] = currentRestart

	if err := a.Client.Update(ctx, a.CNPGCluster); err != nil {
		return providers.ActionResult{}, err
	}

	// Refresh status
	if err := a.Client.Get(ctx, client.ObjectKeyFromObject(a.CNPGCluster), a.CNPGCluster); err != nil {
		return providers.ActionResult{}, err
	}

	// Check if restart completed
	if previousRestart != "" && a.CNPGCluster.Status.Phase == cnpgv1.PhaseHealthy &&
		a.CNPGCluster.Status.ReadyInstances == a.CNPGCluster.Spec.Instances {
		return providers.ActionResult{
			Completed: true,
			Progress:  100,
			Message:   "Cluster restarted successfully",
			Output: map[string]string{
				"restartTime": currentRestart,
			},
		}, nil
	}

	progress := int32(30)
	if a.CNPGCluster.Spec.Instances > 0 {
		progress = 30 + int32(float64(a.CNPGCluster.Status.ReadyInstances)/float64(a.CNPGCluster.Spec.Instances)*60)
	}

	return providers.ActionResult{
		Completed:    false,
		Progress:     progress,
		Message:      fmt.Sprintf("Restarting cluster: %d/%d instances ready", a.CNPGCluster.Status.ReadyInstances, a.CNPGCluster.Spec.Instances),
		RequeueAfter: 15 * time.Second,
	}, nil
}

// ==================== Horizontal Scaling ====================

func (a *applier) validateHorizontalScale(params map[string]string) error {
	replicasStr, ok := params["replicas"]
	if !ok {
		return fmt.Errorf("replicas parameter is required")
	}

	replicas, err := strconv.ParseInt(replicasStr, 10, 32)
	if err != nil {
		return fmt.Errorf("invalid replicas value: %w", err)
	}

	if replicas < 1 {
		return fmt.Errorf("replicas must be at least 1")
	}

	return nil
}

func (a *applier) HorizontalScale(ctx context.Context, params map[string]string) (providers.ActionResult, error) {
	replicas, _ := strconv.ParseInt(params["replicas"], 10, 32)
	targetReplicas := int32(replicas)
	fmt.Println(">>>>>>> 4")
	fmt.Println(">>>>>>> 5", a.EngineSpec.Name)
	fmt.Println(">>>>>>> 6", targetReplicas)
	currentReplicas := a.EngineSpec.Replicas

	// Update cluster spec
	for i := range a.Cluster.Spec.Engines {
		if a.Cluster.Spec.Engines[i].Name == a.EngineSpec.Name {
			a.Cluster.Spec.Engines[i].Replicas = targetReplicas
			break
		}
	}

	if err := a.Client.Update(ctx, a.Cluster); err != nil {
		return providers.ActionResult{}, err
	}

	// // Refresh status
	if err := a.Client.Get(ctx, client.ObjectKeyFromObject(a.CNPGCluster), a.CNPGCluster); err != nil {
		return providers.ActionResult{}, err
	}

	// Check if scaling completed
	if a.CNPGCluster.Status.ReadyInstances == int(targetReplicas) &&
		a.CNPGCluster.Status.Phase == cnpgv1.PhaseHealthy {
		return providers.ActionResult{
			Completed: true,
			Progress:  100,
			Message:   fmt.Sprintf("Scaled from %d to %d replicas successfully", currentReplicas, targetReplicas),
			Output: map[string]string{
				"previousReplicas": strconv.Itoa(int(currentReplicas)),
				"currentReplicas":  strconv.Itoa(int(targetReplicas)),
			},
		}, nil
	}

	progress := int32(20)
	if targetReplicas > 0 {
		progress = 20 + int32(float64(a.CNPGCluster.Status.ReadyInstances)/float64(targetReplicas)*70)
	}

	return providers.ActionResult{
		Completed:    false,
		Progress:     progress,
		Message:      fmt.Sprintf("Scaling in progress: %d/%d replicas ready", a.CNPGCluster.Status.ReadyInstances, targetReplicas),
		RequeueAfter: 15 * time.Second,
	}, nil
	// return providers.ActionResult{}, nil
}

// ==================== Vertical Scaling ====================

func (a *applier) validateVerticalScale(params map[string]string) error {
	cpu, hasCPU := params["cpu"]
	memory, hasMemory := params["memory"]

	if !hasCPU && !hasMemory {
		return fmt.Errorf("at least one of cpu or memory must be specified")
	}

	if hasCPU {
		if _, err := resource.ParseQuantity(cpu); err != nil {
			return fmt.Errorf("invalid cpu value: %w", err)
		}
	}

	if hasMemory {
		if _, err := resource.ParseQuantity(memory); err != nil {
			return fmt.Errorf("invalid memory value: %w", err)
		}
	}

	return nil
}

func (a *applier) VerticalScale(ctx context.Context, params map[string]string) (providers.ActionResult, error) {
	// Update cluster spec
	for i := range a.Cluster.Spec.Engines {
		if a.Cluster.Spec.Engines[i].Name == a.EngineSpec.Name {
			if cpu, ok := params["cpu"]; ok {
				a.Cluster.Spec.Engines[i].Resources.CPU = cpu
			}
			if memory, ok := params["memory"]; ok {
				a.Cluster.Spec.Engines[i].Resources.Memory = memory
			}
			break
		}
	}

	if err := a.Client.Update(ctx, a.Cluster); err != nil {
		return providers.ActionResult{}, err
	}

	// Update CNPG resources
	if &a.CNPGCluster.Spec.Resources == nil {
		a.CNPGCluster.Spec.Resources = corev1.ResourceRequirements{
			Requests: corev1.ResourceList{},
			Limits:   corev1.ResourceList{},
		}
	}

	if cpu, ok := params["cpu"]; ok {
		cpuQuantity, _ := resource.ParseQuantity(cpu)
		a.CNPGCluster.Spec.Resources.Requests[corev1.ResourceCPU] = cpuQuantity
		a.CNPGCluster.Spec.Resources.Limits[corev1.ResourceCPU] = cpuQuantity
	}

	if memory, ok := params["memory"]; ok {
		memQuantity, _ := resource.ParseQuantity(memory)
		a.CNPGCluster.Spec.Resources.Requests[corev1.ResourceMemory] = memQuantity
		a.CNPGCluster.Spec.Resources.Limits[corev1.ResourceMemory] = memQuantity
	}

	if err := a.Client.Update(ctx, a.CNPGCluster); err != nil {
		return providers.ActionResult{}, err
	}

	// Refresh status
	if err := a.Client.Get(ctx, client.ObjectKeyFromObject(a.CNPGCluster), a.CNPGCluster); err != nil {
		return providers.ActionResult{}, err
	}

	// Check if scaling completed (all pods restarted with new resources)
	if a.CNPGCluster.Status.Phase == cnpgv1.PhaseHealthy &&
		a.CNPGCluster.Status.ReadyInstances == a.CNPGCluster.Spec.Instances {
		return providers.ActionResult{
			Completed: true,
			Progress:  100,
			Message:   "Vertical scaling completed successfully",
			Output: map[string]string{
				"cpu":    params["cpu"],
				"memory": params["memory"],
			},
		}, nil
	}

	progress := int32(40)
	if a.CNPGCluster.Spec.Instances > 0 {
		progress = 40 + int32(float64(a.CNPGCluster.Status.ReadyInstances)/float64(a.CNPGCluster.Spec.Instances)*50)
	}

	return providers.ActionResult{
		Completed:    false,
		Progress:     progress,
		Message:      "Vertical scaling in progress",
		RequeueAfter: 20 * time.Second,
	}, nil
}

// ==================== Volume Expansion ====================

func (a *applier) validateVolumeExpansion(params map[string]string) error {
	sizeStr, ok := params["size"]
	if !ok {
		return fmt.Errorf("size parameter is required")
	}

	size, err := resource.ParseQuantity(sizeStr)
	if err != nil {
		return fmt.Errorf("invalid size value: %w", err)
	}

	currentSize, err := resource.ParseQuantity(a.CNPGCluster.Spec.StorageConfiguration.Size)
	if err != nil {
		return fmt.Errorf("failed to parse current size: %w", err)
	}

	if size.Cmp(currentSize) <= 0 {
		return fmt.Errorf("new size must be larger than current size (%s)", currentSize.String())
	}

	return nil
}

func (a *applier) VolumeExpansion(ctx context.Context, params map[string]string) (providers.ActionResult, error) {
	newSize := params["size"]
	currentSize := a.CNPGCluster.Spec.StorageConfiguration.Size

	// Update cluster spec
	for i := range a.Cluster.Spec.Engines {
		if a.Cluster.Spec.Engines[i].Name == a.EngineSpec.Name {
			a.Cluster.Spec.Engines[i].Storage.Size = newSize
			break
		}
	}

	if err := a.Client.Update(ctx, a.Cluster); err != nil {
		return providers.ActionResult{}, err
	}

	// Update CNPG storage
	a.CNPGCluster.Spec.StorageConfiguration.Size = newSize
	if err := a.Client.Update(ctx, a.CNPGCluster); err != nil {
		return providers.ActionResult{}, err
	}

	// Refresh status
	if err := a.Client.Get(ctx, client.ObjectKeyFromObject(a.CNPGCluster), a.CNPGCluster); err != nil {
		return providers.ActionResult{}, err
	}

	// Check if expansion completed
	// if a.CNPGCluster.Status.Phase != cnpgv1.PhaseResizing &&
	if a.CNPGCluster.Status.Phase == cnpgv1.PhaseHealthy {
		return providers.ActionResult{
			Completed: true,
			Progress:  100,
			Message:   fmt.Sprintf("Volume expanded from %s to %s successfully", currentSize, newSize),
			Output: map[string]string{
				"previousSize": currentSize,
				"currentSize":  newSize,
			},
		}, nil
	}

	return providers.ActionResult{
		Completed:    false,
		Progress:     60,
		Message:      "Volume expansion in progress",
		RequeueAfter: 30 * time.Second,
	}, nil
}

// ==================== Reconfigure ====================

func (a *applier) validateReconfigure(params map[string]string) error {
	if len(params) == 0 {
		return fmt.Errorf("at least one configuration parameter must be specified")
	}
	return nil
}

func (a *applier) Reconfigure(ctx context.Context, params map[string]string) (providers.ActionResult, error) {
	// Update cluster spec config
	for i := range a.Cluster.Spec.Engines {
		if a.Cluster.Spec.Engines[i].Name == a.EngineSpec.Name {
			if a.Cluster.Spec.Engines[i].Config.KV == nil {
				a.Cluster.Spec.Engines[i].Config.KV = make(map[string]string)
			}
			for k, v := range params {
				a.Cluster.Spec.Engines[i].Config.KV[k] = v
			}
			break
		}
	}

	if err := a.Client.Update(ctx, a.Cluster); err != nil {
		return providers.ActionResult{}, err
	}

	// Update CNPG PostgreSQL config
	if a.CNPGCluster.Spec.PostgresConfiguration.Parameters == nil {
		a.CNPGCluster.Spec.PostgresConfiguration.Parameters = make(map[string]string)
	}

	for k, v := range params {
		a.CNPGCluster.Spec.PostgresConfiguration.Parameters[k] = v
	}

	if err := a.Client.Update(ctx, a.CNPGCluster); err != nil {
		return providers.ActionResult{}, err
	}

	// Refresh status
	if err := a.Client.Get(ctx, client.ObjectKeyFromObject(a.CNPGCluster), a.CNPGCluster); err != nil {
		return providers.ActionResult{}, err
	}

	// Check if reconfiguration completed
	if a.CNPGCluster.Status.Phase == cnpgv1.PhaseHealthy &&
		a.CNPGCluster.Status.ReadyInstances == a.CNPGCluster.Spec.Instances {
		return providers.ActionResult{
			Completed: true,
			Progress:  100,
			Message:   "Reconfiguration completed successfully",
			Output:    params,
		}, nil
	}

	return providers.ActionResult{
		Completed:    false,
		Progress:     50,
		Message:      "Reconfiguration in progress",
		RequeueAfter: 15 * time.Second,
	}, nil
}

// ==================== Upgrade ====================

func (a *applier) validateUpgrade(params map[string]string) error {
	version, ok := params["version"]
	if !ok {
		return fmt.Errorf("version parameter is required")
	}

	if version == "" {
		return fmt.Errorf("version cannot be empty")
	}

	if version == a.EngineSpec.Version {
		return fmt.Errorf("version %s is already installed", version)
	}

	return nil
}

func (a *applier) Upgrade(ctx context.Context, params map[string]string) (providers.ActionResult, error) {
	targetVersion := params["version"]
	currentVersion := a.EngineSpec.Version

	// Update cluster spec
	for i := range a.Cluster.Spec.Engines {
		if a.Cluster.Spec.Engines[i].Name == a.EngineSpec.Name {
			a.Cluster.Spec.Engines[i].Version = targetVersion
			break
		}
	}

	if err := a.Client.Update(ctx, a.Cluster); err != nil {
		return providers.ActionResult{}, err
	}

	// Update CNPG image
	a.CNPGCluster.Spec.ImageName = fmt.Sprintf(
		"ghcr.io/cloudnative-pg/postgresql:%s",
		targetVersion,
	)

	if err := a.Client.Update(ctx, a.CNPGCluster); err != nil {
		return providers.ActionResult{}, err
	}

	// Refresh status
	if err := a.Client.Get(ctx, client.ObjectKeyFromObject(a.CNPGCluster), a.CNPGCluster); err != nil {
		return providers.ActionResult{}, err
	}

	// Check if upgrade completed
	if a.CNPGCluster.Status.Phase == cnpgv1.PhaseHealthy &&
		a.CNPGCluster.Status.Image != "" &&
		(a.CNPGCluster.Status.Image == a.CNPGCluster.Spec.ImageName ||
			a.CNPGCluster.Status.Image == fmt.Sprintf("ghcr.io/cloudnative-pg/postgresql:%s", targetVersion)) {
		return providers.ActionResult{
			Completed: true,
			Progress:  100,
			Message:   fmt.Sprintf("Upgraded from %s to %s successfully", currentVersion, targetVersion),
			Output: map[string]string{
				"previousVersion": currentVersion,
				"currentVersion":  targetVersion,
			},
		}, nil
	}

	// Calculate progress based on phase
	progress := int32(30)
	if a.CNPGCluster.Status.Phase == string(cnpgv1.PhaseUpgrade) {
		progress = 60
	}

	return providers.ActionResult{
		Completed:    false,
		Progress:     progress,
		Message:      fmt.Sprintf("Upgrading from %s to %s", currentVersion, targetVersion),
		RequeueAfter: 20 * time.Second,
	}, nil
}

// ==================== Backup ====================

func (a *applier) Backup(ctx context.Context, params map[string]string) (providers.ActionResult, error) {
	backupName := params["backupName"]
	if backupName == "" {
		backupName = fmt.Sprintf("%s-backup-%d", a.EngineSpec.Name, time.Now().Unix())
	}

	// Create CNPG Backup object
	backup := &cnpgv1.Backup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      backupName,
			Namespace: a.CNPGCluster.Namespace,
		},
		Spec: cnpgv1.BackupSpec{
			Cluster: cnpgv1.LocalObjectReference{
				Name: a.CNPGCluster.Name,
			},
		},
	}

	// Set owner reference
	if err := controllerutil.SetControllerReference(a.CNPGCluster, backup, a.Scheme); err != nil {
		return providers.ActionResult{}, err
	}

	// Create backup
	if err := a.Client.Create(ctx, backup); err != nil {
		// If already exists, get it
		if !k8serrors.IsAlreadyExists(err) {
			return providers.ActionResult{}, err
		}
		if err := a.Client.Get(ctx, types.NamespacedName{
			Name:      backupName,
			Namespace: a.CNPGCluster.Namespace,
		}, backup); err != nil {
			return providers.ActionResult{}, err
		}
	}

	// Check backup status
	if backup.Status.Phase == cnpgv1.BackupPhaseCompleted {
		return providers.ActionResult{
			Completed: true,
			Progress:  100,
			Message:   "Backup completed successfully",
			Output: map[string]string{
				"backupName": backupName,
				"startTime":  backup.Status.StartedAt.String(),
				"endTime":    backup.Status.StoppedAt.String(),
			},
		}, nil
	}

	if backup.Status.Phase == cnpgv1.BackupPhaseFailed {
		return providers.ActionResult{}, fmt.Errorf("backup failed")
	}

	progress := int32(20)
	if backup.Status.Phase == cnpgv1.BackupPhaseRunning {
		progress = 60
	}

	return providers.ActionResult{
		Completed:    false,
		Progress:     progress,
		Message:      fmt.Sprintf("Backup in progress: %s", backup.Status.Phase),
		RequeueAfter: 30 * time.Second,
	}, nil
}

// ==================== Restore ====================

func (a *applier) Restore(ctx context.Context, params map[string]string) (providers.ActionResult, error) {
	// backupName, ok := params["backupName"]
	// if !ok {
	// 	return providers.ActionResult{}, fmt.Errorf("backupName parameter is required")
	// }

	// Note: CNPG restore requires creating a new cluster with bootstrap recovery
	// This is typically done during cluster creation, not as a runtime operation
	// For runtime restore, we would need to create a new cluster

	return providers.ActionResult{
		Completed: false,
		Progress:  0,
		Message:   "CNPG restore requires cluster recreation with bootstrap recovery",
	}, fmt.Errorf("runtime restore not supported for CNPG, use cluster recreation with dataSource")
}

// ==================== Expose Service ====================

func (a *applier) ExposeService(ctx context.Context, params map[string]string) (providers.ActionResult, error) {
	serviceType := params["serviceType"]
	if serviceType == "" {
		serviceType = "LoadBalancer"
	}

	mode := params["mode"]
	if mode == "" {
		mode = "rw" // read-write by default
	}

	svcName := fmt.Sprintf("%s-%s", a.EngineSpec.Name, mode)

	// Create or update service
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      svcName,
			Namespace: a.CNPGCluster.Namespace,
		},
	}

	_, err := controllerutil.CreateOrUpdate(ctx, a.Client, svc, func() error {
		if err := controllerutil.SetControllerReference(a.CNPGCluster, svc, a.Scheme); err != nil {
			return err
		}

		svc.Labels = map[string]string{
			"cnpg.io/cluster": a.EngineSpec.Name,
			"cnpg.io/role":    mode,
		}

		svc.Spec.Type = corev1.ServiceType(serviceType)
		svc.Spec.Ports = []corev1.ServicePort{
			{
				Name:       "postgres",
				Port:       5432,
				TargetPort: intstr.FromInt(5432),
				Protocol:   corev1.ProtocolTCP,
			},
		}

		svc.Spec.Selector = map[string]string{
			"cnpg.io/cluster": a.EngineSpec.Name,
			"cnpg.io/role":    mode,
		}

		return nil
	})

	if err != nil {
		return providers.ActionResult{}, err
	}

	// Get service to check external IP/hostname
	if err := a.Client.Get(ctx, types.NamespacedName{
		Name:      svcName,
		Namespace: a.CNPGCluster.Namespace,
	}, svc); err != nil {
		return providers.ActionResult{}, err
	}

	// Check if service has external endpoint
	var endpoint string
	if len(svc.Status.LoadBalancer.Ingress) > 0 {
		if svc.Status.LoadBalancer.Ingress[0].IP != "" {
			endpoint = svc.Status.LoadBalancer.Ingress[0].IP
		} else if svc.Status.LoadBalancer.Ingress[0].Hostname != "" {
			endpoint = svc.Status.LoadBalancer.Ingress[0].Hostname
		}
	}

	if endpoint != "" {
		return providers.ActionResult{
			Completed: true,
			Progress:  100,
			Message:   fmt.Sprintf("Service exposed successfully at %s", endpoint),
			Output: map[string]string{
				"serviceName": svcName,
				"serviceType": serviceType,
				"endpoint":    endpoint,
				"port":        "5432",
			},
		}, nil
	}

	return providers.ActionResult{
		Completed:    false,
		Progress:     70,
		Message:      "Waiting for external endpoint assignment",
		RequeueAfter: 10 * time.Second,
	}, nil
}

// ==================== Switchover ====================

func (a *applier) validateSwitchover(params map[string]string) error {
	if a.CNPGCluster.Spec.Instances < 2 {
		return fmt.Errorf("switchover requires at least 2 instances")
	}
	return nil
}

func (a *applier) Switchover(ctx context.Context, params map[string]string) (providers.ActionResult, error) {
	targetPrimary := params["targetPrimary"]

	// Trigger switchover by updating annotations
	if a.CNPGCluster.Annotations == nil {
		a.CNPGCluster.Annotations = make(map[string]string)
	}

	switchoverKey := "cnpg.io/switchover"
	if targetPrimary != "" {
		a.CNPGCluster.Annotations[switchoverKey] = targetPrimary
	} else {
		// Auto-select target
		a.CNPGCluster.Annotations[switchoverKey] = "any"
	}

	if err := a.Client.Update(ctx, a.CNPGCluster); err != nil {
		return providers.ActionResult{}, err
	}

	// Refresh status
	if err := a.Client.Get(ctx, client.ObjectKeyFromObject(a.CNPGCluster), a.CNPGCluster); err != nil {
		return providers.ActionResult{}, err
	}

	// Check if switchover completed
	currentPrimary := a.CNPGCluster.Status.CurrentPrimary
	if a.CNPGCluster.Status.Phase == cnpgv1.PhaseHealthy &&
		(targetPrimary == "" || currentPrimary == targetPrimary) {
		delete(a.CNPGCluster.Annotations, switchoverKey)
		if err := a.Client.Update(ctx, a.CNPGCluster); err != nil {
			return providers.ActionResult{}, err
		}

		return providers.ActionResult{
			Completed: true,
			Progress:  100,
			Message:   fmt.Sprintf("Switchover completed, new primary: %s", currentPrimary),
			Output: map[string]string{
				"newPrimary": currentPrimary,
			},
		}, nil
	}

	return providers.ActionResult{
		Completed:    false,
		Progress:     50,
		Message:      "Switchover in progress",
		RequeueAfter: 10 * time.Second,
	}, nil
}

// ==================== Custom ====================

func (a *applier) Custom(ctx context.Context, params map[string]string) (providers.ActionResult, error) {
	// Custom actions can be implemented based on specific requirements
	return providers.ActionResult{
		Completed: true,
		Progress:  100,
		Message:   "Custom action completed",
		Output:    params,
	}, nil
}
