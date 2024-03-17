# Installation

#### Trivy operator
- Add trivy helm repo
```
$ helm repo add aqua https://aquasecurity.github.io/helm-charts/
$ helm repo update
```
- Install the trivy operator by setting operator.scannerReportTTL to nil
```
$ helm install trivy-operator aqua/trivy-operator \
  --namespace trivy-system \
  --create-namespace \
  --set="operator.scannerReportTTL=""" \
  --version 0.20.6
```

#### Hossted operator 
- Install hossted operator using helm charts.

```
$ helm upgrade --install operator . -n operator --set env.HOSSTED_API_URL="<>",env.HOSSTED_AUTH_TOKEN="<>",env.EMAIL_ID="<>" 
```
