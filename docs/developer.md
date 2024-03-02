## Developer Documentation

- Go version ```1.21```

### Prerequisites
To follow this tutorial you will need:

- The [KIND CLI](https://kind.sigs.k8s.io/) installed.
- The KUBECTL CLI installed.
- Docker up and Running.

### Install Kind Cluster
Create kind cluster on your machine.

```kind create cluster --name druid```

### Run operator locally

- Export the following env var's
```
export API_URL=""
export AUTH_TOKEN=""
```

- Install CRD's on K8s Cluster.

```
kubectl apply -f config/crd/bases
```

- Reconcile Locally
```
make run
```

### Deploy a sample CR

- Install a sample CR

```
kubectl apply -f config/samples/
```

### Status of CR

- View Status of CR

```
kubectl get hosstedproject -o yaml
```
