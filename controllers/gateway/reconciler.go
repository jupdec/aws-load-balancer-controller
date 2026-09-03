package gateway

import (
	"context"
	"k8s.io/client-go/kubernetes"
	elbv2gw "sigs.k8s.io/aws-load-balancer-controller/v3/apis/gateway/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// LBConfigValidator rejects a resolved LoadBalancerConfiguration whose fields aren't supported by the deployment (e.g. EKS Auto Mode).
type LBConfigValidator func(lbConf elbv2gw.LoadBalancerConfiguration) error

// LBConfigValidatorSetter is implemented by gateway reconcilers that accept a deployment-specific LBConfigValidator.
type LBConfigValidatorSetter interface {
	SetLBConfigValidator(v LBConfigValidator)
}

type Reconciler interface {
	Reconcile(ctx context.Context, req reconcile.Request) (ctrl.Result, error)
	SetupWithManager(ctx context.Context, mgr ctrl.Manager) (controller.Controller, error)
	SetupWatches(ctx context.Context, controller controller.Controller, mgr ctrl.Manager, clientSet *kubernetes.Clientset) error
}
