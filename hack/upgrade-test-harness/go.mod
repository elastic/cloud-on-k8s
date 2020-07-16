module github.com/elastic/cloud-on-k8s/hack/upgrade-test-harness

go 1.14

require (
	github.com/ghodss/yaml v1.0.0
	github.com/hashicorp/go-multierror v1.1.0
	github.com/jonboulle/clockwork v0.2.0
	github.com/spf13/cobra v0.0.7
	go.uber.org/zap v1.15.0
	k8s.io/apimachinery v0.18.6
	k8s.io/cli-runtime v0.18.6
	k8s.io/client-go v0.18.6
	k8s.io/kubectl v0.18.6
)
