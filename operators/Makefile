# Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
# or more contributor license agreements. Licensed under the Elastic License;
# you may not use this file except in compliance with the Elastic License.
export SHELL := /bin/bash

OPERATOR_NAMESPACE ?= elastic-operators-system

GCLOUD_PROJECT ?= elastic-cloud-dev
REGISTRY ?= eu.gcr.io
ifeq ($(KUBECTL_CLUSTER), minikube)
	# on minikube, use a local registry
	REGISTRY = localhost:5000
endif
# suffix image name with current user name
IMG_SUFFIX ?= -$(subst _,,$(USER))
IMG ?= $(REGISTRY)/$(GCLOUD_PROJECT)/k8s-operators$(IMG_SUFFIX)
GIT_SHA1 = $(shell git rev-parse --verify HEAD)
OPERATOR_IMAGE ?= $(IMG):$(GIT_SHA1)
OPERATOR_IMAGE_LATEST ?= $(IMG):latest


#####################################
##  --       Development       --  ##
#####################################

all: dep-vendor-only unit integration check-fmt elastic-operator keystore-updater check-license-header

## -- build

dep:
	dep ensure -v

dep-vendor-only:
	# don't attempt to upgrade Gopkg.lock
	dep ensure --vendor-only -v

# Generate API types code and manifests from annotations e.g. CRD, RBAC etc.
generate:
	go generate ./pkg/... ./cmd/...
	go run vendor/sigs.k8s.io/controller-tools/cmd/controller-gen/main.go all

elastic-operator: generate
	go build -o bin/elastic-operator github.com/elastic/k8s-operators/operators/cmd

keystore-updater: generate
	go build -o bin/keystore-updater github.com/elastic/k8s-operators/operators/cmd/keystore-updater

fmt:
	goimports -w pkg cmd

## -- tests

unit:
	go test ./pkg/... ./cmd/... -coverprofile cover.out

integration: generate
	go test -tags=integration ./pkg/... ./cmd/... -coverprofile cover.out

check-fmt:
ifneq ($(shell goimports -l pkg cmd),)
	$(error Invalid go formatting. Please run `make fmt`)
endif
	go vet ./pkg/... ./cmd/...

lint:
	golangci-lint run $(LINTER_ARGS) ./pkg/... ./cmd/... ./test/...


#############################
##  --       Run       --  ##
#############################

install-crds: generate
	kubectl apply -f config/crds

# Run locally against the configured Kubernetes cluster, with port-forwarding enabled so that
# the operator can reach services running in the cluster through k8s port-forward feature
run: install-crds
	# Run the operator locally, with sidecar image set to latest
	AUTO_PORT_FORWARD=true OPERATOR_IMAGE=$(OPERATOR_IMAGE_LATEST) \
		go run ./cmd/main.go manager --development

# Deploy the operator in the configured Kubernetes cluster
deploy: install-crds docker-build docker-push
	kustomize build config/default | kubectl apply -f -
	@ echo "-> type \"make logs\" to tail the operator logs."

logs:
	@ kubectl --namespace=$(OPERATOR_NAMESPACE) logs -f statefulset.apps/elastic-operators-controller-manager

samples:
	@ echo "-> Pushing samples to Kubernetes cluster..."
	@ kubectl apply -f config/samples/deployments_v1alpha1_stack.yaml

# display elasticsearch credentials for the first stack
show-credentials:
	@ echo "elastic:$$(kubectl get secret stack-sample-elastic-user -o json | jq -r '.data.elastic' | base64 -D)"


##########################################
##  --    K8s clusters bootstrap    --  ##
##########################################

KUBECTL_CLUSTER = $(shell kubectl config current-context)

cluster-bootstrap: dep-vendor-only elastic-operator install-crds

clean-k8s-cluster:
	kubectl delete deployments --all
	kubectl delete statefulsets --all
	kubectl delete svc --all
	kubectl delete rc --all
	kubectl delete po --all

## -- minikube

set-context-minikube:
	kubectl config use-context "minikube"
	$(eval KUBECTL_CLUSTER="minikube")

bootstrap-minikube: 
	hack/minikube-cluster.sh
	$(MAKE) set-context-minikube registry lazy-push-latest-operator-image-in-local-registry cluster-bootstrap


## -- gke

set-context-gke:
	minikube stop || true
	kubectl config use-context $(shell hack/gke-cluster.sh name)
	$(eval KUBECTL_CLUSTER=$(shell hack/gke-cluster.sh name))

bootstrap-gke:
	./hack/gke-cluster.sh create
	$(MAKE) set-context-gke cluster-bootstrap

delete-gke:
	./hack/gke-cluster.sh delete

# -- registry in the cluster

registry:
	@ echo "-> Deploying registry..."
	@ hack/registry.sh create

lazy-push-latest-operator-image-in-local-registry:
	@ hack/registry.sh port-forward start
	@ docker pull $(OPERATOR_IMAGE_LATEST) || $(MAKE) docker-build docker-push OPERATOR_IMAGE=$(OPERATOR_IMAGE_LATEST)
	@ hack/registry.sh port-forward stop

force-push-latest-operator-image-in-local-registry:
	@ hack/registry.sh port-forward start
	@ $(MAKE) docker-build docker-push OPERATOR_IMAGE=$(OPERATOR_IMAGE_LATEST)
	@ hack/registry.sh port-forward stop


#################################
##  --    Docker images    --  ##
#################################

docker-build:
	docker build . -t $(OPERATOR_IMAGE)
	@ echo "-> Updating kustomize image patch file for manager resource"
	# TODO: fix this on merge conflict with PR #342
	@ cp config/default/manager_image_patch.orig.yaml config/default/manager_image_patch.yaml
	@ $(eval EXTRASEDFLAG := "''")
ifeq ($(shell uname -s),Linux)
	@ $(eval EXTRASEDFLAG := "-e")
endif
	sed -i ${EXTRASEDFLAG} 's@image: .*@image: '"$(OPERATOR_IMAGE)"'@' config/default/manager_image_patch.yaml
	sed -i ${EXTRASEDFLAG} 's@value: .*@value: '"$(OPERATOR_IMAGE)"'@' config/default/manager_image_patch.yaml

docker-push:
ifeq ($(KUBECTL_CLUSTER), minikube)
	@ hack/registry.sh port-forward start
	docker push $(OPERATOR_IMAGE)
	@ hack/registry.sh port-forward stop
else
	docker push $(OPERATOR_IMAGE)
endif

purge-gcr-images:
	@ for i in $(gcloud container images list-tags $(IMG) | tail +3 | awk '{print $$2}'); \
		do gcloud container images untag $(IMG):$$i; \
	done


###################################
##  --   End to end tests    --  ##
###################################

# can be overriden to eg. TESTS_MATCH=TestMutationMoreNodes to match a single test
TESTS_MATCH ?= ""

# Run e2e tests as a k8s batch job
E2E_IMG ?= $(IMG)-e2e-tests:$(GIT_SHA1)
e2e:
	docker build -t $(E2E_IMG) -f test/e2e/Dockerfile .
	docker push $(E2E_IMG)
	./hack/run-e2e.sh "$(E2E_IMG)" "$(TESTS_MATCH)"

# Run e2e tests locally (not as a k8s job), with a custom http dialer
# that can reach ES services running in the k8s cluster through port-forwarding.
e2e-local:
	kubectl apply -f config/e2e/rbac.yaml
	go test -v -failfast -timeout 1h -tags=e2e ./test/e2e -run "$(TESTS_MATCH)"  --auto-port-forward

# Clean k8s cluster from e2e resources
clean-e2e:
	kubectl delete namespace e2e


##########################################
##  --    Continuous integration    --  ##
##########################################

ci: check-fmt dep-vendor-only unit integration docker-build

# Run e2e tests in a dedicated gke cluster
ci-e2e: bootstrap-gke docker-build docker-push deploy e2e delete-gke


##########################
##  --   Helpers    --  ##
##########################

check-requisites:
	@ hack/check-requisites.sh

check-license-header:
	../build/check-license-header
