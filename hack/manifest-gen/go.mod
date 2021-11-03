module github.com/elastic/cloud-on-k8s/hack/manifest-gen

go 1.16

require (
	github.com/buger/jsonparser v1.1.1 // indirect
	github.com/containernetworking/cni v0.8.1 // indirect
	github.com/spf13/cobra v1.2.1
	helm.sh/helm/v3 v3.7.1
	sigs.k8s.io/kustomize/kyaml v0.12.0
)

exclude k8s.io/kubernetes v1.13.0
