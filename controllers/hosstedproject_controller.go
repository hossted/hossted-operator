package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"

	"time"

	"github.com/google/uuid"
	hosstedcomv1 "github.com/hossted/hossted-operator/api/v1"
	internalHTTP "github.com/hossted/hossted-operator/pkg/http"

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
	logger := ctrl.Log.WithName("controllers").WithName("hosstedproject").WithName(req.Namespace)

	logger.Info("Reconciling")

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

		collector, currentRevision, helmStatus, err := r.collector(ctx, instance)
		if err != nil {
			return ctrl.Result{}, err
		}

		if instance.Status.ClusterUUID == "" {
			// Collect info about resource

			sort.Ints(currentRevision)

			clusterUUID := "K-" + uuid.NewString()
			instance.Status.HelmStatus = helmStatus
			instance.Status.ClusterUUID = clusterUUID
			instance.Status.EmailID = os.Getenv("EMAIL_ID")
			instance.Status.LastReconciledTimestamp = time.Now().String()
			instance.Status.Revision = currentRevision

			///////////////////////// ----------- Register ClusterUUID --------------////////////////////////
			clusterUUIDRegPath := os.Getenv("HOSSTED_API_URL") + "/clusters/" + clusterUUID + "/register"

			type clusterUUIDBody struct {
				Email   string `json:"email"`
				ReqType string `json:"type"`
			}

			clusterUUIDBodyReq := clusterUUIDBody{
				Email:   instance.Status.EmailID,
				ReqType: "k8s",
			}

			body, err := json.Marshal(clusterUUIDBodyReq)
			if err != nil {
				return ctrl.Result{}, err
			}

			resp, err := internalHTTP.HttpRequest(body, clusterUUIDRegPath)
			if err != nil {
				return ctrl.Result{}, err
			}

			logger.Info(fmt.Sprintf("Registering K8s with UUID no [%s] to hossted API", clusterUUID), "body", string(body), "resp", resp.ResponseBody, "statuscode", resp.StatusCode)

			///////////////////////// ----------- ------------------- --------------////////////////////////

			for i := range collector {
				collector[i].AppAPIInfo.ClusterUUID = instance.Status.ClusterUUID
				//collector[i].AppAPIInfo.EmailID = instance.Status.EmailID

				collectorJson, err := json.Marshal(collector[i])
				if err != nil {
					return ctrl.Result{}, err
				}

				appUUIDRegPath := os.Getenv("HOSSTED_API_URL") + "/apps/" + collector[i].AppAPIInfo.AppUUID + "/register"

				resp, err := internalHTTP.HttpRequest(collectorJson, appUUIDRegPath)
				if err != nil {
					return ctrl.Result{}, err
				}

				logger.Info(fmt.Sprintf("Sending req no [%d] to hossted API", i), "resp", resp.ResponseBody, "statuscode", resp.StatusCode)

			}

			if err := r.Status().Update(ctx, instance); err != nil {
				return ctrl.Result{}, err
			}

			return ctrl.Result{RequeueAfter: time.Second * 10}, nil
		}

		if !compareSlices(instance.Status.Revision, currentRevision) {
			for i := range collector {

				collectorJson, err := json.Marshal(collector[i])
				if err != nil {
					return ctrl.Result{}, err
				}

				appUUIDRegPath := os.Getenv("HOSSTED_API_URL") + "/apps/" + collector[i].AppAPIInfo.AppUUID + "/register"

				resp, err := internalHTTP.HttpRequest(collectorJson, appUUIDRegPath)
				if err != nil {
					return ctrl.Result{}, err
				}

				logger.Info(fmt.Sprintf("Sending req no [%d] to hossted API", i), "resp", resp.ResponseBody, "statuscode", resp.StatusCode)

			}

			// Update instance status
			instance.Status.HelmStatus = helmStatus
			instance.Status.Revision = currentRevision
			instance.Status.LastReconciledTimestamp = time.Now().String()
			if err := r.Status().Update(ctx, instance); err != nil {
				return ctrl.Result{}, err
			}

			return ctrl.Result{RequeueAfter: time.Second * 10}, nil
		} else {
			logger.Info("Not State change detected, requeueing")
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
