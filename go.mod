module github.com/elastic/cloud-on-k8s/operatorsv2

go 1.12

require (
	github.com/elastic/cloud-on-k8s v0.0.0-20190809141318-cb8aaf10d2bc
	github.com/elastic/go-ucfg v0.7.0 // indirect
	github.com/go-logr/logr v0.1.0
	github.com/onsi/ginkgo v1.8.0
	github.com/onsi/gomega v1.5.0
	k8s.io/api v0.0.0-20190409021203-6e4e0e4f393b
	k8s.io/apimachinery v0.0.0-20190404173353-6a84e37a896d
	k8s.io/client-go v11.0.1-0.20190409021438-1a26190bd76a+incompatible
	sigs.k8s.io/controller-runtime v0.2.0-beta.4
	sigs.k8s.io/controller-tools v0.2.0 // indirect
)
