# Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
# or more contributor license agreements. Licensed under the Elastic License 2.0;
# you may not use this file except in compliance with the Elastic License 2.0.

# This Makefile is used to run the buildkite agent in virtual machines when Docker access is required.

CI_IMAGE               ?= docker.elastic.co/ci-agent-images/cloud-k8s-operator/buildkite-agent:ef29aa4e
ROOT_DIR               := $(CURDIR)/..
GO_MOUNT_PATH          ?= /go/src/github.com/elastic/cloud-on-k8s
export VAULT_ROOT_PATH = secret/ci/elastic-cloud-on-k8s
# volume used to share files between CI container and containers launched by deployer
ECK_CI_VOLUME          := eck-ci-home-$(shell date '+%N')

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
