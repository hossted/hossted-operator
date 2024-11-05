package controllers

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	trivy "github.com/aquasecurity/trivy-operator/pkg/apis/aquasecurity/v1alpha1"
	"github.com/google/uuid"
	hosstedcomv1 "github.com/hossted/hossted-operator/api/v1"
	helm "github.com/hossted/hossted-operator/pkg/helm"
	"github.com/hossted/hossted-operator/pkg/http"
	helmrelease "helm.sh/helm/v3/pkg/release"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
)

type Collector struct {
	AppAPIInfo AppAPIInfo `json:"app_api_info"`
	AppInfo    AppInfo    `json:"app_info"`
}

type AppInfo struct {
	HelmInfo        hosstedcomv1.HelmInfo `json:"helm_info"`
	AccessInfo      AccessInfo            `json:"access_info"`
	PodInfo         []PodInfo             `json:"pod_info"`
	DeploymentInfo  []DeploymentInfo      `json:"deployment_info"`
	StatefulsetInfo []StatefulsetInfo     `json:"statefulset_info"`
	ServiceInfo     []ServiceInfo         `json:"service_info"`
	VolumeInfo      []VolumeInfo          `json:"volume_info"`
	IngressInfo     []IngressInfo         `json:"ingress_info"`
	ConfigmapInfo   []ConfigmapInfo       `json:"configmap_info"`
	HelmValueInfo   HelmValueInfo         `json:"helmvalue_info"`
	SecurityInfo    []SecurityInfo        `json:"security_info"`
	SecretInfo      []SecretInfo          `json:"secret_info"`
}

type URLInfo struct {
	URL      string `json:"url"`
	User     string `json:"user,omitempty"`
	Password string `json:"password,omitempty"`
}

type AccessInfo struct {
	URLs []URLInfo `json:"urls"`
}

// AppAPIInfo contains basic information about the application API.
type AppAPIInfo struct {
	OrgID       string `json:"org_id"`
	ClusterUUID string `json:"cluster_uuid"`
	AppUUID     string `json:"app_uuid"`
	AppName     string `json:"app_name"`
	Type        string `json:"type"`
	HosstedHelm bool   `json:"hossted_helm"`
}

// ServiceInfo contains information about a Kubernetes service.
type ServiceInfo struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Port      int32  `json:"port"`
}

type IngressInfo struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Domain    string `json:"domain"`
}

type SecretInfo struct {
	Name      string            `json:"name"`
	Namespace string            `json:"namespace"`
	Data      map[string][]byte `json:"data"`
}

// PodInfo contains information about a Kubernetes pod.
type PodInfo struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Image     string `json:"image"`
	Status    string `json:"status"`
}

type DeploymentInfo struct {
	Name      string                `json:"name"`
	Namespace string                `json:"namespace"`
	SecretRef *v1.SecretKeySelector `json:"secretRef"`
}

type StatefulsetInfo struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

type VolumeInfo struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Size      int    `json:"size"`
}

type ConfigmapInfo struct {
	Name string            `json:"name"`
	Data map[string]string `json:"data"`
}

type HelmValueInfo struct {
	ChartName   string                 `json:"chart_name"`
	ChartValues map[string]interface{} `json:"chart_values"`
}

type SecurityInfo struct {
	PodName      string                  `json:"pod_name"`
	PodNamespace string                  `json:"pod_namespace"`
	Containers   []SecurityInfoContainer `json:"containers"`
}

type SecurityInfoContainer struct {
	ContainerImage       string                     `json:"container_image"`
	Type                 string                     `json:"type"`
	VulnerabilitySummary trivy.VulnerabilitySummary `json:"summary"`
	Vulnerabilities      []trivy.Vulnerability      `json:"vulnerabilities"`
}

type PrimaryCreds struct {
	Namespace string `json:"namespace,omitempty"`
	Password  struct {
		Type       string `json:"type,omitempty"`
		Text       string `json:"text,omitempty"`
		Key        string `json:"key"`
		SecretName string `json:"secretName,omitempty"`
		ConfigMap  string `json:"configMap,omitempty"`
	} `json:"password,omitempty"`
	User struct {
		Type       string `json:"type,omitempty"`
		Text       string `json:"text,omitempty"`
		SecretName string `json:"secretName,omitempty"`
		ConfigMap  string `json:"configMap,omitempty"`
		Key        string `json:"key"`
	} `json:"user,omitempty"`
	Secret struct {
		Type  string `json:"type,omitempty"`
		Name  string `json:"name,omitempty"`
		Value string `json:"value,omitempty"`
	} `json:"secret,omitempty"`
}

type DnsInfo struct {
	Name      string `json:"name"`
	Content   string `json:"content"`
	Type      string `json:"type"`
	ClusterId string `json:"clusterid"`
	UserUUID  string `json:"user_id"`
	Env       string `json:"env"`
}

func (r *HosstedProjectReconciler) collector(ctx context.Context, instance *hosstedcomv1.Hosstedproject) ([]*Collector, []int, []hosstedcomv1.HelmInfo, error) {
	var collectors []*Collector
	namespaceList, err := r.listNamespaces(ctx)
	if err != nil {
		return nil, nil, nil, err
	}

	// Assuming instance.Spec.DenyNamespaces is the slice of denied namespaces
	filteredNamespaces := filter(namespaceList, instance.Spec.DenyNamespaces)

	var revisions []int

	var helmStatusMap = make(map[string]hosstedcomv1.HelmInfo) // Use a map to store unique HelmInfo structs
	var helmStatus []hosstedcomv1.HelmInfo

	for _, ns := range filteredNamespaces {

		releases, err := r.listReleases(ns)
		if err != nil {
			return nil, nil, nil, err
		}

		if len(releases) == 0 {
			// If there are no releases in this namespace, skip to the next one
			continue
		}

		// Initialize a slice to collect HelmInfo structs for this iteration

		var (
			helmInfo          hosstedcomv1.HelmInfo
			podHolder         []PodInfo
			svcHolder         []ServiceInfo
			pvcHolder         []VolumeInfo
			ingHolder         []IngressInfo
			securityHolder    []SecurityInfo
			configmapHolder   []ConfigmapInfo
			helmvalueHolder   HelmValueInfo
			deploymentHolder  []DeploymentInfo
			statefulsetHolder []StatefulsetInfo
			secretHolder      []SecretInfo
		)
		for _, release := range releases {
			helmInfo, err = r.getHelmInfo(*release, instance)
			if err != nil {
				return nil, nil, nil, err
			}
			// helmInfo.HosstedHelm = false
			if isHostedHelm(*release) {
				appUUID, err := r.getAppUUIDFromSecret(ctx, release.Namespace)
				if apierrors.IsNotFound(err) {
					helmStatusMap[helmInfo.AppUUID] = helmInfo
				} else {
					helmInfo.AppUUID = "A-" + appUUID
					helmInfo.HosstedHelm = true
					helmStatusMap[helmInfo.AppUUID] = helmInfo
				}
			}

			if instance.Status.DnsUpdated == false {
				err := r.getDns(ctx, instance, release.Namespace, helmInfo.AppUUID)
				if err != nil {
					return nil, nil, nil, err
				}
			}

			podHolder, securityHolder, err = r.getPods(ctx, instance.Spec.CVE.Enable, release.Namespace, release.Name)
			if err != nil {
				return nil, nil, nil, err
			}
			svcHolder, err = r.getServices(ctx, release.Namespace, release.Name)
			if err != nil {
				return nil, nil, nil, err
			}
			pvcHolder, err = r.getVolumes(ctx, release.Namespace, release.Name)
			if err != nil {
				return nil, nil, nil, err
			}
			statefulsetHolder, err = r.getStatefulsets(ctx, release.Namespace, release.Name)
			if err != nil {
				return nil, nil, nil, err
			}
			deploymentHolder, err = r.getDeployments(ctx, release.Namespace, release.Name)
			if err != nil {
				return nil, nil, nil, err
			}
			ingHolder, err = r.getIngress(ctx, release.Namespace, release.Name)
			if err != nil {
				return nil, nil, nil, err
			}
			configmapHolder, err = r.getConfigmaps(ctx, release.Namespace, release.Name)
			if err != nil {
				return nil, nil, nil, err
			}
			helmvalueHolder, err = r.getHelmInfoValues(release.Name, release.Namespace)
			if err != nil {
				return nil, nil, nil, err
			}
			secretHolder, err = r.getSecrets(ctx, release.Namespace, release.Name)
			if err != nil {
				return nil, nil, nil, err
			}

			revisions = append(revisions, helmInfo.Revision)

			accessInfo, err := r.getAccessInfo(ctx)
			if err != nil {
				return nil, nil, nil, err
			}

			// After collecting all HelmInfo structs for this iteration, assign to instance.Status.HelmStatus
			appInfo := AppInfo{
				HelmInfo:        helmInfo,
				AccessInfo:      accessInfo,
				PodInfo:         podHolder,
				StatefulsetInfo: statefulsetHolder,
				DeploymentInfo:  deploymentHolder,
				ServiceInfo:     svcHolder,
				VolumeInfo:      pvcHolder,
				IngressInfo:     ingHolder,
				ConfigmapInfo:   configmapHolder,
				HelmValueInfo:   helmvalueHolder,
				SecurityInfo:    securityHolder,
				SecretInfo:      secretHolder,
			}

			collector := &Collector{
				AppAPIInfo: AppAPIInfo{
					AppName:     appInfo.HelmInfo.Name,
					OrgID:       os.Getenv("HOSSTED_ORG_ID"),
					ClusterUUID: instance.Status.ClusterUUID,
					AppUUID:     appInfo.HelmInfo.AppUUID,
					Type:        "k8s",
					HosstedHelm: appInfo.HelmInfo.HosstedHelm,
				},
				AppInfo: appInfo,
			}
			collectors = append(collectors, collector)
			helmStatus = append(helmStatus, appInfo.HelmInfo)
		}

		sort.Ints(revisions)

	}
	sendK8sEvents(instance.Status.ClusterUUID)
	return collectors, revisions, helmStatus, nil
}

// listReleases retrieves all Helm releases in the specified namespace.
func (r *HosstedProjectReconciler) listReleases(namespace string) ([]*helmrelease.Release, error) {
	return helm.ListReleases(namespace)
}

// listReleases retrieves all Helm releases in the specified namespace.
func (r *HosstedProjectReconciler) getHelmInfoValues(name, namespace string) (HelmValueInfo, error) {
	values, err := helm.GetReleaseValues(name, namespace)
	if err != nil {
		return HelmValueInfo{}, err
	}
	return HelmValueInfo{
		ChartName:   name,
		ChartValues: values,
	}, nil
}

// getPods retrieves pods for a given release in the specified namespace.
func (r *HosstedProjectReconciler) getPods(ctx context.Context, cve bool, namespace, releaseName string) ([]PodInfo, []SecurityInfo, error) {
	pods, err := r.listPods(ctx, namespace, map[string]string{
		"app.kubernetes.io/instance":   releaseName,
		"app.kubernetes.io/managed-by": "Helm",
	})
	if err != nil {
		return nil, nil, err
	}

	var vulns *[]trivy.VulnerabilityReport
	if cve {
		vulns, err = r.listVunerability(ctx, namespace)
		if err != nil {
			return nil, nil, err
		}
	}
	var podHolder []PodInfo
	var securityInfoHolder []SecurityInfo
	var securityInfoContainerHolder []SecurityInfoContainer

	for _, po := range pods.Items {
		podInfo := PodInfo{
			Name:      po.Name,
			Namespace: po.Namespace,
			Image:     po.Spec.Containers[0].Image,
			Status:    string(po.Status.Phase),
		}

		if vulns != nil {
			for _, container := range po.Spec.Containers {
				for _, vuln := range *vulns {
					if container.Name == vuln.GetLabels()["trivy-operator.container.name"] {
						if vuln.Report.Vulnerabilities != nil {
							securityInfoContainer := SecurityInfoContainer{
								ContainerImage:       container.Image,
								Type:                 "k8s",
								VulnerabilitySummary: vuln.Report.Summary,
								Vulnerabilities:      vuln.Report.Vulnerabilities,
							}
							securityInfoContainerHolder = append(securityInfoContainerHolder, securityInfoContainer)
						}
					}
				}
			}
		}

		securityInfo := SecurityInfo{
			PodName:      po.Name,
			PodNamespace: po.Namespace,
			Containers:   securityInfoContainerHolder,
		}
		podHolder = append(podHolder, podInfo)

		securityInfoHolder = append(securityInfoHolder, securityInfo)
	}

	return podHolder, securityInfoHolder, nil
}

// getStatefulsets retrieves statefulset for a given release in the specified namespace.
func (r *HosstedProjectReconciler) getStatefulsets(ctx context.Context, namespace, releaseName string) ([]StatefulsetInfo, error) {
	statefulsets, err := r.listStatefulsets(ctx, namespace, map[string]string{
		"app.kubernetes.io/instance":   releaseName,
		"app.kubernetes.io/managed-by": "Helm",
	})
	if err != nil {
		return nil, err
	}

	var statefulsetHolder []StatefulsetInfo

	for _, deploy := range statefulsets.Items {
		statefulInfo := StatefulsetInfo{
			Name:      deploy.Name,
			Namespace: deploy.Namespace,
		}
		statefulsetHolder = append(statefulsetHolder, statefulInfo)
	}

	return statefulsetHolder, nil
}

// getSecrets retrieves secrets for a given release in the specified namespace.
func (r *HosstedProjectReconciler) getSecrets(ctx context.Context, namespace, releaseName string) ([]SecretInfo, error) {
	secretInfo, err := r.listSecrets(ctx, namespace, map[string]string{
		"app.kubernetes.io/instance":   releaseName,
		"app.kubernetes.io/managed-by": "Helm",
	})
	if err != nil {
		return nil, err
	}

	var secretInfoHolder []SecretInfo

	for _, secret := range secretInfo.Items {
		secretInfo := SecretInfo{
			Name:      secret.Name,
			Namespace: secret.Namespace,
			Data:      secret.Data,
		}
		secretInfoHolder = append(secretInfoHolder, secretInfo)
	}

	return secretInfoHolder, nil
}

// getDeployments retrieves deployments for a given release in the specified namespace.
func (r *HosstedProjectReconciler) getDeployments(ctx context.Context, namespace, releaseName string) ([]DeploymentInfo, error) {
	deployments, err := r.listDeployments(ctx, namespace, map[string]string{
		"app.kubernetes.io/instance":   releaseName,
		"app.kubernetes.io/managed-by": "Helm",
	})
	if err != nil {
		return nil, err
	}

	var deploymentHolder []DeploymentInfo

	for _, deploy := range deployments.Items {
		for _, container := range deploy.Spec.Template.Spec.Containers {
			for _, env := range container.Env {
				if env.ValueFrom == nil || env.ValueFrom.SecretKeyRef == nil {
					deploymentInfo := DeploymentInfo{
						Name:      deploy.Name,
						Namespace: deploy.Namespace,
						//SecretRef: env.ValueFrom.SecretKeyRef,
					}
					deploymentHolder = append(deploymentHolder, deploymentInfo)
				} else {
					deploymentInfo := DeploymentInfo{
						Name:      deploy.Name,
						Namespace: deploy.Namespace,
						SecretRef: env.ValueFrom.SecretKeyRef,
					}
					deploymentHolder = append(deploymentHolder, deploymentInfo)
				}

			}
		}

	}

	return deploymentHolder, nil
}

// getServices retrieves services for a given release in the specified namespace.
func (r *HosstedProjectReconciler) getServices(ctx context.Context, namespace, releaseName string) ([]ServiceInfo, error) {
	svcs, err := r.listServices(ctx, namespace, map[string]string{
		"app.kubernetes.io/instance":   releaseName,
		"app.kubernetes.io/managed-by": "Helm",
	})
	if err != nil {
		return nil, err
	}

	var svcHolder []ServiceInfo
	for _, svc := range svcs.Items {
		svcInfo := ServiceInfo{
			Name:      svc.Name,
			Namespace: svc.Namespace,
			Port:      svc.Spec.Ports[0].Port,
		}
		svcHolder = append(svcHolder, svcInfo)
	}

	return svcHolder, nil
}

// getConfigmap retrieves services for a given release in the specified namespace.
func (r *HosstedProjectReconciler) getConfigmaps(ctx context.Context, namespace, releaseName string) ([]ConfigmapInfo, error) {
	cms, err := r.listConfigmap(ctx, namespace, map[string]string{
		"app.kubernetes.io/instance":   releaseName,
		"app.kubernetes.io/managed-by": "Helm",
	})
	if err != nil {
		return nil, err
	}

	var cmHolder []ConfigmapInfo
	for _, cm := range cms.Items {
		svcInfo := ConfigmapInfo{
			Name: cm.Name,
			Data: cm.Data,
		}
		cmHolder = append(cmHolder, svcInfo)
	}

	return cmHolder, nil
}

// getVolumes retrieves volumes for a given release in the specified namespace.
func (r *HosstedProjectReconciler) getVolumes(ctx context.Context, namespace, releaseName string) ([]VolumeInfo, error) {
	pvcs, err := r.listVolumes(ctx, namespace, map[string]string{
		"app.kubernetes.io/instance": releaseName,
	})
	if err != nil {
		return nil, err
	}
	var pvcHolder []VolumeInfo
	for _, pvc := range pvcs.Items {
		pvcInfo := VolumeInfo{
			Name:      pvc.Name,
			Namespace: pvc.Namespace,
			Size:      pvc.Spec.Size(),
		}
		pvcHolder = append(pvcHolder, pvcInfo)
	}

	return pvcHolder, nil
}

// getIngress retrieves ingress for a given release in the specified namespace.
func (r *HosstedProjectReconciler) getIngress(ctx context.Context, namespace, releaseName string) ([]IngressInfo, error) {
	ings, err := r.listIngresses(ctx, namespace, map[string]string{
		"app.kubernetes.io/instance":   releaseName,
		"app.kubernetes.io/managed-by": "Helm",
	})
	if err != nil {
		return nil, err
	}

	var ingHolder []IngressInfo
	for _, ing := range ings.Items {
		ingInfo := IngressInfo{
			Name:      ing.Name,
			Namespace: ing.Namespace,
			Domain:    ing.Spec.Rules[0].Host,
		}
		ingHolder = append(ingHolder, ingInfo)
	}

	return ingHolder, nil
}

// getHelmInfo retrieves Helm release information.
func (r *HosstedProjectReconciler) getHelmInfo(release helmrelease.Release, instance *hosstedcomv1.Hosstedproject) (hosstedcomv1.HelmInfo, error) {
	helmStatus := instance.Status.HelmStatus // Get the current HelmStatus

	existingUUID := findExistingUUID(helmStatus, release.Name, release.Namespace)
	if existingUUID != "" {
		return hosstedcomv1.HelmInfo{
			Name:       release.Name,
			Namespace:  release.Namespace,
			AppUUID:    existingUUID,
			Revision:   release.Version,
			Updated:    release.Info.LastDeployed.Time.String(),
			Status:     string(release.Info.Status),
			Chart:      release.Chart.Name(),
			AppVersion: release.Chart.AppVersion(),
		}, nil
	}

	return hosstedcomv1.HelmInfo{
		Name:       release.Name,
		Namespace:  release.Namespace,
		AppUUID:    "A-" + uuid.NewString(),
		Revision:   release.Version,
		Updated:    release.Info.LastDeployed.Time.String(),
		Status:     string(release.Info.Status),
		Chart:      release.Chart.Name(),
		AppVersion: release.Chart.AppVersion(),
	}, nil
}

// findExistingUUID checks if the appUUID already exists in the status
func findExistingUUID(helmStatus []hosstedcomv1.HelmInfo, releaseName, namespace string) string {
	for _, info := range helmStatus {
		if info.Name == releaseName && info.Namespace == namespace {
			return info.AppUUID
		}
	}
	return ""
}

func (r *HosstedProjectReconciler) getAppUUIDFromSecret(ctx context.Context, namespace string) (string, error) {

	secret, err := r.getSecret(ctx, "uuid", namespace)
	if err != nil {
		return "", err
	}

	return string(secret.Data["uuid"]), nil
}

func isHostedHelm(release helmrelease.Release) bool {

	key := "app.kubernetes.io/managed-by"
	value := "Helm"

	// key := "hossted_helm"
	// value := "true"

	pattern := fmt.Sprintf(`\b%s:\s*%s\b`, key, value)

	re := regexp.MustCompile(pattern)
	if re.MatchString(release.Manifest) {
		return true
	} else {
		return false
	}
}

func (r *HosstedProjectReconciler) getAccessInfo(ctx context.Context) (AccessInfo, error) {
	cm := v1.ConfigMap{}
	err := r.Client.Get(ctx, types.NamespacedName{
		Namespace: "hossted-platform",
		Name:      "access-object-info",
	}, &cm)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return AccessInfo{}, nil
		}
		return AccessInfo{}, nil
	}

	var pmc PrimaryCreds
	// Extract the JSON string from the ConfigMap's data
	if jsonString, ok := cm.Data["access-object.json"]; ok {
		// Unmarshal the JSON string into the PrimaryCreds struct
		err = json.Unmarshal([]byte(jsonString), &pmc)
		if err != nil {
			return AccessInfo{}, fmt.Errorf("failed to unmarshal access-object.json: %w", err)
		}
	} else {
		return AccessInfo{}, fmt.Errorf("access-object.json key not found in ConfigMap")
	}

	var user, password string
	// Fetch user from ConfigMap or Secret
	if pmc.User.ConfigMap != "" {
		cmInfo := v1.ConfigMap{}
		err = r.Client.Get(ctx, types.NamespacedName{
			Namespace: pmc.Namespace,
			Name:      pmc.User.ConfigMap,
		}, &cmInfo)
		if err != nil {
			return AccessInfo{}, fmt.Errorf("failed to get user ConfigMap: %w", err)
		}
		user = cmInfo.Data[pmc.User.Key]
	} else if pmc.User.SecretName != "" {
		secretInfo := v1.Secret{}
		err := r.Client.Get(ctx, types.NamespacedName{
			Namespace: pmc.Namespace,
			Name:      pmc.User.SecretName,
		}, &secretInfo)
		if err != nil {
			return AccessInfo{}, fmt.Errorf("failed to get user Secret: %w", err)
		}
		// Check if the user type is "text", if so, search for the value in the file
		if pmc.User.Type == "file" {
			user, err = searchForTextInFile(string(secretInfo.Data[pmc.User.Key]), pmc.User.Text)
			if err != nil {
				return AccessInfo{}, fmt.Errorf("failed to find text '%s' in user Secret: %w", pmc.User.Text, err)
			}
		} else {
			user = string(secretInfo.Data[pmc.User.Key])
		}
	}

	// Fetch password from Secret or ConfigMap
	if pmc.Password.SecretName != "" {
		secretInfo := v1.Secret{}
		err := r.Client.Get(ctx, types.NamespacedName{
			Namespace: pmc.Namespace,
			Name:      pmc.Password.SecretName,
		}, &secretInfo)
		if err != nil {
			return AccessInfo{}, fmt.Errorf("failed to get password Secret: %w", err)
		}
		password = string(secretInfo.Data[pmc.Password.Key])

		// If the password type is "text", search for the text value inside the secret file
		if pmc.Password.Type == "file" {
			password, err = searchForTextInFile(string(secretInfo.Data[pmc.Password.Key]), pmc.Password.Text)
			if err != nil {
				return AccessInfo{}, fmt.Errorf("failed to find text '%s' in password Secret: %w", pmc.Password.Text, err)
			}
		}
	} else if pmc.Password.ConfigMap != "" {
		cmInfo := v1.ConfigMap{}
		err := r.Client.Get(ctx, types.NamespacedName{
			Namespace: pmc.Namespace,
			Name:      pmc.Password.ConfigMap,
		}, &cmInfo)
		if err != nil {
			return AccessInfo{}, fmt.Errorf("failed to get password ConfigMap: %w", err)
		}
		password = cmInfo.Data[pmc.Password.Key]
	}

	// Create AccessInfo from the retrieved credentials
	access := AccessInfo{}
	ingressList := networkingv1.IngressList{}
	err = r.Client.List(ctx, &ingressList, &client.ListOptions{Namespace: pmc.Namespace})
	if err != nil {
		return AccessInfo{}, fmt.Errorf("failed to list Ingresses: %w", err)
	}

	// Extract user and password from pmc.Secret.Value (format: "user:password")
	var buser, bpassword string
	if pmc.Secret.Name != "" && pmc.Secret.Value != "" {
		creds := strings.SplitN(pmc.Secret.Value, ":", 2)
		if len(creds) != 2 {
			return AccessInfo{}, fmt.Errorf("invalid format for secret value, expected 'user:password'")
		}
		buser = creds[0]
		bpassword = creds[1]
	}

	if pmc.Secret.Name != "" {
		if pmc.Secret.Type == "basic-auth" {
			_, err = createBasicAuthSecret(ctx, r, pmc.Namespace, pmc.Secret.Name, buser, bpassword)
			if err != nil {
				return AccessInfo{}, fmt.Errorf("failed to create basic-auth secret: %w", err)
			}
		}
	}

	ingressClassName := "hossted-operator"
	for _, ingress := range ingressList.Items {
		if ingress.Spec.IngressClassName != nil && *ingress.Spec.IngressClassName == ingressClassName {
			if len(ingress.Spec.Rules) > 0 {
				url := ingress.Spec.Rules[0].Host
				var urlInfo URLInfo
				if user != "" && password != "" {
					urlInfo = URLInfo{
						URL:      url,
						User:     toBase64(user),
						Password: toBase64(password),
					}
				} else {
					urlInfo = URLInfo{
						URL:      url,
						User:     toBase64(buser),
						Password: toBase64(bpassword),
					}
				}
				access.URLs = append(access.URLs, urlInfo)
			}
		}
	}

	log.Printf("access info object %v", access)

	if len(access.URLs) == 0 {
		log.Printf("no matching Ingress found with class %s\n", ingressClassName)
		return AccessInfo{}, nil
	}

	return access, nil
}

// getCustomIngressName checks if 'custom-values-holder' ConfigMap has a custom ingress name
func (r *HosstedProjectReconciler) getCustomIngressName(ctx context.Context, namespace string) (string, error) {
	cm := v1.ConfigMap{}
	err := r.Client.Get(ctx, types.NamespacedName{
		Namespace: namespace,
		Name:      "custom-values-holder",
	}, &cm)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return "", nil // ConfigMap not found, no custom ingress name
		}
		return "", err
	}

	// Check for "hosstedCustomIngressName" in custom-values.json
	if jsonString, ok := cm.Data["custom-values.json"]; ok {
		var data map[string][]string
		err = json.Unmarshal([]byte(jsonString), &data)
		if err != nil {
			return "", fmt.Errorf("failed to unmarshal custom-values.json: %w", err)
		}
		if len(data["hosstedCustomIngressName"]) > 0 {
			return data["hosstedCustomIngressName"][0], nil // Return the first ingress name if available
		}
	}
	return "", nil // No custom ingress name found
}

// getDns retrieves ingress and returns DnsInfo
func (r *HosstedProjectReconciler) getDns(ctx context.Context, instance *hosstedcomv1.Hosstedproject, releaseNamespace, appUUID string) error {
	cm := v1.ConfigMap{}
	err := r.Client.Get(ctx, types.NamespacedName{
		Namespace: "hossted-platform",
		Name:      "access-object-info",
	}, &cm)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return err
		}
		return err
	}

	var pmc PrimaryCreds
	if jsonString, ok := cm.Data["access-object.json"]; ok {
		err = json.Unmarshal([]byte(jsonString), &pmc)
		if err != nil {
			return fmt.Errorf("failed to unmarshal access-object.json: %w", err)
		}
	} else {
		return fmt.Errorf("access-object.json key not found in ConfigMap")
	}

	if releaseNamespace != pmc.Namespace {
		log.Println("Release Namespace ", releaseNamespace, "not a marketplace app, ignoring")
		return nil
	}

	ing := &networkingv1.Ingress{}
	dnsinfo := DnsInfo{}
	var dnsName string
	retryCount := 0
	maxRetries := 5
	retryInterval := 10 * time.Second

	// Retrieve custom ingress name from 'custom-values-holder' ConfigMap, if it exists
	customIngressName, err := r.getCustomIngressName(ctx, pmc.Namespace)
	if err != nil {
		return err
	}

	// Use custom ingress name if available, otherwise fall back to pmc.Namespace
	ingressName := pmc.Namespace
	if customIngressName != "" {
		ingressName = customIngressName
	}

	for retryCount < maxRetries {
		err := r.Client.Get(ctx, types.NamespacedName{
			Namespace: pmc.Namespace,
			Name:      ingressName,
		}, ing)
		if err != nil {
			return err
		}

		dnsName = appUUID + "." + "f.hossted.app"
		if ing != (&networkingv1.Ingress{}) {
			if ing.Status.LoadBalancer.Ingress == nil {
				log.Print("Ingress Status has no LB address, retrying in 30 seconds...")
				time.Sleep(retryInterval)
				retryCount++
				continue
			}

			if ing.Status.LoadBalancer.Ingress[0].IP != "" {
				dnsinfo.Content = ing.Status.LoadBalancer.Ingress[0].IP
				dnsinfo.Type = "A"
			} else if ing.Status.LoadBalancer.Ingress[0].Hostname != "" {
				dnsinfo.Content = ing.Status.LoadBalancer.Ingress[0].Hostname
				dnsinfo.Type = "CNAME"
			}

			dnsinfo.Name = toLowerCase(dnsName)
			dnsinfo.ClusterId = instance.Status.ClusterUUID
			dnsinfo.UserUUID = os.Getenv("HOSSTED_USER_ID")
			dnsinfo.Env = "dev"

			break // Exit loop if LB is available
		}
	}

	if retryCount >= maxRetries {
		return fmt.Errorf("LoadBalancer failed to become available after %d retries", maxRetries)
	}

	dnsByte, err := json.Marshal(dnsinfo)
	if err != nil {
		return err
	}

	err = sendEvent("info", init_dns_registeration, os.Getenv("HOSSTED_ORG_ID"), instance.Status.ClusterUUID)
	if err != nil {
		log.Print(err)
	}

	resp, err := http.HttpRequest(dnsByte, os.Getenv("HOSSTED_API_URL")+"/clusters/dns")
	if err != nil {
		return err
	}

	log.Println(string(dnsByte))
	log.Println(string(resp.ResponseBody))

	ing.Spec.Rules[0].Host = toLowerCase(dnsName)
	for _, h := range instance.Spec.Helm {
		newHelm := helm.Helm{
			ReleaseName: h.ReleaseName,
			Namespace:   h.Namespace,
			Values:      tweakIngressHostname(h.Values, toLowerCase(dnsName)),
			RepoName:    h.RepoName,
			ChartName:   h.ChartName,
			RepoUrl:     h.RepoUrl,
		}
		log.Println("Performing upgrade for ", h.ReleaseName, " with hostname ", toLowerCase(dnsName))
		err := helm.Upgrade(newHelm)
		if err != nil {
			return err
		}

		instance.Status.DnsUpdated = true
		if err := r.Status().Update(ctx, instance); err != nil {
			return err
		}
	}
	return nil
}

func toLowerCase(input string) string {
	return strings.ToLower(input)
}

func tweakIngressHostname(existingStrings []string, dnsName string) []string {
	updatedStrings := make([]string, 0)

	// Convert the DNS name to lowercase
	lowercaseDNSName := strings.ToLower(dnsName)

	// Iterate through the existing strings
	for _, str := range existingStrings {
		// Check if the string contains "ingress.hostname=" or "ingress.hosts[0]="
		if strings.Contains(str, "ingress.hostname=") {
			// Modify the string by appending the lowercase DNS name
			str = "ingress.hostname=" + lowercaseDNSName
		} else if strings.Contains(str, "ingress.hosts[0]=") {
			// Modify the string by appending the lowercase DNS name
			str = "ingress.hosts[0]=" + lowercaseDNSName
		} else if strings.Contains(str, "ingress.hosts[0].host=") {
			str = "ingress.hosts[0].host=" + lowercaseDNSName
		}
		// Keep the existing string (either modified or original) in the updated list
		updatedStrings = append(updatedStrings, str)
	}

	// Return the updated list of strings
	return updatedStrings
}

func searchForTextInFile(fileContent, searchText string) (string, error) {
	scanner := bufio.NewScanner(strings.NewReader(fileContent))
	for scanner.Scan() {
		line := scanner.Text()
		// Check if the line contains the search text
		if strings.HasPrefix(line, searchText) {
			// Split the line based on '=' to get the value
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				// Return the value after trimming spaces
				return strings.TrimSpace(parts[1]), nil
			}
		}
	}
	return "", fmt.Errorf("text '%s' not found in the file", searchText)
}

// Helper function to create a Kubernetes Secret for basic authentication
func createBasicAuthSecret(ctx context.Context, r *HosstedProjectReconciler, namespace, secretName, user, password string) (string, error) {
	// Generate bcrypt hash of the password (similar to htpasswd)
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("failed to hash password: %w", err)
	}

	// Base64 encode the "username:hashedPassword" as expected for basic auth
	auth := []byte(fmt.Sprintf("%s:%s", user, string(hashedPassword)))

	// Create the Secret object
	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: namespace,
		},
		Type: v1.SecretTypeOpaque,
		Data: map[string][]byte{
			"auth": auth,
		},
	}

	// Check if the secret already exists
	existingSecret := &v1.Secret{}
	err = r.Client.Get(ctx, types.NamespacedName{
		Namespace: namespace,
		Name:      secretName,
	}, existingSecret)
	if err != nil && errors.IsNotFound(err) {
		// Secret does not exist, create it
		err = r.Client.Create(ctx, secret)
		if err != nil {
			return "", fmt.Errorf("failed to create basic-auth secret: %w", err)
		}
		log.Printf("Secret %s created successfully in namespace %s\n", secretName, namespace)
	} else if err != nil {
		// Other errors
		return "", fmt.Errorf("failed to get basic-auth secret: %w", err)
	}

	return "", nil
}

func toBase64(input string) string {
	encoded := base64.StdEncoding.EncodeToString([]byte(input))
	return encoded
}

func sendK8sEvents(clusterUUID string) {
	// Retrieve the HOSSTED_ORG_ID once and reuse it
	hosstedOrgID := os.Getenv("HOSSTED_ORG_ID")

	// Send event for Pod information collection
	err := sendEvent("info", init_pod_info_collection, hosstedOrgID, clusterUUID)
	if err != nil {
		log.Print(err)
	}

	// Send event for Service information collection
	err = sendEvent("info", init_service_info_collection, hosstedOrgID, clusterUUID)
	if err != nil {
		log.Print(err)
	}

	// Send event for Volume (PVC) information collection
	err = sendEvent("info", init_volume_info_collection, hosstedOrgID, clusterUUID)
	if err != nil {
		log.Print(err)
	}

	// Send event for Ingress information collection
	err = sendEvent("info", init_ingress_info_collection, hosstedOrgID, clusterUUID)
	if err != nil {
		log.Print(err)
	}

	// Send event for Security information collection
	err = sendEvent("info", init_security_info_collection, hosstedOrgID, clusterUUID)
	if err != nil {
		log.Print(err)
	}

	// Send event for Configmap information collection
	err = sendEvent("info", init_configmap_info_collection, hosstedOrgID, clusterUUID)
	if err != nil {
		log.Print(err)
	}

	// Send event for Helm value information collection
	err = sendEvent("info", init_helmvalue_info_collection, hosstedOrgID, clusterUUID)
	if err != nil {
		log.Print(err)
	}

	// Send event for Deployment information collection
	err = sendEvent("info", init_deployment_info_collection, hosstedOrgID, clusterUUID)
	if err != nil {
		log.Print(err)
	}

	// Send event for StatefulSet information collection
	err = sendEvent("info", init_statefulset_info_collection, hosstedOrgID, clusterUUID)
	if err != nil {
		log.Print(err)
	}

	// Send event for Secret information collection
	err = sendEvent("info", init_secret_info_collection, hosstedOrgID, clusterUUID)
	if err != nil {
		log.Print(err)
	}
}
