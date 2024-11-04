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

#### Installation


##### Hossted operator 

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
