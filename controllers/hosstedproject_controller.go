/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	hosstedcomv1 "github.com/hossted/hossted-operator/api/v1"
	"github.com/hossted/hossted-operator/pkg/helm"
)

// HosstedProjectReconciler reconciles a HosstedProject object
type HosstedProjectReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=hossted.com,resources=hosstedprojects,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=hossted.com,resources=hosstedprojects/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=hossted.com,resources=hosstedprojects/finalizers,verbs=update

func (r *HosstedProjectReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)

	collector, err := r.collector(ctx)
	if err != nil {
		fmt.Println(err)
	}

	j, _ := json.Marshal(collector)
	fmt.Println(string(j))
	return ctrl.Result{RequeueAfter: time.Second * 3}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *HosstedProjectReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&hosstedcomv1.Hosstedproject{}).
		Complete(r)
}

// Collector collects information about the application, Helm releases, pods, and services.
type Collector struct {
	App []AppInfo `json:"app_info"`
}

// AppInfo contains information about an application, including its API, Helm release, pods, and services.
type AppInfo struct {
	AppAPIInfo  AppAPIInfo    `json:"app_api_info"`
	HelmInfo    HelmInfo      `json:"helm_info"`
	PodInfo     []PodInfo     `json:"pod_info"`
	ServiceInfo []ServiceInfo `json:"service_info"`
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

// PodInfo contains information about a Kubernetes pod.
type PodInfo struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Image     string `json:"image"`
	Status    string `json:"status"`
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

// collector collects information about pods, services, and Helm releases across all namespaces.
func (r *HosstedProjectReconciler) collector(ctx context.Context) (*Collector, error) {
	namespaceList, err := r.listNamespaces(ctx)
	if err != nil {
		return nil, err
	}

	var appHolder []AppInfo
	for _, ns := range namespaceList {
		releases, err := helm.ListReleases(ns)
		if err != nil {
			return nil, err
		}

		for _, release := range releases {
			helmInfo := HelmInfo{
				Name:       release.Name,
				Namespace:  release.Namespace,
				Revision:   release.Version,
				Updated:    release.Info.LastDeployed.Time,
				Status:     string(release.Info.Status),
				Chart:      release.Chart.Name(),
				AppVersion: release.Chart.AppVersion(),
			}

			pods, err := r.listPods(ctx, release.Namespace, map[string]string{
				"app.kubernetes.io/instance":   release.Name,
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

			svcs, err := r.listServices(ctx, release.Namespace, map[string]string{
				"app.kubernetes.io/instance":   release.Name,
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

			app := AppInfo{
				AppAPIInfo: AppAPIInfo{
					AppName: release.Name,
				},
				HelmInfo:    helmInfo,
				PodInfo:     podHolder,
				ServiceInfo: svcHolder,
			}

			appHolder = append(appHolder, app)
		}
	}

	return &Collector{
		App: appHolder,
	}, nil
}

// listNamespaces lists all namespaces in the cluster.
func (r *HosstedProjectReconciler) listNamespaces(ctx context.Context) ([]string, error) {
	namespaces := &corev1.NamespaceList{}
	err := r.Client.List(ctx, namespaces, &client.ListOptions{})
	if err != nil {
		return nil, err
	}

	var nsList []string
	for _, namespace := range namespaces.Items {
		nsList = append(nsList, namespace.Name)
	}

	return nsList, nil
}

// listPods lists pods in a given namespace with specific labels.
func (r *HosstedProjectReconciler) listPods(ctx context.Context, namespace string, labels map[string]string) (*corev1.PodList, error) {
	poList := &corev1.PodList{}

	listOpts := []client.ListOption{
		client.InNamespace(namespace),
		client.MatchingLabels(labels),
	}

	err := r.Client.List(ctx, poList, listOpts...)
	if err != nil {
		return nil, err
	}

	return poList, nil
}

// listServices lists services in a given namespace with specific labels.
func (r *HosstedProjectReconciler) listServices(ctx context.Context, namespace string, labels map[string]string) (*corev1.ServiceList, error) {
	serviceList := &corev1.ServiceList{}

	listOpts := []client.ListOption{
		client.InNamespace(namespace),
		client.MatchingLabels(labels),
	}

	err := r.Client.List(ctx, serviceList, listOpts...)
	if err != nil {
		return nil, err
	}

	return serviceList, nil
}
