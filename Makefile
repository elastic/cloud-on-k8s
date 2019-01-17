MOUNT_PATH ?= /go/src/github.com/elastic/stack-operators
CI_IMAGE_NAME ?= stack-operators-ci

build-image:
	docker build -f Dockerfile.ci -t $(CI_IMAGE_NAME) .

ci: build-image
	docker run --rm -t \
		-v $(CURDIR):$(MOUNT_PATH) \
		-w $(MOUNT_PATH) \
		$(CI_IMAGE_NAME) \
		bash -c \
			"make -C stack-operator ci && \
			 make -C local-volume ci"
