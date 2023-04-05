# Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
# or more contributor license agreements. Licensed under the Elastic License 2.0;
# you may not use this file except in compliance with the Elastic License 2.0.

# This Makefile is mostly used for continuous integration.

ROOT_DIR               := $(CURDIR)/..
GO_MOUNT_PATH          ?= /go/src/github.com/elastic/cloud-on-k8s

# This is used to get IMG_VERSION in yaml-upload
-include $(ROOT_DIR)/.env

vault := ../hack/retry.sh 5 vault

# This is set to avoid the issue described in https://github.com/hashicorp/vault/issues/6710
VAULT_CLIENT_TIMEOUT = 120

export VAULT_ROOT_PATH = secret/ci/elastic-cloud-on-k8s
ifdef BUILD_ID
export VAULT_ROOT_PATH = secret/devops-ci/cloud-on-k8s
endif

# BUILD_ID is present during run on Jenkins machine, but not on dev box, hence using it here to distinguish between those cases
ifndef VAULT_TOKEN
ifdef BUILD_ID
export VAULT_TOKEN = $(shell $(vault) write -field=token auth/approle/login role_id=$(VAULT_ROLE_ID) secret_id=$(VAULT_SECRET_ID))
else
export VAULT_TOKEN = $(shell $(vault) write -field=token auth/github/login token=$(GITHUB_TOKEN))
# we use roleId as a string that has to be there for authn/z for CI, but it's empty and not needed for local execution
NOT_USED := $(shell test -e $(ROOT_DIR)/deployer-config.yml && sed "s;roleId:.*;token: $(GITHUB_TOKEN);g" $(ROOT_DIR)/deployer-config.yml > tmp && mv tmp $(ROOT_DIR)/deployer-config.yml)
endif
endif

CI_IMAGE ?= docker.elastic.co/ci-agent-images/cloud-k8s-operator/buildkite-agent:01671ac6

# volume used to share files between CI container and containers launched by deployer
ECK_CI_VOLUME := eck-ci-home-$(shell date '+%N')

print-ci-image:
	@ echo $(CI_IMAGE)

# runs $TARGET in context of CI container and dev makefile
ci:
	@ $(MAKE) DOCKER_CMD="make $(TARGET)" ci-internal

ci-interactive:
	@ $(MAKE) DOCKER_OPTS=-i DOCKER_CMD=bash ci-internal

ci-internal:
	@ # --userns=host to support Docker daemon host configured to run containers only in user namespaces
	@ docker volume create --name $(ECK_CI_VOLUME) > /dev/null && \
	  docker run --userns=host --rm -t $(DOCKER_OPTS) \
		-v /var/run/docker.sock:/var/run/docker.sock \
		-v $(ECK_CI_VOLUME):/root/ \
		-v $(ROOT_DIR):$(GO_MOUNT_PATH) \
		-e CI -e SHARED_VOLUME_NAME=$(ECK_CI_VOLUME) -e ENABLE_FIPS -e BUILD_LICENSE_PUBKEY \
		-e VAULT_ADDR -e VAULT_TOKEN -e VAULT_ROLE_ID -e VAULT_SECRET_ID -e VAULT_ROOT_PATH \
		-w $(GO_MOUNT_PATH) \
		--network="host" \
		$(CI_IMAGE) \
		bash -c "$(DOCKER_CMD)" ; exit=$$?; \
	  docker volume rm $(ECK_CI_VOLUME) > /dev/null; exit $$exit

##  Build

# read Elastic public key from Vault into license.key, to build the operator for E2E tests or for a release
license.key:
	@ VAULT_TOKEN=$(VAULT_TOKEN) $(vault) read -address=$(VAULT_ADDR) -field=$(SECRET_FIELD_PREFIX)pubkey ${VAULT_ROOT_PATH}/license | base64 --decode > license.key

##  Release

# read AWS creds from Vault for YAML upload to S3
yaml-upload: VAULT_AWS_CREDS = secret/cloud-team/cloud-ci/eck-release
yaml-upload: AWS_ACCESS_KEY_ID = $(shell VAULT_TOKEN=$(VAULT_TOKEN) $(vault) read -address=$(VAULT_ADDR) -field=access-key-id $(VAULT_AWS_CREDS))
yaml-upload: AWS_SECRET_ACCESS_KEY = $(shell VAULT_TOKEN=$(VAULT_TOKEN) $(vault) read -address=$(VAULT_ADDR) -field=secret-access-key $(VAULT_AWS_CREDS))
yaml-upload:
ifndef IMG_VERSION
	$(error IMG_VERSION not set to upload YAML to S3)
endif
	@ $(MAKE) \
		DOCKER_OPTS="-e VERSION=$(IMG_VERSION) -e AWS_ACCESS_KEY_ID=$(AWS_ACCESS_KEY_ID) -e AWS_SECRET_ACCESS_KEY=$(AWS_SECRET_ACCESS_KEY)" \
		DOCKER_CMD="hack/manifest-upload.sh" \
		ci-internal
