# Setting up Your Development Environment

This page explains you how to set up your development environment.

## Requirements

Before you start, install the following tools and packages:

* [go](https://golang.org/dl/) (>= 1.11)
* [golangci-lint](https://github.com/golangci/golangci-lint)
* [kubectl](https://kubernetes.io/docs/tasks/tools/install-kubectl/) (>= 1.14)
* [kubebuilder](https://github.com/kubernetes-sigs/kubebuilder)
* [minikube](https://kubernetes.io/docs/tasks/tools/install-minikube/)
* [docker](https://docs.docker.com/)
* sha1sum (for Mac `brew install md5sha1sum`)
* [gcloud](https://cloud.google.com/sdk/gcloud/) 
  * Install beta and docker-credential-gcr components:  `gcloud components install beta docker-credential-gcr`


### Get sources 

```bash
git clone https://github.com/elastic/cloud-on-k8s.git
cd cloud-on-k8s
```

### Check prerequisites

Run `make check-requisites` to check that all dependencies are installed.

## Development

1. Run `make dependencies` to download the Go libraries needed to compile the project.

2. Get a working development Kubernetes cluster. You can use:

    [Minikube](https://kubernetes.io/docs/tasks/tools/install-minikube/#install-minikube)

      ```bash
      make bootstrap-minikube
      # Sets up a Minikube cluster with required resources
      ```

    [GKE](https://cloud.google.com/kubernetes-engine/)

      Make sure that container registry authentication is correctly configured as described [here](https://cloud.google.com/container-registry/docs/advanced-authentication).

      ```bash
      export GCLOUD_PROJECT=my-project-id
      make bootstrap-cloud
      # Sets up GKE cluster (by default) with required resources
      ```

    [Kind](https://kind.sigs.k8s.io/)

      ```bash
      make bootstrap-kind
      # Sets up a kind cluster with required resources
      ```

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
