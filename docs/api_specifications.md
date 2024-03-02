# Hosstedproject API

## Overview

The Hosstedproject API facilitates the management of configurations for Hossted projects CR.

## Usage

### Resource Definition

```go
type HosstedprojectSpec struct {
    Stop           bool     `json:"stop,omitempty"`
    DenyNamespaces []string `json:"denyNamespaces,omitempty"`
}

type HosstedprojectStatus struct {
    ClusterUUID             string            `json:"clusterUUID,omitempty"`
    LastReconciledTimestamp string            `json:"lastReconcileTimestamp,omitempty"`
    ReconciledHelmReleases  map[string]string `json:"reconcileHelmReleases,omitempty"`
    Revision                []int             `json:"revision,omitempty"`
}

type Hosstedproject struct {
    metav1.TypeMeta   `json:",inline"`
    metav1.ObjectMeta `json:"metadata,omitempty"`

    Spec   HosstedprojectSpec   `json:"spec,omitempty"`
    Status HosstedprojectStatus `json:"status,omitempty"`
}

type HosstedprojectList struct {
    metav1.TypeMeta `json:",inline"`
    metav1.ListMeta `json:"metadata,omitempty"`
    Items           []Hosstedproject `json:"items"`
}
```

# Object Fields

## Spec:

- **Stop**: Specifies whether the project should be stopped.
- **DenyNamespaces**: A list of namespaces to deny access to.

## Status:

- **ClusterUUID**: Unique identifier for the cluster.
- **LastReconciledTimestamp**: Timestamp of the last reconciliation.
- **ReconciledHelmReleases**: Map of reconciled Helm releases.
- **Revision**: List of revisions.
