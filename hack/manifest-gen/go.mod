module github.com/elastic/cloud-on-k8s/hack/manifest-gen

go 1.16

require (
	github.com/spf13/cobra v1.2.1
	helm.sh/helm/v3 v3.7.1
	k8s.io/api v0.22.2 // indirect
	sigs.k8s.io/kustomize/kyaml v0.12.0
)

exclude github.com/containerd/containerd v1.5.1
