# Setting up Your Development Environment

This page explains you how to set up your development environment.

## Requirements

Before you start, install the following tools and packages:

* [go](https://golang.org/dl/)
* [goimports](https://godoc.org/golang.org/x/tools/cmd/goimports)
* [dep](https://github.com/golang/dep)
* [golangci-lint](https://github.com/golangci/golangci-lint)
* [kubectl](https://kubernetes.io/docs/tasks/tools/install-kubectl/) (>= 1.11)
* [kubebuilder](https://github.com/kubernetes-sigs/kubebuilder)
* [minikube](https://kubernetes.io/docs/tasks/tools/install-minikube/)
* [docker](https://docs.docker.com/)
* [gcloud](https://cloud.google.com/sdk/gcloud/) (Install `beta` components)
* sha1sum (for Mac `brew install md5sha1sum`)

### Get sources using go get

```bash
go get -u github.com/elastic/cloud-on-k8s
cd ${GOPATH:-$HOME/go}/src/github.com/elastic/cloud-on-k8s
```

### Check prerequisites

Run `make check-requisites` to check that all dependencies are installed.

## Development

1. Run `make dep-vendor-only` to download extra Go libraries needed to compile the project and store them in the vendor directory.

2. Get a working development Kubernetes cluster. You can either use:

    [Minikube](https://kubernetes.io/docs/tasks/tools/install-minikube/#install-minikube)

    ```bash
      make bootstrap-minikube
      # Sets up a Minikube cluster with required resources
      ```

      or [GKE](https://cloud.google.com/kubernetes-engine/)

      Make sure that container registry authentication is correctly configured as described [here](https://cloud.google.com/container-registry/docs/advanced-authentication).

      ```bash
      export GCLOUD_PROJECT=my-project-id
      make bootstrap-gke
      # Sets up GKE cluster with required resources
      ```

3. Deploy the operator.

   * `make run` to run the operator locally, or `make deploy` to deploy the operators into the configured k8s cluster.
   * `make samples` to apply a sample stack resource.

### Running E2E tests

E2E tests will run in the `e2e` namespace. An operator must run and manage resources in the `e2e` namespace.
To do that, run `MANAGED_NAMESPACE=e2e make run`. Then you can run E2E tests in a separate shell `make e2e-local`.

## Recommended reading

* [Resources](https://book.kubebuilder.io/basics/what_is_a_resource.html)
* [Controllers](https://book.kubebuilder.io/basics/what_is_a_controller.html)
* [Controller Managers](https://book.kubebuilder.io/basics/what_is_the_controller_manager.html)
