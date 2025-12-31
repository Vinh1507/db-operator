// internal/controller/providers/cnpg/provider.go
package cnpg

import (
	"context"
	"time"

	everestv1alpha1 "github.com/Vinh1507/db-operator/api/v1alpha1"
	cnpgv1 "github.com/cloudnative-pg/cloudnative-pg/api/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Provider implements the provider interface for CloudNativePG
type Provider struct {
	Client      client.Client
	Scheme      *runtime.Scheme
	Cluster     *everestv1alpha1.Cluster
	CNPGCluster *cnpgv1.Cluster
	EngineSpec  everestv1alpha1.EngineSpec

	// Track current spec for diffing
	currentClusterSpec cnpgv1.ClusterSpec
}

// New creates a new CNPG provider instance
func New(
	ctx context.Context,
	c client.Client,
	scheme *runtime.Scheme,
	cluster *everestv1alpha1.Cluster,
	engineSpec everestv1alpha1.EngineSpec,
) (*Provider, error) {
	// Try to get existing CNPG cluster
	cnpgCluster := &cnpgv1.Cluster{}
	err := c.Get(
		ctx,
		types.NamespacedName{
			Name:      engineSpec.Name,
			Namespace: cluster.GetNamespace(),
		},
		cnpgCluster,
	)
	if err != nil && !k8serrors.IsNotFound(err) {
		return nil, err
	}

	// Store current spec for later comparison
	currentSpec := cnpgCluster.Spec

	// Initialize with default spec if not found
	if k8serrors.IsNotFound(err) {
		cnpgCluster.Spec = defaultSpec()
	}

	p := &Provider{
		Client:             c,
		Scheme:             scheme,
		Cluster:            cluster,
		CNPGCluster:        cnpgCluster,
		EngineSpec:         engineSpec,
		currentClusterSpec: currentSpec,
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
) (everestv1alpha1.EngineStatus, error) {
	status := everestv1alpha1.EngineStatus{}
	cnpg := p.CNPGCluster

	// Map CNPG phase to our status
	// p.Cluster.Spec
	status.Name = p.CNPGCluster.Name
	status.Type = "cnpg"
	status.Phase = mapCNPGPhase(cnpg.Status.Phase)
	status.Ready = int32(cnpg.Status.ReadyInstances)
	status.Total = int32(cnpg.Spec.Instances)
	// status.Message = strings.Join(getConditionMessages(cnpg.Status.Conditions), "; ")
	status.Conditions = cnpg.Status.Conditions

	// Check if upgrade in progress
	// if p.EngineSpec.Version != "" &&
	// 	cnpg.Status.Image != "" &&
	// 	!strings.Contains(cnpg.Status.Image, p.EngineSpec.Version) {
	// 	status.Phase = "Upgrading"
	// }

	return status, nil
}

// RunPreReconcileHook checks if reconciliation should proceed
func (p *Provider) RunPreReconcileHook(ctx context.Context) (bool, time.Duration, string, error) {
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
			Name:      p.EngineSpec.Name,
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
			Name:      p.EngineSpec.Name,
			Namespace: p.Cluster.GetNamespace(),
		}, cnpg),
	), nil
}

// DBObjects returns the underlying CNPG cluster object
func (p *Provider) DBObjects() []client.Object {
	p.CNPGCluster.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   cnpgv1.SchemeGroupVersion.Group,
		Version: cnpgv1.SchemeGroupVersion.Version,
		Kind:    "Cluster",
	})
	return []client.Object{p.CNPGCluster}
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
