# Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
# or more contributor license agreements. Licensed under the Elastic License 2.0;
# you may not use this file except in compliance with the Elastic License 2.0.

RESTRICTED_NS:= elastic-system
DOCKER_IMAGE := docker.elastic.co/eck/manifest-gen:$(shell cat ../../VERSION)

.PHONY: gen-global
gen-global:
	@ ./manifest-gen.sh -g --profile=global

.PHONY: gen-restricted
gen-restricted:
	@ ./manifest-gen.sh -g --profile=restricted --namespace=$(RESTRICTED_NS) --set=managedNamespaces="{$(RESTRICTED_NS)}"

.PHONY: docker-build
docker-build:
	@ ./manifest-gen.sh -u
	@ docker build -t $(DOCKER_IMAGE) .

.PHONY: docker-push
docker-push:
	@ ../docker.sh -l -p $(DOCKER_IMAGE)

.PHONY: docker-gen-global
docker-gen-global: docker-build
	@ docker run --rm $(DOCKER_IMAGE) generate --profile=global

.PHONY: docker-gen-restricted
docker-gen-restricted: docker-build
	@ docker run --rm $(DOCKER_IMAGE) generate --profile=restricted --set=namespace=$(RESTRICTED_NS) --set=managedNamespaces="{$(RESTRICTED_NS)}"

.PHONY: test
test:
	@ ./test.sh
