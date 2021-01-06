module github.com/elastic/cloud-on-k8s/hack/upgrade-test-harness

go 1.14

require (
	github.com/blang/semver/v4 v4.0.0
	github.com/ghodss/yaml v1.0.0
	github.com/hashicorp/go-multierror v1.1.0
	github.com/jonboulle/clockwork v0.2.2
	github.com/spf13/cobra v1.1.1
	go.uber.org/zap v1.16.0
	k8s.io/api v0.19.0
	k8s.io/apimachinery v0.19.0
	k8s.io/cli-runtime v0.19.0
	k8s.io/client-go v0.19.0
	k8s.io/kubectl v0.19.0
)
