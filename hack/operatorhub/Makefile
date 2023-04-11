# Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
# or more contributor license agreements. Licensed under the Elastic License 2.0;
# you may not use this file except in compliance with the Elastic License 2.0.

DRY_RUN ?= true
TAG     ?= $$(cat ../../VERSION)

build:
	go build -o bin/operatorhub main.go

container-push:
	bin/operatorhub container push --dry-run="$(DRY_RUN)"

preflight:
	../../.buildkite/scripts/release/redhat-preflight.sh $(TAG)

container-publish:
	bin/operatorhub container publish --dry-run="$(DRY_RUN)"

generate-manifests:
	bin/operatorhub generate-manifests --yaml-manifest=../../config/crds.yaml --yaml-manifest=../../config/operator.yaml

bundle-generate:
	echo bin/operatorhub bundle generate --dir="$(shell pwd)"

create-pr:
	@ bin/operatorhub bundle create-pr --enable-vault=false --dir="$(shell pwd)" --dry-run="$(DRY_RUN)" -D=false \
		-f "$(GITHUB_FULLNAME)" -e "$(GITHUB_EMAIL)" -u "$(GITHUB_USERNAME)" -g "$(GITHUB_TOKEN)"
