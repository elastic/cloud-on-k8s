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
* [gcloud](https://cloud.google.com/sdk/gcloud/) (Install `beta` components)
* sha1sum (For Mac `brew install md5sha1sum`)

## Quickstart

See the [quickstart tutorial](docs/quickstart.md).

## Development

Run `make check-requisites` to check that all dependencies are installed.    
After installing the [requirements](#requirements), you can jump straight to development with `make bootstrap-gke` or `make bootstrap-minikube` to setup a development kubernetes cluster.    
Then, use either `make run` to run the operator locally, or `make deploy` to deploy the operators on the cluster.

### Running E2E tests   

E2E tests will run in the `e2e` namespace. An operator needs to be running and managing resources in the `e2e` namespace.   
To do that run `MANAGED_NAMESPACE=e2e make run`. After that you can run e2e tests in a separate shell `make e2e-local`.

### Useful development targets

* `make bootstrap-minikube`: Sets up a Minikube cluster with required resources.
* `make bootstrap-gke`: Sets up a Minikube cluster with required resources.
* `make run`: Run the operator locally.
* `make deploy`: Deploy the operators into the configured k8s cluster.
* `make samples`: Apply a sample stack resource.

### Using snapshot repositories

* Restrictions:
    * Currently only gcs is supported
* Either create a new bucket/service account or reuse our dev bucket (see Keybase)
* Create a secret with your [service account bucket credentials](https://www.elastic.co/guide/en/elasticsearch/plugins/master/repository-gcs-usage.html#repository-gcs-using-service-account)

     `kubectl create secret generic gcs-repo-account --from-file service-account.json`

* Specify in your stack resource that you want to use a repository like so:

    ```
     snapshotRepository:
      type: "gcs"
      settings:
        bucketName: "stack-sample-snapshot-repo"
        credentials:
          namespace: "default"
          name: "gcs-repo-account"
    ```
   

## Recommended reading

* [Resources](https://book.kubebuilder.io/basics/what_is_a_resource.html)
* [Controllers](https://book.kubebuilder.io/basics/what_is_a_controller.html)
* [Controller Managers](https://book.kubebuilder.io/basics/what_is_the_controller_manager.html)
