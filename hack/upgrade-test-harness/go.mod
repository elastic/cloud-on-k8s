module github.com/elastic/cloud-on-k8s/hack/upgrade-test-harness

go 1.16

require (
	github.com/blang/semver/v4 v4.0.0
	github.com/ghodss/yaml v1.0.0
	github.com/hashicorp/go-multierror v1.1.1
	github.com/jonboulle/clockwork v0.2.2
	github.com/spf13/cobra v1.3.0
	go.uber.org/zap v1.20.0
	gopkg.in/yaml.v2 v2.4.0
	k8s.io/api v0.22.4
	k8s.io/apimachinery v0.22.4
	k8s.io/cli-runtime v0.22.4
	k8s.io/client-go v0.22.4
	k8s.io/kubectl v0.22.4
)
