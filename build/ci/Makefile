# Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
# or more contributor license agreements. Licensed under the Elastic License;
# you may not use this file except in compliance with the Elastic License.

# This Makefile is mostly used for continuous integration.

ROOT_DIR = $(CURDIR)/../..
GO_MOUNT_PATH ?= /go/src/github.com/elastic/cloud-on-k8s

CI_IMAGE ?= docker.elastic.co/eck/eck-ci:$(shell md5sum $(ROOT_DIR)/operators/Gopkg.lock $(ROOT_DIR)/build/ci/Dockerfile $(ROOT_DIR)/local-volume/Gopkg.lock | awk '{print $$1}' | md5sum | awk '{print $$1}')

VAULT_GKE_CREDS_SECRET ?= secret/cloud-team/cloud-ci/ci-gcp-k8s-operator
GKE_CREDS_FILE ?= credentials.json
VAULT_PUBLIC_KEY ?= secret/release/license
PUBLIC_KEY_FILE ?= license.key
VAULT_DOCKER_CREDENTIALS ?= secret/devops-ci/cloud-on-k8s/eckadmin
DOCKER_LOGIN ?= eckadmin
DOCKER_CREDENTIALS_FILE ?= docker_credentials.file
VAULT_AWS_CREDS ?= secret/cloud-team/cloud-ci/eck-release
VAULT_AWS_ACCESS_KEY_FILE ?= aws_access_key.file
VAULT_AWS_SECRET_KEY_FILE ?= aws_secret_key.file

VAULT_TOKEN ?= $(shell vault write -field=token auth/approle/login role_id=$(VAULT_ROLE_ID) secret_id=$(VAULT_SECRET_ID))

check-license-header:
	./../check-license-header.sh

show-image:
	@ echo $(CI_IMAGE)

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
	@ docker run --rm -t \
		-v /var/run/docker.sock:/var/run/docker.sock \
		-v $(ROOT_DIR):$(GO_MOUNT_PATH) \
		-w $(GO_MOUNT_PATH) \
		-e "IMG_SUFFIX=-ci" \
		--net=host \
		$(CI_IMAGE) \
		bash -c \
			"make -C operators ci && \
			 make -C local-volume ci"

## -- Release job

ci-release: vault-public-key vault-docker-creds
	@ docker run --rm -t \
    	-v /var/run/docker.sock:/var/run/docker.sock \
    	-v $(ROOT_DIR):$(GO_MOUNT_PATH) \
    	-w $(GO_MOUNT_PATH) \
    	-e "ELASTIC_DOCKER_LOGIN=$(DOCKER_LOGIN)" \
    	-e "ELASTIC_DOCKER_PASSWORD=$(shell cat $(DOCKER_CREDENTIALS_FILE))" \
    	-e "USE_ELASTIC_DOCKER_REGISTRY=true" \
    	-e "OPERATOR_IMAGE=$(OPERATOR_IMAGE)" \
    	-e "LATEST_RELEASED_IMG=$(LATEST_RELEASED_IMG)" \
    	-e "VERSION=$(VERSION)" \
    	-e "SNAPSHOT=$(SNAPSHOT)" \
    	$(CI_IMAGE) \
    	bash -c "make -C operators ci-release"

# Will be uploaded to https://download.elastic.co/downloads/eck/$TAG_NAME/all-in-one.yaml
yaml-upload: vault-aws-creds
	@ docker run --rm -t \
        -v $(ROOT_DIR):$(GO_MOUNT_PATH) \
        -w $(GO_MOUNT_PATH) \
        -e "AWS_ACCESS_KEY_ID=$(shell cat $(VAULT_AWS_ACCESS_KEY_FILE))" \
        -e "AWS_SECRET_ACCESS_KEY=$(shell cat $(VAULT_AWS_SECRET_KEY_FILE))" \
        $(CI_IMAGE) \
        bash -c "aws s3 cp $(GO_MOUNT_PATH)/operators/config/all-in-one.yaml \
		s3://download.elasticsearch.org/downloads/eck/$(TAG_NAME)/all-in-one.yaml"

## -- End-to-end tests job

ci-e2e:
	@ docker run --rm -t \
		-v /var/run/docker.sock:/var/run/docker.sock \
		-v $(ROOT_DIR):$(GO_MOUNT_PATH) \
		-w $(GO_MOUNT_PATH) \
		-e "IMG_SUFFIX=-ci" \
		-e "GCLOUD_PROJECT=$(GCLOUD_PROJECT)" \
		-e "REGISTRY=$(REGISTRY)" \
		-e "REPOSITORY=$(GCLOUD_PROJECT)" \
		-e "TESTS_MATCH=$(TESTS_MATCH)" \
		-e "SKIP_DOCKER_COMMAND=$(SKIP_DOCKER_COMMAND)" \
		-e "OPERATOR_IMAGE=$(OPERATOR_IMAGE)" \
		-e "STACK_VERSION=$(STACK_VERSION)" \
		$(CI_IMAGE) \
		bash -c "make -C operators ci-e2e"

ci-run-deployer:
	@ docker run --rm -t \
		-v /var/run/docker.sock:/var/run/docker.sock \
		-v $(ROOT_DIR):$(GO_MOUNT_PATH) \
		-w $(GO_MOUNT_PATH) \
		-e "GCLOUD_PROJECT=$(GCLOUD_PROJECT)" \
		$(CI_IMAGE) \
		bash -c "make -C operators run-deployer"

# Check if Docker image exists by trying to pull it. If there is no image, then build and push it.
ci-build-image: vault-docker-creds
	@ docker pull $(CI_IMAGE) || (docker build -f $(ROOT_DIR)/build/ci/Dockerfile -t push.$(CI_IMAGE) \
	--label "commit.hash=$(shell git rev-parse --short --verify HEAD)" $(ROOT_DIR) &&\
	docker login -u $(DOCKER_LOGIN) -p $(shell cat $(DOCKER_CREDENTIALS_FILE)) push.docker.elastic.co &&\
	docker push push.$(CI_IMAGE))
