MOUNT_PATH ?= /go/src/github.com/elastic/k8s-operators
CI_IMAGE_NAME ?= elastic-operators-ci

build-image:
	docker build -f Dockerfile.ci -t $(CI_IMAGE_NAME) .

ci: build-image
	docker run --rm -t \
		-v $(CURDIR):$(MOUNT_PATH) \
		-w $(MOUNT_PATH) \
		$(CI_IMAGE_NAME) \
		bash -c \
			"make -C elastic-operator ci && \
			 make -C local-volume ci"
