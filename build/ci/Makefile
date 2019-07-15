# Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
# or more contributor license agreements. Licensed under the Elastic License;
# you may not use this file except in compliance with the Elastic License.

# This Makefile is mostly used for continuous integration.

ROOT_DIR = $(CURDIR)/../..
GO_MOUNT_PATH ?= /go/src/github.com/elastic/cloud-on-k8s

VAULT_GKE_CREDS_SECRET ?= secret/cloud-team/cloud-ci/ci-gcp-k8s-operator
GKE_CREDS_FILE ?= credentials.json
VAULT_PUBLIC_KEY ?= secret/release/license
PUBLIC_KEY_FILE ?= license.key
VAULT_DOCKER_CREDENTIALS ?= secret/cloud-team/cloud-ci/cloudadmin
DOCKER_CREDENTIALS_FILE ?= docker_credentials.file
VAULT_AWS_CREDS ?= secret/cloud-team/cloud-ci/eck-release
VAULT_AWS_ACCESS_KEY_FILE ?= aws_access_key.file
VAULT_AWS_SECRET_KEY_FILE ?= aws_secret_key.file

DOCKER_LOGIN ?= cloudadmin

VAULT_TOKEN ?= $(shell vault write -field=token auth/approle/login role_id=$(VAULT_ROLE_ID) secret_id=$(VAULT_SECRET_ID))

check-license-header:
	./../check-license-header.sh

# login to vault and retrieve gke creds into $GKE_CREDS_FILE
vault-gke-creds:
	@ VAULT_TOKEN=$(VAULT_TOKEN) \
		vault read \
		-address=$(VAULT_ADDR) \
		-field=service-account \
		$(VAULT_GKE_CREDS_SECRET) \
		> $(GKE_CREDS_FILE)

# reads Elastic public key from Vault into $PUBLIC_KEY_FILE
vault-public-key:
	@ VAULT_TOKEN=$(VAULT_TOKEN) \
	 	vault read \
		-address=$(VAULT_ADDR) \
		-field=pubkey \
		$(VAULT_PUBLIC_KEY) \
		| base64 --decode \
		> $(PUBLIC_KEY_FILE)

# reads Docker password from Vault
vault-docker-creds:
	@ VAULT_TOKEN=$(VAULT_TOKEN) \
	 	vault read \
		-address=$(VAULT_ADDR) \
		-field=password \
		$(VAULT_DOCKER_CREDENTIALS) \
		> $(DOCKER_CREDENTIALS_FILE)

# reads AWS creds for yaml upload
vault-aws-creds:
	@ VAULT_TOKEN=$(VAULT_TOKEN) \
		vault read \
		-address=$(VAULT_ADDR) \
		-field=access-key-id \
		$(VAULT_AWS_CREDS) \
		> $(VAULT_AWS_ACCESS_KEY_FILE)
	@ VAULT_TOKEN=$(VAULT_TOKEN) \
		vault read \
		-address=$(VAULT_ADDR) \
		-field=secret-access-key \
		$(VAULT_AWS_CREDS) \
		> $(VAULT_AWS_SECRET_KEY_FILE)

## -- Job executed on all PRs

ci-pr: check-license-header
	docker build -f Dockerfile -t cloud-on-k8s-ci-pr .
	docker run --rm -t \
		-v /var/run/docker.sock:/var/run/docker.sock \
		-v $(ROOT_DIR):$(GO_MOUNT_PATH) \
		-w $(GO_MOUNT_PATH) \
		-e "IMG_SUFFIX=-ci" \
		--net=host \
		cloud-on-k8s-ci-pr \
		bash -c \
			"make -C operators ci && \
			 make -C local-volume ci"

## -- Release job

ci-release: vault-public-key vault-docker-creds
	docker build -f Dockerfile -t cloud-on-k8s-ci-release .
	docker run --rm -t \
    	-v /var/run/docker.sock:/var/run/docker.sock \
    	-v $(ROOT_DIR):$(GO_MOUNT_PATH) \
    	-w $(GO_MOUNT_PATH) \
    	-e "TAG=$(TAG_NAME)" \
    	-e "IMG_SUFFIX=$(IMG_SUFFIX)" \
    	-e "IMG_NAME=$(IMG_NAME)" \
    	-e "REGISTRY=$(REGISTRY)" \
    	-e "REPOSITORY=$(REPOSITORY)" \
    	-e "ELASTIC_DOCKER_LOGIN=$(DOCKER_LOGIN)" \
    	-e "ELASTIC_DOCKER_PASSWORD=$(shell cat $(DOCKER_CREDENTIALS_FILE))" \
    	-e "USE_ELASTIC_DOCKER_REGISTRY=true" \
    	-e "SNAPSHOT_RELEASE=$(SNAPSHOT_RELEASE)" \
    	cloud-on-k8s-ci-release \
    	bash -c "make -C operators ci-release"

# Will be uploaded to https://download.elastic.co/downloads/eck/$TAG_NAME/all-in-one.yaml
yaml-upload: vault-aws-creds
	docker build -f Dockerfile -t cloud-on-k8s-ci-release .
	docker run --rm -t \
        -v $(ROOT_DIR):$(GO_MOUNT_PATH) \
        -w $(GO_MOUNT_PATH) \
        -e "AWS_ACCESS_KEY_ID=$(shell cat $(VAULT_AWS_ACCESS_KEY_FILE))" \
        -e "AWS_SECRET_ACCESS_KEY=$(shell cat $(VAULT_AWS_SECRET_KEY_FILE))" \
        cloud-on-k8s-ci-release \
        bash -c "aws s3 cp $(GO_MOUNT_PATH)/operators/config/all-in-one.yaml \
		s3://download.elasticsearch.org/downloads/eck/$(TAG_NAME)/all-in-one.yaml"

## -- End-to-end tests job

# Spawn a k8s cluster, and run e2e tests against it
ci-e2e: vault-gke-creds
	docker build -f Dockerfile -t cloud-on-k8s-ci-e2e .
	docker run --rm -t \
		-v /var/run/docker.sock:/var/run/docker.sock \
		-v $(ROOT_DIR):$(GO_MOUNT_PATH) \
		-w $(GO_MOUNT_PATH) \
		-e "IMG_SUFFIX=-ci" \
		-e "GCLOUD_PROJECT=$(GCLOUD_PROJECT)" \
		-e "REGISTRY=$(REGISTRY)" \
		-e "REPOSITORY=$(GCLOUD_PROJECT)" \
		-e "GKE_CLUSTER_NAME=$(GKE_CLUSTER_NAME)" \
		-e "GKE_SERVICE_ACCOUNT_KEY_FILE=$(GO_MOUNT_PATH)/build/ci/$(GKE_CREDS_FILE)" \
		-e "TESTS_MATCH=$(TESTS_MATCH)" \
		-e "GKE_CLUSTER_VERSION=$(GKE_CLUSTER_VERSION)" \
		cloud-on-k8s-ci-e2e \
		bash -c "make -C operators ci-e2e"

# Run e2e tests in GKE against provided ECK image
ci-e2e-rc: vault-gke-creds
	docker build -f Dockerfile -t cloud-on-k8s-ci-e2e .
	docker run --rm -t \
		-v /var/run/docker.sock:/var/run/docker.sock \
		-v $(ROOT_DIR):$(GO_MOUNT_PATH) \
		-w $(GO_MOUNT_PATH) \
		-e "IMG_SUFFIX=-ci" \
		-e "GCLOUD_PROJECT=$(GCLOUD_PROJECT)" \
		-e "REGISTRY=$(REGISTRY)" \
		-e "REPOSITORY=$(GCLOUD_PROJECT)" \
		-e "GKE_CLUSTER_NAME=$(GKE_CLUSTER_NAME)" \
		-e "GKE_SERVICE_ACCOUNT_KEY_FILE=$(GO_MOUNT_PATH)/build/ci/$(GKE_CREDS_FILE)" \
		-e "TESTS_MATCH=$(TESTS_MATCH)" \
		-e "GKE_CLUSTER_VERSION=$(GKE_CLUSTER_VERSION)" \
		-e "OPERATOR_IMAGE=$(OPERATOR_IMAGE)" \
		cloud-on-k8s-ci-e2e \
		bash -c "make -C operators ci-e2e-rc"

# Remove k8s cluster
ci-e2e-delete-cluster: vault-gke-creds
	docker build -f Dockerfile -t cloud-on-k8s-ci-e2e .
	docker run --rm -t \
    	-v /var/run/docker.sock:/var/run/docker.sock \
    	-v $(ROOT_DIR):$(GO_MOUNT_PATH) \
    	-w $(GO_MOUNT_PATH) \
    	-e "GCLOUD_PROJECT=$(GCLOUD_PROJECT)" \
    	-e "GKE_CLUSTER_NAME=$(GKE_CLUSTER_NAME)" \
    	-e "GKE_SERVICE_ACCOUNT_KEY_FILE=$(GO_MOUNT_PATH)/build/ci/$(GKE_CREDS_FILE)" \
    	cloud-on-k8s-ci-e2e \
    	bash -c "make -C operators set-context-gke delete-gke"

# Remove all unused resources in GKE
ci-gke-cleanup: ci-e2e-delete-cluster
	docker run --rm -t \
    	-v $(ROOT_DIR):$(GO_MOUNT_PATH) \
    	-w $(GO_MOUNT_PATH) \
    	-e "GCLOUD_PROJECT=$(GCLOUD_PROJECT)" \
    	-e "GKE_CLUSTER_NAME=$(GKE_CLUSTER_NAME)" \
    	-e "GKE_SERVICE_ACCOUNT_KEY_FILE=$(GO_MOUNT_PATH)/build/ci/$(GKE_CREDS_FILE)" \
    	cloud-on-k8s-ci-e2e \
    	bash -c "GKE_CLUSTER_VERSION=1.11 $(GO_MOUNT_PATH)/operators/hack/gke-cluster.sh auth && \
    	 	$(GO_MOUNT_PATH)/build/ci/delete_unused_disks.py"

# Run docs build
ci-build-docs:
	docker run --rm -t \
		-v $(ROOT_DIR):$(GO_MOUNT_PATH) \
        docker.elastic.co/docs/build:1 \
        bash -c "git clone https://github.com/elastic/docs.git && \
        	/docs/build_docs.pl --doc $(GO_MOUNT_PATH)/docs/index.asciidoc --out $(GO_MOUNT_PATH)/docs/html --chunk 1 && \
        	test -e $(GO_MOUNT_PATH)/docs/html/index.html"
