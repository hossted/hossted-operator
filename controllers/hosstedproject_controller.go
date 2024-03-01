package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/google/uuid"
	hosstedcomv1 "github.com/hossted/hossted-operator/api/v1"

	// internalhttp "github.com/hossted/hossted-operator/pkg/http"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// HosstedProjectReconciler reconciles a HosstedProject object
type HosstedProjectReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=hossted.com,resources=hosstedprojects,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=hossted.com,resources=hosstedprojects/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=hossted.com,resources=hosstedprojects/finalizers,verbs=update

// Reconcile reconciles the Hosstedproject custom resource.
func (r *HosstedProjectReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := ctrl.Log.WithName("controllers").WithName("hosstedproject")

	// Get Hosstedproject custom resource
	instance := &hosstedcomv1.Hosstedproject{}
	err := r.Get(ctx, req.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Check if reconciliation should proceed
	if !instance.Spec.Stop {
		// Patch the status with ClusterUUID
		_, _, err := r.patchStatus(ctx, instance, func(obj client.Object) client.Object {
			in := obj.(*hosstedcomv1.Hosstedproject)
			if in.Status.ClusterUUID == "" {
				in.Status.ClusterUUID = uuid.NewString()
			}
			return in
		})
		if err != nil {
			return ctrl.Result{}, err
		}

		// Collect info about resources
		collectors, err := r.collector(ctx, instance)
		if err != nil {
			return ctrl.Result{}, err
		}

		// Marshal collectors into JSON
		desiredState, err := json.Marshal(collectors)
		if err != nil {
			return ctrl.Result{}, err
		}

		// Check if the desired state is different from the current state
		ok, err := IsEqualJson(string(desiredState), instance.Status.CurrentState)
		if !ok {
			_, _, err := r.patchStatus(ctx, instance, func(obj client.Object) client.Object {
				in := obj.(*hosstedcomv1.Hosstedproject)
				if in.Status.CurrentState == "" {
					in.Status.CurrentState = string(desiredState)
					return in
				}
				in.Status.CurrentState = string(desiredState)
				return in
			})
			if err != nil {
				return ctrl.Result{}, err
			} else {
				logger.Info("Desired state is same as current state. Skipping patching.")
				return ctrl.Result{RequeueAfter: time.Second * 10}, nil
			}

		}
	}

	logger.Info("Reconciliation stopped", "name", req.Name)
	return ctrl.Result{RequeueAfter: time.Second * 10}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *HosstedProjectReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&hosstedcomv1.Hosstedproject{}).
		Complete(r)
}

func IsEqualJson(s1, s2 string) (bool, error) {
	var o1 interface{}
	var o2 interface{}

	var err error
	err = json.Unmarshal([]byte(s1), &o1)
	if err != nil {
		return false, fmt.Errorf("error mashalling string 1 :: %s", err.Error())
	}
	err = json.Unmarshal([]byte(s2), &o2)
	if err != nil {
		return false, fmt.Errorf("error mashalling string 2 :: %s", err.Error())
	}

	return reflect.DeepEqual(o1, o2), nil
}
