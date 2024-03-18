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
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// HosstedProjectReconciler reconciles a HosstedProject object
type HosstedProjectReconciler struct {
	client.Client
	Scheme            *runtime.Scheme
	ReconcileDuration time.Duration
}

// +kubebuilder:rbac:groups=hossted.com,resources=hosstedprojects,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=hossted.com,resources=hosstedprojects/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=hossted.com,resources=hosstedprojects/finalizers,verbs=update
// +kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses,verbs=list;get;watch
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=list;get;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=daemonsets,verbs=list;get;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=pods,verbs=list;get;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=list;get;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=services,verbs=list;get;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=secrets,verbs=list;get;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=persistentvolumeclaims,verbs=list;get;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aquasecurity.github.io,resources=vulnerabilityreports,verbs=get;list;watch

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
		err := r.handleReconciliation(ctx, instance, logger)
		if err != nil {
			return ctrl.Result{}, err
		} else {
			fmt.Println("hello")
			return ctrl.Result{RequeueAfter: time.Second * 30}, nil
		}
	}

	logger.Info("Reconciliation stopped", "name", req.Name)
	return ctrl.Result{RequeueAfter: r.ReconcileDuration}, nil
}

// handleReconciliation handles the reconciliation process.
func (r *HosstedProjectReconciler) handleReconciliation(ctx context.Context, instance *hosstedcomv1.Hosstedproject, logger logr.Logger) error {
	var err error
	var collector []*Collector
	var currentRevision []int
	var helmStatus []hosstedcomv1.HelmInfo

	if instance.Status.ClusterUUID == "" {
		clusterUUID := "K-" + uuid.NewString()
		instance.Status.ClusterUUID = clusterUUID
		// Update status
		if err := r.Status().Update(ctx, instance); err != nil {
			logger.Error(err, "Failed to update status")
			return err
		}
		collector, currentRevision, helmStatus, err = r.collector(ctx, instance)
		if err != nil {
			return err
		}
		//return r.handleNewCluster(ctx, instance, collector, currentRevision, helmStatus, logger)

	}
	collector, currentRevision, helmStatus, err = r.collector(ctx, instance)
	if err != nil {
		return err
	}

	err = r.handleExistingCluster(ctx, instance, collector, currentRevision, helmStatus, logger)
	if err != nil {
		return err
	}

	return nil
}

// handleNewCluster handles reconciliation when a new cluster is created.
func (r *HosstedProjectReconciler) handleNewCluster(ctx context.Context, instance *hosstedcomv1.Hosstedproject, collector []*Collector, currentRevision []int, helmStatus []hosstedcomv1.HelmInfo, logger logr.Logger) (ctrl.Result, error) {

	sort.Ints(currentRevision)

	instance.Status.HelmStatus = helmStatus
	instance.Status.EmailID = os.Getenv("EMAIL_ID")
	instance.Status.LastReconciledTimestamp = time.Now().String()
	instance.Status.Revision = currentRevision

	// Update status
	if err := r.Status().Update(ctx, instance); err != nil {
		logger.Error(err, "Failed to update status")
		return ctrl.Result{}, err
	}

	if err := r.registerApps(ctx, instance, collector, logger); err != nil {
		return ctrl.Result{}, err
	}

	if err := r.registerClusterUUID(ctx, instance, instance.Status.ClusterUUID, logger); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: r.ReconcileDuration}, nil
}

// handleExistingCluster handles reconciliation for an existing cluster.
func (r *HosstedProjectReconciler) handleExistingCluster(ctx context.Context, instance *hosstedcomv1.Hosstedproject, collector []*Collector, currentRevision []int, helmStatus []hosstedcomv1.HelmInfo, logger logr.Logger) error {

	if err := r.registerApps(ctx, instance, collector, logger); err != nil {
		return err
	}

	// Update instance status
	instance.Status.HelmStatus = helmStatus
	instance.Status.Revision = currentRevision
	instance.Status.LastReconciledTimestamp = time.Now().String()

	// Update status
	if err := r.Status().Update(ctx, instance); err != nil {
		logger.Error(err, "Failed to update status")
		return err
	}

	err := r.handleMonitoring(ctx, instance, logger)
	if err != nil {
		return err
	}

	logger.Info("No state change detected, requeueing")
	return nil
}

// registerApps registers applications with the Hossted API.
func (r *HosstedProjectReconciler) registerApps(ctx context.Context, instance *hosstedcomv1.Hosstedproject, collector []*Collector, logger logr.Logger) error {

	b, _ := json.Marshal(collector)
	fmt.Println(string(b))
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
		ok, err := helm.ListRelease(h.ChartName, h.Namespace)
		if err != nil {
			return err
		}

		if !ok {
			// Install Grafana Agent
			err = helm.Apply(h)
			if err != nil {
				return fmt.Errorf("enabling grafana-agent for monitoring failed %w", err)
			}
			return nil
		}

	} else {
		// If monitoring is not enabled, check if Grafana Agent release exists
		ok, err := helm.ListRelease(h.ChartName, h.Namespace)
		if err != nil {
			return err
		}
		if ok {
			// Delete Grafana Agent release if it exists
			err_del := helm.DeleteRelease(h.ChartName, h.Namespace)
			if err_del != nil {
				return fmt.Errorf("grafana Agent deletion failed %w", err_del)
			}

			err := r.Client.Delete(ctx, &appsv1.DaemonSet{
				ObjectMeta: v1.ObjectMeta{
					Name:      h.ChartName,
					Namespace: h.Namespace,
				},
			})
			if err != nil {
				return fmt.Errorf("failed to delete DaemonSet: %w", err)
			}
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
