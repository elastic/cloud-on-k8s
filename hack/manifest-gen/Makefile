RESTRICTED_NS := elastic-system
DOCKER_IMAGE  := docker.elastic.co/eck/manifest-gen:$(shell cat ../../VERSION)

.PHONY: gen-global
gen-global:
	@ ./manifest-gen.sh -g --profile=global

.PHONY: gen-restricted
gen-restricted:
	@ ./manifest-gen.sh -g --profile=restricted --set=operator.namespace=$(RESTRICTED_NS) --set=config.managedNamespaces="{$(RESTRICTED_NS)}"

.PHONY: docker-build
docker-build:
	@ ./manifest-gen.sh -u
	@ docker build -t $(DOCKER_IMAGE) .

.PHONY: docker-gen-global
docker-gen-global: docker-build
	@ docker run --rm $(DOCKER_IMAGE) generate --profile=global

.PHONY: docker-gen-restricted
docker-gen-restricted: docker-build
	@ docker run --rm $(DOCKER_IMAGE) generate --profile=restricted --set=operator.namespace=$(RESTRICTED_NS) --set=config.managedNamespaces="{$(RESTRICTED_NS)}"
