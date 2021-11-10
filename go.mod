module github.com/elastic/cloud-on-k8s

go 1.16

require (
	github.com/Masterminds/sprig/v3 v3.2.2
	github.com/blang/semver/v4 v4.0.0
	github.com/bmizerany/perks v0.0.0-20141205001514-d9a9656a3a4b // indirect
	github.com/davecgh/go-spew v1.1.1
	github.com/dgryski/go-gk v0.0.0-20140819190930-201884a44051 // indirect
	github.com/elastic/go-ucfg v0.8.4
	github.com/elazarl/goproxy v0.0.0-20190711103511-473e67f1d7d2 // indirect
	github.com/fsnotify/fsnotify v1.5.1
	github.com/ghodss/yaml v1.0.0
	github.com/go-logr/logr v0.4.0
	github.com/go-test/deep v1.0.8
	github.com/gobuffalo/flect v0.2.4
	github.com/google/go-cmp v0.5.6
	github.com/google/gofuzz v1.2.0 // indirect
	github.com/google/uuid v1.3.0
	github.com/hashicorp/go-multierror v1.1.1
	github.com/hashicorp/vault/api v1.3.0
	github.com/imdario/mergo v0.3.12
	github.com/influxdata/tdigest v0.0.1 // indirect
	github.com/magiconair/properties v1.8.5
	github.com/pkg/errors v0.9.1
	github.com/prometheus/client_golang v1.11.0
	github.com/prometheus/common v0.26.0
	github.com/sethvargo/go-password v0.2.0
	github.com/spf13/cobra v1.2.1
	github.com/spf13/pflag v1.0.5
	github.com/spf13/viper v1.9.0
	github.com/streadway/quantile v0.0.0-20150917103942-b0c588724d25 // indirect
	github.com/stretchr/testify v1.7.0
	github.com/tsenart/vegeta v12.7.0+incompatible
	go.elastic.co/apm v1.14.0
	go.elastic.co/apm/module/apmelasticsearch v1.14.0
	go.uber.org/automaxprocs v1.4.0
	go.uber.org/zap v1.19.1
	golang.org/x/crypto v0.0.0-20210817164053-32db794688a5
	gopkg.in/yaml.v2 v2.4.0
	gopkg.in/yaml.v3 v3.0.0-20210107192922-496545a6307b
	k8s.io/api v0.21.3
	k8s.io/apimachinery v0.21.3
	k8s.io/client-go v0.21.3
	k8s.io/klog/v2 v2.10.0
	k8s.io/utils v0.0.0-20210722164352-7f3ee0f31471
	sigs.k8s.io/controller-runtime v0.9.6
	sigs.k8s.io/controller-tools v0.6.2
)

// this is used by vegeta, but the version they use is older and did not include a licence. we require the licence and so pin this
replace github.com/dgryski/go-gk => github.com/dgryski/go-gk v0.0.0-20200319235926-a69029f61654
