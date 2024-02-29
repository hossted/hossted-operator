package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	hosstedcomv1 "github.com/hossted/hossted-operator/api/v1"
	internalhttp "github.com/hossted/hossted-operator/pkg/http"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"time"
)

// HosstedProjectReconciler reconciles a HosstedProject object
type HosstedProjectReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=hossted.com,resources=hosstedprojects,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=hossted.com,resources=hosstedprojects/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=hossted.com,resources=hosstedprojects/finalizers,verbs=update

func (r *HosstedProjectReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)

	instance := &hosstedcomv1.Hosstedproject{}
	err := r.Get(ctx, req.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	collector, err := r.collector(ctx, instance)
	if err != nil {
		return ctrl.Result{}, err
	} else {
		j, _ := json.Marshal(collector)
		// resp, err := internalhttp.HttpRequest(j)

		fmt.Println(string(j))
		return ctrl.Result{RequeueAfter: time.Second * 10}, nil
	}

}

// SetupWithManager sets up the controller with the Manager.
func (r *HosstedProjectReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&hosstedcomv1.Hosstedproject{}).
		Complete(r)
}
