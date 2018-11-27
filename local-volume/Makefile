DEP := $(shell command -v dep)
IMG_TAG ?= latest
IMG_MINIKUBE = localhost:5000/elastic-cloud-dev/elastic-local
IMG_GKE ?= eu.gcr.io/elastic-cloud-dev/elastic-local-${USER}

##
## Go stuff
## --------

.PHONY: build
build:
	mkdir -p bin
	go build -o bin/driverclient ./cmd/driverclient
	go build -o bin/driverdaemon ./cmd/driverdaemon
	go build -o bin/provisioner  ./cmd/provisioner

.PHONY: vendor
vendor:
ifndef DEP
	@ echo "-> dep binary missing, $(INSTALL_HELP)"
	@ exit 1
endif
	@ echo "-> Running dep..."
	@ dep ensure

.PHONY: unit
unit:
	@ go test -cover ./...

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
