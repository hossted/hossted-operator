<h2 align="center">
  <br>
  Hossted Kubernetes Operator
</h2>

<div align="center">

[![Go Report Card](https://goreportcard.com/badge/github.com/hossted/hossted-operator)](https://goreportcard.com/report/github.com/hossted/hossted-operator)

</div>

- Hossted Operator collects information about hossted deployed apps in a kubernetes cluster.
- It is built in Golang using [kubebuilder](https://github.com/kubernetes-sigs/kubebuilder).
- Refer to [Documentation](./docs/README.md) for getting started.

### Supported CR's

- The operator supports CR's of type ```HosstedProject```.
- ```HosstedProject``` CR belongs to api Group ```hossted.com``` and version ```v1```

## Quick start

### Installation


### Hossted operator 

- Add Hossted Helm Repo
```
helm repo add hossted https://charts.hossted.com
```
- Search operator versions
```
helm search repo hossted --versions
```

## Upgrade Process

### 1. Build Docker Image
```bash
docker build -t hossted/operator:${version}
```
Replace `${version}` with your new version number (e.g., 0.7.6)

### 2. Update Helm Chart
In the helm chart directory, update `Chart.yaml`. Both `version` and `appVersion` need to match your new version:

```yaml
apiVersion: v2
name: hossted-operator
description: A Helm chart for Kubernetes
type: application
version: 0.7.6      # Update this version
appVersion: 0.7.6   # Update this version to match
```

### 3. Package and Release
1. Package the helm chart from within the helm directory:
   ```bash
   helm package .
   ```

2. Move the packaged artifact to helm-release directory

3. Update the Helm repository index:
   ```bash
   helm repo update --index charts.hossted.com .
   ```

### 4. Git Workflow
1. Push the updated artifacts to your branch
2. Create a pull request and merge to main

### 5. Adding Helm Repository
```bash
helm repo add charts.hossted.com <repository-url>
```
Note: Replace `<repository-url>` with the actual hossted charts repository URL.

## Configuration Values

⚠️ Important: The following values are typically managed by the hossted CLI. Only modify manually if you're doing a local upgrade and know the correct values for your environment.

```yaml
env:
  HOSSTED_API_URL: "https://api.hossted.com/v1/instances"
  RECONCILE_DURATION: "10s"
  HOSSTED_ORG_ID: "123-123-123-123"
  HOSSTED_TOKEN: ""
  HOSSTED_USER_ID: ""
  CONTEXT_NAME: ""
  LOKI_PASSWORD: ""
  LOKI_URL: ""
  LOKI_USERNAME: ""
  MIMIR_PASSWORD: ""
  MIMIR_URL: ""
  MIMIR_USERNAME: ""
  CLOUD_PROVIDER: ""
  PRODUCT_CODE: ""
  AWS_REGION: ""
  MARKET_PLACE: ""
replicas: 1
secret:
  HOSSTED_AUTH_TOKEN: ""
stop: false
cve:
  enable: true
monitoring:
  enable: true
logging:
  enable: false
ingress:
  enable: true
serviceAccount:
  create: false          # Set to true if you want to create a new ServiceAccount
  name: "my-service-account"   # Specify the name of an existing ServiceAccount
servicemonitors:
  create: false
nameOverride: hossted
operator:
  image:
    repository: hossted/operator
    tag: 0.7.0          # Make sure this matches your new version
helm:
  - releaseName: "kube-state-metrics"
    namespace: "ksm"
    values:  
      - "selfMonitor.enabled=true"
    repoName: "grafana"
    chartName: "kube-state-metrics"
    repoUrl: "https://prometheus-community.github.io/helm-charts"
primaryCreds: {}
```

- pass the above values file whenever you do an upgrade using helm command. ```helm upgrade --install hossted-operator  hossted/hossted-operator -n hossted-operator -f values.yaml```

#### Helm chart values

##### Global ENV values
Some of the environment variables are filled by hossted-cli https://github.com/hossted/cli after running "hossted activate -t k8s"
| Name  | Description|Values|Default Value| 
| ------------- | ------------- |------------- |------------- |
| env.HOSSTED_API_URL | URL of dashboard  |"https://api.hossted.com/v1/instances"  |"https://api.hossted.com/v1/instances"|
| env.EMAIL_ID  | Email of organization in dashboard. Filled by hossted cli  |"" |"" |
| env.RECONCILE_DURATION  | Operator reconcile duration  |"10s" |"10s" |
| env.HOSSTED_ORG_ID  | Organization ID in dashboard. Connected to EMAIL_I. Filled by hossted cliD  |"" |"" |
| env.CONTEXT_NAME  | Cluster or VM name  |"" |"" |
| env.LOKI_PASSWORD  | Password to Loki. Filled by hossted cli  |"" |"" |
| env.LOKI_URL  | URL for Loki. Filled by hossted cli  |"" |"" |
| env.LOKI_USERNAME  | Username for Loki. Filled by hossted cli  |"" |"" |
| env.MIMIR_URL  | URL for Mimir. Filled by hossted cli  |"" |"" |
| env.MIMIR_USERNAME  | Username for Mimir. Filled by hossted cli  |"" |"" |
| env.MIMIR_PASSWORD  | Password for Mimir. Filled by hossted cli  |"" |"" |
| env.CLOUD_PROVIDER  | Cloud provider. Should be provided when MARKET_PLACE=enabled.  |"AWS" or "Azure" |"" |
| env.MARKET_PLACE  | If operator is used for marketplace  |"false" or "true" |"false" |
| env.PRODUCT_CODE  | If CLOUD_PROVIDER="AWS". Used for Metering service   |"" |"" |
| env.AWS_REGION  | If CLOUD_PROVIDER="AWS". Used for Metering service  |"" |"" |
| env.HOSSTED_TOKEN  | Filled by hossted cli  |"" |"" |
| env.HOSSTED_USER_ID  | Filled by hossted cli  |"" |"" |

##### Other global values
| Name  | Description|Values|Default Value| 
| ------------- | ------------- |------------- |------------- |
| replicas| Number of replicas  |1  |1 |
| secret.HOSSTED_AUTH_TOKEN| Filled by hossted cli  |""  |"notoken" |
| stop| If the operator should be installed on standby mode(it will not send any registration request) |true or false  |false |
| cve.enable| If trivy should be installed. Filled by hossted cli |true or false  |false |
| monitoring.enable| If grafana alloy agent should be installed. Filled by hossted cli |true or false  |false |
| logging.enable| If grafana alloy agent should send requests to Loki. Filled by hossted cli |true or false  |false|
| ingress.enable| If ingress-controller should be installed and DNS record created. Filled by hossted cli |true or false |false|
| nameOverride|   |"hossted"  |"hossted"|
| operator.image.repository| Image repository name  |hossted/operator  |hossted/operator  |
| operator.image.tag| Image repository tag  |""  |""  |
| servicemonitors.create| If servicemonitors should be installed in cluster  |true or false  |false  |

##### Application related values
| Name  | Description|Sample Values| 
| ------------- | ------------- |------------- |
| helm[].releaseName| Release name of installed helm chart  |"hossted-argocd" |
| helm[].namespace| Namespace where application should be installed  |"hossted-argocd" |
| helm[].values[]| The necessary command for installation helm chart of application  |"server.ingress.enabled=true"|
| helm[].repoName| Application repository name  |"argo-cd"|
| helm[].chartName| Application chart name   |"argo"|
| helm[].repoUrl| Application repository URL   |"https://argoproj.github.io/argo-helm"|
| serviceAccount.create| If the service account should be created in cluster. Necessary for AWS marketplace|"true"|
| serviceAccount.name| Serviceaccount name for application   |"hossted-argocd"|
| customValuesHolder.grafana_product_name| Should be specified if application has specific Grafana dashboard |"argocd"|
| customValuesHolder.hosstedCustomIngressName| Should be specified if application has specific ingress name |"hossted-argocd-server"|
| primaryCreds.namespace| Namespace where UI user/password is stored. |"hossted-argocd"|
| primaryCreds.user.type| Type of secret for username for UI |"plaintext" or "file"|
| primaryCreds.user.text| Plaintext for username for UI or name of key for username inside secret |"admin"|
| primaryCreds.user.key| Key inside Secret or ConfigMap where username can be read by operator  |"rabbitmq.conf"|
| primaryCreds.user.secretName| Secret name where user for UI is stored|"hossted-rabbitmq-config"|
| primaryCreds.user.configMap| ConfigMap name where user for UI is stored|"hossted-rabbitmq-config"|
| primaryCreds.password.key| Key inside Secret where password can be read by operator |"password"|
| primaryCreds.password.secretName| Secret name where password for UI is stored |"argocd-initial-admin-secret"|
