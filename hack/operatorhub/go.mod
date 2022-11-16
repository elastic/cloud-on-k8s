module github.com/elastic/cloud-on-k8s/hack/operatorhub

go 1.16

require (
	github.com/Masterminds/sprig/v3 v3.2.2
	github.com/docker/docker v20.10.20+incompatible
	github.com/ghodss/yaml v1.0.1-0.20190212211648-25d852aebe32
	github.com/go-git/go-git/v5 v5.4.2
	github.com/operator-framework/operator-registry v1.19.5
	github.com/pterm/pterm v0.12.33
	github.com/spf13/cobra v1.6.1
	github.com/spf13/viper v1.14.0
	k8s.io/api v0.25.4
	k8s.io/apiextensions-apiserver v0.25.0
	k8s.io/apimachinery v0.25.4
	k8s.io/kubectl v0.22.4
)

replace (
	github.com/hashicorp/vault/api/auth/approle => github.com/hashicorp/vault/api/auth/approle v0.1.0
	github.com/hashicorp/vault/api/auth/userpass => github.com/hashicorp/vault/api/auth/userpass v0.1.0
	github.com/redhat-openshift-ecosystem/openshift-preflight => github.com/naemono/openshift-preflight v0.0.0-20221116163335-ef831c9d35ed
)

require (
	github.com/containerd/continuity v0.2.1 // indirect
	github.com/docker/cli v20.10.21+incompatible
	github.com/go-test/deep v1.0.8 // indirect
	github.com/google/go-containerregistry v0.12.1
	github.com/hashicorp/go-retryablehttp v0.7.0 // indirect
	github.com/hashicorp/go-secure-stdlib/parseutil v0.1.2 // indirect
	github.com/hashicorp/go-version v1.3.0 // indirect
	github.com/hashicorp/hcl v1.0.1-vault-3 // indirect
	github.com/hashicorp/vault/api v1.3.0
	github.com/hashicorp/vault/sdk v0.3.1-0.20211209192327-a0822e64eae0 // indirect
	github.com/hashicorp/yamux v0.0.0-20181012175058-2f1d1f20f75d // indirect
	github.com/huandu/xstrings v1.3.2 // indirect
	github.com/mitchellh/go-testing-interface v1.14.0 // indirect
	github.com/mitchellh/reflectwalk v1.0.2 // indirect
	github.com/otiai10/copy v1.7.0
	github.com/redhat-openshift-ecosystem/openshift-preflight v0.0.0-00010101000000-000000000000
	github.com/rogpeppe/go-internal v1.6.2 // indirect
	github.com/sirupsen/logrus v1.9.0
	gopkg.in/square/go-jose.v2 v2.6.0 // indirect
)
