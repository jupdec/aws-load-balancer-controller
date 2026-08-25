package test_resources

import (
	"context"
	"strconv"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/v3/apis/gateway/v1"
	"sigs.k8s.io/aws-load-balancer-controller/v3/pkg/k8s"
	"sigs.k8s.io/aws-load-balancer-controller/v3/test/framework"
	"sigs.k8s.io/aws-load-balancer-controller/v3/test/framework/utils"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwbeta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
)

// nsDeleteWaitTimeout bounds how long DeleteNamespace waits for a Gateway API test
// namespace to fully drain before force-stripping controller finalizers on its child
// CRs. Chosen large enough for a healthy `stackDeployer.Deploy(delete)` on an NLB + TGs
// (~90-180s worst case with AWS API throttling), short enough to make progress within
// a 60-90 min suite when a controller reconcile is stuck.
const nsDeleteWaitTimeout = 3 * time.Minute

// nsDeleteRetryTimeout is a second, shorter wait after we've stripped finalizers to
// give the apiserver time to garbage-collect the child CRs and complete namespace
// deletion.
const nsDeleteRetryTimeout = 90 * time.Second

const (
	CrossNamespacePort = 5000
)

func NewCommonResourceStack(dps []*appsv1.Deployment, svcs []*corev1.Service, gwc *gwv1.GatewayClass, gw *gwv1.Gateway, lbc *elbv2gw.LoadBalancerConfiguration, tgcs []*elbv2gw.TargetGroupConfiguration, lrcs []*elbv2gw.ListenerRuleConfiguration, baseName string, namespaceLabels map[string]string) *CommonResourceStack {
	return &CommonResourceStack{
		Dps:             dps,
		Svcs:            svcs,
		Gwc:             gwc,
		Gw:              gw,
		Lbc:             lbc,
		Tgcs:            tgcs,
		Lrcs:            lrcs,
		BaseName:        baseName,
		NamespaceLabels: namespaceLabels,
	}
}

// CommonResourceStack contains resources that are common between nlb / alb gateways
type CommonResourceStack struct {
	// configurations
	Svcs            []*corev1.Service
	Dps             []*appsv1.Deployment
	Gwc             *gwv1.GatewayClass
	Gw              *gwv1.Gateway
	Lbc             *elbv2gw.LoadBalancerConfiguration
	Tgcs            []*elbv2gw.TargetGroupConfiguration
	Lrcs            []*elbv2gw.ListenerRuleConfiguration
	Ns              *corev1.Namespace
	BaseName        string
	NamespaceLabels map[string]string

	// runtime variables
	CreatedGW *gwv1.Gateway
}

func (s *CommonResourceStack) Deploy(ctx context.Context, f *framework.Framework, resourceSpecificCreation func(ctx context.Context, f *framework.Framework, namespace string) error) error {
	ns, err := AllocateNamespace(ctx, f, s.BaseName, s.NamespaceLabels)
	if err != nil {
		return err
	}
	s.Ns = ns

	for _, v := range s.Dps {
		v.Namespace = s.Ns.Name
	}

	for _, v := range s.Svcs {
		v.Namespace = s.Ns.Name
	}

	if s.Tgcs != nil {
		for _, v := range s.Tgcs {
			v.Namespace = s.Ns.Name
		}
	}

	if s.Lrcs != nil {
		for _, v := range s.Lrcs {
			v.Namespace = s.Ns.Name
		}
	}

	s.Gw.Namespace = s.Ns.Name
	s.Lbc.Namespace = s.Ns.Name

	if err := CreateGatewayClass(ctx, f, s.Gwc); err != nil {
		return err
	}
	if err := CreateLoadBalancerConfig(ctx, f, s.Lbc); err != nil {
		return err
	}
	if err := CreateTargetGroupConfigs(ctx, f, s.Tgcs); err != nil {
		return err
	}
	if err := CreateListenerRuleConfigs(ctx, f, s.Lrcs); err != nil {
		return err
	}
	if err := CreateDeployments(ctx, f, s.Dps); err != nil {
		return err
	}
	if err := CreateServices(ctx, f, s.Svcs); err != nil {
		return err
	}

	if err := CreateGateway(ctx, f, s.Gw); err != nil {
		return err
	}

	if err := resourceSpecificCreation(ctx, f, s.Ns.Name); err != nil {
		return err
	}

	if err := WaitUntilDeploymentReady(ctx, f, s.Dps); err != nil {
		return err
	}

	if err := WaitUntilServiceReady(ctx, f, s.Svcs); err != nil {
		return err
	}

	observedGateway, err := WaitUntilGatewayReady(ctx, f, s.Gw)
	if err != nil {
		return err
	}
	s.CreatedGW = observedGateway
	return nil
}

func (s *CommonResourceStack) Cleanup(ctx context.Context, f *framework.Framework) error {
	if err := DeleteNamespace(ctx, f, s.Ns); err != nil {
		return err
	}
	return DeleteGatewayClass(ctx, f, s.Gwc)
}

func (s *CommonResourceStack) GetLoadBalancerIngressHostname() string {
	return s.CreatedGW.Status.Addresses[0].Value
}

func (s *CommonResourceStack) GetListenersPortMap() map[string]string {
	listenersMap := map[string]string{}
	for _, l := range s.CreatedGW.Spec.Listeners {
		listenersMap[strconv.Itoa(int(l.Port))] = string(l.Protocol)
	}
	return listenersMap
}

func CreateDeployments(ctx context.Context, f *framework.Framework, dps []*appsv1.Deployment) error {
	for _, dp := range dps {
		f.Logger.Info("creating deployment", "dp", k8s.NamespacedName(dp))
		if err := f.K8sClient.Create(ctx, dp); err != nil {
			f.Logger.Info("failed to create deployment")
			return err
		}
		f.Logger.Info("created deployment", "dp", k8s.NamespacedName(dp))
	}
	return nil
}

func WaitUntilDeploymentReady(ctx context.Context, f *framework.Framework, dps []*appsv1.Deployment) error {
	for _, dp := range dps {
		f.Logger.Info("waiting until deployment becomes ready", "dp", k8s.NamespacedName(dp))
		_, err := f.DPManager.WaitUntilDeploymentReady(ctx, dp)
		if err != nil {
			f.Logger.Info("failed waiting for deployment")
			return err
		}
		f.Logger.Info("deployment is ready", "dp", k8s.NamespacedName(dp))
	}
	return nil
}

func CreateServices(ctx context.Context, f *framework.Framework, svcs []*corev1.Service) error {
	for _, svc := range svcs {
		f.Logger.Info("creating service", "svc", k8s.NamespacedName(svc))
		if err := f.K8sClient.Create(ctx, svc); err != nil {
			f.Logger.Info("failed to create service")
			return err
		}
		f.Logger.Info("created service", "svc", k8s.NamespacedName(svc))
	}
	return nil
}

func CreateReferenceGrants(ctx context.Context, f *framework.Framework, refGrants []*gwbeta1.ReferenceGrant) error {
	f.Logger.Info("About to create ref grant")
	for _, refg := range refGrants {
		f.Logger.Info("creating ref grant", "refg", k8s.NamespacedName(refg))
		if err := f.K8sClient.Create(ctx, refg); err != nil {
			f.Logger.Error(err, "failed to create ref grant")
			return err
		}
		f.Logger.Info("created ref grant", "refg", k8s.NamespacedName(refg))
	}
	return nil
}

func deleteReferenceGrants(ctx context.Context, f *framework.Framework, refGrants []*gwbeta1.ReferenceGrant) error {
	f.Logger.Info("About to delete ref grant")
	for _, refg := range refGrants {
		f.Logger.Info("deleting ref grant", "refg", k8s.NamespacedName(refg))
		if err := f.K8sClient.Delete(ctx, refg); err != nil {
			f.Logger.Error(err, "failed to delete ref grant")
			return err
		}
		f.Logger.Info("deleted ref grant", "refg", k8s.NamespacedName(refg))
	}
	return nil
}

func CreateGatewayClass(ctx context.Context, f *framework.Framework, gwc *gwv1.GatewayClass) error {
	f.Logger.Info("creating gateway class", "gwc", k8s.NamespacedName(gwc))
	err := f.K8sClient.Create(ctx, gwc)
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return err
	}
	if apierrors.IsAlreadyExists(err) {
		f.Logger.Info("gateway class already exists", "gwc", k8s.NamespacedName(gwc))
	}
	return nil
}

func CreateLoadBalancerConfig(ctx context.Context, f *framework.Framework, lbc *elbv2gw.LoadBalancerConfiguration) error {
	f.Logger.Info("creating loadbalancer config", "lbc", k8s.NamespacedName(lbc))
	return f.K8sClient.Create(ctx, lbc)
}

func CreateTargetGroupConfigs(ctx context.Context, f *framework.Framework, tgcs []*elbv2gw.TargetGroupConfiguration) error {
	for _, tgc := range tgcs {
		f.Logger.Info("creating target group config", "tgc", k8s.NamespacedName(tgc))
		err := f.K8sClient.Create(ctx, tgc)
		if err != nil {
			f.Logger.Error(err, "failed to create target group config")
			return err
		}
		f.Logger.Info("created target group config", "tgc", k8s.NamespacedName(tgc))
	}
	return nil
}

func CreateListenerRuleConfigs(ctx context.Context, f *framework.Framework, lrcs []*elbv2gw.ListenerRuleConfiguration) error {
	for _, lrc := range lrcs {
		f.Logger.Info("creating listener rule config", "lrc", k8s.NamespacedName(lrc))
		err := f.K8sClient.Create(ctx, lrc)
		if err != nil {
			f.Logger.Error(err, "failed to create listener rule config")
			return err
		}
		f.Logger.Info("created listener rule config", "tgc", k8s.NamespacedName(lrc))
	}
	return nil
}

func CreateGateway(ctx context.Context, f *framework.Framework, gw *gwv1.Gateway) error {
	f.Logger.Info("creating gateway", "gw", k8s.NamespacedName(gw))
	return f.K8sClient.Create(ctx, gw)
}

func WaitUntilServiceReady(ctx context.Context, f *framework.Framework, svcs []*corev1.Service) error {
	for _, svc := range svcs {
		observedSvc := &corev1.Service{}
		err := f.K8sClient.Get(ctx, k8s.NamespacedName(svc), observedSvc)
		if err != nil {
			f.Logger.Error(err, "unable to observe service go ready")
			return err
		}
	}
	return nil
}

func WaitUntilGatewayReady(ctx context.Context, f *framework.Framework, gw *gwv1.Gateway) (*gwv1.Gateway, error) {
	observedGw := &gwv1.Gateway{}

	err := wait.PollImmediateUntil(utils.PollIntervalShort, func() (bool, error) {
		if err := f.K8sClient.Get(ctx, k8s.NamespacedName(gw), observedGw); err != nil {
			return false, err
		}

		if observedGw.Status.Conditions != nil {
			for _, cond := range observedGw.Status.Conditions {
				if cond.Type == string(gwv1.GatewayConditionProgrammed) && cond.Status == metav1.ConditionTrue {
					return true, nil
				}
			}
		}

		return false, nil
	}, ctx.Done())
	if err != nil {
		return nil, err
	}
	return observedGw, nil
}

func DeleteGatewayClass(ctx context.Context, f *framework.Framework, gwc *gwv1.GatewayClass) error {
	return f.K8sClient.Delete(ctx, gwc)
}

func DeleteNamespace(ctx context.Context, tf *framework.Framework, ns *corev1.Namespace) error {
	tf.Logger.Info("deleting namespace", "ns", k8s.NamespacedName(ns))
	if err := tf.K8sClient.Delete(ctx, ns); err != nil && !apierrors.IsNotFound(err) {
		tf.Logger.Info("failed to delete namespace", "ns", k8s.NamespacedName(ns))
		return err
	}

	// Bound the wait. If the LBC controller is stuck reconciling a delete (partial
	// stack teardown, AWS throttling, TG-deregister waits), Gateway/LBC/TGC finalizers
	// stay set and namespace drain blocks indefinitely. Cap the first wait at
	// nsDeleteWaitTimeout, then force-strip finalizers and retry a shorter wait.
	waitCtx, cancel := context.WithTimeout(ctx, nsDeleteWaitTimeout)
	defer cancel()
	if err := tf.NSManager.WaitUntilNamespaceDeleted(waitCtx, ns); err == nil {
		tf.Logger.Info("deleted namespace", "ns", k8s.NamespacedName(ns))
		return nil
	} else {
		tf.Logger.Info("namespace deletion timed out; stripping gateway-API CR finalizers and retrying",
			"ns", k8s.NamespacedName(ns), "waitErr", err.Error())
	}

	// Use a fresh Background context for the strip so a canceled parent doesn't skip cleanup.
	stripGatewayAPIFinalizers(context.Background(), tf, ns.Name)

	retryCtx, retryCancel := context.WithTimeout(ctx, nsDeleteRetryTimeout)
	defer retryCancel()
	if err := tf.NSManager.WaitUntilNamespaceDeleted(retryCtx, ns); err != nil {
		tf.Logger.Info("failed to wait for namespace deletion after finalizer strip",
			"ns", k8s.NamespacedName(ns), "err", err.Error())
		return err
	}
	tf.Logger.Info("deleted namespace (after finalizer strip)", "ns", k8s.NamespacedName(ns))
	return nil
}

// stripGatewayAPIFinalizers clears finalizers on Gateway API + gateway.k8s.aws CRs in the
// given namespace so namespace deletion can proceed when the controller is stuck
// reconciling its delete path. Best-effort: individual errors are logged but not
// returned; a caller that needs a hard failure should re-check namespace state after.
func stripGatewayAPIFinalizers(ctx context.Context, tf *framework.Framework, nsName string) {
	// Gateway objects hold the primary controller finalizer.
	gwList := &gwv1.GatewayList{}
	if err := tf.K8sClient.List(ctx, gwList, client.InNamespace(nsName)); err == nil {
		for i := range gwList.Items {
			gw := &gwList.Items[i]
			if len(gw.Finalizers) == 0 {
				continue
			}
			gw.Finalizers = nil
			if err := tf.K8sClient.Update(ctx, gw); err != nil && !apierrors.IsNotFound(err) {
				tf.Logger.Info("strip Gateway finalizer failed", "gw", k8s.NamespacedName(gw), "err", err.Error())
			}
		}
	}

	// LoadBalancerConfiguration — released when no Gateway references it (see
	// loadbalancer_configuration_controller.go handleDelete). Nulling the finalizer
	// short-circuits that chain when the corresponding Gateway is also stuck.
	lbcList := &elbv2gw.LoadBalancerConfigurationList{}
	if err := tf.K8sClient.List(ctx, lbcList, client.InNamespace(nsName)); err == nil {
		for i := range lbcList.Items {
			lbc := &lbcList.Items[i]
			if len(lbc.Finalizers) == 0 {
				continue
			}
			lbc.Finalizers = nil
			if err := tf.K8sClient.Update(ctx, lbc); err != nil && !apierrors.IsNotFound(err) {
				tf.Logger.Info("strip LBC finalizer failed", "lbc", k8s.NamespacedName(lbc), "err", err.Error())
			}
		}
	}

	// TargetGroupConfiguration.
	tgcList := &elbv2gw.TargetGroupConfigurationList{}
	if err := tf.K8sClient.List(ctx, tgcList, client.InNamespace(nsName)); err == nil {
		for i := range tgcList.Items {
			tgc := &tgcList.Items[i]
			if len(tgc.Finalizers) == 0 {
				continue
			}
			tgc.Finalizers = nil
			if err := tf.K8sClient.Update(ctx, tgc); err != nil && !apierrors.IsNotFound(err) {
				tf.Logger.Info("strip TGC finalizer failed", "tgc", k8s.NamespacedName(tgc), "err", err.Error())
			}
		}
	}

	// ListenerRuleConfiguration.
	lrcList := &elbv2gw.ListenerRuleConfigurationList{}
	if err := tf.K8sClient.List(ctx, lrcList, client.InNamespace(nsName)); err == nil {
		for i := range lrcList.Items {
			lrc := &lrcList.Items[i]
			if len(lrc.Finalizers) == 0 {
				continue
			}
			lrc.Finalizers = nil
			if err := tf.K8sClient.Update(ctx, lrc); err != nil && !apierrors.IsNotFound(err) {
				tf.Logger.Info("strip LRC finalizer failed", "lrc", k8s.NamespacedName(lrc), "err", err.Error())
			}
		}
	}
}
