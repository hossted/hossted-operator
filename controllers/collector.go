package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"

	networkingv1 "k8s.io/api/networking/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	trivy "github.com/aquasecurity/trivy-operator/pkg/apis/aquasecurity/v1alpha1"
	"github.com/google/uuid"
	hosstedcomv1 "github.com/hossted/hossted-operator/api/v1"
	helm "github.com/hossted/hossted-operator/pkg/helm"
	"github.com/hossted/hossted-operator/pkg/http"
	helmrelease "helm.sh/helm/v3/pkg/release"
	v1 "k8s.io/api/core/v1"
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
	Namespace string `json:"namespace"`
	Password  struct {
		Key        string `json:"key"`
		SecretName string `json:"secretName,omitempty"`
		ConfigMap  string `json:"configMap,omitempty"`
	} `json:"password"`
	User struct {
		SecretName string `json:"secretName,omitempty"`
		ConfigMap  string `json:"configMap,omitempty"`
		Key        string `json:"key"`
	} `json:"user"`
}

type DnsInfo struct {
	Name      string `json:"name"`
	Content   string `json:"content"`
	Type      string `json:"type"`
	ClusterId string `json:"clusterid"`
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
			accessInfo        *AccessInfo
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
			accessInfo, err = r.getAccessInfo(ctx, instance, release.Name, helmInfo.AppUUID)
			if err != nil {
				return nil, nil, nil, err
			}
			// After collecting all HelmInfo structs for this iteration, assign to instance.Status.HelmStatus
			appInfo := AppInfo{
				HelmInfo:        helmInfo,
				AccessInfo:      *accessInfo,
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
	// Convert map values to slice

	// for _, helmInfo := range helmStatusMap {
	// 	fmt.Println("Helm status map ", helmInfo)
	// 	helmStatus = append(helmStatus, helmInfo)

	// }

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

func (r *HosstedProjectReconciler) getAccessInfo(ctx context.Context, instance *hosstedcomv1.Hosstedproject, appName, appUUID string) (*AccessInfo, error) {
	cm := v1.ConfigMap{}
	err := r.Client.Get(ctx, types.NamespacedName{
		Namespace: "hossted-platform",
		Name:      "access-object-info",
	}, &cm)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, nil
	}

	var pmc PrimaryCreds
	// Extract the JSON string from the ConfigMap's data
	if jsonString, ok := cm.Data["access-object.json"]; ok {
		// Unmarshal the JSON string into the PrimaryCreds struct
		err = json.Unmarshal([]byte(jsonString), &pmc)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal access-object.json: %w", err)
		}
	} else {
		return nil, fmt.Errorf("access-object.json key not found in ConfigMap")
	}

	var user, password []byte
	// Fetch user from ConfigMap or Secret
	if pmc.User.ConfigMap != "" {
		cmInfo := v1.ConfigMap{}
		err = r.Client.Get(ctx, types.NamespacedName{
			Namespace: pmc.Namespace,
			Name:      pmc.User.ConfigMap,
		}, &cmInfo)
		if err != nil {
			return nil, fmt.Errorf("failed to get user ConfigMap: %w", err)
		}
		user = []byte(cmInfo.Data[pmc.User.Key])
	} else if pmc.User.SecretName != "" {
		secretInfo := v1.Secret{}
		err := r.Client.Get(ctx, types.NamespacedName{
			Namespace: pmc.Namespace,
			Name:      pmc.User.SecretName,
		}, &secretInfo)
		if err != nil {
			return nil, fmt.Errorf("failed to get user Secret: %w", err)
		}
		user = secretInfo.Data[pmc.User.Key]
	}

	// Fetch password from Secret or ConfigMap
	if pmc.Password.SecretName != "" {
		secretInfo := v1.Secret{}
		err := r.Client.Get(ctx, types.NamespacedName{
			Namespace: pmc.Namespace,
			Name:      pmc.Password.SecretName,
		}, &secretInfo)
		if err != nil {
			return nil, fmt.Errorf("failed to get password Secret: %w", err)
		}
		password = secretInfo.Data[pmc.Password.Key]
	} else if pmc.Password.ConfigMap != "" {
		cmInfo := v1.ConfigMap{}
		err := r.Client.Get(ctx, types.NamespacedName{
			Namespace: pmc.Namespace,
			Name:      pmc.Password.ConfigMap,
		}, &cmInfo)
		if err != nil {
			return nil, fmt.Errorf("failed to get password ConfigMap: %w", err)
		}
		password = []byte(cmInfo.Data[pmc.Password.Key])
	}

	access := AccessInfo{}
	ingressList := networkingv1.IngressList{}
	err = r.Client.List(ctx, &ingressList, &client.ListOptions{Namespace: pmc.Namespace})
	if err != nil {
		return nil, fmt.Errorf("failed to list Ingresses: %w", err)
	}

	ingressClassName := "hossted-operator"
	for _, ingress := range ingressList.Items {
		if ingress.Spec.IngressClassName != nil && *ingress.Spec.IngressClassName == ingressClassName {
			if len(ingress.Spec.Rules) > 0 {
				url := ingress.Spec.Rules[0].Host
				urlInfo := URLInfo{
					URL:      url,
					User:     string(user),
					Password: string(password),
				}
				access.URLs = append(access.URLs, urlInfo)
			}
		}
	}

	if len(access.URLs) == 0 {
		fmt.Printf("no matching Ingress found with class %s\n", ingressClassName)
		return &access, nil
	}

	err = r.getDns(ctx, instance, pmc.Namespace, appName, appUUID)
	if err != nil {
		return &access, err
	}

	return &access, nil
}

// getIngDns retrieves ingress and return DnsInfo
func (r *HosstedProjectReconciler) getDns(ctx context.Context, instance *hosstedcomv1.Hosstedproject, releaseNamespace, appName, appUUID string) error {
	ing := &networkingv1.Ingress{}
	dnsinfo := DnsInfo{}
	err := r.Client.Get(ctx, types.NamespacedName{
		Namespace: releaseNamespace,
		Name:      appName,
	}, ing)
	if err != nil {
		return nil
	}

	dnsName := appUUID + "." + "f.hossted.app"
	if ing != (&networkingv1.Ingress{}) {
		if ing.Status.LoadBalancer.Ingress == nil {
			fmt.Println("Ingress Status has no Lb address, this can take time if ingress just installed.")
			return nil
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

		dnsinfo.Env = "dev"
	}

	dnsByte, err := json.Marshal(dnsinfo)
	if err != nil {
		return err
	}

	resp, err := http.HttpRequest(dnsByte, os.Getenv("HOSSTED_API_URL")+"/clusters/dns")
	if err != nil {
		return err
	}
	fmt.Println(string(dnsByte))
	fmt.Println(string(resp.ResponseBody))

	if resp.StatusCode == 200 {
		ing.Spec.Rules[0].Host = toLowerCase(dnsName)
		for _, h := range instance.Spec.Helm {
			newHelm := helm.Helm{
				ReleaseName: h.ReleaseName,
				Namespace:   h.Namespace,
				Values: []string{
					"ingress.hostname=" + toLowerCase(dnsName),
				},
				RepoName:  h.RepoName,
				ChartName: h.ChartName,
				RepoUrl:   h.RepoUrl,
			}
			fmt.Println("Perfoming upgrade for ", h.ReleaseName, "with hostname ", toLowerCase(dnsName))
			err := helm.Upgrade(newHelm)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func toLowerCase(input string) string {
	return strings.ToLower(input)
}
