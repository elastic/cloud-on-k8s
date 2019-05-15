# Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
# or more contributor license agreements. Licensed under the Elastic License;
# you may not use this file except in compliance with the Elastic License.

#################################
##  --      Variables      --  ##
#################################

ROOT_DIR = $(shell dirname $(CURDIR))

# make sure sub-commands don't use eg. fish shell
export SHELL := /bin/bash

KUBECTL_CLUSTER := $(shell kubectl config current-context 2> /dev/null)

## -- Docker image

REPOSITORY ?= elastic-dev
# on GKE, use GCR and GCLOUD_PROJECT
ifneq ($(findstring gke_,$(KUBECTL_CLUSTER)),)
	REGISTRY ?= eu.gcr.io
	REPOSITORY = ${GCLOUD_PROJECT}
endif
# default to local registry
ifeq ($(REGISTRY),)
	REGISTRY ?= localhost:5000
endif
# suffix image name with current user name
IMG_SUFFIX ?= -$(subst _,,$(USER))
IMG_NAME ?= eck-operators
IMG ?= $(REGISTRY)/$(REPOSITORY)/$(IMG_NAME)$(IMG_SUFFIX)
# use git sha1 as the default image tag
TAG ?= $(shell git rev-parse --verify HEAD)
OPERATOR_IMAGE ?= $(IMG):$(TAG)
OPERATOR_IMAGE_LATEST ?= $(IMG):latest
LATEST_RELEASED_IMG ?= "docker.elastic.co/eck/eck-operator:0.8.0"

## -- Namespaces

# namespace in which the global operator is deployed (see config/global-operator)
GLOBAL_OPERATOR_NAMESPACE ?= elastic-system
# namespace in which the namespace operator is deployed (see config/namespace-operator)
NAMESPACE_OPERATOR_NAMESPACE ?= elastic-namespace-operators
# namespace in which the namespace operator should watch resources
MANAGED_NAMESPACE ?= default


#####################################
##  --       Development       --  ##
#####################################

all: dep-vendor-only unit integration e2e-compile check-fmt elastic-operator process-manager cert-initializer check-license-header

## -- build

dep:
	dep ensure -v

dep-vendor-only:
	# don't attempt to upgrade Gopkg.lock
	dep ensure --vendor-only -v

# Generate API types code and manifests from annotations e.g. CRD, RBAC etc.
generate:
	go generate -tags='$(GO_TAGS)' ./pkg/... ./cmd/...
	go run vendor/sigs.k8s.io/controller-tools/cmd/controller-gen/main.go all
	$(MAKE) --no-print-directory generate-all-in-one

elastic-operator: generate
	go build -tags='$(GO_TAGS)' -o bin/elastic-operator github.com/elastic/cloud-on-k8s/operators/cmd

process-manager:
	go build  -tags='$(GO_TAGS)' -o bin/process-manager github.com/elastic/cloud-on-k8s/operators/cmd/process-manager

cert-initializer:
	go build  -tags='$(GO_TAGS)' -o bin/cert-initializer github.com/elastic/cloud-on-k8s/operators/cmd/cert-initializer

fmt:
	goimports -w pkg cmd

clean:
	rm -f pkg/controller/common/license/zz_generated.pubkey.go

## -- tests

unit: clean
	go test ./pkg/... ./cmd/... -coverprofile cover.out -race

integration: GO_TAGS += integration
integration: clean generate
	go test -tags='$(GO_TAGS)' ./pkg/... ./cmd/... -coverprofile cover.out

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
    # Run the operator locally with role All, with operator image set to latest and operator namespace as for a global operator
	AUTO_PORT_FORWARD=true OPERATOR_IMAGE=$(OPERATOR_IMAGE_LATEST) \
		go run -race ./cmd/main.go manager \
			--development --operator-roles=global,namespace \
			--ca-cert-validity=10h --ca-cert-rotate-before=1h \
			--operator-namespace=elastic-system \
			--auto-install-webhooks=false

# if the current k8s cluster is on GKE, GCLOUD_PROJECT must be set
check-gke:
ifneq ($(findstring gke_,$(KUBECTL_CLUSTER)),)
ifndef GCLOUD_PROJECT
	$(error GCLOUD_PROJECT not set while GKE detected)
endif
endif

# Deploy both the global and namespace operators against the current k8s cluster
deploy: check-gke install-crds docker-build docker-push
	OPERATOR_IMAGE=$(OPERATOR_IMAGE) \
	NAMESPACE=$(GLOBAL_OPERATOR_NAMESPACE) \
		$(MAKE) --no-print-directory -sC config/operator generate-global | kubectl apply -f -
	OPERATOR_IMAGE=$(OPERATOR_IMAGE) \
	NAMESPACE=$(NAMESPACE_OPERATOR_NAMESPACE) \
	MANAGED_NAMESPACE=$(MANAGED_NAMESPACE) \
		$(MAKE) --no-print-directory -sC config/operator generate-namespace | kubectl apply -f -

generate-crds:
	for yaml in $$(ls config/crds/*); do \
		cat $$yaml && echo -e "\n---\n" ; \
	done

generate-all-in-one:
	$(MAKE) --no-print-directory -s generate-crds > config/all-in-one.yaml
	# TODO: replace the image name with the final released name (docker.elastic.co/k8s-operators/k8s-operators ?)
	OPERATOR_IMAGE=$(LATEST_RELEASED_IMG) \
	NAMESPACE=$(GLOBAL_OPERATOR_NAMESPACE) \
		$(MAKE) --no-print-directory -sC config/operator generate-all-in-one >> config/all-in-one.yaml

# Deploy an all in one operator against the current k8s cluster
deploy-all-in-one: GO_TAGS ?= release
deploy-all-in-one: docker-build docker-push
	kubectl apply -f config/all-in-one.yaml

logs-namespace-operator:
	@ kubectl --namespace=$(NAMESPACE_OPERATOR_NAMESPACE) logs -f statefulset.apps/elastic-namespace-operator

logs-global-operator:
	@ kubectl --namespace=$(GLOBAL_OPERATOR_NAMESPACE) logs -f statefulset.apps/elastic-global-operator

samples:
	@ echo "-> Pushing samples to Kubernetes cluster..."
	@ kubectl apply -f config/samples/kibana/kibana_es.yaml

# Display elasticsearch credentials of the first stack
show-credentials:
	@ echo "elastic:$$(kubectl get secret elasticsearch-sample-elastic-user -o json | jq -r '.data.elastic' | base64 -D)"


##########################################
##  --    K8s clusters bootstrap    --  ##
##########################################

cluster-bootstrap: dep-vendor-only install-crds

clean-k8s-cluster:
	kubectl delete --ignore-not-found=true  ValidatingWebhookConfiguration validating-webhook-configuration
	for ns in $(NAMESPACE_OPERATOR_NAMESPACE) $(GLOBAL_OPERATOR_NAMESPACE) $(MANAGED_NAMESPACE); do \
		echo "Deleting resources in $$ns"; \
		kubectl delete statefulsets -n $$ns --all; \
		kubectl delete deployments -n $$ns --all; \
		kubectl delete svc -n $$ns --all; \
		kubectl delete rc -n $$ns --all; \
		kubectl delete po -n $$ns --all; \
	done

## -- minikube

set-context-minikube:
	kubectl config use-context "minikube"
	$(eval KUBECTL_CLUSTER="minikube")

bootstrap-minikube:
	hack/minikube-cluster.sh
	$(MAKE) set-context-minikube registry lazy-push-latest-operator-image-in-local-registry cluster-bootstrap

## -- gke

require-gcloud-project:
ifndef GCLOUD_PROJECT
	$(error GCLOUD_PROJECT not set)
endif

set-context-gke: require-gcloud-project
	kubectl config use-context $(shell hack/gke-cluster.sh name)
	$(eval KUBECTL_CLUSTER=$(shell hack/gke-cluster.sh name))

bootstrap-gke: require-gcloud-project
	./hack/gke-cluster.sh create
	$(MAKE) set-context-gke cluster-bootstrap
	# push "latest" operator image to be used for init containers when running the operator locally
	$(MAKE) docker-build docker-push OPERATOR_IMAGE=$(OPERATOR_IMAGE_LATEST)

delete-gke: require-gcloud-project
	./hack/gke-cluster.sh delete

## -- registry in the cluster

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
	docker build . --build-arg GO_TAGS='$(GO_TAGS)' -t $(OPERATOR_IMAGE)

docker-push:
ifeq ($(RELEASE), true)
	@ docker login -u $(ELASTIC_DOCKER_LOGIN) -p $(ELASTIC_DOCKER_PASSWORD) push.docker.elastic.co
endif
ifeq ($(KUBECTL_CLUSTER), minikube)
	# use the minikube registry
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

# Setup for running e2e tests
e2e-setup:
	kubectl apply -f config/e2e/rbac.yaml

# Run e2e tests as a k8s batch job
E2E_IMG ?= $(IMG)-e2e-tests:$(TAG)
e2e: e2e-setup
	# push the e2e tests docker image
	docker build -t $(E2E_IMG) -f test/e2e/Dockerfile .
	docker push $(E2E_IMG)
	# deploy an operator to manage the e2e namespace
	$(MAKE) MANAGED_NAMESPACE=e2e deploy
	./hack/run-e2e.sh "$(E2E_IMG)" "$(TESTS_MATCH)"

# Verify e2e tests compile with no errors, don't run them
e2e-compile:
	go test ./test/e2e -c -o /dev/null

# Run e2e tests locally (not as a k8s job), with a custom http dialer
# that can reach ES services running in the k8s cluster through port-forwarding.
e2e-local: e2e-setup
	go test -v -failfast -timeout 1h -tags=e2e ./test/e2e -run "$(TESTS_MATCH)"  --auto-port-forward

# Clean k8s cluster from e2e resources
clean-e2e:
	kubectl delete namespace e2e


##########################################
##  --    Continuous integration    --  ##
##########################################

ci: dep-vendor-only check-fmt generate check-local-changes unit integration e2e-compile docker-build

# Run e2e tests in a dedicated gke cluster,
# that we delete if everything went fine
ci-e2e:
	$(MAKE) bootstrap-gke dep-vendor-only docker-build docker-push deploy e2e delete-gke || ($(MAKE) delete-gke; exit 1)

ci-release: export GO_TAGS = release
ci-release: export LICENSE_PUBKEY = $(ROOT_DIR)/build/ci/license.key
ci-release:
	@ $(MAKE) dep-vendor-only generate docker-build docker-push


##########################
##  --   Helpers    --  ##
##########################

check-requisites:
	@ hack/check-requisites.sh

check-license-header:
	../build/check-license-header.sh

# Check if some changes exist in the workspace (eg. `make generate` added some changes)
check-local-changes:
	@ [[ "$$(git status --porcelain)" == "" ]] \
		|| ( echo -e "\nError: dirty local changes"; git status --porcelain; exit 1 )
