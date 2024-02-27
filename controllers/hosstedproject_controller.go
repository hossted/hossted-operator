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

// HosstedprojectReconciler reconciles a Hosstedproject object
type HosstedprojectReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=hossted.com,resources=hosstedprojects,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=hossted.com,resources=hosstedprojects/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=hossted.com,resources=hosstedprojects/finalizers,verbs=update

func (r *HosstedprojectReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)
	fmt.Println("Reconcile Request", req)
	collector, err := r.collector(ctx)
	if err != nil {
		fmt.Println(err)
	}

	fmt.Println(collector)
	return ctrl.Result{RequeueAfter: time.Second * 3}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *HosstedprojectReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&hosstedcomv1.Hosstedproject{}).
		Complete(r)
}

type Collector struct {
	Helm []HelmInfo
}

type PodInfo struct {
	Name  string `json:"name"`
	Image string `json:"image"`
}

type HelmInfo struct {
	Name       string    `json:"name"`
	Namespace  string    `json:"namespace"`
	Revision   int       `json:"revision"`
	Updated    time.Time `json:"updated"`
	Status     string    `json:"status"`
	Chart      string    `json:"chart"`
	AppVersion string    `json:"appVersion"`
}

func (r *HosstedprojectReconciler) collector(ctx context.Context) (collector *Collector, error error) {
	namespaceList, err := r.listNamespace(ctx)
	if err != nil {
		return nil, err
	}

	var helmHolder []HelmInfo
	for _, ns := range namespaceList {
		releases, err := helm.ListReleases(ns)
		if err != nil {
			return nil, err
		}

		for _, release := range releases {

			helm := HelmInfo{
				Name:       release.Name,
				Namespace:  release.Namespace,
				Revision:   release.Version,
				Updated:    release.Info.LastDeployed.Time,
				Status:     string(release.Info.Status),
				Chart:      release.Chart.Name(),
				AppVersion: release.Chart.AppVersion(),
			}
			helmHolder = append(helmHolder, helm)

			pods, err := r.listPods(ctx, release.Namespace, map[string]string{
				"app.kubernetes.io/instance":   release.Name,
				"app.kubernetes.io/managed-by": "Helm",
			})
			if err != nil {
				return nil, err
			}

			for _, po := range pods.Items {
				fmt.Println(po)
			}
		}
	}

	collector = &Collector{
		Helm: helmHolder,
	}

	return collector, nil
}

func (r *HosstedprojectReconciler) listNamespace(ctx context.Context) (nsList []string, error error) {
	namespaces := &corev1.NamespaceList{}
	err := r.Client.List(ctx, namespaces, &client.ListOptions{})
	if err != nil {
		return nil, err
	}

	for _, namespace := range namespaces.Items {
		nsList = append(nsList, namespace.Name)
	}

	return nsList, nil
}

func (r *HosstedprojectReconciler) listPods(ctx context.Context, namespace string, labels map[string]string) (podList corev1.PodList, error error) {
	poList := &corev1.PodList{}

	listOpts := []client.ListOption{
		client.InNamespace(namespace),
		client.MatchingLabels(labels),
	}

	err := r.Client.List(ctx, poList, listOpts...)
	if err != nil {
		return corev1.PodList{}, err
	}

	return *poList, nil
}
