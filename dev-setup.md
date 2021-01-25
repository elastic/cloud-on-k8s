# Setting up Your Development Environment

This page explains you how to set up your development environment.

## Requirements

Before you start, install the following tools and packages:

* [go](https://golang.org/dl/) (>= 1.13)
* [golangci-lint](https://github.com/golangci/golangci-lint)
* [kubectl](https://kubernetes.io/docs/tasks/tools/install-kubectl/) (>= 1.14)
* [kubebuilder](https://github.com/kubernetes-sigs/kubebuilder) (>= 2.0.0)
* [docker](https://docs.docker.com/) (>= 19.0.0 with optional `buildx` extension for multi-arch builds)
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
2. Get a working development Kubernetes cluster. You can use:

* [Minikube](https://kubernetes.io/docs/tasks/tools/install-minikube/#install-minikube)

  ```bash
  make bootstrap-minikube
  ```

* [Kind](https://kind.sigs.k8s.io/)

  The Make target assumes that your local kind binaries are suffixed with their version. For example `kind-0.9.0` and `kind-0.8.1`. The purpose of this convention is to allow running both versions of kind next to each other. For older Kubernetes versions up to 1.13 use `kind-0.8.1` and for the newer versions of Kubernetes starting with 1.14 use `kind-0.9.0`.

  ```bash
  make bootstrap-kind
  ```

* Cloud providers

  Use [deployer](/hack/deployer/README.md) (note that some [one time configuration](/hack/deployer/README.md#typical-usage) is required):

  * [GKE](https://cloud.google.com/kubernetes-engine/): `make switch-gke bootstrap-cloud`
  * [AKS](https://azure.microsoft.com/en-us/services/kubernetes-service/): `make switch-aks bootstrap-cloud`

3. Docker registry

The `docker.elastic.co` registry and the `eck-dev` namespace are setup by default.

It is up to you to manage the authentication (`docker login -u $username docker.elastic.co`) to be able to push images into it.

A file `.registry.env` can be created to use another Docker registry.

`make switch-registry-gcr` configures this file to use Google Container Registry with:

```sh
REGISTRY = eu.gcr.io
REGISTRY_NAMESPACE = my-gcloud-project
E2E_REGISTRY_NAMESPACE = my-gcloud-project
```

4. Deploy the operator

* `make run` to run the operator locally, or `make deploy` to deploy the operators into the configured k8s cluster.
* `make samples` to apply a sample stack resource.

### Running unit and integration tests

```
make unit integration
```

### Running E2E tests

E2E tests will run in the `e2e-mercury` and `e2e-venus` namespaces.

Run `make run` to start the operator and then run `make e2e-local` in a separate shell to run the E2E tests.

### Enabling APM tracing

ECK is instrumented with Elastic APM tracing. To run ECK locally with tracing enabled, run:

```
ENABLE_TRACING=true ELASTIC_APM_SERVER_URL=https://<apm-server-url> ELASTIC_APM_SECRET_TOKEN=<token> ELASTIC_APM_VERIFY_SERVER_CERT=false make run
```

### Development mode

Starting the operator with the `--development` flag enables the development mode. The following set of flags become available for use in this mode.

| Flag | Description |
| ---- | ----------- |
| `auto-port-forward` | Allows the operator to be run locally (outside of a Kubernetes cluster) by port-forwarding to the remote cluster. |
| `debug-http-listen` | Address to start the debug server which provides access to pprof endpoints. Default is `localhost:6060`. |

## Recommended reading

* [Resources](https://book.kubebuilder.io/basics/what_is_a_resource.html)
* [Controllers](https://book.kubebuilder.io/basics/what_is_a_controller.html)
* [Controller Managers](https://book.kubebuilder.io/basics/what_is_the_controller_manager.html)
