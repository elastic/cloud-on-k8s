# Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
# or more contributor license agreements. Licensed under the Elastic License;
# you may not use this file except in compliance with the Elastic License.

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
controller-gen:
ifneq ($(shell controller-gen --version 2> /dev/null), Version: v0.4.1)
	@(cd /tmp; GO111MODULE=on go get sigs.k8s.io/controller-tools/cmd/controller-gen@v0.4.1)
CONTROLLER_GEN=$(GOBIN)/controller-gen
else
CONTROLLER_GEN=$(shell which controller-gen)
endif

## -- Docker image

# for dev, suffix image name with current user name
IMG_SUFFIX ?= -$(subst _,,$(shell whoami))

REGISTRY           ?= docker.elastic.co
REGISTRY_NAMESPACE ?= eck-dev
NAME               ?= eck-operator
SNAPSHOT           ?= true
VERSION            ?= $(shell cat VERSION)
TAG                ?= $(shell git rev-parse --short=8 --verify HEAD)
IMG_NAME           ?= $(NAME)$(IMG_SUFFIX)
IMG_VERSION        ?= $(VERSION)-$(TAG)

BASE_IMG       := $(REGISTRY)/$(REGISTRY_NAMESPACE)/$(IMG_NAME)
OPERATOR_IMAGE ?= $(BASE_IMG):$(IMG_VERSION)

print-operator-image:
	@ echo $(OPERATOR_IMAGE)

GO_LDFLAGS := -X github.com/elastic/cloud-on-k8s/pkg/about.version=$(VERSION) \
	-X github.com/elastic/cloud-on-k8s/pkg/about.buildHash=$(TAG) \
	-X github.com/elastic/cloud-on-k8s/pkg/about.buildDate=$(shell date -u +'%Y-%m-%dT%H:%M:%SZ') \
	-X github.com/elastic/cloud-on-k8s/pkg/about.buildSnapshot=$(SNAPSHOT)

# options for 'go test'. for instance, set to "-race" to enable the race checker
TEST_OPTS ?=

## -- Namespaces

# namespace in which the operator is deployed
OPERATOR_NAMESPACE ?= elastic-system
# name of the operator statefulset and related resources
OPERATOR_NAME ?= elastic-operator
# comma separated list of namespaces in which the operator should watch resources
MANAGED_NAMESPACES ?=

## -- Security

# should environments be configured with PSP ?
# TODO: only relevant on GKE for e2e tests for the moment
PSP ?= 0

#####################################
##  --       Development       --  ##
#####################################

all: dependencies lint check-license-header unit integration e2e-compile elastic-operator reattach-pv

## -- build

dependencies:
	go mod tidy -v && go mod download

# Generate code, CRDs and documentation
ALL_CRDS=config/crds/all-crds.yaml
generate: tidy generate-crds generate-config-file generate-api-docs generate-notice-file

tidy:
	go mod tidy

go-generate:
	# we use this in pkg/controller/common/license
	go generate -tags='$(GO_TAGS)' ./pkg/... ./cmd/...

generate-crds: go-generate controller-gen
	# Generate webhook manifest
	# Webhook definitions exist in both pkg/apis and pkg/controller/elasticsearch/validation
	$(CONTROLLER_GEN) webhook object:headerFile=./hack/boilerplate.go.txt paths=./pkg/apis/... paths=./pkg/controller/elasticsearch/validation/...
	# Generate manifests e.g. CRD, RBAC etc.
	$(CONTROLLER_GEN) crd:crdVersions=v1beta1 paths="./pkg/apis/..." output:crd:artifacts:config=config/crds/bases
	# apply patches to work around some CRD generation issues, and merge them into a single file
	kubectl kustomize config/crds/patches > $(ALL_CRDS)
	# generate an all-in-one version including the operator manifests
	$(MAKE) --no-print-directory generate-all-in-one

generate-config-file:
	@hack/config-extractor/extract.sh

generate-api-docs:
	@hack/api-docs/build.sh

generate-notice-file:
	@hack/licence-detector/generate-notice.sh

generate-image-dependencies:
	@hack/licence-detector/generate-image-deps.sh

elastic-operator: generate
	go build -mod=readonly -ldflags "$(GO_LDFLAGS)" -tags='$(GO_TAGS)' -o bin/elastic-operator github.com/elastic/cloud-on-k8s/cmd

clean:
	rm -f pkg/controller/common/license/zz_generated.pubkey.go

reattach-pv:
	# just check that reattach-pv still compiles
	go build -o /dev/null hack/reattach-pv/main.go

## -- tests

unit: clean
	ECK_TEST_LOG_LEVEL=$(LOG_VERBOSITY) go test ./pkg/... ./cmd/... -cover $(TEST_OPTS)

unit-xml: clean
	ECK_TEST_LOG_LEVEL=$(LOG_VERBOSITY) gotestsum --junitfile unit-tests.xml -- -cover ./pkg/... ./cmd/... $(TEST_OPTS)

integration: GO_TAGS += integration
integration: clean generate-crds
	ECK_TEST_LOG_LEVEL=$(LOG_VERBOSITY) go test -tags='$(GO_TAGS)' ./pkg/... ./cmd/... -cover $(TEST_OPTS)

integration-xml: GO_TAGS += integration
integration-xml: clean generate-crds
	ECK_TEST_LOG_LEVEL=$(LOG_VERBOSITY) gotestsum --junitfile integration-tests.xml -- -tags='$(GO_TAGS)' -cover ./pkg/... ./cmd/... $(TEST_OPTS)

lint:
	golangci-lint run
	hack/manifest-gen/test.sh

shellcheck:
	shellcheck $(shell find . -type f -name "*.sh" -not -path "./vendor/*")

upgrade-test: docker-build docker-push
	@hack/upgrade-test-harness/run.sh

#############################
##  --       Run       --  ##
#############################

install-crds: generate-crds
	kubectl apply -f $(ALL_CRDS)

# Run locally against the configured Kubernetes cluster, with port-forwarding enabled so that
# the operator can reach services running in the cluster through k8s port-forward feature
run: install-crds go-run

go-run:
	# Run the operator locally with debug logs and operator image set to latest
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
				2>&1 | grep -v "dev-portforward" # remove dev-portforward logs from the output

go-debug:
	@(cd cmd &&	AUTO_PORT_FORWARD=true dlv debug \
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

build-operator-image:
	@ docker pull $(OPERATOR_IMAGE) \
	&& echo "OK: image $(OPERATOR_IMAGE) already published" \
	|| $(MAKE) docker-build docker-push

build-operator-multiarch-image:
	@ docker buildx imagetools inspect $(OPERATOR_IMAGE) | grep -q 'linux/arm64' 2>&1 >/dev/null \
	&& echo "OK: image $(OPERATOR_IMAGE) already published" \
	|| $(MAKE) docker-multiarch-build

# if the current k8s cluster is on GKE, GCLOUD_PROJECT must be set
check-gke:
ifneq ($(findstring gke_,$(KUBECTL_CLUSTER)),)
ifndef GCLOUD_PROJECT
	$(error GCLOUD_PROJECT not set while GKE detected)
endif
endif

# Deploy the operator against the current k8s cluster
deploy: check-gke install-crds build-operator-image apply-operator

apply-operator:
ifeq ($(strip $(MANAGED_NAMESPACES)),)
	@ ./hack/manifest-gen/manifest-gen.sh -g \
		--namespace=$(OPERATOR_NAMESPACE) \
		--set=image.tag=$(IMG_VERSION) \
		--set=image.repository=$(BASE_IMG) \
		--set=nameOverride=$(OPERATOR_NAME) \
		--set=fullnameOverride=$(OPERATOR_NAME) | kubectl apply -f -
else
	@ ./hack/manifest-gen/manifest-gen.sh -g \
		--profile=restricted \
		--namespace=$(OPERATOR_NAMESPACE) \
		--set=installCRDs=true \
		--set=image.tag=$(IMG_VERSION) \
		--set=image.repository=$(BASE_IMG) \
		--set=nameOverride=$(OPERATOR_NAME) \
		--set=fullnameOverride=$(OPERATOR_NAME) \
		--set=managedNamespaces="{$(MANAGED_NAMESPACES)}" | kubectl apply -f -
endif

apply-psp:
	kubectl apply -f config/dev/elastic-psp.yaml

ALL_IN_ONE_OUTPUT_FILE=config/all-in-one.yaml

# merge all-in-one crds with operator manifests
generate-all-in-one:
	@ ./hack/manifest-gen/manifest-gen.sh -g \
		--namespace=$(OPERATOR_NAMESPACE) \
		--set=telemetry.distributionChannel=all-in-one \
		--set=image.tag=$(IMG_VERSION) \
		--set=image.repository=$(BASE_IMG) \
		--set=nameOverride=$(OPERATOR_NAME) \
		--set=fullnameOverride=$(OPERATOR_NAME) > $(ALL_IN_ONE_OUTPUT_FILE)

# Deploy an all in one operator against the current k8s cluster
deploy-all-in-one: GO_TAGS ?= release
deploy-all-in-one: docker-build docker-push
	kubectl apply -f $(ALL_IN_ONE_OUTPUT_FILE)

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
ifeq ($(PSP), 1)
	$(MAKE) apply-psp
endif

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

#################################
##  --    Docker images    --  ##
#################################
docker-multiarch-build: go-generate generate-config-file 
	@ hack/docker.sh -l -m $(OPERATOR_IMAGE)
	docker buildx build . \
		--progress=plain \
		--build-arg GO_LDFLAGS='$(GO_LDFLAGS)' \
		--build-arg GO_TAGS='$(GO_TAGS)' \
		--build-arg VERSION='$(VERSION)' \
		--platform linux/amd64,linux/arm64 \
		--push \
		-t $(OPERATOR_IMAGE)

docker-build: go-generate generate-config-file 
	DOCKER_BUILDKIT=1 docker build . \
		--progress=plain \
		--build-arg GO_LDFLAGS='$(GO_LDFLAGS)' \
		--build-arg GO_TAGS='$(GO_TAGS)' \
		--build-arg VERSION='$(VERSION)' \
		-t $(OPERATOR_IMAGE)

docker-push:
	@ hack/docker.sh -l -p $(OPERATOR_IMAGE)

purge-gcr-images:
	@ for i in $(gcloud container images list-tags $(BASE_IMG) | tail +3 | awk '{print $$2}'); \
		do gcloud container images untag $(BASE_IMG):$$i; \
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

E2E_REGISTRY_NAMESPACE     ?= eck-dev
E2E_IMG                    ?= $(REGISTRY)/$(E2E_REGISTRY_NAMESPACE)/eck-e2e-tests:$(TAG)
TESTS_MATCH                ?= "^Test" # can be overriden to eg. TESTS_MATCH=TestMutationMoreNodes to match a single test
E2E_STACK_VERSION          ?= 7.10.1
E2E_JSON                   ?= false
TEST_TIMEOUT               ?= 30m
E2E_SKIP_CLEANUP           ?= false
E2E_DEPLOY_CHAOS_JOB       ?= false
E2E_TAGS                   ?= e2e  # go build constraints potentially restricting the tests to run
E2E_TEST_ENV_TAGS          ?= ""   # tags conveying information about the test environment to the test runner

# clean to remove irrelevant/build-breaking generated public keys
e2e-docker-build: clean
	DOCKER_BUILDKIT=1 docker build --progress=plain --build-arg E2E_JSON=$(E2E_JSON) --build-arg GO_TAGS=$(E2E_TAGS) \
       -t $(E2E_IMG) -f test/e2e/Dockerfile .

e2e-docker-push:
	@ hack/docker.sh -l -p $(E2E_IMG)

e2e-docker-multiarch-build: clean
	@ hack/docker.sh -l -m $(E2E_IMG)
	docker buildx build \
		--progress=plain \
		--file test/e2e/Dockerfile \
		--build-arg E2E_JSON=$(E2E_JSON) \
		--build-arg GO_TAGS=$(E2E_TAGS) \
		--platform linux/amd64,linux/arm64 \
		--push \
		-t $(E2E_IMG) .

e2e-run:
	@go run test/e2e/cmd/main.go run \
		--operator-image=$(OPERATOR_IMAGE) \
		--e2e-image=$(E2E_IMG) \
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
e2e-compile:
	ECK_TEST_LOG_LEVEL=$(LOG_VERBOSITY) go test ./test/e2e/... -run=dryrun -tags=$(E2E_TAGS) $(TEST_OPTS) > /dev/null

# Run e2e tests locally (not as a k8s job), with a custom http dialer
# that can reach ES services running in the k8s cluster through port-forwarding.
e2e-local: LOCAL_E2E_CTX := /tmp/e2e-local.json
e2e-local:
	@go run test/e2e/cmd/main.go run \
		--test-run-name=e2e \
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
	@E2E_JSON=$(E2E_JSON) GO_TAGS=$(E2E_TAGS) test/e2e/run.sh -run $(TESTS_MATCH) -args -testContextPath $(LOCAL_E2E_CTX)

##########################################
##  --    Continuous integration    --  ##
##########################################

ci-check: check-license-header lint shellcheck generate check-local-changes

ci: unit-xml integration-xml docker-build reattach-pv

setup-e2e: e2e-compile run-deployer install-crds apply-psp e2e-docker-multiarch-build

ci-e2e: E2E_JSON := true
ci-e2e: setup-e2e e2e-run

ci-build-operator-e2e-run: E2E_JSON := true
ci-build-operator-e2e-run: setup-e2e build-operator-image e2e-run

run-deployer: build-deployer
	./hack/deployer/deployer execute --plans-file hack/deployer/config/plans.yml --config-file deployer-config.yml

ci-release: clean ci-check build-operator-multiarch-image
	@ echo $(OPERATOR_IMAGE) was pushed!

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
		|| ( echo -e "\nError: dirty local changes"; git status --porcelain; exit 1 )

# Runs small Go tool to validate syntax correctness of Jenkins pipelines
validate-jenkins-pipelines:
	@ go run ./hack/pipeline-validator/main.go

#########################
# Kind specific targets #
#########################
KIND_VERSION ?= 0.9.0
KIND_NODES ?= 3
KIND_NODE_IMAGE ?= kindest/node:v1.20.0
KIND_CLUSTER_NAME ?= eck

kind-node-variable-check:
ifndef KIND_NODE_IMAGE
	$(error KIND_NODE_IMAGE is mandatory when using Kind)
endif

bootstrap-kind:
	KIND_CLUSTER_NAME=${KIND_CLUSTER_NAME} \
		$(MAKE) kind-cluster-$(KIND_NODES)
	@ echo "Run the following command to update your current context:"
	@ echo "kubectl config set-context kind-${KIND_CLUSTER_NAME}"

## Start a Kind cluster with just the CRDs, e.g.:
# "make kind-cluster-0 KIND_NODE_IMAGE=kindest/node:v1.15.0" # start a 1-node cluster
# "make kind-cluster-3 KIND_NODE_IMAGE=kindest/node:v1.15.0" # start a 1-master 3-nodes cluster
kind-cluster-%: kind-node-variable-check
	go run ./hack/kind/main.go start \
		--nodes "${*}" \
		--cluster-name $(KIND_CLUSTER_NAME) \
		--kind-version $(KIND_VERSION) \
		--node-image $(KIND_NODE_IMAGE)
	make install-crds

## Same as above but build and deploy the operator image
kind-with-operator-%: kind-node-variable-check docker-build
	go run ./hack/kind/main.go start \
		--load-image $(OPERATOR_IMAGE) \
		--nodes "${*}" \
		--cluster-name $(KIND_CLUSTER_NAME) \
		--node-image $(KIND_NODE_IMAGE) \
		--kind-version $(KIND_VERSION)
	make install-crds apply-operator

## Run all e2e tests in a Kind cluster
set-kind-e2e-image:
ifneq ($(OPERATOR_IMAGE),)
	@docker pull $(OPERATOR_IMAGE)
else
	$(MAKE) go-generate docker-build
endif

kind-e2e: export E2E_JSON := true
kind-e2e: export KUBECONFIG = ${HOME}/.kube/kind-config-eck-e2e
kind-e2e: kind-node-variable-check set-kind-e2e-image e2e-docker-build
	go run ./hack/kind/main.go start \
		--load-image $(OPERATOR_IMAGE) \
		--load-image $(E2E_IMG) \
		--ip-family ${IP_FAMILY} \
		--nodes 3 \
		--cluster-name $(KIND_CLUSTER_NAME) \
		--node-image $(KIND_NODE_IMAGE) \
		--kind-version $(KIND_VERSION)
	make e2e-run OPERATOR_IMAGE=$(OPERATOR_IMAGE)

## Cleanup
delete-kind:
	go run ./hack/kind/main.go stop --cluster-name $(KIND_CLUSTER_NAME)
