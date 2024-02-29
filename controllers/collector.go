package controllers

import (
	"context"
	"time"

	hosstedcomv1 "github.com/hossted/hossted-operator/api/v1"
	helm "github.com/hossted/hossted-operator/pkg/helm"
	helmrelease "helm.sh/helm/v3/pkg/release"
)

type Collector struct {
	AppAPIInfo AppAPIInfo `json:"app_api_info"`
	AppInfo    AppInfo    `json:"app_info"`
}

type AppInfo struct {
	HelmInfo    HelmInfo      `json:"helm_info"`
	PodInfo     []PodInfo     `json:"pod_info"`
	ServiceInfo []ServiceInfo `json:"service_info"`
	VolumeInfo  []VolumeInfo  `json:"volume_info"`
	IngressInfo []IngressInfo `json:"ingress_info"`
}

// AppAPIInfo contains basic information about the application API.
type AppAPIInfo struct {
	ClusterUUID string `json:"cluster_uuid"`
	AppUUID     string `json:"app_uuid"`
	AppName     string `json:"app_name"`
	AllGood     int    `json:"all_good"`
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

// PodInfo contains information about a Kubernetes pod.
type PodInfo struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Image     string `json:"image"`
	Status    string `json:"status"`
}

type VolumeInfo struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Size      int    `json:"size"`
}

// HelmInfo contains information about a Helm release.
type HelmInfo struct {
	Name       string    `json:"name"`
	Namespace  string    `json:"namespace"`
	Revision   int       `json:"revision"`
	Updated    time.Time `json:"updated"`
	Status     string    `json:"status"`
	Chart      string    `json:"chart"`
	AppVersion string    `json:"appVersion"`
}

func (r *HosstedProjectReconciler) collector(ctx context.Context, instance *hosstedcomv1.Hosstedproject) ([]*Collector, error) {

	var collectors []*Collector
	namespaceList, err := r.listNamespaces(ctx)
	if err != nil {
		return nil, err
	}

	// Assuming instance.Spec.DenyNamespaces is the slice of denied namespaces
	filteredNamespaces := filter(namespaceList, instance.Spec.DenyNamespaces)

	for _, ns := range filteredNamespaces {

		releases, err := r.listReleases(ctx, ns)
		if err != nil {
			return nil, err
		}

		if len(releases) == 0 {
			// If there are no releases in this namespace, skip to the next one
			continue
		}

		var (
			helmInfo  HelmInfo
			podHolder []PodInfo
			svcHolder []ServiceInfo
			pvcHolder []VolumeInfo
			ingHolder []IngressInfo
			appUUID   string
		)

		for _, release := range releases {
			helmInfo, err = r.getHelmInfo(ctx, *release)
			if err != nil {
				return nil, err
			}

			podHolder, err = r.getPods(ctx, release.Namespace, release.Name)
			if err != nil {
				return nil, err
			}

			svcHolder, err = r.getServices(ctx, release.Namespace, release.Name)
			if err != nil {
				return nil, err
			}

			pvcHolder, err = r.getVolumes(ctx, release.Namespace, release.Name)
			if err != nil {
				return nil, err
			}

			ingHolder, err = r.getIngress(ctx, release.Namespace, release.Name)
			if err != nil {
				return nil, err
			}

			appUUID, err = r.getAppUUID(ctx, "uuid", release.Namespace)
			if err != nil {
				return nil, err
			}
		}

		appInfo := AppInfo{
			HelmInfo:    helmInfo,
			PodInfo:     podHolder,
			ServiceInfo: svcHolder,
			VolumeInfo:  pvcHolder,
			IngressInfo: ingHolder,
		}
		collector := &Collector{
			AppAPIInfo: AppAPIInfo{
				AppName:     appInfo.HelmInfo.Name,
				ClusterUUID: instance.Status.ClusterUUID,
				AppUUID:     appUUID,
			},
			AppInfo: appInfo,
		}
		collectors = append(collectors, collector)
	}

	return collectors, nil
}

// listReleases retrieves all Helm releases in the specified namespace.
func (r *HosstedProjectReconciler) listReleases(ctx context.Context, namespace string) ([]*helmrelease.Release, error) {
	return helm.ListReleases(namespace)
}

// getHelmInfo retrieves Helm release information.
func (r *HosstedProjectReconciler) getHelmInfo(ctx context.Context, release helmrelease.Release) (HelmInfo, error) {
	return HelmInfo{
		Name:       release.Name,
		Namespace:  release.Namespace,
		Revision:   release.Version,
		Updated:    release.Info.LastDeployed.Time,
		Status:     string(release.Info.Status),
		Chart:      release.Chart.Name(),
		AppVersion: release.Chart.AppVersion(),
	}, nil
}

// getPods retrieves pods for a given release in the specified namespace.
func (r *HosstedProjectReconciler) getPods(ctx context.Context, namespace, releaseName string) ([]PodInfo, error) {
	pods, err := r.listPods(ctx, namespace, map[string]string{
		"app.kubernetes.io/instance":   releaseName,
		"app.kubernetes.io/managed-by": "Helm",
	})
	if err != nil {
		return nil, err
	}

	var podHolder []PodInfo
	for _, po := range pods.Items {
		podInfo := PodInfo{
			Name:      po.Name,
			Namespace: po.Namespace,
			Image:     po.Spec.Containers[0].Image,
			Status:    string(po.Status.Phase),
		}
		podHolder = append(podHolder, podInfo)
	}

	return podHolder, nil
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

// getAppUUID from secret data uuid
func (r *HosstedProjectReconciler) getAppUUID(ctx context.Context, name, namespace string) (string, error) {
	secret, err := r.getSecret(ctx, name, namespace)
	if err != nil {
		return "", err
	}

	return string(secret.Data["uuid"]), nil
}
