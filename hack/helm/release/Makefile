# Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
# or more contributor license agreements. Licensed under the Elastic License 2.0;
# you may not use this file except in compliance with the Elastic License 2.0.

build:
	go build -o bin/releaser main.go

go-run:
	go run main.go --env=dev --dry-run=true --keep-tmp-dir=true --charts-dir=../../../deploy

test-release-eck-stack-dev:
	DRY_RUN=true ./trigger-helm-release.sh "eck-stack dev"

release-eck-operator:
	DRY_RUN=false ./trigger-helm-release.sh eck-operator

release-eck-stack:
	DRY_RUN=false ./trigger-helm-release.sh eck-stack
