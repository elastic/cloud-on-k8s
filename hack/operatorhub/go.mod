module github.com/elastic/cloud-on-k8s/hack/operatorhub

go 1.16

require (
	github.com/Masterminds/sprig/v3 v3.2.2
	github.com/docker/docker v20.10.12+incompatible
	github.com/ghodss/yaml v1.0.1-0.20190212211648-25d852aebe32
	github.com/go-git/go-git/v5 v5.4.2
	github.com/operator-framework/operator-registry v1.19.5
	github.com/pterm/pterm v0.12.33
	github.com/spf13/cobra v1.3.0
	github.com/spf13/viper v1.10.0
	k8s.io/api v0.22.4
	k8s.io/apiextensions-apiserver v0.22.0
	k8s.io/apimachinery v0.22.4
	k8s.io/kubectl v0.22.4
)

replace (
	github.com/hashicorp/vault/api/auth/approle => github.com/hashicorp/vault/api/auth/approle v0.1.0
	github.com/hashicorp/vault/api/auth/userpass => github.com/hashicorp/vault/api/auth/userpass v0.1.0
)

require (
	github.com/Microsoft/go-winio v0.5.1 // indirect
	github.com/armon/go-metrics v0.3.10 // indirect
	github.com/benbjohnson/clock v1.1.0 // indirect
	github.com/bketelsen/crypt v0.0.4 // indirect
	github.com/containerd/containerd v1.5.8 // indirect
	github.com/containerd/continuity v0.2.1 // indirect
	github.com/containerd/stargz-snapshotter/estargz v0.10.1 // indirect
	github.com/docker/cli v20.10.12+incompatible
	github.com/docker/docker-credential-helpers v0.6.4 // indirect
	github.com/fatih/color v1.13.0 // indirect
	github.com/go-test/deep v1.0.8 // indirect
	github.com/google/go-containerregistry v0.5.0
	github.com/google/uuid v1.3.0 // indirect
	github.com/hashicorp/go-cleanhttp v0.5.2 // indirect
	github.com/hashicorp/go-hclog v1.0.0 // indirect
	github.com/hashicorp/go-retryablehttp v0.7.0 // indirect
	github.com/hashicorp/go-secure-stdlib/parseutil v0.1.2 // indirect
	github.com/hashicorp/go-version v1.3.0 // indirect
	github.com/hashicorp/hcl v1.0.1-vault-3 // indirect
	github.com/hashicorp/vault/api v1.3.0
	github.com/hashicorp/vault/sdk v0.3.1-0.20211209192327-a0822e64eae0 // indirect
	github.com/hashicorp/yamux v0.0.0-20181012175058-2f1d1f20f75d // indirect
	github.com/huandu/xstrings v1.3.2 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/klauspost/compress v1.13.6 // indirect
	github.com/kr/pretty v0.3.0 // indirect
	github.com/linuxkit/virtsock v0.0.0-20201010232012-f8cee7dfc7a3 // indirect
	github.com/mattn/go-shellwords v1.0.6 // indirect
	github.com/mitchellh/go-testing-interface v1.14.0 // indirect
	github.com/mitchellh/reflectwalk v1.0.2 // indirect
	github.com/moby/term v0.0.0-20210619224110-3f7ff695adc6 // indirect
	github.com/onsi/gomega v1.16.0 // indirect
	github.com/opencontainers/image-spec v1.0.2 // indirect
	github.com/otiai10/copy v1.7.0
	github.com/prometheus/client_golang v1.11.0 // indirect
	github.com/prometheus/common v0.28.0 // indirect
	github.com/rogpeppe/go-internal v1.6.2 // indirect
	github.com/sirupsen/logrus v1.8.1
	golang.org/x/crypto v0.0.0-20210817164053-32db794688a5 // indirect
	golang.org/x/mod v0.5.1 // indirect
	golang.org/x/net v0.0.0-20211216030914-fe4d6282115f // indirect
	golang.org/x/sys v0.0.0-20211216021012-1d35b9e2eb4e // indirect
	golang.org/x/text v0.3.7 // indirect
	golang.org/x/tools v0.1.8 // indirect
	google.golang.org/grpc v1.43.0 // indirect
	gopkg.in/square/go-jose.v2 v2.6.0 // indirect
	k8s.io/client-go v0.22.4 // indirect
	k8s.io/klog/v2 v2.30.0 // indirect
	k8s.io/kube-openapi v0.0.0-20211115234752-e816edb12b65 // indirect
	k8s.io/utils v0.0.0-20210930125809-cb0fa318a74b // indirect
	rsc.io/letsencrypt v0.0.3 // indirect
)
