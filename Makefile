# Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
# or more contributor license agreements. Licensed under the Elastic License 2.0;
# you may not use this file except in compliance with the Elastic License 2.0.

#################################
##  --      Variables      --  ##
#################################

# Read env files, ignores if they doesn't exist
-include .registry.env
-include .env

# make sure sub-commands don't use eg. fish shell
export SHELL := /bin/bash

KUBECTL_CLUSTER := $(shell kubectl config current-context 2> /dev/null)

# Default to debug logging
LOG_VERBOSITY ?= 1

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
GOBIN := $(or $(shell go env GOBIN 2>/dev/null), $(shell go env GOPATH 2>/dev/null)/bin)

# find or download controller-gen
controller-gen: CONTROLLER_TOOLS_VERSION = $(shell grep controller-tools go.mod | grep -o "v[0-9\.]*")
controller-gen:
ifneq ($(shell controller-gen --version 2> /dev/null), Version: ${CONTROLLER_TOOLS_VERSION})
	@(cd /tmp; GO111MODULE=on go install sigs.k8s.io/controller-tools/cmd/controller-gen@${CONTROLLER_TOOLS_VERSION})
CONTROLLER_GEN=$(GOBIN)/controller-gen
else
CONTROLLER_GEN=$(shell which controller-gen)
endif

## -- Docker image

REGISTRY            ?= docker.elastic.co

export SNAPSHOT     ?= true
export VERSION      ?= $(shell cat VERSION)
export SHA1         ?= $(shell git rev-parse --short=8 --verify HEAD)
export ARCH         ?= $(shell uname -m | sed -e "s|x86_|amd|" -e "s|aarch|arm|")

# for dev, suffix image name with current user name
IMAGE_SUFFIX        ?= -$(subst _,,$(shell whoami))
REGISTRY_NAMESPACE  ?= eck-dev

export IMAGE_NAME   ?= $(REGISTRY)/$(REGISTRY_NAMESPACE)/eck-operator$(IMAGE_SUFFIX)
export IMAGE_TAG    ?= $(VERSION)-$(SHA1)
OPERATOR_IMAGE      ?= $(IMAGE_NAME):$(IMAGE_TAG)

print-%:
	@ echo $($*)

GO_LDFLAGS := -X github.com/elastic/cloud-on-k8s/v2/pkg/about.version=$(VERSION) \
	-X github.com/elastic/cloud-on-k8s/v2/pkg/about.buildHash=$(SHA1) \
	-X github.com/elastic/cloud-on-k8s/v2/pkg/about.buildDate=$(shell date -u +'%Y-%m-%dT%H:%M:%SZ') \
	-X github.com/elastic/cloud-on-k8s/v2/pkg/about.buildSnapshot=$(SNAPSHOT)

# options for 'go test'. for instance, set to "-race" to enable the race checker
TEST_OPTS ?=

## -- Namespaces

# namespace in which the operator is deployed
OPERATOR_NAMESPACE ?= elastic-system
# name of the operator statefulset and related resources
OPERATOR_NAME ?= elastic-operator
# comma separated list of namespaces in which the operator should watch resources
MANAGED_NAMESPACES ?=

#####################################
##  --       Development       --  ##
#####################################

all: dependencies lint check-license-header unit integration e2e-compile elastic-operator reattach-pv

## -- build

dependencies: tidy
	go mod download

tidy:
	go mod tidy

go-build: go-generate
	go build \
		-mod readonly \
		-ldflags "$(GO_LDFLAGS)" -tags="$(GO_TAGS)" -a \
		 -o elastic-operator github.com/elastic/cloud-on-k8s/v2/cmd

reattach-pv:
	# just check that reattach-pv still compiles
	go build -o /dev/null support/reattach-pv/main.go

compile-all:
	@ go build ./...
	@ $(MAKE) e2e-compile

## -- generate code, CRDs and documentation

go-generate:
	@ # generate use this in pkg/controller/common/license
	go generate -tags='$(GO_TAGS)' ./pkg/... ./cmd/...

generate: tidy generate-manifests generate-config-file generate-api-docs generate-notice-file

ALL_V1_CRDS=config/crds/v1/all-crds.yaml

generate-manifests: controller-gen
	# -- generate  webhook manifest
	@ $(CONTROLLER_GEN) webhook object:headerFile=./hack/boilerplate.go.txt paths=./pkg/...
	# -- generate  crd bases manifests
	@ $(CONTROLLER_GEN) crd:crdVersions=v1,generateEmbeddedObjectMeta=true paths="./pkg/apis/..." output:crd:artifacts:config=config/crds/v1/bases
	# -- kustomize crd manifests
	@ kubectl kustomize config/crds/v1/patches > $(ALL_V1_CRDS)
	# -- generate  crds manifest
	@ ./hack/manifest-gen/manifest-gen.sh -c -g > config/crds.yaml
	# -- generate  operator manifest
	./hack/manifest-gen/manifest-gen.sh -g \
		--namespace=$(OPERATOR_NAMESPACE) \
		--profile=global \
		--set=installCRDs=false \
		--set=telemetry.distributionChannel=all-in-one \
		--set=image.tag=$(IMAGE_TAG) \
		--set=image.repository=$(IMAGE_NAME) \
		--set=nameOverride=$(OPERATOR_NAME) \
		--set=fullnameOverride=$(OPERATOR_NAME) > config/operator.yaml

generate-config-file:
	@hack/config-extractor/extract.sh

generate-api-docs:
	@hack/api-docs/build.sh

generate-notice-file:
	@hack/licence-detector/generate-notice.sh

generate-image-dependencies:
	@hack/licence-detector/generate-image-deps.sh

clean:
	rm -f pkg/controller/common/license/zz_generated.pubkey.go

## -- tests

unit: clean
	ECK_TEST_LOG_LEVEL=$(LOG_VERBOSITY) go test ./pkg/... ./cmd/... -cover $(TEST_OPTS)

unit-xml: clean
	ECK_TEST_LOG_LEVEL=$(LOG_VERBOSITY) gotestsum --junitfile unit-tests.xml -- -cover ./pkg/... ./cmd/... $(TEST_OPTS)

helm-test:
	@hack/helm/test.sh

integration: GO_TAGS += integration
integration: clean
	@ for pkg in $$(grep 'go:build integration' -rl | grep _test.go | xargs -n1 dirname | uniq); do \
		KUBEBUILDER_ASSETS=/usr/local/bin ECK_TEST_LOG_LEVEL=$(LOG_VERBOSITY) \
			go test $$(pwd)/$$pkg -tags='$(GO_TAGS)' -cover $(TEST_OPTS) ; \
	done

integration-xml: GO_TAGS += integration
integration-xml: clean
	@ for pkg in $$(grep 'go:build integration' -rl | grep _test.go | xargs -n1 dirname | uniq); do \
	KUBEBUILDER_ASSETS=/usr/local/bin ECK_TEST_LOG_LEVEL=$(LOG_VERBOSITY) \
		gotestsum --junitfile integration-tests.xml -- $$(pwd)/$$pkg -tags='$(GO_TAGS)' -cover $(TEST_OPTS) ; \
	done

lint:
	GOGC=40 golangci-lint run --verbose

manifest-gen-test:
	hack/manifest-gen/test.sh

shellcheck:
	shellcheck -x $(shell find . -type f -name "*.sh" -not -path "./vendor/*")

upgrade-test: docker-build docker-push
	@hack/upgrade-test-harness/run.sh

#############################
##  --       Run       --  ##
#############################

install-crds: generate-manifests
	kubectl apply -f $(ALL_V1_CRDS)

# Run locally against the configured Kubernetes cluster, with port-forwarding enabled so that
# the operator can reach services running in the cluster through k8s port-forward feature
run: install-crds go-run

go-run:
	@ # Run the operator locally with debug logs and operator image set to latest
	AUTO_PORT_FORWARD=true \
		go run \
			-ldflags "$(GO_LDFLAGS)" \
			-tags "$(GO_TAGS)" \
			./cmd/main.go manager \
				--development \
				--enable-leader-election=false \
				--log-verbosity=$(LOG_VERBOSITY) \
				--ca-cert-validity=10h --ca-cert-rotate-before=1h \
				--operator-namespace=default \
				--namespaces=$(MANAGED_NAMESPACES) \
				--manage-webhook-certs=false \
				--exposed-node-labels=topology.kubernetes.io/.*,failure-domain.beta.kubernetes.io/.* \
				2>&1 | grep -v "dev-portforward" # remove dev-portforward logs from the output

go-debug:
	@ (cd cmd &&	AUTO_PORT_FORWARD=true dlv debug \
		--build-flags="-ldflags '$(GO_LDFLAGS)'" \
		-- \
		manager \
		--development \
		--log-verbosity=$(LOG_VERBOSITY) \
		--ca-cert-validity=10h \
		--ca-cert-rotate-before=1h \
		--operator-namespace=default \
		--namespaces=$(MANAGED_NAMESPACES) \
		--enable-leader-election=false \
		--manage-webhook-certs=false)

# if the current k8s cluster is on GKE, GCLOUD_PROJECT must be set
check-gke:
ifneq ($(findstring gke_,$(KUBECTL_CLUSTER)),)
ifndef GCLOUD_PROJECT
	$(error GCLOUD_PROJECT not set while GKE detected)
endif
endif

# Deploy the operator against the current k8s cluster
deploy: check-gke install-crds docker-push-operator apply-operator

apply-operator:
ifeq ($(strip $(MANAGED_NAMESPACES)),)
	@ ./hack/manifest-gen/manifest-gen.sh -g \
		--namespace=$(OPERATOR_NAMESPACE) \
		--set=image.tag=$(IMAGE_TAG) \
		--set=image.repository=$(IMAGE_NAME) \
		--set=nameOverride=$(OPERATOR_NAME) \
		--set=fullnameOverride=$(OPERATOR_NAME) | kubectl apply -f -
else
	@ ./hack/manifest-gen/manifest-gen.sh -g \
		--profile=restricted \
		--namespace=$(OPERATOR_NAMESPACE) \
		--set=installCRDs=true \
		--set=image.tag=$(IMAGE_TAG) \
		--set=image.repository=$(IMAGE_NAME) \
		--set=nameOverride=$(OPERATOR_NAME) \
		--set=fullnameOverride=$(OPERATOR_NAME) \
		--set=managedNamespaces="{$(MANAGED_NAMESPACES)}" | kubectl apply -f -
endif

logs-operator:
	@ kubectl --namespace=$(OPERATOR_NAMESPACE) logs -f statefulset.apps/$(OPERATOR_NAME)

samples:
	@ echo "-> Pushing samples to Kubernetes cluster..."
	@ kubectl apply -f config/samples/kibana/kibana_es.yaml

# Display elasticsearch credentials of the first stack
show-credentials:
	@ echo "elastic:$$(kubectl get secret elasticsearch-sample-es-elastic-user -o json | jq -r '.data.elastic' | base64 -D)"

##########################################
##  --    K8s clusters bootstrap    --  ##
##########################################

cluster-bootstrap: install-crds

clean-k8s-cluster:
	kubectl delete --ignore-not-found=true  ValidatingWebhookConfiguration validating-webhook-configuration
	for ns in $(OPERATOR_NAMESPACE) $(MANAGED_NAMESPACES); do \
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
	hack/dev/minikube-cluster.sh
	$(MAKE) set-context-minikube cluster-bootstrap

## -- clouds

PROVIDER=$(shell test -f hack/deployer/config/provider && cat hack/deployer/config/provider)
DEPLOYER=./hack/deployer/deployer --plans-file=hack/deployer/config/plans.yml --config-file=hack/deployer/config/deployer-config-$(PROVIDER).yml

build-deployer:
	@ go build -mod=readonly -o ./hack/deployer/deployer ./hack/deployer/main.go

run-deployer: build-deployer
	./hack/deployer/deployer execute --plans-file hack/deployer/config/plans.yml --config-file deployer-config.yml

run-deployer-cleanup: build-deployer
	./hack/deployer/deployer cleanup --plans-file hack/deployer/config/plans.yml --cluster-prefix $(E2E_TEST_CLUSTER_PREFIX) --config-file deployer-config.yml

set-kubeconfig: build-deployer
	./hack/deployer/deployer get credentials --plans-file hack/deployer/config/plans.yml --config-file deployer-config.yml

create-default-config:
ifeq ($(wildcard hack/deployer/config/deployer-config-$(PROVIDER).yml),)
	@ ./hack/deployer/deployer create defaultConfig --path=hack/deployer/config --provider=$(PROVIDER)
endif

setup-deployer: build-deployer
ifeq ($(PROVIDER),)
	$(MAKE) switch-gke
endif
	$(MAKE) create-default-config

get-deployer-config: setup-deployer
	@ $(DEPLOYER) get config

credentials: setup-deployer
	@ $(DEPLOYER) get credentials

set-context: credentials
	$(eval KUBECTL_CLUSTER=$($(DEPLOYER) get clusterName))

bootstrap-k8s: setup-deployer
	@ $(DEPLOYER) execute

bootstrap-cloud: bootstrap-k8s
	$(MAKE) cluster-bootstrap

delete-cloud: setup-deployer
	@ $(DEPLOYER) execute --operation=delete

switch-gke:
	@ echo "gke" > hack/deployer/config/provider

switch-aks:
	@ echo "aks" > hack/deployer/config/provider

switch-ocp:
	@ echo "ocp" > hack/deployer/config/provider

switch-eks:
	@ echo "eks" > hack/deployer/config/provider

switch-kind:
	@ echo "kind" > hack/deployer/config/provider

switch-tanzu:
	@ echo "tanzu" > hack/deployer/config/provider

#################################
##  --    Docker images    --  ##
#################################

# build amd64 image for dev purposes
BUILD_PLATFORM ?= "linux/amd64"
docker-push-operator:
	docker buildx build . \
	 	-f build/Dockerfile \
		--progress=plain \
		--build-arg GO_LDFLAGS='$(GO_LDFLAGS)' \
		--build-arg GO_TAGS='$(GO_TAGS)' \
		--build-arg VERSION='$(VERSION)' \
		--platform $(BUILD_PLATFORM) \
		--push \
		-t $(OPERATOR_IMAGE)

drivah-generate-operator:
	@ build/gen-drivah.toml.sh

# standard way to build operator image(s) using drivah
export BUILD_FLAVORS ?= dev
drivah-build-operator: drivah-generate-operator
	drivah build ./build

purge-gcr-images:
	@ for i in $(gcloud container images list-tags $(IMAGE_NAME) | tail +3 | awk '{print $$2}'); \
		do gcloud container images untag $(IMAGE_NAME):$$i; \
	done

switch-registry-gcr:
ifndef GCLOUD_PROJECT
	$(error GCLOUD_PROJECT not set to use GCR)
endif
	@ echo "REGISTRY = eu.gcr.io"               > .registry.env
	@ echo "REGISTRY_NAMESPACE = ${GCLOUD_PROJECT}"     >> .registry.env
	@ echo "E2E_REGISTRY_NAMESPACE = ${GCLOUD_PROJECT}" >> .registry.env

switch-registry-dev: # just use the default values of variables
	@ rm -f .registry.env

###################################
##  --   End to end tests    --  ##
###################################

# -- build

E2E_REGISTRY_NAMESPACE     ?= eck-dev
E2E_TEST_CLUSTER_PREFIX    ?= "eck-e2e"
export E2E_IMAGE_NAME      ?= $(REGISTRY)/$(E2E_REGISTRY_NAMESPACE)/eck-e2e-tests
export E2E_IMAGE_TAG       ?= $(SHA1)
E2E_IMG                    ?= $(E2E_IMAGE_NAME):$(E2E_IMAGE_TAG)

# push amd64 image for dev purposes
docker-push-e2e:
	docker buildx build \
		--progress=plain \
		--file test/e2e/Dockerfile \
		--platform $(BUILD_PLATFORM) \
		--push \
		-t $(E2E_IMAGE_NAME):$(E2E_IMAGE_TAG) .

# standard way to build e2e tests image using drivah
drivah-build-e2e:
	drivah build test/e2e

# -- run

E2E_STACK_VERSION          ?= 8.13.2
# regexp to filter tests to run
export TESTS_MATCH         ?= "^Test"
export E2E_JSON            ?= false
TEST_TIMEOUT               ?= 15m
E2E_SKIP_CLEANUP           ?= false
E2E_DEPLOY_CHAOS_JOB       ?= false
# go build constraints potentially restricting the tests to run
E2E_TAGS                   ?= e2e
# tags conveying information about the test environment to the test runner
E2E_TEST_ENV_TAGS          ?= ""

# combine e2e tags (es, kb, apm etc.)  with go tags (release) to ensure test code that imports generated code works
ifneq (,$(GO_TAGS))
E2E_TAGS := $(GO_TAGS),$(E2E_TAGS)
endif

e2e-run: go-generate
	go run -tags='$(GO_TAGS)' test/e2e/cmd/main.go run \
		--operator-image=$(OPERATOR_IMAGE) \
		--e2e-image=$(E2E_IMG) \
		--e2e-tags='$(E2E_TAGS)' \
		--test-regex=$(TESTS_MATCH) \
		--test-license=$(TEST_LICENSE) \
		--test-license-pkey-path=$(TEST_LICENSE_PKEY_PATH) \
		--elastic-stack-version=$(E2E_STACK_VERSION) \
		--log-verbosity=$(LOG_VERBOSITY) \
		--log-to-file=$(E2E_JSON) \
		--test-timeout=$(TEST_TIMEOUT) \
		--pipeline=$(PIPELINE) \
		--build-number=$(BUILD_NUMBER) \
		--provider=$(E2E_PROVIDER) \
		--clusterName=$(CLUSTER_NAME) \
		--monitoring-secrets=$(MONITORING_SECRETS) \
		--skip-cleanup=$(E2E_SKIP_CLEANUP) \
		--deploy-chaos-job=$(E2E_DEPLOY_CHAOS_JOB) \
		--test-env-tags=$(E2E_TEST_ENV_TAGS)

e2e-generate-xml:
	@ hack/ci/generate-junit-xml-report.sh e2e-tests.json

# Verify e2e tests compile with no errors, don't run them
e2e-compile: go-generate
	@go test ./test/e2e/... -run=dryrun -tags='$(E2E_TAGS)' $(TEST_OPTS) > /dev/null

# Run e2e tests locally (not as a k8s job), with a custom http dialer
# that can reach ES services running in the k8s cluster through port-forwarding.
e2e-local: LOCAL_E2E_CTX := /tmp/e2e-local.json
e2e-local: go-generate
	@go run -tags '$(GO_TAGS)' test/e2e/cmd/main.go run \
		--test-run-name=e2e \
		--operator-image=$(OPERATOR_IMAGE) \
		--test-context-out=$(LOCAL_E2E_CTX) \
		--test-license=$(TEST_LICENSE) \
		--test-license-pkey-path=$(TEST_LICENSE_PKEY_PATH) \
		--elastic-stack-version=$(E2E_STACK_VERSION) \
		--auto-port-forwarding \
		--local \
		--log-verbosity=$(LOG_VERBOSITY) \
		--ignore-webhook-failures \
		--test-timeout=$(TEST_TIMEOUT) \
		--test-env-tags=$(E2E_TEST_ENV_TAGS)

##########################
##  --   Helpers    --  ##
##########################

check-requisites:
	@ hack/check/check-requisites.sh

check-license-header:
	@ hack/check/check-license-header.sh

# Check if some changes exist in the workspace (eg. `make generate` added some changes)
check-local-changes:
	@ [[ "$$(git status --porcelain)" == "" ]] \
		|| ( echo -e "\nError: dirty local changes"; git status --porcelain; git --no-pager diff; exit 1 )

# Check if the predicate names in upgrade_predicates.go, are equal to the predicate names
# defined in the user documentation in orchestration.asciidoc.
check-predicates: CODE = pkg/controller/elasticsearch/driver/upgrade_predicates.go
check-predicates: DOC = docs/orchestrating-elastic-stack-applications/elasticsearch/orchestration.asciidoc
check-predicates: PREDICATE_PATTERN = [a-z]*_[A-Za-z_]*
check-predicates:
	@ diff \
		<(grep "name:" "$(CODE)" | grep -o "$(PREDICATE_PATTERN)" ) \
		<(grep '\*\* [a-z]' "$(DOC)" | grep -o "$(PREDICATE_PATTERN)" )
