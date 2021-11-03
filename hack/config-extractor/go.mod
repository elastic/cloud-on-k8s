module github.com/elastic/cloud-on-k8s/hack/config-extractor

go 1.16

require (
	k8s.io/api v0.22.2
	k8s.io/apiextensions-apiserver v0.22.2
	k8s.io/apimachinery v0.22.2
	k8s.io/kubectl v0.22.2
)

exclude github.com/miekg/dns v1.0.14
