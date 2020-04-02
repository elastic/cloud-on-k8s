# Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
# or more contributor license agreements. Licensed under the Elastic License;
# you may not use this file except in compliance with the Elastic License.

# This Makefile is mostly used for continuous integration.

ROOT_DIR := $(CURDIR)/..
GO_MOUNT_PATH ?= /go/src/github.com/elastic/cloud-on-k8s
ALL_IN_ONE_OUTPUT_FILE ?= config/all-in-one.yaml
S3_ECK_DIR             ?= s3://download.elasticsearch.org/downloads/eck

-include $(ROOT_DIR)/.env

# This is set to avoid the issue described in https://github.com/hashicorp/vault/issues/6710
VAULT_CLIENT_TIMEOUT = 120

# BUILD_ID is present during run on Jenkins machine, but not on dev box, hence using it here to distinguish between those cases
ifndef VAULT_TOKEN
ifdef BUILD_ID
VAULT_TOKEN = $(shell vault write -address=$(VAULT_ADDR) -field=token auth/approle/login role_id=$(VAULT_ROLE_ID) secret_id=$(VAULT_SECRET_ID))
else
VAULT_TOKEN = $(shell vault write -address=$(VAULT_ADDR) -field=token auth/github/login token=$(GITHUB_TOKEN))
# we use roleId as a string that has to be there for authn/z for CI, but it's empty and not needed for local execution
NOT_USED := $(shell test -e $(ROOT_DIR)/deployer-config.yml && sed "s;roleId:.*;token: $(GITHUB_TOKEN);g" $(ROOT_DIR)/deployer-config.yml > tmp && mv tmp $(ROOT_DIR)/deployer-config.yml)
endif
endif

CI_IMAGE ?= docker.elastic.co/eck/eck-ci:$(shell md5sum $(ROOT_DIR)/go.mod $(ROOT_DIR)/.ci/Dockerfile | awk '{print $$1}' | md5sum | awk '{print $$1}')

print-ci-image:
	@ echo $(CI_IMAGE)

# runs $TARGET in context of CI container and dev makefile
ci:
	@ $(MAKE) DOCKER_CMD="make $(TARGET)" ci-internal

ci-interactive:
	@ $(MAKE) DOCKER_OPTS=-i DOCKER_CMD=bash ci-internal

ci-internal: ci-build-image
	@ docker run --rm -t $(DOCKER_OPTS) \
		-v /var/run/docker.sock:/var/run/docker.sock \
		-v $(ROOT_DIR):$(GO_MOUNT_PATH) \
		-w $(GO_MOUNT_PATH) \
		--network="host" \
		$(CI_IMAGE) \
		bash -c "$(DOCKER_CMD)"

# build and push the CI image only if it does not yet exist
ci-build-image: DOCKER_PASSWORD = $(shell VAULT_TOKEN=$(VAULT_TOKEN) vault read -address=$(VAULT_ADDR) -field=value secret/devops-ci/cloud-on-k8s/eckadmin)
ci-build-image:
	@ docker pull -q $(CI_IMAGE) || \
	( \
		docker build -f $(ROOT_DIR)/.ci/Dockerfile -t push.$(CI_IMAGE) --label "commit.hash=$(shell git rev-parse --short --verify HEAD)" $(ROOT_DIR) && \
		docker login -u eckadmin -p $(DOCKER_PASSWORD) push.docker.elastic.co && \
		docker push push.$(CI_IMAGE) \
	)

ifneq (,$(IS_SNAPSHOT_BUILD))
       SECRET_FIELD_PREFIX ?= dev-
endif

get-test-artifacts: get-monitoring-secrets get-test-license get-elastic-public-key

##  Build

# read Elastic public key from Vault into license.key, to build the operator for E2E tests or for a release
get-elastic-public-key:
	@ VAULT_TOKEN=$(VAULT_TOKEN) vault read -address=$(VAULT_ADDR) -field=$(SECRET_FIELD_PREFIX)pubkey secret/devops-ci/cloud-on-k8s/license | base64 --decode > license.key

##  Test

# read some test licenses from Vault for E2E license tests
get-test-license:
	@ VAULT_TOKEN=$(VAULT_TOKEN) vault read -address=$(VAULT_ADDR) -field=$(SECRET_FIELD_PREFIX)enterprise  secret/devops-ci/cloud-on-k8s/test-licenses > test-license.json

# read connection info and credentials to the E2E tests monitoring Elasticsearch cluster, to be used during E2E tests
get-monitoring-secrets:
	@ VAULT_TOKEN=$(VAULT_TOKEN) vault read -address=$(VAULT_ADDR) -field=data -format=json secret/devops-ci/cloud-on-k8s/monitoring-cluster > monitoring-secrets.json

##  Release

# read docker password from Vault to push Docker images in the Elastic registry
get-docker-creds:
	@ echo "ELASTIC_DOCKER_PASSWORD = $(shell VAULT_TOKEN=$(VAULT_TOKEN) vault read -address=$(VAULT_ADDR) -field=value secret/devops-ci/cloud-on-k8s/eckadmin)" >> ${ROOT_DIR}/.env

# read AWS creds from Vault for YAML upload to S3
yaml-upload: VAULT_AWS_CREDS = secret/cloud-team/cloud-ci/eck-release
yaml-upload: AWS_ACCESS_KEY_ID = $(shell VAULT_TOKEN=$(VAULT_TOKEN) vault read -address=$(VAULT_ADDR) -field=access-key-id $(VAULT_AWS_CREDS))
yaml-upload: AWS_SECRET_ACCESS_KEY = $(shell VAULT_TOKEN=$(VAULT_TOKEN) vault read -address=$(VAULT_ADDR) -field=secret-access-key $(VAULT_AWS_CREDS))
yaml-upload: YAML_SRC = "$(GO_MOUNT_PATH)/$(ALL_IN_ONE_OUTPUT_FILE)"
yaml-upload: YAML_DST = "$(S3_ECK_DIR)/$(VERSION)/all-in-one.yaml"
yaml-upload:
ifndef VERSION
	$(error VERSION not set to upload YAML to S3)
endif
	@ $(MAKE) \
		DOCKER_OPTS="-e AWS_ACCESS_KEY_ID=$(AWS_ACCESS_KEY_ID) -e AWS_SECRET_ACCESS_KEY=$(AWS_SECRET_ACCESS_KEY)" \
		DOCKER_CMD="aws s3 ls $(YAML_DST) && echo OK: $(YAML_DST) already uploaded || aws s3 cp $(YAML_SRC) $(YAML_DST)" \
		ci-internal

