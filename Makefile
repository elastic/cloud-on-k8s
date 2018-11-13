SHELL := /bin/bash
INSTALL_HELP = "please refer to the README.md for how to install it."
GO := $(shell command -v go)
GOIMPORTS := $(shell command -v goimports)
MINIKUBE := $(shell command -v minikube)
KUBECTL := $(shell command -v kubectl)
KUBEBUILDER := $(shell command -v kubebuilder)
DEP := $(shell command -v dep)
GCLOUD := $(shell command -v gcloud)

KUBECTL_CONFIG ?= $(shell cat $(CONFIG_FILE) 2> /dev/null)
CONFIG_FILE ?= .devenv
MINIKUBE_KUBERNETES_VERSION ?= v1.12.0
MINIKUBE_MEMORY ?= 8192

GCLOUD_PROJECT ?= elastic-cloud-dev
GCLOUD_CLUSTER_NAME ?= $(subst _,,$(USER))-dev-cluster

GKE_CLUSTER_REGION ?= europe-west3
GKE_ADMIN_USERNAME ?= admin
GKE_CLUSTER_VERSION = $(shell gcloud container get-server-config --region europe-west3 --format='value(validMasterVersions[0])' 2>/dev/null)
GKE_MACHINE_TYPE ?= n1-highmem-4
GKE_LOCAL_SSD_COUNT ?= 1
GKE_NODE_COUNT_PER_ZONE ?= 1
GKE_KUBECTL_CONFIG = gke_$(GCLOUD_PROJECT)_$(GKE_CLUSTER_REGION)_$(GCLOUD_CLUSTER_NAME)

GCR_REGION_NAME ?= eu.gcr.io
# Image URL to use all building/pushing image targets
IMG_TAG ?= $(shell find pkg -type f -print0 | xargs -0 sha1sum | sha1sum | awk '{print $$1}')
IMG ?= $(GCR_REGION_NAME)/$(GCLOUD_PROJECT)/$(subst _,,$(USER))

OPERATOR_NAMESPACE ?= stack-operators-system

### Start of Autogenerated ###

all: unit integration manager

# Run tests
unit:
	go test ./pkg/... ./cmd/... -coverprofile cover.out

integration: generate fmt vet manifests
	go test -tags=integration ./pkg/... ./cmd/... -coverprofile cover.out

# Build manager binary
manager: generate fmt vet
	go build -o bin/manager github.com/elastic/stack-operators/cmd/manager

# Run against the configured Kubernetes cluster in ~/.kube/config
run: generate fmt vet
ifeq ($(KUBECTL_CONFIG),$(GKE_KUBECTL_CONFIG))
	@ gcloud auth configure-docker
	@ $(MAKE) docker-build docker-push deploy
	@ echo "-> type \"make logs\" to tail the controller's logs."
else
	USE_MINIKUBE=true go run ./cmd/manager/main.go
endif

.PHONY: logs
logs:
	@ kubectl --namespace=$(OPERATOR_NAMESPACE) logs -f statefulset.apps/stack-operators-controller-manager

# Install CRDs into a cluster
install: manifests
	kubectl --cluster=$(KUBECTL_CONFIG) apply -f config/crds

# Deploy controller in the configured Kubernetes cluster in ~/.kube/config
deploy: manifests
	kubectl --cluster=$(KUBECTL_CONFIG) apply -f config/crds
	kustomize build config/default | kubectl --cluster=$(KUBECTL_CONFIG) apply -f -

# Generate manifests e.g. CRD, RBAC etc.
manifests:
	go run vendor/sigs.k8s.io/controller-tools/cmd/controller-gen/main.go all

# Run go fmt against code
fmt:
	goimports -w pkg cmd

# Run go vet against code
vet:
	go vet ./pkg/... ./cmd/...

# Generate code
generate:
	go generate ./pkg/... ./cmd/...

# Build the docker image
docker-build: unit
	docker build . -t $(IMG):$(IMG_TAG)
	@echo "updating kustomize image patch file for manager resource"
	@ cp config/default/manager_image_patch.orig.yaml config/default/manager_image_patch.yaml
	sed -i '' 's@image: .*@image: '"$(IMG):$(IMG_TAG)"'@' config/default/manager_image_patch.yaml

# Push the docker image
docker-push:
	docker push $(IMG):$(IMG_TAG)


### End of Autogenerated ###

.PHONY: requisites
requisites:
ifndef GO
	@ echo "-> go binary missing, $(INSTALL_HELP)"
	@ exit 1
endif
ifndef GOIMPORTS
	@ echo "-> goimports binary missing, $(INSTALL_HELP)"
	@ exit 1
endif
ifeq ($(KUBECTL_CONFIG),$(GKE_KUBECTL_CONFIG))
ifndef GCLOUD
	@ echo "-> gcloud binary missing, $(INSTALL_HELP)"
	@ exit 6
endif
else
ifndef MINIKUBE
	@ echo "-> minikube binary missing, $(INSTALL_HELP)"
	@ exit 2
endif
endif
ifndef KUBECTL
	@ echo "-> kubectl binary missing, $(INSTALL_HELP)"
	@ exit 3
endif
ifndef KUBEBUILDER
	@ echo "-> kubebuilder binary missing, $(INSTALL_HELP)"
	@ exit 4
endif

# dev
.PHONY: dev
dev: dev-cluster vendor unit manager install samples
	@ echo "-> Development environment started"
ifeq ($(KUBECTL_CONFIG),$(GKE_KUBECTL_CONFIG))
	@ echo "-> Run \"make run\" to build, push and deploy the controller in a docker image."
else
	@ echo "-> Run \"make run\" to start the manager process localy"
endif

.PHONY: dev-cluster
dev-cluster:
ifeq ($(KUBECTL_CONFIG),$(GKE_KUBECTL_CONFIG))
	@ $(MAKE) gke
else
	@ $(MAKE) minikube
endif

.PHONY: delete-dev
delete-dev: purge-env
ifeq ($(KUBECTL_CONFIG),$(GKE_KUBECTL_CONFIG))
	@ echo "-> Deleting GKE cluster..."
	@ gcloud beta --project $(GCLOUD_PROJECT) container clusters delete $(GCLOUD_CLUSTER_NAME) --region $(GKE_CLUSTER_REGION)
else
	@ echo "-> Deleting minikube cluster..."
	@ minikube stop && minikube delete
endif

.PHONY: gke
gke:
	@ echo "-> Checking GKE status..."
	@./hack/gcp-k8s-cluster.sh $(GCLOUD_PROJECT) $(GCLOUD_CLUSTER_NAME) $(GKE_CLUSTER_REGION) $(GKE_ADMIN_USERNAME) \
	$(GKE_CLUSTER_VERSION) $(GKE_MACHINE_TYPE) $(GKE_LOCAL_SSD_COUNT) $(GKE_NODE_COUNT_PER_ZONE)
	@ echo "$(GKE_KUBECTL_CONFIG)" > $(CONFIG_FILE)
	@ gcloud beta --project $(GCLOUD_PROJECT) container clusters get-credentials $(GCLOUD_CLUSTER_NAME) --region $(GKE_CLUSTER_REGION)

# minikube ensures that there's a local minikube environment running
.PHONY: minikube
minikube: requisites
ifneq ($(shell minikube status --format '{{.MinikubeStatus}}'),Running)
	@ echo "-> Starting minikube..."
	@ minikube start --kubernetes-version $(MINIKUBE_KUBERNETES_VERSION) --memory ${MINIKUBE_MEMORY}
else
	@ echo "-> minikube already started, skipping..."
endif
	@ echo "minikube" > $(CONFIG_FILE)

# samples pushes the samples to the configured Kubernetes cluster.
.PHONY: samples
samples: requisites generate
	@ echo "-> Pushing samples to Kubernetes cluster..."
	@ kubectl --cluster=$(KUBECTL_CONFIG) apply -f config/samples

.PHONY: vendor
vendor:
ifndef DEP
	@ echo "-> dep binary missing, $(INSTALL_HELP)"
	@ exit 5
endif
	@ echo "-> Running dep..."
	@ dep ensure

.PHONY: set-dev-gke
set-dev-gke:
ifdef MINIKUBE
ifeq ($(shell minikube status --format '{{.MinikubeStatus}}'),Running)
	minikube stop
endif
endif
	@ echo $(GKE_KUBECTL_CONFIG) > $(CONFIG_FILE)

.PHONY: set-dev-minikube
set-dev-minikube:
	@ echo "minikube" > $(CONFIG_FILE)

.PHONY: purge-env
purge-env:
	@ echo "-> Purging cluster $(KUBECTL_CONFIG)..."
	@ echo "-> Purging $(OPERATOR_NAMESPACE) namespace..."
	@ kubectl --cluster=$(KUBECTL_CONFIG) --namespace=$(OPERATOR_NAMESPACE) delete statefulsets --all
	@ kubectl --cluster=$(KUBECTL_CONFIG) --namespace=$(OPERATOR_NAMESPACE) delete po --all
	@ kubectl --cluster=$(KUBECTL_CONFIG) --namespace=$(OPERATOR_NAMESPACE) delete svc --all
	@ echo "-> Purging default namespace..."
	@ kubectl --cluster=$(KUBECTL_CONFIG) delete deployments --all
	@ kubectl --cluster=$(KUBECTL_CONFIG) delete svc --all
	@ kubectl --cluster=$(KUBECTL_CONFIG) delete rc --all
	@ kubectl --cluster=$(KUBECTL_CONFIG) delete po --all

.PHONY: purge-gcr-images
purge-gcr-images:
	@ for i in $(gcloud container images list-tags $(IMG) | tail +3 | awk '{print $$2}'); \
		do gcloud container images untag $(IMG):$$i; \
	done

.PHONY: show-credentials
show-credentials:
	@ echo "elastic:$$(kubectl get secret stack-sample-elastic-user -o json | jq -r '.data.elastic' | base64 -D)"
