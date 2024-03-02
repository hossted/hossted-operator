# Internal Documentation

### Description

The `Collector` struct aggregates information about the application's API, including Helm releases, pods, services, volumes, and ingress.

### Fields

- **AppAPIInfo**: Contains basic information about the application API.
- **AppInfo**: Contains detailed information about the application, including Helm releases, pods, services, volumes, and ingress.

## AppInfo

### Description

The `AppInfo` struct holds detailed information about the application, including Helm release status, pod details, service information, volume information, and ingress details.

### Fields

- **HelmInfo**: Information about the Helm release, including name, namespace, revision, last update, status, chart, and application version.
- **PodInfo**: Information about Kubernetes pods associated with the application, including name, namespace, image, and status.
- **ServiceInfo**: Information about Kubernetes services associated with the application, including name, namespace, and port.
- **VolumeInfo**: Information about volumes associated with the application, including name, namespace, and size.
- **IngressInfo**: Information about ingress associated with the application, including name, namespace, and domain.

## HelmInfo

### Description

The `HelmInfo` struct holds information about a Helm release, including name, namespace, revision, last update, status, chart, and application version.

## Functions

### Collector Function

#### Description

The `collector` function collects information about Helm releases, pods, services, volumes, and ingress for a given Hossted project.

#### Parameters

- **ctx**: Context object.
- **instance**: Instance of the Hossted project.

#### Returns

- **collectors**: A slice containing collected information about the project.
- **revisions**: A sorted slice of revisions.
- **error**: An error, if any, encountered during collection.

### Collector Core Function Flow

1. **List Namespaces**: Retrieve a list of namespaces from the Kubernetes cluster.
2. **Filter Namespaces**: Filter out denied namespaces based on the Hossted project's configuration.
3. **Iterate Over Namespaces**:
    - For each namespace, retrieve Helm releases.
    - If there are no releases, skip to the next namespace.
    - Collect information about Helm releases, including HelmInfo, PodInfo, ServiceInfo, VolumeInfo, and IngressInfo.
    - Append collected information to the collectors slice.
    - Append release revisions to the revisions slice.
4. **Sort Revisions**: Sort the revisions slice.
5. **Patch Status**: Update the Hossted project's status with reconciled Helm releases.
6. **Return**: Return the collectors slice, sorted revisions, and any encountered errors.

# Sample Json Emitted

```json
[
    {
        "app_api_info": {
            "cluster_uuid": "",
            "app_uuid": "",
            "app_name": "test",
            "all_good": 0
        },
        "app_info": {
            "helm_info": {
                "name": "test",
                "namespace": "default",
                "revision": 1,
                "updated": "2024-02-28T01:32:37.338988+05:30",
                "status": "deployed",
                "chart": "keycloak",
                "appVersion": "1.0"
            },
            "pod_info": [
                {
                    "name": "test-keycloak-0",
                    "namespace": "default",
                    "image": "docker.io/bitnami/keycloak:23.0.5-debian-11-r0",
                    "status": "Running"
                },
                {
                    "name": "test-postgresql-0",
                    "namespace": "default",
                    "image": "docker.io/bitnami/postgresql:16.1.0-debian-11-r24",
                    "status": "Running"
                }
            ],
            "service_info": [
                {
                    "name": "test-postgresql-hl",
                    "namespace": "default",
                    "port": 5432
                },
                {
                    "name": "test-keycloak-headless",
                    "namespace": "default",
                    "port": 80
                },
                {
                    "name": "test-postgresql",
                    "namespace": "default",
                    "port": 5432
                },
                {
                    "name": "test-keycloak",
                    "namespace": "default",
                    "port": 80
                }
            ],
            "volume_info": [
                {
                    "name": "data-test-postgresql-0",
                    "namespace": "default",
                    "size": 99
                }
            ],
            "ingress_info": [
                {
                    "name": "keycloak",
                    "namespace": "default",
                    "domain": "keycloak.datainfra.io"
                }
            ]
        }
    },
    {
        "app_api_info": {
            "cluster_uuid": "",
            "app_uuid": "",
            "app_name": "my-nginx",
            "all_good": 0
        },
        "app_info": {
            "helm_info": {
                "name": "my-nginx",
                "namespace": "nginx-ingress",
                "revision": 1,
                "updated": "2024-02-28T19:28:33.004501+05:30",
                "status": "deployed",
                "chart": "ingress-nginx",
                "appVersion": "1.9.6"
            },
            "pod_info": [
                {
                    "name": "my-nginx-ingress-nginx-controller-5d759d6cbf-vqvxz",
                    "namespace": "nginx-ingress",
                    "image": "registry.k8s.io/ingress-nginx/controller:v1.9.6@sha256:1405cc613bd95b2c6edd8b2a152510ae91c7e62aea4698500d23b2145960ab9c",
                    "status": "Running"
                }
            ],
            "service_info": [
                {
                    "name": "my-nginx-ingress-nginx-controller-admission",
                    "namespace": "nginx-ingress",
                    "port": 443
                },
                {
                    "name": "my-nginx-ingress-nginx-controller",
                    "namespace": "nginx-ingress",
                    "port": 80
                }
            ],
            "volume_info": [
                {
                    "name": "data-test-postgresql-0",
                    "namespace": "default",
                    "size": 99
                }
            ],
            "ingress_info": [
                {
                    "name": "keycloak",
                    "namespace": "default",
                    "domain": "keycloak.datainfra.io"
                }
            ]
        }
    }
]
```