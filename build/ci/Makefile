# Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
# or more contributor license agreements. Licensed under the Elastic License;
# you may not use this file except in compliance with the Elastic License.

# This Makefile is mostly used for continuous integration.

ROOT_DIR = $(CURDIR)/../..
GO_MOUNT_PATH ?= /go/src/github.com/elastic/cloud-on-k8s

IMAGE ?= docker.elastic.co/eck/eck-ci:$(shell md5sum $(ROOT_DIR)/operators/Gopkg.lock $(ROOT_DIR)/build/ci/Dockerfile $(ROOT_DIR)/local-volume/Gopkg.lock | awk '{print $$1}' | md5sum | awk '{print $$1}')


VAULT_GKE_CREDS_SECRET ?= secret/cloud-team/cloud-ci/ci-gcp-k8s-operator
GKE_CREDS_FILE ?= credentials.json
VAULT_PUBLIC_KEY ?= secret/release/license
PUBLIC_KEY_FILE ?= license.key
VAULT_DOCKER_CREDENTIALS ?= secret/devops-ci/cloud-on-k8s/eckadmin
VAULT_DOCKER_LOGIN ?= secret/cloud-team/cloud-ci/eck-docker
DOCKER_CREDENTIALS_FILE ?= docker_credentials.file
DOCKER_LOGIN_FILE ?= docker_login.file
DOCKER_ENVIRONMENT ?= environment
VAULT_AWS_CREDS ?= secret/cloud-team/cloud-ci/eck-release
VAULT_AWS_ACCESS_KEY_FILE ?= aws_access_key.file
VAULT_AWS_SECRET_KEY_FILE ?= aws_secret_key.file

VAULT_TOKEN ?= $(shell vault write -field=token auth/approle/login role_id=$(VAULT_ROLE_ID) secret_id=$(VAULT_SECRET_ID))

check-license-header:
	./../check-license-header.sh

create-env-file:
	@ $(shell ./create_env_file.py)

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
		-field=value \
		$(VAULT_DOCKER_CREDENTIALS) \
		> $(DOCKER_CREDENTIALS_FILE)
	@ VAULT_TOKEN=$(VAULT_TOKEN) \
    	vault read \
    	-address=$(VAULT_ADDR) \
    	-field=value \
    	$(VAULT_DOCKER_LOGIN) \
    	> $(DOCKER_LOGIN_FILE)

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

ci-pr: check-license-header create-env-file
	@ docker run --rm -t \
		-v /var/run/docker.sock:/var/run/docker.sock \
		-v $(ROOT_DIR):$(GO_MOUNT_PATH) \
		-w $(GO_MOUNT_PATH) \
		--env-file $(DOCKER_ENVIRONMENT) \
		--net=host \
		$(IMAGE) \
		bash -c \
			"make -C operators ci && \
			 make -C local-volume ci"

## -- Release job

ci-release: vault-public-key vault-docker-creds create-env-file
	@ docker run --rm -t \
    	-v /var/run/docker.sock:/var/run/docker.sock \
    	-v $(ROOT_DIR):$(GO_MOUNT_PATH) \
    	-w $(GO_MOUNT_PATH) \
    	--env-file $(DOCKER_ENVIRONMENT) \
    	$(IMAGE) \
    	bash -c "make -C operators ci-release"

# Will be uploaded to https://download.elastic.co/downloads/eck/$TAG_NAME/all-in-one.yaml
yaml-upload: vault-aws-creds create-env-file
	@ docker run --rm -t \
        -v $(ROOT_DIR):$(GO_MOUNT_PATH) \
        -w $(GO_MOUNT_PATH) \
        --env-file $(DOCKER_ENVIRONMENT) \
        $(IMAGE) \
        bash -c "aws s3 cp $(GO_MOUNT_PATH)/operators/config/all-in-one.yaml \
		s3://download.elasticsearch.org/downloads/eck/$(TAG_NAME)/all-in-one.yaml"

## -- End-to-end tests job

ci-e2e-new:
	@ docker run --rm -t \
		-v /var/run/docker.sock:/var/run/docker.sock \
		-v $(ROOT_DIR):$(GO_MOUNT_PATH) \
		-w $(GO_MOUNT_PATH) \
		--env-file $(DOCKER_ENVIRONMENT) \
		$(IMAGE) \
		bash -c "make -C operators ci-e2e-new"

ci-deployer-new:
	@ docker run --rm -t \
		-v /var/run/docker.sock:/var/run/docker.sock \
		-v $(ROOT_DIR):$(GO_MOUNT_PATH) \
		-w $(GO_MOUNT_PATH) \
		--env-file $(DOCKER_ENVIRONMENT) \
		$(IMAGE) \
		bash -c "make -C operators deployer-new"

# Spawn a k8s cluster, and run e2e tests against it
ci-e2e: vault-gke-creds create-env-file
	@ docker run --rm -t \
		-v /var/run/docker.sock:/var/run/docker.sock \
		-v $(ROOT_DIR):$(GO_MOUNT_PATH) \
		-w $(GO_MOUNT_PATH) \
		--env-file $(DOCKER_ENVIRONMENT) \
		$(IMAGE) \
		bash -c "make -C operators ci-e2e"

## -- Support jobs

# Remove k8s cluster
ci-e2e-delete-cluster: vault-gke-creds create-env-file
	@ docker run --rm -t \
    	-v /var/run/docker.sock:/var/run/docker.sock \
    	-v $(ROOT_DIR):$(GO_MOUNT_PATH) \
    	-w $(GO_MOUNT_PATH) \
    	--env-file $(DOCKER_ENVIRONMENT) \
    	$(IMAGE) \
    	bash -c "make -C operators set-context-gke delete-gke"

# Remove all unused resources in GKE
ci-gke-cleanup: ci-e2e-delete-cluster create-env-file
	@ docker run --rm -t \
    	-v $(ROOT_DIR):$(GO_MOUNT_PATH) \
    	-w $(GO_MOUNT_PATH) \
    	--env-file $(DOCKER_ENVIRONMENT) \
    	$(IMAGE) \
    	bash -c "GKE_CLUSTER_VERSION=1.11 $(GO_MOUNT_PATH)/operators/hack/gke-cluster.sh auth && \
    	 	$(GO_MOUNT_PATH)/build/ci/delete_unused_disks.py"

# Check if Docker image exists by trying to pull it. If there is no image, then build and push it.
ci-build-image: vault-docker-creds
	@ docker pull $(IMAGE) || (docker build -f $(ROOT_DIR)/build/ci/Dockerfile -t push.$(IMAGE) \
	--label "commit.hash=$(shell git rev-parse --short --verify HEAD)" $(ROOT_DIR) &&\
	docker login -u $(shell cat $(DOCKER_LOGIN_FILE)) -p $(shell cat $(DOCKER_CREDENTIALS_FILE)) push.docker.elastic.co &&\
	docker push push.$(IMAGE))
