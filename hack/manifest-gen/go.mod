module github.com/elastic/cloud-on-k8s/hack/manifest-gen

go 1.16

require (
	github.com/spf13/cobra v1.2.1
	helm.sh/helm/v3 v3.7.1
	sigs.k8s.io/kustomize/kyaml v0.13.0
)

exclude (
	github.com/buger/jsonparser v0.0.0-20180808090653-f4dd9f5a6b44
	github.com/containernetworking/cni v0.7.1
	github.com/containernetworking/cni v0.8.0
	k8s.io/kubernetes v1.13.0
)
