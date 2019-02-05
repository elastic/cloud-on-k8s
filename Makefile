# Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
# or more contributor license agreements. Licensed under the Elastic License;
# you may not use this file except in compliance with the Elastic License.

MOUNT_PATH ?= /go/src/github.com/elastic/k8s-operators
CI_IMAGE_NAME ?= elastic-operators-ci

build-image:
	docker build -f Dockerfile.ci -t $(CI_IMAGE_NAME) .

ci: check-license-header build-image
	docker run --rm -t \
		-v $(CURDIR):$(MOUNT_PATH) \
		-w $(MOUNT_PATH) \
		$(CI_IMAGE_NAME) \
		bash -c \
			"make -C operators ci && \
			 make -C local-volume ci"

check-license-header:
	@ files=$$(grep \
		--include=\*.go --exclude-dir=vendor \
		--include=\*.sh \
		--include=Makefile \
	-L "Copyright Elasticsearch B.V." \
	-r *); \
	[ "$$files" != "" ] \
		&& echo "Error: file(s) without license header:\n$$files" && exit 1 \
		|| exit 0
