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
- Install Operator
```
helm upgrade --install hossted-operator hossted/hossted-operator -n hossted-operator --set env.HOSSTED_API_URL="<>",env.HOSSTED_AUTH_TOKEN="<>",env.EMAIL_ID="<>"
 ```

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