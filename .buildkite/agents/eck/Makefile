NAME := docker.elastic.co/eck-dev/bk-agent
TAG  := $(shell sha1sum Dockerfile | cut -c-8)

build:
	docker build -t $(NAME):$(TAG) .

push:
	docker tag $(NAME):$(TAG) $(NAME):latest
	docker push $(NAME):$(TAG)
	docker push $(NAME):latest
