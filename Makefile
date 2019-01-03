MOUNT_PATH ?= /go/src/github.com/elastic/stack-operators
BASE_IMAGE ?= golang:1.11

ci:
	docker run --rm -t \
		-v $(CURDIR):$(MOUNT_PATH) \
		-w $(MOUNT_PATH) \
		$(BASE_IMAGE) \
		make -C stack-operator ci && \
		make -C local-volume ci
