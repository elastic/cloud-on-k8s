# Elastic operators and controllers for Kubernetes

Managed Elastic products and services in Kubernetes.

## Requirements

* [go](https://golang.org/dl/)
* [goimports](https://godoc.org/golang.org/x/tools/cmd/goimports)
* [dep](https://github.com/golang/dep)
* [kustomize](https://github.com/kubernetes-sigs/kustomize)
* [kubectl](https://kubernetes.io/docs/tasks/tools/install-kubectl/)
* [kubebuilder](https://github.com/kubernetes-sigs/kubebuilder)
* [minikube](https://kubernetes.io/docs/tasks/tools/install-minikube/)
* [gcloud](https://cloud.google.com/sdk/gcloud/)

## Development

After installing the [requirements](#requirements), you can jump straing to development with `make dev`.
By default it will use `minikube` as the environment to develop against, if you wish to use a GKE cluster use
`make set-dev-gke`. After, you can run `make dev` and the creation of the environment will be taken care of.

### Minikube

```console
$ make bootstrap-minikube
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
CRD manifests generated under '.../go/src/github.com/elastic/k8s-operators/operators/config/crds'
RBAC manifests generated under '.../go/src/github.com/elastic/k8s-operators/operators/config/rbac'
go test ./pkg/... ./cmd/... -coverprofile cover.out
?   	github.com/elastic/k8s-operators/operators/pkg/apis	[no test files]
?   	github.com/elastic/k8s-operators/operators/pkg/apis/deployments	[no test files]
ok  	github.com/elastic/k8s-operators/operators/pkg/apis/deployments/v1alpha1	9.841s	coverage: 20.0% of statements
?   	github.com/elastic/k8s-operators/operators/pkg/controller	[no test files]
ok  	github.com/elastic/k8s-operators/operators/pkg/controller/stack	11.236s	coverage: 67.6% of statements
?   	github.com/elastic/k8s-operators/operators/pkg/webhook	[no test files]
?   	github.com/elastic/k8s-operators/operators/cmd/manager	[no test files]
go build -o bin/manager github.com/elastic/k8s-operators/operators/cmd/manager
kubectl apply -f config/crds
customresourcedefinition.apiextensions.k8s.io/stacks.deployments.k8s.elastic.co configured
stack.deployments.k8s.elastic.co/stack-sample unchanged
-> Development environment started
-> Run "make run" to start the manager process localy
```

After that you can run `make start-port-forward` to start forwarding any services you want to interact with from your local controller. This will also create some entries in your `/etc/hosts` file.

`make stop-port-forward` will end the forwarding. 

### GKE Development workflow

When selecting GKE as your development environment note that the controller will be packed in a docker image,
tagged using a hash of the pkg/ directory as a tag and uploaded to a GCP docker image repository. After your GKE
cluster has been created you can build the docker image and deploy the controller by typing `make run`.

After you've performed changes in the controller code you can re-deploy the image by running `make run` again.

### Useful development targets

* `make samples`: Updates the samples.
* `make bootstrap-minikube`: Sets up a Minikube cluster with required resources.
* `make bootstrap-gke`: Sets up a Minikube cluster with required resources.
* `make run`: Run the operator locally.
* `make deploy`: Deploy the operator into the configured k8s cluster.

### Using snapshot repositories

* Restrictions:
    * Currently only gcs is supported
    * We currently update the keystore only on pod initialisation, so adding or removing of repositories requires pod deletion/recreation at the moment until we have a sidecar to do this
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
