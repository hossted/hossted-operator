package controllers

import (
	"context"
	"sort"
	"time"

	"github.com/google/uuid"
	hosstedcomv1 "github.com/hossted/hossted-operator/api/v1"
	helm "github.com/hossted/hossted-operator/pkg/helm"
	helmrelease "helm.sh/helm/v3/pkg/release"
)

type Collector struct {
	AppAPIInfo AppAPIInfo `json:"app_api_info"`
	AppInfo    AppInfo    `json:"app_info"`
}

type AppInfo struct {
	HelmInfo    hosstedcomv1.HelmInfo `json:"helm_info"`
	PodInfo     []PodInfo             `json:"pod_info"`
	ServiceInfo []ServiceInfo         `json:"service_info"`
	VolumeInfo  []VolumeInfo          `json:"volume_info"`
	IngressInfo []IngressInfo         `json:"ingress_info"`
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
	AppUUID    string    `json:"appUUID"`
	Revision   int       `json:"revision"`
	Updated    time.Time `json:"updated"`
	Status     string    `json:"status"`
	Chart      string    `json:"chart"`
	AppVersion string    `json:"appVersion"`
}

func (r *HosstedProjectReconciler) collector(ctx context.Context, instance *hosstedcomv1.Hosstedproject) ([]*Collector, []int, error) {

	var collectors []*Collector
	namespaceList, err := r.listNamespaces(ctx)
	if err != nil {
		return nil, nil, err
	}

	// Assuming instance.Spec.DenyNamespaces is the slice of denied namespaces
	filteredNamespaces := filter(namespaceList, instance.Spec.DenyNamespaces)

	//var reconciledHelmReleases = make(map[string]string)
	var revisions []int
	var helmStatusMap = make(map[string]hosstedcomv1.HelmInfo) // Use a map to store unique HelmInfo structs

	for _, ns := range filteredNamespaces {

		releases, err := r.listReleases(ctx, ns)
		if err != nil {
			return nil, nil, err
		}

		if len(releases) == 0 {
			// If there are no releases in this namespace, skip to the next one
			continue
		}

		// Initialize a slice to collect HelmInfo structs for this iteration

		var (
			helmInfo  hosstedcomv1.HelmInfo
			podHolder []PodInfo
			svcHolder []ServiceInfo
			pvcHolder []VolumeInfo
			ingHolder []IngressInfo
		)

		for _, release := range releases {
			helmInfo, err = r.getHelmInfo(ctx, *release, instance)
			if err != nil {
				return nil, nil, err
			}
			helmStatusMap[helmInfo.AppUUID] = helmInfo // Add HelmInfo to the map using AppUUID as key

			podHolder, err = r.getPods(ctx, release.Namespace, release.Name)
			if err != nil {
				return nil, nil, err
			}

			svcHolder, err = r.getServices(ctx, release.Namespace, release.Name)
			if err != nil {
				return nil, nil, err
			}

			pvcHolder, err = r.getVolumes(ctx, release.Namespace, release.Name)
			if err != nil {
				return nil, nil, err
			}

			ingHolder, err = r.getIngress(ctx, release.Namespace, release.Name)
			if err != nil {
				return nil, nil, err
			}

		}

		// After collecting all HelmInfo structs for this iteration, assign to instance.Status.HelmStatus

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
				AppUUID:     appInfo.HelmInfo.AppUUID,
			},
			AppInfo: appInfo,
		}
		collectors = append(collectors, collector)

	}

	sort.Ints(revisions)
	// Convert map values to slice
	var helmStatus []hosstedcomv1.HelmInfo
	for _, helmInfo := range helmStatusMap {
		helmStatus = append(helmStatus, helmInfo)
	}

	// Update instance status
	instance.Status.HelmStatus = helmStatus
	if err := r.Status().Update(ctx, instance); err != nil {
		return []*Collector{}, nil, err
	}

	return collectors, revisions, nil
}

// listReleases retrieves all Helm releases in the specified namespace.
func (r *HosstedProjectReconciler) listReleases(ctx context.Context, namespace string) ([]*helmrelease.Release, error) {
	return helm.ListReleases(namespace)
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

// getHelmInfo retrieves Helm release information.
func (r *HosstedProjectReconciler) getHelmInfo(ctx context.Context, release helmrelease.Release, instance *hosstedcomv1.Hosstedproject) (hosstedcomv1.HelmInfo, error) {
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
		AppUUID:    uuid.NewString(),
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
