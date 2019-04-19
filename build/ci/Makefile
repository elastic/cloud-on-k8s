# Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
# or more contributor license agreements. Licensed under the Elastic License;
# you may not use this file except in compliance with the Elastic License.

# This Makefile is mostly used for continuous integration.

ROOT_DIR = $(CURDIR)/../..
GO_MOUNT_PATH ?= /go/src/github.com/elastic/k8s-operators

VAULT_GKE_CREDS_SECRET ?= secret/cloud-team/cloud-ci/ci-gcp-k8s-operator
GKE_CREDS_FILE ?= credentials.json

check-license-header:
	./../check-license-header.sh

# login to vault and retrieve gke creds into $GKE_CREDS_FILE
vault-gke-creds:
	VAULT_TOKEN=$$(vault write -field=token auth/approle/login role_id=$(VAULT_ROLE_ID) secret_id=$(VAULT_SECRET_ID)) \
		vault read \
			-address=$(VAULT_ADDR) \
			-field=service-account \
			$(VAULT_GKE_CREDS_SECRET) \
			> $(GKE_CREDS_FILE)

## -- Job executed on all PRs

ci-pr: check-license-header
	docker build -f pr/Dockerfile -t k8s-operators-ci-pr .
	docker run --rm -t \
		-v /var/run/docker.sock:/var/run/docker.sock \
		-v $(ROOT_DIR):$(GO_MOUNT_PATH) \
		-w $(GO_MOUNT_PATH) \
		-e "IMG_SUFFIX=-ci" \
		--net=host \
		k8s-operators-ci-pr \
		bash -c \
			"make -C operators ci && \
			 make -C local-volume ci"

## -- Release job

# TODO

## -- End-to-end tests job

# Spawn a k8s cluster, and run e2e tests against it
ci-e2e: vault-gke-creds
	docker build -f e2e/Dockerfile -t k8s-operators-ci-e2e .
	docker run --rm -t \
		-v /var/run/docker.sock:/var/run/docker.sock \
		-v $(ROOT_DIR):$(GO_MOUNT_PATH) \
		-w $(GO_MOUNT_PATH) \
		-e "IMG_SUFFIX=-ci" \
		-e "GCLOUD_PROJECT=$(GCLOUD_PROJECT)" \
		-e "REGISTRY=$(REGISTRY)" \
		-e "REPOSITORY=$(GCLOUD_PROJECT)" \
		-e "GKE_CLUSTER_NAME=e2e-qa-$(shell date +'%Y%m%d-%H%M%S')" \
		-e "GKE_SERVICE_ACCOUNT_KEY_FILE=$(GO_MOUNT_PATH)/build/ci/$(GKE_CREDS_FILE)" \
		k8s-operators-ci-e2e \
		bash -c "make -C operators ci-e2e"
