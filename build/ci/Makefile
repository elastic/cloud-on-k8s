# Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
# or more contributor license agreements. Licensed under the Elastic License;
# you may not use this file except in compliance with the Elastic License.

# This Makefile is mostly used for continuous integration.

ROOT_DIR = $(CURDIR)/../..
GO_MOUNT_PATH ?= /go/src/github.com/elastic/cloud-on-k8s
ALL_IN_ONE_OUTPUT_FILE ?= config/all-in-one.yaml

# This is set to avoid the issue described in https://github.com/hashicorp/vault/issues/6710
VAULT_CLIENT_TIMEOUT = 120

# BUILD_ID is present during run on Jenkins machine, but not on dev box, hence using it here to distinguish between those cases
ifdef BUILD_ID
VAULT_TOKEN = $(shell vault write -address=$(VAULT_ADDR) -field=token auth/approle/login role_id=$(VAULT_ROLE_ID) secret_id=$(VAULT_SECRET_ID))
else
VAULT_TOKEN = $(shell vault write -address=$(VAULT_ADDR) -field=token auth/github/login token=$(GITHUB_TOKEN))
# we use roleId as a string that has to be there for authn/z for CI, but it's empty and not needed for local execution
NOT_USED = $(shell test -e ../../deployer-config.yml && sed -i -e "s;roleId:;token: $(GITHUB_TOKEN);g" ../../deployer-config.yml)
endif

CI_IMAGE ?= docker.elastic.co/eck/eck-ci:$(shell md5sum $(ROOT_DIR)/go.mod $(ROOT_DIR)/build/ci/Dockerfile | awk '{print $$1}' | md5sum | awk '{print $$1}')

show-image:
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


# reads Docker password from Vault,
# checks if Docker image exists by trying to pull it. If there is no image, then build and push it.
ci-build-image:
	@ docker pull $(CI_IMAGE) || (docker build -f $(ROOT_DIR)/build/ci/Dockerfile -t push.$(CI_IMAGE) \
		--label "commit.hash=$(shell git rev-parse --short --verify HEAD)" $(ROOT_DIR) && docker login -u eckadmin \
		-p $(shell VAULT_TOKEN=$(VAULT_TOKEN) vault read -address=$(VAULT_ADDR) -field=value secret/devops-ci/cloud-on-k8s/eckadmin) \
		push.docker.elastic.co && docker push push.$(CI_IMAGE))

VAULT_AWS_CREDS = secret/cloud-team/cloud-ci/eck-release
AWS_ACCESS_KEY_ID = $(shell VAULT_TOKEN=$(VAULT_TOKEN) vault read -address=$(VAULT_ADDR) -field=access-key-id $(VAULT_AWS_CREDS))
AWS_SECRET_ACCESS_KEY = $(shell VAULT_TOKEN=$(VAULT_TOKEN) vault read -address=$(VAULT_ADDR) -field=secret-access-key $(VAULT_AWS_CREDS))
# reads AWS creds for yaml upload to https://download.elastic.co/downloads/eck/$TAG_NAME/all-in-one.yaml
yaml-upload:
	@ $(MAKE) \
		DOCKER_OPTS="-e AWS_ACCESS_KEY_ID=$(AWS_ACCESS_KEY_ID) -e AWS_SECRET_ACCESS_KEY=$(AWS_SECRET_ACCESS_KEY)" \
		DOCKER_CMD="aws s3 cp $(GO_MOUNT_PATH)/$(ALL_IN_ONE_OUTPUT_FILE) \
		s3://download.elasticsearch.org/downloads/eck/$(TAG_NAME)/all-in-one.yaml" ci-internal

# reads Elastic public key from Vault into license.key
get-elastic-public-key:
	@ VAULT_TOKEN=$(VAULT_TOKEN) vault read -address=$(VAULT_ADDR) -field=pubkey secret/release/license | base64 --decode > license.key

get-docker-creds:
	@ echo "ELASTIC_DOCKER_PASSWORD = $(shell VAULT_TOKEN=$(VAULT_TOKEN) vault read -address=$(VAULT_ADDR) -field=value secret/devops-ci/cloud-on-k8s/eckadmin)" >> ../../.env
