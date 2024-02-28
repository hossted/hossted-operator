package controllers

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

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

// listVolumes lists volumes in a given namespace with specific labels.
func (r *HosstedProjectReconciler) listVolumes(ctx context.Context, namespace string, labels map[string]string) (*corev1.PersistentVolumeClaimList, error) {
	pvcList := &corev1.PersistentVolumeClaimList{}

	listOpts := []client.ListOption{
		client.InNamespace(namespace),
		client.MatchingLabels(labels),
	}

	err := r.Client.List(ctx, pvcList, listOpts...)
	if err != nil {
		return nil, err
	}

	return pvcList, nil
}
