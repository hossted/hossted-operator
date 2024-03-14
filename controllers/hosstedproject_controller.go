package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"

	"time"

	"github.com/go-logr/logr"
	"github.com/google/uuid"
	hosstedcomv1 "github.com/hossted/hossted-operator/api/v1"
	helm "github.com/hossted/hossted-operator/pkg/helm"
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

// +kubebuilder:rbac:groups=hossted.com,resources=hosstedprojects,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=hossted.com,resources=hosstedprojects/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=hossted.com,resources=hosstedprojects/finalizers,verbs=update
// +kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses,verbs=list;get;watch
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=list;get;watch
// +kubebuilder:rbac:groups="",resources=pods,verbs=list;get;watch
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=list;get;watch
// +kubebuilder:rbac:groups="",resources=services,verbs=list;get;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=list;get;watch
// +kubebuilder:rbac:groups="",resources=persistentvolumeclaims,verbs=list;get;watch
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
		return r.handleReconciliation(ctx, instance, logger)
	}

	logger.Info("Reconciliation stopped", "name", req.Name)
	return ctrl.Result{RequeueAfter: time.Second * 10}, nil
}

// handleReconciliation handles the reconciliation process.
func (r *HosstedProjectReconciler) handleReconciliation(ctx context.Context, instance *hosstedcomv1.Hosstedproject, logger logr.Logger) (ctrl.Result, error) {
	collector, currentRevision, helmStatus, err := r.collector(ctx, instance)
	if err != nil {
		return ctrl.Result{}, err
	}

	if instance.Status.ClusterUUID == "" {
		return r.handleNewCluster(ctx, instance, collector, currentRevision, helmStatus, logger)
	}

	return r.handleExistingCluster(ctx, instance, collector, currentRevision, helmStatus, logger)

}

// handleNewCluster handles reconciliation when a new cluster is created.
func (r *HosstedProjectReconciler) handleNewCluster(ctx context.Context, instance *hosstedcomv1.Hosstedproject, collector []*Collector, currentRevision []int, helmStatus []hosstedcomv1.HelmInfo, logger logr.Logger) (ctrl.Result, error) {
	sort.Ints(currentRevision)

	clusterUUID := "K-" + uuid.NewString()
	instance.Status.HelmStatus = helmStatus
	instance.Status.ClusterUUID = clusterUUID
	instance.Status.EmailID = os.Getenv("EMAIL_ID")
	instance.Status.LastReconciledTimestamp = time.Now().String()
	instance.Status.Revision = currentRevision

	if err := r.registerApps(ctx, instance, collector, logger); err != nil {
		return ctrl.Result{}, err
	}

	if err := r.registerClusterUUID(ctx, instance, clusterUUID, logger); err != nil {
		return ctrl.Result{}, err
	}

	// Update status
	if err := r.Status().Update(ctx, instance); err != nil {
		logger.Error(err, "Failed to update status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: time.Second * 10}, nil
}

// handleExistingCluster handles reconciliation for an existing cluster.
func (r *HosstedProjectReconciler) handleExistingCluster(ctx context.Context, instance *hosstedcomv1.Hosstedproject, collector []*Collector, currentRevision []int, helmStatus []hosstedcomv1.HelmInfo, logger logr.Logger) (ctrl.Result, error) {
	if !compareSlices(instance.Status.Revision, currentRevision) {
		if err := r.registerApps(ctx, instance, collector, logger); err != nil {
			return ctrl.Result{}, err
		}

		// Update instance status
		instance.Status.HelmStatus = helmStatus
		instance.Status.Revision = currentRevision
		instance.Status.LastReconciledTimestamp = time.Now().String()

		// Update status
		if err := r.Status().Update(ctx, instance); err != nil {
			logger.Error(err, "Failed to update status")
			return ctrl.Result{}, err
		}

		return ctrl.Result{RequeueAfter: time.Second * 10}, nil
	}
	err := r.handleMonitoring(ctx, instance, logger)
	if err != nil {
		return ctrl.Result{}, err
	}

	logger.Info("No state change detected, requeueing")
	return ctrl.Result{RequeueAfter: time.Second * 10}, nil
}

// registerApps registers applications with the Hossted API.
func (r *HosstedProjectReconciler) registerApps(ctx context.Context, instance *hosstedcomv1.Hosstedproject, collector []*Collector, logger logr.Logger) error {
	for i := range collector {
		collector[i].AppAPIInfo.ClusterUUID = instance.Status.ClusterUUID
		collectorJson, err := json.Marshal(collector[i])
		if err != nil {
			return err
		}

		appUUIDRegPath := os.Getenv("HOSSTED_API_URL") + "/apps"
		resp, err := internalHTTP.HttpRequest(collectorJson, appUUIDRegPath)
		if err != nil {
			return err
		}

		logger.Info(fmt.Sprintf("Sending req no [%d] to hossted API", i), "appUUID", collector[i].AppAPIInfo.AppUUID, "resp", resp.ResponseBody, "statuscode", resp.StatusCode)
	}

	time.Sleep(time.Second * 10)
	return nil
}

// registerClusterUUID registers the cluster UUID with the Hossted API.
func (r *HosstedProjectReconciler) registerClusterUUID(ctx context.Context, instance *hosstedcomv1.Hosstedproject, clusterUUID string, logger logr.Logger) error {
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
		return err
	}

	resp, err := internalHTTP.HttpRequest(body, clusterUUIDRegPath)
	if err != nil {
		return err
	}

	logger.Info(fmt.Sprintf("Registering K8s with UUID no [%s] to hossted API", clusterUUID), "body", string(body), "resp", resp.ResponseBody, "statuscode", resp.StatusCode)

	return nil
}

// enable monitoring using grafana-agent.
func (r *HosstedProjectReconciler) handleMonitoring(ctx context.Context, instance *hosstedcomv1.Hosstedproject, logger logr.Logger) error {
	// Helm configuration for Grafana Agent
	h := helm.Helm{
		ChartName: "hossted-grafana-agent",
		RepoName:  "grafana",
		RepoUrl:   "https://charts.hossted.com",
		Namespace: "grafana-agent",
	}

	// Check if monitoring is enabled
	if instance.Spec.Monitoring.Enable {
		// Check if Grafana Agent release already exists
		err := helm.ListRelease(h.ChartName, h.Namespace)
		if err == nil {
			return nil
		}

		// Install Grafana Agent
		err = helm.Apply(h)
		if err != nil {
			return fmt.Errorf("enabling grafana-agent for monitoring failed %w", err)
		}
		return nil

	}
	// If monitoring is not enabled, check if Grafana Agent release exists
	err := helm.ListRelease(h.ChartName, h.Namespace)
	if err == nil {
		// Delete Grafana Agent release if it exists
		err_del := helm.DeleteRelease(h.ChartName, h.Namespace)
		if err_del != nil {
			return fmt.Errorf("grafana Agent deletion failed %w", err_del)
		}
	}
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *HosstedProjectReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&hosstedcomv1.Hosstedproject{}).
		Complete(r)
}
