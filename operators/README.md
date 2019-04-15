# Elastic operators and controllers for Kubernetes

Managed Elastic products and services in Kubernetes.

## Requirements

* [go](https://golang.org/dl/)
* [goimports](https://godoc.org/golang.org/x/tools/cmd/goimports)
* [dep](https://github.com/golang/dep)
* [golangci-lint](https://github.com/golangci/golangci-lint)
* [kustomize](https://github.com/kubernetes-sigs/kustomize)
* [kubectl](https://kubernetes.io/docs/tasks/tools/install-kubectl/) (>= 1.11)
* [kubebuilder](https://github.com/kubernetes-sigs/kubebuilder)
* [minikube](https://kubernetes.io/docs/tasks/tools/install-minikube/)
* [docker](https://docs.docker.com/)
* [gcloud](https://cloud.google.com/sdk/gcloud/) (Install `beta` components)
* sha1sum (for Mac `brew install md5sha1sum`)

Run `make check-requisites` to check that all dependencies are installed.

## Development

To start, get a working development Kubernetes cluster using [Minikube](https://kubernetes.io/docs/tasks/tools/install-minikube/#install-minikube) or [GKE](https://cloud.google.com/kubernetes-engine/):

```bash
 make bootstrap-minikube
 # Sets up a Minikube cluster with required resources
 ```
 or
 ```bash
 GCLOUD_PROJECT=my-project-id make bootstrap-gke
 # Sets up GKE cluster with required resources		
 ```

Then, proceed as follows:

* `make dep-vendor-only`: Downloads extra Go libraries needed to compile the project and stores them in the vendor directory.
* `make run`: Run the operator locally.
* `make deploy`: Deploy the operators into the configured k8s cluster.
* `make samples`: Apply a sample stack resource.

### Running E2E tests

E2E tests will run in the `e2e` namespace. An operator needs to be running and managing resources in the `e2e` namespace.
To do that run `MANAGED_NAMESPACE=e2e make run`. After that you can run e2e tests in a separate shell `make e2e-local`.

## Recommended reading

* [Resources](https://book.kubebuilder.io/basics/what_is_a_resource.html)
* [Controllers](https://book.kubebuilder.io/basics/what_is_a_controller.html)
* [Controller Managers](https://book.kubebuilder.io/basics/what_is_the_controller_manager.html)
