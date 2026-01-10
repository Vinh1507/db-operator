// internal/controller/providers/cnpg/provider.go
package cnpg

import (
	"context"
	"time"

	everestv1alpha1 "github.com/Vinh1507/db-operator/api/v1alpha1"
	cnpgv1 "github.com/cloudnative-pg/cloudnative-pg/api/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Provider implements the provider interface for CloudNativePG
type Provider struct {
	Client      client.Client
	Scheme      *runtime.Scheme
	Cluster     *everestv1alpha1.Cluster
	CNPGCluster *cnpgv1.Cluster
}

func New(
	ctx context.Context,
	c client.Client,
	scheme *runtime.Scheme,
	cluster *everestv1alpha1.Cluster,
) (*Provider, error) {
	// Try to get existing CNPG cluster
	cnpgCluster := &cnpgv1.Cluster{}
	for _, engine := range cluster.Spec.Engines {
		if engine.Type == "postgresql" {
			err := c.Get(
				ctx,
				types.NamespacedName{
					Name:      engine.Name,
					Namespace: cluster.GetNamespace(),
				},
				cnpgCluster,
			)
			if err != nil && !k8serrors.IsNotFound(err) {
				return nil, err
			}
			// Initialize with default spec if not found
			if k8serrors.IsNotFound(err) {
				cnpgCluster.Spec = defaultSpec()
			}
		}
	}

	p := &Provider{
		Client:      c,
		Scheme:      scheme,
		Cluster:     cluster,
		CNPGCluster: cnpgCluster,
	}
	return p, nil
}

// Apply returns the applier for this provider
func (p *Provider) Apply(ctx context.Context) Applier {
	return &applier{
		Provider: p,
		ctx:      ctx,
	}
}

// Status builds status information from CNPG cluster
func (p *Provider) Status(
	ctx context.Context,
) (everestv1alpha1.ClusterStatus, error) {

	cluster := p.Cluster
	clusterStatus := cluster.Status

	cnpg := p.CNPGCluster
	engineType := "postgresql"

	credentialsSecrets := []everestv1alpha1.CredentialSecretRef{}
	for _, credential := range cluster.Spec.Engines[0].CredentialsSecretNames {
		credentialsSecrets = append(credentialsSecrets, everestv1alpha1.CredentialSecretRef{
			Name:         credential,
			RefName:      credential,
			RefNamespace: cluster.Namespace,
		})
	}
	newEngineStatus := everestv1alpha1.EngineStatus{
		Name:               cnpg.Name,
		Type:               engineType,
		Phase:              mapCNPGPhase(cnpg.Status.Phase),
		Ready:              int32(cnpg.Status.ReadyInstances),
		Total:              int32(cnpg.Spec.Instances),
		Conditions:         cnpg.Status.Conditions,
		CredentialsSecrets: credentialsSecrets,
	}

	// ---- Sync engine status ----
	found := false
	for i, engine := range clusterStatus.Engines {
		if engine.Type == engineType {
			clusterStatus.Engines[i] = newEngineStatus
			found = true
			break
		}
	}

	if !found {
		clusterStatus.Engines = append(clusterStatus.Engines, newEngineStatus)
	}

	// ---- Calculate cluster phase ----
	allReady := true
	anyFailed := false

	for _, engine := range clusterStatus.Engines {
		switch engine.Phase {
		case "Failed":
			anyFailed = true
			allReady = false
		case "Ready":
			// ok
		default:
			allReady = false
		}
	}

	switch {
	case anyFailed:
		clusterStatus.Phase = "Failed"
	case allReady && len(clusterStatus.Engines) == len(cluster.Spec.Engines):
		clusterStatus.Phase = "Ready"
	default:
		clusterStatus.Phase = "Initializing"
	}

	// ---- Init endpoints if nil ----
	// if clusterStatus.Endpoints == nil {
	clusterStatus.Endpoints = []everestv1alpha1.Endpoint{}
	// }

	var svcList corev1.ServiceList
	if err := p.Client.List(
		ctx,
		&svcList,
		client.InNamespace(cluster.Namespace),
		client.MatchingLabels{
			"cnpg.io/cluster": cluster.Name,
		},
	); err != nil {
		return clusterStatus, nil
	}

	for i := range svcList.Items {
		svc := &svcList.Items[i]
		if len(svc.Status.LoadBalancer.Ingress) > 0 {
			clusterStatus.Endpoints = append(clusterStatus.Endpoints, everestv1alpha1.Endpoint{
				Name:     svc.Name,
				Endpoint: svc.Status.LoadBalancer.Ingress[0].IP + ":5432",
			})
		}
	}
	return clusterStatus, nil
}

// RunPreReconcileHook checks if reconciliation should proceed
func (p *Provider) RunPreReconcileHook(ctx context.Context, cluster everestv1alpha1.Cluster) (bool, time.Duration, string, error) {
	p.SetName(cluster.Name)
	p.SetNamespace(cluster.Namespace)
	// Check if restore is in progress
	if restoring, err := p.isRestoreInProgress(ctx); err != nil {
		return false, 0, "", err
	} else if restoring {
		return false, 15 * time.Second, "Restore is in progress", nil
	}

	return true, 0, "", nil
}

// isRestoreInProgress checks for ongoing restore operations
func (p *Provider) isRestoreInProgress(ctx context.Context) (bool, error) {
	// Check if CNPG cluster has bootstrap recovery configured
	if p.CNPGCluster.Spec.Bootstrap != nil &&
		p.CNPGCluster.Spec.Bootstrap.Recovery != nil {
		// Still restoring if cluster not ready
		if p.CNPGCluster.Status.Phase != cnpgv1.PhaseHealthy {
			return true, nil
		}
	}
	return false, nil
}

// Cleanup handles finalizer and cleanup logic
func (p *Provider) Cleanup(ctx context.Context) (bool, error) {
	// Delete CNPG cluster
	cnpg := &cnpgv1.Cluster{}
	err := p.Client.Get(
		ctx,
		types.NamespacedName{
			Name:      p.Cluster.GetName(),
			Namespace: p.Cluster.GetNamespace(),
		},
		cnpg,
	)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return true, nil // Already deleted
		}
		return false, err
	}

	// Delete the cluster
	if err := p.Client.Delete(ctx, cnpg); err != nil {
		return false, err
	}

	// Check if deletion completed
	return k8serrors.IsNotFound(
		p.Client.Get(ctx, types.NamespacedName{
			Name:      p.Cluster.GetName(),
			Namespace: p.Cluster.GetNamespace(),
		}, cnpg),
	), nil
}

type DBObjectWithMutate struct {
	Object     client.Object
	MutateFunc func() error
}

// SetName sets the name of CNPG cluster
func (p *Provider) SetName(name string) {
	p.CNPGCluster.SetName(name)
}

// SetNamespace sets the namespace of CNPG cluster
func (p *Provider) SetNamespace(namespace string) {
	p.CNPGCluster.SetNamespace(namespace)
}

// Helper functions

func mapCNPGPhase(phase string) everestv1alpha1.EnginePhase {
	switch phase {
	case string(cnpgv1.PhaseHealthy):
		return everestv1alpha1.EnginePhaseReady
	case string(cnpgv1.PhaseUpgrade):
		return everestv1alpha1.EnginePhaseUpgrading
	case string(cnpgv1.PhaseWaitingForUser):
		return everestv1alpha1.EnginePhasePaused
	case string(cnpgv1.PhaseUnrecoverable):
		return everestv1alpha1.EnginePhaseFailed
	default:
		return everestv1alpha1.EnginePhaseInitializing
	}
}

func getConditionMessages(conditions []cnpgv1.ClusterConditionType) []string {
	messages := []string{}
	// for _, cond := range conditions {
	// 	if cond.Message != "" {
	// 		messages = append(messages, cond.Message)
	// 	}
	// }
	return messages
}
