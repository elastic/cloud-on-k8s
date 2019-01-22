# Stack Operator

Manage an Elastic stack in Kubernetes.

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
CRD manifests generated under '/Users/marc/go/src/github.com/elastic/stack-operators/stack-operator/config/crds'
RBAC manifests generated under '/Users/marc/go/src/github.com/elastic/stack-operators/stack-operator/config/rbac'
go test ./pkg/... ./cmd/... -coverprofile cover.out
?   	github.com/elastic/stack-operators/stack-operator/pkg/apis	[no test files]
?   	github.com/elastic/stack-operators/stack-operator/pkg/apis/deployments	[no test files]
ok  	github.com/elastic/stack-operators/stack-operator/pkg/apis/deployments/v1alpha1	9.841s	coverage: 20.0% of statements
?   	github.com/elastic/stack-operators/stack-operator/pkg/controller	[no test files]
ok  	github.com/elastic/stack-operators/stack-operator/pkg/controller/stack	11.236s	coverage: 67.6% of statements
?   	github.com/elastic/stack-operators/stack-operator/pkg/webhook	[no test files]
?   	github.com/elastic/stack-operators/stack-operator/cmd/manager	[no test files]
go build -o bin/manager github.com/elastic/stack-operators/stack-operator/cmd/manager
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
* `make minikube`: Ensures that minikube is started.
* `make vendor`: Runs `dep ensure`
* `make set-dev-gke`: Sets the development environment to target GKE.
* `make set-dev-minikube`: Sets the development environment to run in `minikube`.
* `make dev-cluster`: Starts / Ensures that the development resources are created and started, to select which environment you want to use, run one of the two targets above. Defaults to `minikube`.
* `make delete-dev`: Deletes the currently set development resources.
* `make purge-env`: Deletes all the resources from the configured Kubernetes cluster on the `default` namespace.
* `make run`: Builds, pushes and deploys the controller's docker image to the GKE cluster or runs the manager locally against the local Minikube environment.



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
