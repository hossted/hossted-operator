package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

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
//+kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses,verbs=list;get;watch
//+kubebuilder:rbac:groups="",resources=namespaces,verbs=list;get;watch
//+kubebuilder:rbac:groups="",resources=pods,verbs=list;get;watch
//+kubebuilder:rbac:groups="",resources=configmaps,verbs=list;get;watch
//+kubebuilder:rbac:groups="",resources=services,verbs=list;get;watch
//+kubebuilder:rbac:groups="",resources=secrets,verbs=list;get;watch
//+kubebuilder:rbac:groups="",resources=persistentvolumeclaims,verbs=list;get;watch

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
		collector, currentRevision, err := r.collector(ctx, instance)
		if err != nil {
			return ctrl.Result{}, err
		}

		sort.Ints(currentRevision)

		_, _, err = r.patchStatus(ctx, instance, func(obj client.Object) client.Object {
			in := obj.(*hosstedcomv1.Hosstedproject)
			if in.Status.Revision == nil {
				in.Status.Revision = currentRevision
			}
			return in
		})
		if err != nil {
			return ctrl.Result{}, err
		}

		if !compareSlices(instance.Status.Revision, currentRevision) {
			// Marshal collectors into JSON
			collectorJson, err := json.Marshal(collector)
			if err != nil {
				return ctrl.Result{}, err
			}
			fmt.Println(string(collectorJson))
			// post request
			logger.Info("Current state differs from last state", "name", instance.Name)

			_, _, err = r.patchStatus(ctx, instance, func(obj client.Object) client.Object {
				in := obj.(*hosstedcomv1.Hosstedproject)
				in.Status.Revision = currentRevision
				return in
			})
			if err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{RequeueAfter: time.Second * 10}, nil
		} else {
			return ctrl.Result{RequeueAfter: time.Second * 10}, nil
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
