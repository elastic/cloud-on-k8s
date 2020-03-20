# Setting up Your Development Environment

This page explains you how to set up your development environment.

## Requirements

Before you start, install the following tools and packages:

* [go](https://golang.org/dl/) (>= 1.13)
* [golangci-lint](https://github.com/golangci/golangci-lint)
* [kubectl](https://kubernetes.io/docs/tasks/tools/install-kubectl/) (>= 1.14)
* [kubebuilder](https://github.com/kubernetes-sigs/kubebuilder) (>= 2.0.0)
* [docker](https://docs.docker.com/)
* Kubernetes distribution such as [minikube](https://kubernetes.io/docs/tasks/tools/install-minikube/) or [kind](https://kind.sigs.k8s.io), or access to a hosted Kubernetes service such as [GKE](https://cloud.google.com/kubernetes-engine) or [AKS](https://azure.microsoft.com/en-us/services/kubernetes-service/)

### Get sources

```bash
git clone https://github.com/elastic/cloud-on-k8s.git
cd cloud-on-k8s
```

### Check prerequisites

Run `make check-requisites` to check that all dependencies are installed.

## Development

1. Run `make dependencies` to download the Go libraries needed to compile the project.
1. Get a working development Kubernetes cluster. You can use:

* [Minikube](https://kubernetes.io/docs/tasks/tools/install-minikube/#install-minikube)

  ```bash
  make bootstrap-minikube
  ```

* [Kind](https://kind.sigs.k8s.io/)

  ```bash
  make bootstrap-kind
  ```

* Cloud providers

  Use [deployer](/hack/deployer/README.md) (note that some [one time configuration](/hack/deployer/README.md#typical-usage) is required):

  * [GKE](https://cloud.google.com/kubernetes-engine/): `make switch-gke bootstrap-cloud`
  * [AKS](https://azure.microsoft.com/en-us/services/kubernetes-service/): `make switch-aks bootstrap-cloud`

3. Deploy the operator.

* `make run` to run the operator locally, or `make deploy` to deploy the operators into the configured k8s cluster.
* `make samples` to apply a sample stack resource.

### Running E2E tests

E2E tests will run in the `e2e-mercury` and `e2e-venus` namespaces.
Run `make run` to start the operator and then run `make e2e-local` in a separate shell to run the tests.

## Recommended reading

* [Resources](https://book.kubebuilder.io/basics/what_is_a_resource.html)
* [Controllers](https://book.kubebuilder.io/basics/what_is_a_controller.html)
* [Controller Managers](https://book.kubebuilder.io/basics/what_is_the_controller_manager.html)
