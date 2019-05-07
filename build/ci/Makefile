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

# reads Elastic public key from Vault into $PUBLIC_KEY_FILE
vault-public-key:
	VAULT_TOKEN=$$(vault write -field=token auth/approle/login role_id=$(VAULT_ROLE_ID) secret_id=$(VAULT_SECRET_ID)) \
		vault read \
			-address=$(VAULT_ADDR) \
			-field=pubkey \
			$(VAULT_PUBLIC_KEY) \
			| base64 --decode \
			> $(PUBLIC_KEY_FILE)

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

ci-release: vault-gke-creds
	docker build -f Dockerfile -t cloud-on-k8s-ci-release .
	docker run --rm -t \
		-v /var/run/docker.sock:/var/run/docker.sock \
		-v $(ROOT_DIR):$(GO_MOUNT_PATH) \
		-w $(GO_MOUNT_PATH) \
		-e "USER=random_fellow" \
		-e "TAG=${TAG_NAME}" \
		-e "IMG_SUFFIX=-release" \
		-e "REGISTRY=${REGISTRY}" \
		-e "REPOSITORY=${GCLOUD_PROJECT}" \
		-e "GCLOUD_PROJECT=${GCLOUD_PROJECT}" \
		-e "GKE_SERVICE_ACCOUNT_KEY_FILE=$(GO_MOUNT_PATH)/build/ci/$(GKE_CREDS_FILE)" \
		cloud-on-k8s-ci-release \
		bash -c "make -C operators ci-release"

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
		-e "GKE_CLUSTER_NAME=e2e-qa-$(shell date +'%Y%m%d-%H%M%S')" \
		-e "GKE_SERVICE_ACCOUNT_KEY_FILE=$(GO_MOUNT_PATH)/build/ci/$(GKE_CREDS_FILE)" \
		cloud-on-k8s-ci-e2e \
		bash -c "make -C operators ci-e2e GKE_MACHINE_TYPE=n1-standard-8"
