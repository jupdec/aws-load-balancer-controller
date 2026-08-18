package shared_constants

const (
	// ExplicitGroupFinalizerPrefix the prefix for finalizers applied to an ingress group
	ExplicitGroupFinalizerPrefix = "group.ingress.k8s.aws/"

	// ImplicitGroupFinalizer the finalizer used on an ingress resource
	ImplicitGroupFinalizer = "ingress.k8s.aws/resources"

	// ServiceFinalizer the finalizer used on service resources
	ServiceFinalizer = "service.k8s.aws/resources"

	// NLBGatewayFinalizer the finalizer we attach to an NLB Gateway resource
	NLBGatewayFinalizer = "gateway.k8s.aws/nlb"

	// ALBGatewayFinalizer the finalizer we attach to an ALB Gateway resource
	ALBGatewayFinalizer = "gateway.k8s.aws/alb"

	// GlobalAcceleratorFinalizer the finalizer we attach to a global accelerator resource
	GlobalAcceleratorFinalizer = "aga.k8s.aws/resources"
)

// Finalizers on GatewayClass, LoadBalancerConfiguration, TargetGroupConfiguration, and
// ListenerRuleConfiguration are var so importers embedding this controller can rebrand them at
// init time (mirroring the pkg/gateway/constants controllerName vars introduced in #4878).
var (
	// GatewayClassFinalizer the finalizer we attach to an in-use LBC GatewayClass
	GatewayClassFinalizer = "gateway.k8s.aws/gatewayclass"

	// TargetGroupConfigurationFinalizer the finalizer we attach to a target group configuration resource
	TargetGroupConfigurationFinalizer = "gateway.k8s.aws/targetgroupconfigurations"

	// LoadBalancerConfigurationFinalizer the finalizer we attach to a load balancer configuration resource
	LoadBalancerConfigurationFinalizer = "gateway.k8s.aws/loadbalancerconfigurations"

	// ListenerRuleConfigurationFinalizer the finalizer we attach to a listener rule configuration resource
	ListenerRuleConfigurationFinalizer = "gateway.k8s.aws/listenerruleconfigurations"
)
