# TODO: parametrize for GKE
# IMG_TAG ?= $(shell find pkg -type f -print0 | xargs -0 sha1sum | sha1sum | awk '{print $$1}')
IMG_TAG ?= latest
REGISTRY ?= localhost:5000
# TODO: parametrize for GKE
# REGISTRY ?= eu.gcr.io
IMG = $(REGISTRY)/elastic-cloud-dev/elastic-local

build:
	mkdir -p bin
	go build -o bin/driverclient ./cmd/driverclient
	go build -o bin/driverdaemon ./cmd/driverdaemon
	go build -o bin/provisioner  ./cmd/provisioner

docker-build:
	docker build -t $(IMG):$(IMG_TAG) .

docker-push:
	docker push $(IMG):$(IMG_TAG)

deploy:
	kubectl apply -f config/rbac.yaml -f config/storageclass.yaml -f config/provisioner.yaml -f config/driver.yaml

# run a docker registry in the minikube VM
minikube-registry:
	eval $$(minikube docker-env) ;\
	docker run -d -p 5000:5000 --restart=always --name registry registry:2

# create a new disk and attach it to minikube as /dev/sdb
EXTRA_DISK_FILE = ${HOME}/.minikube/machines/minikube/extra-disk.vmdk
minikube-attach-disk:
	VBoxManage createmedium disk \
		--filename $(EXTRA_DISK_FILE) \
		--format VMDK \
		--size 100 # megabytes
	VBoxManage storageattach minikube \
		--storagectl SATA \
		--type hdd \
		--port 2 \
		--medium $(EXTRA_DISK_FILE)

# create a logical volume group in minikube
VG_NAME = elastic-local-vg
minikube-create-vg:
	minikube ssh "sudo pvcreate /dev/sdb && sudo vgcreate $(VG_NAME) /dev/sdb"

redeploy:
	kubectl delete -f config/provisioner.yaml -f config/driver.yaml
	kubectl apply -f config/provisioner.yaml -f config/driver.yaml

driver-logs:
	kubectl -n elastic-local logs -f $$(kubectl -n elastic-local get pod | grep "elastic-local-driver" | head -n 1 |awk '{print $$1}')

provisioner-logs:
	kubectl -n elastic-local logs -f $$(kubectl -n elastic-local get pod | grep "elastic-local-provisioner" | awk '{print $$1}')
