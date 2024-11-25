package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sort"

	"time"

	trivy "github.com/aquasecurity/trivy-operator/pkg/apis/aquasecurity/v1alpha1"
	"github.com/go-logr/logr"
	"github.com/google/uuid"
	hosstedcomv1 "github.com/hossted/hossted-operator/api/v1"
	helm "github.com/hossted/hossted-operator/pkg/helm"
	internalHTTP "github.com/hossted/hossted-operator/pkg/http"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
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
			// instance not found check if req is of type VulnerabilityReport
			if instance.Spec.CVE.Enable {
				vr := &trivy.VulnerabilityReport{}
				err = r.Get(ctx, req.NamespacedName, vr)
				if err != nil {
					if errors.IsNotFound(err) {
						return ctrl.Result{}, nil
					}
					return ctrl.Result{}, err
				}
				// send vunerability report
				err = r.handleVulnReports(ctx, req.NamespacedName.Namespace, logger)
				if err != nil {
					return ctrl.Result{}, nil
				}
				return ctrl.Result{}, nil
			}
		}
		return ctrl.Result{}, err
	}

	// Check if reconciliation should proceed
	if !instance.Spec.Stop {
		err = r.handleReconciliation(ctx, instance, logger)
		if err != nil {
			return ctrl.Result{}, err
		} else {
			return ctrl.Result{RequeueAfter: r.ReconcileDuration}, nil
		}
	} else {
		if instance.Spec.Helm != nil {
			for _, h := range instance.Spec.Helm {
				newHelm := helm.Helm{
					ReleaseName: h.ReleaseName,
					Namespace:   h.Namespace,
					Values:      h.Values,
					RepoName:    h.RepoName,
					ChartName:   h.ChartName,
					RepoUrl:     h.RepoUrl,
				}
				ok, err := helm.ListRelease(h.ReleaseName, h.Namespace)
				if err != nil {
					return ctrl.Result{}, err
				}
				err = sendEvent("info", init_marketplace_app_installation, os.Getenv("HOSSTED_ORG_ID"), instance.Status.ClusterUUID)
				if err != nil {
					log.Print(err)
				}
				if !ok {
					err := helm.Apply(newHelm)
					if err != nil {
						return ctrl.Result{}, err
					}
				}
			}
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
			return err
		}
		logger.Info("Handling Ingress")

		if err := r.handleIngress(instance); err != nil {
			return err
		}

		logger.Info("First Deployment perform cluster registration, status updates and app registration")

		err = r.handleNewCluster(ctx, instance, logger)

		if err != nil {
			return err
		}

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
func (r *HosstedProjectReconciler) handleNewCluster(ctx context.Context, instance *hosstedcomv1.Hosstedproject, logger logr.Logger) error {

	logger.Info("Registering Cluster")

	if err := r.registerClusterUUID(instance, instance.Status.ClusterUUID, logger); err != nil {
		return err
	}

	err := sendEvent("info", init_cluster_registeration, os.Getenv("HOSSTED_ORG_ID"), instance.Status.ClusterUUID)
	if err != nil {
		log.Print(err)
	}

	collector, currentRevision, helmStatus, err := r.collector(ctx, instance)
	if err != nil {
		return err
	}
	sort.Ints(currentRevision)

	instance.Status.HelmStatus = helmStatus
	instance.Status.LastReconciledTimestamp = time.Now().String()
	instance.Status.Revision = currentRevision

	logger.Info("Update Status")

	// Update status
	if err := r.Status().Update(ctx, instance); err != nil {
		return err
	}

	logger.Info("Registering Apps")

	err = sendEvent("info", init_cluster_registeration, os.Getenv("HOSSTED_ORG_ID"), instance.Status.ClusterUUID)
	if err != nil {
		log.Print(err)
	}

	if err := r.registerApps(instance, collector, logger); err != nil {
		return err
	}

	return nil
}

// handleExistingCluster handles reconciliation for an existing cluster.
func (r *HosstedProjectReconciler) handleExistingCluster(ctx context.Context, instance *hosstedcomv1.Hosstedproject, collector []*Collector, currentRevision []int, helmStatus []hosstedcomv1.HelmInfo, logger logr.Logger) error {
	if !compareSlices(instance.Status.Revision, currentRevision) {
		if err := r.registerApps(instance, collector, logger); err != nil {
			return err
		}

		// Update instance status
		instance.Status.HelmStatus = helmStatus
		instance.Status.Revision = currentRevision
		instance.Status.LastReconciledTimestamp = time.Now().String()

		// Update status
		if err := r.Status().Update(ctx, instance); err != nil {
			return err
		}

		return nil
	}

	err := r.handleMonitoring(ctx, instance)
	if err != nil {
		return err
	}

	logger.Info("No state change detected, requeueing")
	return nil
}

// registerApps registers applications with the Hossted API.
func (r *HosstedProjectReconciler) registerApps(instance *hosstedcomv1.Hosstedproject, collector []*Collector, logger logr.Logger) error {

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
func (r *HosstedProjectReconciler) registerClusterUUID(instance *hosstedcomv1.Hosstedproject, clusterUUID string, logger logr.Logger) error {
	clusterUUIDRegPath := os.Getenv("HOSSTED_API_URL") + "/clusters/" + clusterUUID + "/register"

	type clusterUUIDBody struct {
		ReqType      string `json:"type"`
		OrgID        string `json:"org_id"`
		ContextName  string `json:"context_name"`
		OptionsState struct {
			Monitoring bool `json:"monitoring"`
			Logging    bool `json:"logging"`
			CVE        bool `json:"cve_scan"`
			Ingress    bool `json:"ingress"`
		} `json:"options_state"`
	}

	clusterUUIDBodyReq := clusterUUIDBody{
		ReqType:     "k8s",
		OrgID:       os.Getenv("HOSSTED_ORG_ID"),
		ContextName: os.Getenv("CONTEXT_NAME"),
		OptionsState: struct {
			Monitoring bool `json:"monitoring"`
			Logging    bool `json:"logging"`
			CVE        bool `json:"cve_scan"`
			Ingress    bool `json:"ingress"`
		}{
			Monitoring: instance.Spec.Monitoring.Enable,
			Logging:    instance.Spec.Logging.Enable,
			CVE:        instance.Spec.CVE.Enable,
			Ingress:    instance.Spec.Ingress.Enable,
		},
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
func (r *HosstedProjectReconciler) handleMonitoring(ctx context.Context, instance *hosstedcomv1.Hosstedproject) error {
	// Helm configuration for Grafana Agent
	// enableLog := "false"
	// if instance.Spec.Logging.Enable {
	// 	enableLog = "true"
	// }

	uuid := instance.Status.ClusterUUID
	lokiURL := os.Getenv("LOKI_URL")
	lokiUser := os.Getenv("LOKI_USERNAME")
	lokiPass := os.Getenv("LOKI_PASSWORD")
	mimirURL := os.Getenv("MIMIR_URL")
	mimirUser := os.Getenv("MIMIR_USERNAME")
	mimirPass := os.Getenv("MIMIR_PASSWORD")

	debug := generateConfigMap(
		uuid, lokiURL, lokiUser, lokiPass, mimirURL, mimirUser, mimirPass,
	)
	fmt.Println(debug)

	h := helm.Helm{
		ReleaseName: "hossted-grafana-alloy",
		ChartName:   "alloy",
		RepoName:    "grafana",
		RepoUrl:     "https://grafana.github.io/helm-charts",
		Namespace:   "hossted-platform",
		Values: []string{
			fmt.Sprintf("alloy.configMap.content='%s'", generateConfigMap(uuid, lokiURL, lokiUser, lokiPass, mimirURL, mimirUser, mimirPass)),
		},
	}

	ksm := helm.Helm{
		ReleaseName: "kube-state-metrics",
		ChartName:   "kube-state-metrics",
		RepoName:    "grafana",
		RepoUrl:     "https://prometheus-community.github.io/helm-charts",
		Namespace:   "hossted-platform",
		Values: []string{
			"selfMonitor.enabled=true",
		},
	}

	// Check if monitoring is enabled
	if instance.Spec.Monitoring.Enable {
		// Check if Grafana Agent release already exists
		ok, err := helm.ListRelease(h.ChartName, h.Namespace)
		if err != nil {
			return err
		}

		if !ok {

			// install kubestate metrics
			// Install Grafana Agent
			err = helm.Apply(h)
			if err != nil {
				fmt.Println("grafana-allow for monitoring failed %w", err)
			}
			err = helm.Apply(ksm)
			if err != nil {
				fmt.Println("ksm-agent for monitoring failed %w", err)
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
			err_del := helm.DeleteRelease(ksm.ChartName, ksm.Namespace)
			if err_del != nil {
				return fmt.Errorf("kube state metrics deletion failed %w", err_del)
			}

			// Delete Grafana Agent release if it exists
			err_del = helm.DeleteRelease(h.ChartName, h.Namespace)
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

func (r *HosstedProjectReconciler) handleIngress(instance *hosstedcomv1.Hosstedproject) error {

	// Helm configuration for ingress controller
	ing := helm.Helm{
		ReleaseName: "hossted-ingress-nginx",
		Namespace:   "hossted-platform",
		ChartName:   "ingress-nginx",
		RepoName:    "ingress-nginx",
		RepoUrl:     "https://kubernetes.github.io/ingress-nginx",
		Version:     "4.11.1",
		Values: []string{
			"controller.replicaCount=2",
			"controller.service.ports.http=80",
			"controller.service.ports.https=443",
			"controller.ingressClassByName=true",
			"controller.ingressClassResource.name=hossted-operator",
			"controller.ingressClassResource.controllerValue=k8s.io/hossted-operator",
			"controller.ingressClassResource.enabled=true",
			"controller.ingressClass=hossted-operator",
			"controller.admissionWebhooks.enabled=false",
		},
	}

	// Check if the environment variable CLOUD_PROVIDER is set to azure
	if os.Getenv("CLOUD_PROVIDER") == "azure" {
		// Add Azure-specific annotations to the service
		ing.Values = append(ing.Values,
			"controller.service.annotations.service\\.beta\\.kubernetes\\.io/azure-load-balancer-health-probe-request-path=/healthz",
		)
	}

	// Check if monitoring is enabled
	if instance.Spec.Ingress.Enable {
		// Check if Grafana Agent release already exists
		ok, err := helm.ListRelease(ing.ChartName, ing.Namespace)
		if err != nil {
			return err
		}

		if !ok {
			// install ingress controller
			err = sendEvent("info", init_ingress_installation, os.Getenv("HOSSTED_ORG_ID"), instance.Status.ClusterUUID)
			if err != nil {
				log.Print(err)
			}

			err = helm.Apply(ing)
			if err != nil {
				fmt.Println("ingress controller installation failed %w", err)
			}
			return nil
		}

	} else {
		// If ingress is not enabled, check if ingress controller release exists
		ok, err := helm.ListRelease(ing.ChartName, ing.Namespace)
		if err != nil {
			return err
		}
		if ok {
			// Delete ingress controller release if it exists
			err_del := helm.DeleteRelease(ing.ChartName, ing.Namespace)
			if err_del != nil {
				return fmt.Errorf("ingress controller deletion failed %w", err_del)
			}
		}
	}
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *HosstedProjectReconciler) SetupWithManager(mgr ctrl.Manager) error {

	instance := &hosstedcomv1.Hosstedproject{}

	_ = r.Get(context.TODO(), types.NamespacedName{Namespace: "", Name: "hossted-operator-cr"}, instance)

	if instance.Spec.CVE.Enable {

		return ctrl.NewControllerManagedBy(mgr).
			For(&hosstedcomv1.Hosstedproject{}).
			Watches(
				&trivy.VulnerabilityReport{},
				// handler.EnqueueRequestsFromMapFunc(r.findObjectsForVR),
				handler.Funcs{
					CreateFunc: func(ctx context.Context, e event.CreateEvent, q workqueue.RateLimitingInterface) {
						q.Add(reconcile.Request{NamespacedName: types.NamespacedName{
							Name:      e.Object.GetName(),
							Namespace: e.Object.GetNamespace(),
						}})
					},
				},
				builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
			).
			Complete(r)
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&hosstedcomv1.Hosstedproject{}).
		Complete(r)
}

// handleExistingCluster handles reconciliation for an existing cluster.
func (r *HosstedProjectReconciler) handleVulnReports(ctx context.Context, namespace string, logger logr.Logger) error {
	var err error
	var collector []*Collector

	instance := &hosstedcomv1.HosstedprojectList{}
	listOps := &client.ListOptions{}
	err = r.List(ctx, instance, listOps)
	if err != nil {
		return err
	}

	inst := &hosstedcomv1.Hosstedproject{}
	inst = &instance.Items[0]

	for _, ns := range inst.Spec.DenyNamespaces {
		if ns == namespace {
			return fmt.Errorf("Namespace in deny list for VR")
		}
	}

	collector, _, _, err = r.collector(ctx, inst)
	if err != nil {
		return err
	}

	if err := r.registerApps(inst, collector, logger); err != nil {
		return err
	}
	return nil
}

// // handleExistingCluster handles reconciliation for an existing cluster.
// func (r *HosstedProjectReconciler) registerDns(ctx context.Context, instance *hosstedcomv1.Hosstedproject) {
// 	dnsrequest, _ := r.getDns(ctx, instance)
// 	body, _ := json.Marshal(dnsrequest)
// 	dnsURL := os.Getenv("HOSSTED_API_URL") + "/clusters/dns"
// 	resp, _ := internalHTTP.HttpRequest(body, dnsURL)
// 	instance.Status.DnsResponse = resp.StatusCode
// 	if instance.Status.DnsResponse != 200 || instance.Status.DnsResponse == 0 {
// 		_ = r.Status().Update(ctx, instance)
// 	}

// }

func generateConfigMap(uuid, lokiURL, lokiUser, lokiPass, mimirURL, mimirUser, mimirPass string) string {
	configMapTemplate := `
      discovery.kubernetes "metrics_integrations_kubernetes_nodes_cadvisor" {
        role = "node"
      }

      discovery.kubernetes "metrics_integrations_integrations_kubernetes_kube_state_metrics" {
        role = "pod"
      }

      discovery.relabel "metrics_integrations_kubernetes_nodes_cadvisor" {
        targets = discovery.kubernetes.metrics_integrations_kubernetes_nodes_cadvisor.targets

        rule {
          regex  = "__meta_kubernetes_node_label_(.+)"
          action = "labelmap"
        }

        rule {
          target_label = "__address__"
          replacement  = "kubernetes.default.svc:443"
        }

        rule {
          source_labels = ["__meta_kubernetes_node_name"]
          regex         = "(.+)"
          target_label  = "__metrics_path__"
          replacement   = "/api/v1/nodes/$1/proxy/metrics/cadvisor"
        }
      }

      discovery.relabel "metrics_integrations_integrations_kubernetes_kube_state_metrics" {
        targets = discovery.kubernetes.metrics_integrations_integrations_kubernetes_kube_state_metrics.targets

        rule {
          source_labels = ["__meta_kubernetes_pod_label_app_kubernetes_io_name"]
          regex         = "kube-state-metrics"
          action        = "keep"
        }
      }

      prometheus.scrape "metrics_integrations_kubernetes_nodes_cadvisor" {
        targets         = discovery.relabel.metrics_integrations_kubernetes_nodes_cadvisor.output
        forward_to      = [prometheus.remote_write.metrics_integrations.receiver]
        job_name        = "kubernetes-nodes-cadvisor"
        scrape_interval = "30s"
        scheme          = "https"

        authorization {
          type             = "Bearer"
          credentials_file = "/var/run/secrets/kubernetes.io/serviceaccount/token"
        }

        tls_config {
          ca_file              = "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt"
          insecure_skip_verify = true
        }
      }

      prometheus.scrape "metrics_integrations_integrations_kubernetes_kube_state_metrics" {
        targets         = discovery.relabel.metrics_integrations_integrations_kubernetes_kube_state_metrics.output
        forward_to      = [prometheus.relabel.metrics_integrations_integrations_kubernetes_kube_state_metrics.receiver]
        job_name        = "integrations/kubernetes/kube-state-metrics"
        scrape_interval = "30s"
      }

      prometheus.relabel "metrics_integrations_integrations_kubernetes_kube_state_metrics" {
        forward_to = [prometheus.remote_write.metrics_integrations.receiver]

        rule {
          source_labels = ["__name__"]
          regex         = ".*"
          action        = "keep"
        }
      }

      prometheus.operator.servicemonitors "services" {
        forward_to = [prometheus.remote_write.metrics_integrations.receiver]
      }

      prometheus.remote_write "metrics_integrations" {
        external_labels = {
          uuid = "%s",
        }

        endpoint {
          url  = "%s"
          basic_auth {
            username = "%s"
            password = "%s"
          }
        }
      }

      discovery.kubernetes "logs_default_integrations_kubernetes_pod_logs" {
        role = "pod"
      }

      discovery.relabel "logs_default_integrations_kubernetes_pod_logs" {
        targets = discovery.kubernetes.logs_default_integrations_kubernetes_pod_logs.targets

        rule {
          source_labels = ["__meta_kubernetes_pod_node_name"]
          target_label  = "__host__"
        }

        rule {
          source_labels = ["__meta_kubernetes_namespace", "__meta_kubernetes_pod_name"]
          separator     = "/"
          target_label  = "job"
        }

        rule {
          source_labels = ["__meta_kubernetes_namespace"]
          target_label  = "namespace"
        }

        rule {
          source_labels = ["__meta_kubernetes_pod_name"]
          target_label  = "pod"
        }

        rule {
          source_labels = ["__meta_kubernetes_pod_container_name"]
          target_label  = "container"
        }

        rule {
          source_labels = ["__meta_kubernetes_pod_uid", "__meta_kubernetes_pod_container_name"]
          separator     = "/"
          target_label  = "__path__"
          replacement   = "/var/log/pods/*$1/*.log"
        }
      }

      local.file_match "logs_default_integrations_kubernetes_pod_logs" {
        path_targets = discovery.relabel.logs_default_integrations_kubernetes_pod_logs.output
      }

      loki.process "logs_default_integrations_kubernetes_pod_logs" {
        forward_to = [loki.write.logs_default.receiver]
        stage.docker { }
      }

      loki.source.file "logs_default_integrations_kubernetes_pod_logs" {
        targets               = local.file_match.logs_default_integrations_kubernetes_pod_logs.targets
        forward_to            = [loki.process.logs_default_integrations_kubernetes_pod_logs.receiver]
        legacy_positions_file = "/tmp/positions.yaml"
      }

      loki.write "logs_default" {
        endpoint {
          url = "%s"
          basic_auth {
            username = "%s"
            password = "%s"
          }
        }
        external_labels = {
          uuid = "%s",
        }
      }

      logging {
        level = "debug"
      }`

	return fmt.Sprintf(configMapTemplate,
		uuid, mimirURL, mimirUser, mimirPass,
		lokiURL, lokiUser, lokiPass, uuid)
}
