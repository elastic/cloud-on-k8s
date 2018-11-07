# Stack Operators

This repository contains logic responsible to manage Elastic Stack resources
within a Kubernetes cluster.

## Requirements

* [go](https://golang.org/dl/)
* [goimports](https://godoc.org/golang.org/x/tools/cmd/goimports)
* [dep](https://github.com/golang/dep)
* [kubectl](https://kubernetes.io/docs/tasks/tools/install-kubectl/)
* [kubebuilder](https://github.com/kubernetes-sigs/kubebuilder)
* [minikube](https://kubernetes.io/docs/tasks/tools/install-minikube/)
* [gcloud](https://cloud.google.com/sdk/gcloud/)

## Development

After installing the [requirements](#requirements), you can jump straing to development with `make dev`:

```console
$ make dev
-> Starting minikube...
Starting local Kubernetes v1.12.0 cluster...
Starting VM...
Getting VM IP address...
Moving files into cluster...
Setting up certs...
Connecting to cluster...
Setting up kubeconfig...
Starting cluster components...
Kubectl is now configured to use the cluster.
Loading cached images from config file.
-> Running dep...
go generate ./pkg/... ./cmd/...
go fmt ./pkg/... ./cmd/...
go vet ./pkg/... ./cmd/...
go run vendor/sigs.k8s.io/controller-tools/cmd/controller-gen/main.go all
CRD manifests generated under '/Users/marc/go/src/github.com/elastic/stack-operators/config/crds'
RBAC manifests generated under '/Users/marc/go/src/github.com/elastic/stack-operators/config/rbac'
go test ./pkg/... ./cmd/... -coverprofile cover.out
?   	github.com/elastic/stack-operators/pkg/apis	[no test files]
?   	github.com/elastic/stack-operators/pkg/apis/deployments	[no test files]
ok  	github.com/elastic/stack-operators/pkg/apis/deployments/v1alpha1	9.841s	coverage: 20.0% of statements
?   	github.com/elastic/stack-operators/pkg/controller	[no test files]
ok  	github.com/elastic/stack-operators/pkg/controller/stack	11.236s	coverage: 67.6% of statements
?   	github.com/elastic/stack-operators/pkg/webhook	[no test files]
?   	github.com/elastic/stack-operators/cmd/manager	[no test files]
go build -o bin/manager github.com/elastic/stack-operators/cmd/manager
kubectl apply -f config/crds
customresourcedefinition.apiextensions.k8s.io/stacks.deployments.k8s.elastic.co configured
stack.deployments.k8s.elastic.co/stack-sample unchanged
-> Development environment started
-> Run "make run" to start the manager process localy
```

### Useful development targets

* `make samples`: Updates the samples.
* `make minikube`: Ensures that minikube is started.
* `make vendor`: Runs `dep ensure`
* `make set-dev-gke`: Sets the development environment to target GKE.
* `make set-dev-minikube`: Sets the development environment to run in `minikube`.

### Makefile variables

* `KUBECTL_CONFIG`: Sets up the config on which to work (defaults to `minikube`.
* `MINIKUBE_KUBERNETES_VERSION`: Configures the Kubernetes version that `minikube` will use for the VM.
* `GCLOUD_PROJECT`: Sets the gcloud project to run aginst (defaults to `elastic-cloud-dev`).
* `GCLOUD_CLUSTER_NAME`: Sets the GKE cluster name to be created (defaults to `${USER}-dev-cluster`).
* `GKE_CLUSTER_REGION`: Sets the gcloud region to run aginst (defaults to `europe-west3`).
* `GKE_ADMIN_USERNAME`: Sets the GKE cluster administrative user (defaults to `admin`).
* `GKE_CLUSTER_VERSION`: Sets the GKE kubernetes version to use (defaults to `1.11.2-gke.15`).
* `GKE_MACHINE_TYPE`: Sets the GCP instance types to use (defaults to `n1-highmem-4`).
* `GKE_LOCAL_SSD_COUNT`: Sets the number of locally attached SSD disks attached to each GCP instance (defaults to `1`). Each disk is 375GB.
* `GKE_NODE_COUNT_PER_ZONE`: Sets the amount of nodes per GCP zone (defaults to `1`). By default the GKE cluster is spun accross 3 zones.

## Recommended reading

* [Resources](https://book.kubebuilder.io/basics/what_is_a_resource.html)
* [Controllers](https://book.kubebuilder.io/basics/what_is_a_controller.html)
* [Controller Managers](https://book.kubebuilder.io/basics/what_is_the_controller_manager.html)
