# Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
# or more contributor license agreements. Licensed under the Elastic License;
# you may not use this file except in compliance with the Elastic License.

export SHELL := /bin/bash
DEP := $(shell command -v dep)
IMG_TAG ?= latest
IMG_MINIKUBE = localhost:5000/elastic-cloud-dev/elastic-local
IMG_GKE ?= eu.gcr.io/elastic-cloud-dev/elastic-local/$(subst _,,$(USER))

##
## Go stuff
## --------

.PHONY: build
build:
	mkdir -p bin
	go build -o bin/driverclient ./cmd/driverclient
	go build -o bin/driverdaemon ./cmd/driverdaemon
	go build -o bin/provisioner  ./cmd/provisioner

.PHONY: dep
dep:
	dep ensure -v

dep-vendor-only:
	# don't attempt to upgrade Gopkg.lock
	dep ensure --vendor-only -v

.PHONY: unit
unit:
	@ go test -cover ./...

.PHONY: ci
ci: dep-vendor-only build unit


check-license-header:
	../build/check-license-header

##
## Docker stuff
## ------------

docker-minikube:
	eval $$(minikube docker-env) ;\
	docker build -t $(IMG_MINIKUBE):$(IMG_TAG) . && \
	docker push $(IMG_MINIKUBE):$(IMG_TAG)

docker-gke:
	docker build -t $(IMG_GKE):$(IMG_TAG) .
	docker push $(IMG_GKE):$(IMG_TAG)

##
## Deployment stuff
## ----------------

# deploy everything to the minikube environment
deploy-minikube: deploy-base deploy-provisioner-minikube deploy-driver-minikube

# deploy everything to the gke environment
deploy-gke: deploy-base deploy-provisioner-gke deploy-driver-gke

deploy-base:
	kubectl apply -f config/rbac.yaml -f config/storageclass.yaml

deploy-provisioner-minikube:
	cat config/provisioner.yaml | sed "s;\$$IMG;$(IMG_MINIKUBE):$(IMG_TAG);g" | kubectl apply -f -

deploy-provisioner-gke:
	cat config/provisioner.yaml | sed "s;\$$IMG;$(IMG_GKE):$(IMG_TAG);g" | kubectl apply -f -

deploy-driver-minikube:
	cat config/driver.yaml | sed "s;\$$IMG;$(IMG_MINIKUBE):$(IMG_TAG);g" | kubectl apply -f -

deploy-driver-gke:
	cat config/driver-gke.yaml | sed "s;\$$IMG;$(IMG_GKE):$(IMG_TAG);g" | kubectl apply -f -

delete-provisioner-driver:
	kubectl delete --ignore-not-found -f config/provisioner.yaml -f config/driver.yaml

redeploy-provisioner-driver-gke: delete-provisioner-driver deploy-provisioner-gke deploy-driver-gke

redeploy-provisioner-driver-minikube: delete-provisioner-driver deploy-provisioner-minikube deploy-driver-minikube

redeploy-samples:
	kubectl delete --ignore-not-found -f config/pvc-sample.yaml -f config/pod-sample.yaml
	kubectl apply -f config/pvc-sample.yaml -f config/pod-sample.yaml

driver-logs:
	kubectl -n elastic-local logs -f $$(kubectl -n elastic-local get pod | grep "elastic-local-driver" | grep "Running" | head -n 1 |awk '{print $$1}')

provisioner-logs:
	kubectl -n elastic-local logs -f $$(kubectl -n elastic-local get pod | grep "elastic-local-provisioner" | grep "Running" | awk '{print $$1}')

##
## Minikube stuff
## --------------

# run a docker registry in the minikube VM
minikube-registry:
	eval $$(minikube docker-env) ;\
	docker run -d -p 5000:5000 --restart=always --name registry registry:2

# create a new disk and attach it to minikube as /dev/sdb
MINIKUBE_EXTRA_DISK_FILE = ${HOME}/.minikube/machines/minikube/extra-disk.vmdk
minikube-attach-disk:
	VBoxManage createmedium disk \
		--filename $(MINIKUBE_EXTRA_DISK_FILE) \
		--format VMDK \
		--size 100 # megabytes
	VBoxManage storageattach minikube \
		--storagectl SATA \
		--type hdd \
		--port 2 \
		--medium $(MINIKUBE_EXTRA_DISK_FILE)

##
## Volume group stuff
## ------------------

VG_NAME = elastic-local-vg

# create a logical volume group in minikube
minikube-create-vg:
	minikube ssh "sudo pvcreate /dev/sdb && sudo vgcreate $(VG_NAME) /dev/sdb"

# create a logical volume group in GKE
GKE_EXTRA_DISK ?= /dev/sdb
GKE_EXTRA_DISK_MOUNT ?= /mnt/disks/ssd0
gke-create-vg:
	# Get all instances containing $$USER and their zone
	# for each one, ssh into it, run a centos7 privileged container,
	# unmount the existing extra ssd,
	# then run pvcreate & vgcreate.
	# It can take some time (a few minutes per host for lvm2 pkg install).
	gcloud compute instances list | grep $(subst _,,$(USER)) | awk '{print "--zone " $$2 " " $$1 }' | \
		xargs -t -L 1 gcloud compute ssh --command \
		"docker run --rm --privileged \
			-v /dev:/dev \
			-v /run:/run \
			-v /mnt/disks:/mnt/disks:rshared \
			centos:7 bash -c \
				'yum install -y lvm2 && \
					umount $(GKE_EXTRA_DISK_MOUNT) && \
					pvcreate -y $(GKE_EXTRA_DISK) && \
					vgcreate $(VG_NAME) $(GKE_EXTRA_DISK)'"
