package crddetect

import (
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/aws-load-balancer-controller/v3/pkg/config"
	"sigs.k8s.io/aws-load-balancer-controller/v3/pkg/k8s"
)

const (
	// GatewayV1GroupVersion is the stable Gateway API group version.
	GatewayV1GroupVersion = "gateway.networking.k8s.io/v1"
	// LBCGatewayGroupVersion is the LBC-specific Gateway CRD group version.
	LBCGatewayGroupVersion = "gateway.k8s.aws/v1"
)

var (
	// LBCGatewayKinds are the AWS-vended CRDs required by both ALB and NLB gateway controllers.
	LBCGatewayKinds = []string{"TargetGroupConfiguration", "LoadBalancerConfiguration", "ListenerRuleConfiguration"}

	// ALBKinds is the exact CRD subset the ALB gateway reconciler requires.
	ALBKinds = map[string][]string{
		GatewayV1GroupVersion:  {"Gateway", "GatewayClass", "HTTPRoute", "GRPCRoute", "ReferenceGrant"},
		LBCGatewayGroupVersion: LBCGatewayKinds,
	}

	// NLBKinds is the exact CRD subset the NLB gateway reconciler requires.
	NLBKinds = map[string][]string{
		GatewayV1GroupVersion:  {"Gateway", "GatewayClass", "TLSRoute", "TCPRoute", "UDPRoute", "ReferenceGrant"},
		LBCGatewayGroupVersion: LBCGatewayKinds,
	}

	// ListenerSetKinds is the CRD subset that gates the GatewayListenerSet feature.
	ListenerSetKinds = map[string][]string{GatewayV1GroupVersion: {"ListenerSet"}}
)

// GatewayCRDPresence is the pure result of DetectGatewayCRDs: what's served
// under the gate-relevant group versions, plus the missing-kinds lists for
// each of the three gate-relevant subsets. Nil-safe — an empty missing slice
// means "nothing missing." Consumers that install CRDs dynamically can consult
// this to decide what to install next without also mutating feature gates.
type GatewayCRDPresence struct {
	AvailableResources map[string]sets.Set[string]
	ALBMissing         []string
	NLBMissing         []string
	ListenerSetMissing []string
}

// DetectGatewayCRDs reads the apiserver Discovery once and returns the served
// kinds and which are missing from each gate-relevant CRD subset. Pure read;
// no side effects. Safe to call in a loop (poller tick, retry, etc.).
func DetectGatewayCRDs(client k8s.DiscoveryClient) (GatewayCRDPresence, error) {
	availableResources, err := k8s.DetectCRDs(client, sets.New(GatewayV1GroupVersion, LBCGatewayGroupVersion))
	if err != nil {
		return GatewayCRDPresence{}, err
	}
	return GatewayCRDPresence{
		AvailableResources: availableResources,
		ALBMissing:         MissingKinds(ALBKinds, availableResources),
		NLBMissing:         MissingKinds(NLBKinds, availableResources),
		ListenerSetMissing: MissingKinds(ListenerSetKinds, availableResources),
	}, nil
}

// ApplyGatewayCRDDetection checks for the presence of Gateway API CRDs and
// disables the corresponding feature flags when required CRDs are missing.
// It is called from main() after the k8s client is ready and before any
// controller reads the feature flags.
func ApplyGatewayCRDDetection(client k8s.DiscoveryClient, featureGates config.FeatureGates, logger logr.Logger) error {

	allDefaulted := featureGates.GetFeatureStatus(config.ALBGatewayAPI).IsDefaulted ||
		featureGates.GetFeatureStatus(config.NLBGatewayAPI).IsDefaulted ||
		featureGates.GetFeatureStatus(config.GatewayListenerSet).IsDefaulted

	if !allDefaulted {
		// User set all flags directly, do nothing.
		return nil
	}

	availableResources, err := k8s.DetectCRDs(client, sets.New(GatewayV1GroupVersion, LBCGatewayGroupVersion))
	if err != nil {
		return err
	}

	applyGatewayFeatureFlags(availableResources, featureGates, logger)
	return nil
}

func applyGatewayFeatureFlags(availableResources map[string]sets.Set[string], featureGates config.FeatureGates, logger logr.Logger) {

	albMissingKinds := MissingKinds(ALBKinds, availableResources)
	if len(albMissingKinds) > 0 {
		logger.Info("Disabling ALBGatewayAPI: missing required CRDs",
			"missing", albMissingKinds)
		featureGates.Disable(config.ALBGatewayAPI)
	}

	nlbMissingKinds := MissingKinds(NLBKinds, availableResources)
	if len(nlbMissingKinds) > 0 && featureGates.GetFeatureStatus(config.NLBGatewayAPI).IsDefaulted {
		logger.Info("Disabling NLBGatewayAPI: missing required CRDs",
			"missing", nlbMissingKinds)
		featureGates.Disable(config.NLBGatewayAPI)
	}

	listenerSetMissing := MissingKinds(ListenerSetKinds, availableResources)
	if len(listenerSetMissing) > 0 && featureGates.GetFeatureStatus(config.GatewayListenerSet).IsDefaulted {
		logger.Info("Disabling GatewayListenerSet: missing required CRDs", "missing", listenerSetMissing)
		featureGates.Disable(config.GatewayListenerSet)
	}
}

// MissingKinds returns the kinds in desiredKinds that aren't served per the
// availableResources snapshot from k8s.DetectCRDs. Exposed for downstream
// callers that vend their own kind tables — for example, an operator that
// installs Gateway API CRDs dynamically and needs to know which kinds to
// install next based on the current Discovery state.
func MissingKinds(desiredKinds map[string][]string, availableResources map[string]sets.Set[string]) []string {
	missing := make([]string, 0)

	for apiVersion, kinds := range desiredKinds {
		var ok bool
		var availableKinds sets.Set[string]
		if availableKinds, ok = availableResources[apiVersion]; !ok {
			missing = append(missing, kinds...)
			continue
		}
		for _, kind := range kinds {
			if !availableKinds.Has(kind) {
				missing = append(missing, kind)
			}
		}
	}

	return missing
}
