# Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
# or more contributor license agreements. Licensed under the Elastic License;
# you may not use this file except in compliance with the Elastic License.

# This Makefile is mostly used for continuous integration.

ROOT_DIR = $(CURDIR)/../..
GO_MOUNT_PATH ?= /go/src/github.com/elastic/cloud-on-k8s

# BUILD_ID is present during run on Jenkins machine, but not on dev box, hence using it here to distinguish between those cases
ifdef BUILD_ID
VAULT_TOKEN ?= $(shell vault write -field=token auth/approle/login role_id=$(VAULT_ROLE_ID) secret_id=$(VAULT_SECRET_ID))
else
VAULT_TOKEN = $(shell vault write -address=$(VAULT_ADDR) -field=token auth/github/login token=$(GITHUB_TOKEN))
# we use roleId as a string that has to be there for authn/z for CI, but it's empty and not needed for local execution
NOT_USED = $(shell test -e ../../run-config.yml && sed -i -e "s;roleId:;token: $(GITHUB_TOKEN);g" ../../run-config.yml)
endif

CI_IMAGE ?= docker.elastic.co/eck/eck-ci:$(shell md5sum $(ROOT_DIR)/Gopkg.lock $(ROOT_DIR)/build/ci/Dockerfile | awk '{print $$1}' | md5sum | awk '{print $$1}')

show-image:
	@ echo $(CI_IMAGE)

# runs $COMMAND in context of CI container and dev makefile
ci: ci-build-image
	@ docker run --rm -t \
		-v /var/run/docker.sock:/var/run/docker.sock \
		-v $(ROOT_DIR):$(GO_MOUNT_PATH) \
		-w $(GO_MOUNT_PATH) \
		-e "VAULT_TOKEN=$(VAULT_TOKEN)" \
		$(CI_IMAGE) \
		bash -c "make $(COMMAND)"

ci-interactive: ci-build-image
	@ docker run --rm -t -i \
		-v /var/run/docker.sock:/var/run/docker.sock \
		-v $(ROOT_DIR):$(GO_MOUNT_PATH) \
		-w $(GO_MOUNT_PATH) \
		-e "VAULT_TOKEN=$(VAULT_TOKEN)" \
		$(CI_IMAGE) \
		bash

# reads Docker password from Vault,
# checks if Docker image exists by trying to pull it. If there is no image, then build and push it.
ci-build-image:
	@ docker pull $(CI_IMAGE) || (docker build -f $(ROOT_DIR)/build/ci/Dockerfile -t push.$(CI_IMAGE) \
		--label "commit.hash=$(shell git rev-parse --short --verify HEAD)" $(ROOT_DIR) && docker login -u eckadmin \
		-p $(shell vault read -address=$(VAULT_ADDR) -field=value secret/devops-ci/cloud-on-k8s/eckadmin) \
		push.docker.elastic.co && docker push push.$(CI_IMAGE))
