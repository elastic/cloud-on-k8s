# Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
# or more contributor license agreements. Licensed under the Elastic License 2.0;
# you may not use this file except in compliance with the Elastic License 2.0.

BINARY_NAME := autoops-mock
REGISTRY_NAMESPACE ?= mmontg1
VERSION ?= $(shell cat ../../VERSION)
SHA1 ?= $(shell git rev-parse --short=8 --verify HEAD)
ARCH ?= $(shell uname -m | sed -e "s|x86_|amd|" -e "s|aarch|arm|")

IMAGE_NAME ?= $(REGISTRY_NAMESPACE)/$(BINARY_NAME)
IMAGE_TAG ?= $(VERSION)-$(SHA1)
DOCKER_IMAGE ?= $(IMAGE_NAME):$(IMAGE_TAG)

.PHONY: build
build:
	@echo "Building $(BINARY_NAME)..."
	@CGO_ENABLED=0 GOOS=linux go build -a -o $(BINARY_NAME) main.go

.PHONY: run
run: build
	@./$(BINARY_NAME)

.PHONY: docker-build
docker-build:
	@echo "Building Docker image $(DOCKER_IMAGE)..."
	@docker buildx build \
		-f Dockerfile \
		--progress=plain \
		--platform linux/amd64 \
		-t $(DOCKER_IMAGE) \
		.

.PHONY: docker-push
docker-push: docker-build
	@echo "Pushing Docker image $(DOCKER_IMAGE)..."
		@docker buildx build \
		-f Dockerfile \
		--progress=plain \
		--platform linux/amd64 \
		-t $(DOCKER_IMAGE) \
		--push \
		.

.PHONY: clean
clean:
	@rm -f $(BINARY_NAME)

.PHONY: help
help:
	@echo "Available targets:"
	@echo "  build        - Build the binary"
	@echo "  run          - Build and run the binary"
	@echo "  docker-build - Build the Docker image"
	@echo "  docker-push  - Build and push the Docker image"
	@echo "  clean        - Remove built binary"
	@echo "  help         - Show this help message"
