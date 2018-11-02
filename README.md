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

### Makefile variables

* `KUBECTL_CONFIG`: Sets up the config on which to work (defaults to `minikube`.
* `MINIKUBE_KUBERNETES_VERSION`: Configures the Kubernetes version that `minikube` will use for the VM.

## Recommended reading

* [Resources](https://book.kubebuilder.io/basics/what_is_a_resource.html)
* [Controllers](https://book.kubebuilder.io/basics/what_is_a_controller.html)
* [Controller Managers](https://book.kubebuilder.io/basics/what_is_the_controller_manager.html)
