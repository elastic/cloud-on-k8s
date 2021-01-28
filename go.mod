module github.com/elastic/cloud-on-k8s

go 1.13

require (
	github.com/Masterminds/sprig/v3 v3.2.0
	github.com/blang/semver/v4 v4.0.0
	github.com/bmizerany/perks v0.0.0-20141205001514-d9a9656a3a4b // indirect
	github.com/davecgh/go-spew v1.1.1
	github.com/dgryski/go-gk v0.0.0-20140819190930-201884a44051 // indirect
	github.com/docker/spdystream v0.0.0-20181023171402-6480d4af844c // indirect
	github.com/elastic/go-ucfg v0.8.3
	github.com/elazarl/goproxy v0.0.0-20190711103511-473e67f1d7d2 // indirect
	github.com/elazarl/goproxy/ext v0.0.0-20190711103511-473e67f1d7d2 // indirect
	github.com/fsnotify/fsnotify v1.4.9
	github.com/ghodss/yaml v1.0.0
	github.com/go-logr/logr v0.4.0
	github.com/go-logr/zapr v0.4.0 // indirect
	github.com/go-test/deep v1.0.7
	github.com/gobuffalo/flect v0.2.2
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/google/go-cmp v0.5.4
	github.com/google/gofuzz v1.2.0 // indirect
	github.com/google/uuid v1.2.0
	github.com/googleapis/gnostic v0.5.3 // indirect
	github.com/hashicorp/go-multierror v1.1.0
	github.com/hashicorp/vault/api v1.0.4
	github.com/imdario/mergo v0.3.11
	github.com/influxdata/tdigest v0.0.1 // indirect
	github.com/magiconair/properties v1.8.4
	github.com/pelletier/go-toml v1.4.0 // indirect
	github.com/pkg/errors v0.9.1
	github.com/prometheus/client_golang v1.9.0
	github.com/prometheus/common v0.15.0
	github.com/prometheus/procfs v0.3.0 // indirect
	github.com/sethvargo/go-password v0.2.0
	github.com/spf13/cobra v1.1.1
	github.com/spf13/jwalterweatherman v1.1.0 // indirect
	github.com/spf13/pflag v1.0.5
	github.com/spf13/viper v1.7.1
	github.com/streadway/quantile v0.0.0-20150917103942-b0c588724d25 // indirect
	github.com/stretchr/testify v1.7.0
	github.com/tsenart/vegeta v12.7.0+incompatible
	go.elastic.co/apm v1.8.0
	go.elastic.co/apm/module/apmelasticsearch v1.8.0
	go.uber.org/automaxprocs v1.3.0
	go.uber.org/multierr v1.6.0 // indirect
	go.uber.org/zap v1.16.0
	golang.org/x/crypto v0.0.0-20201221181555-eec23a3978ad
	golang.org/x/net v0.0.0-20210119194325-5f4716e94777 // indirect
	golang.org/x/oauth2 v0.0.0-20210113205817-d3ed898aa8a3 // indirect
	golang.org/x/sys v0.0.0-20210124154548-22da62e12c0c // indirect
	golang.org/x/term v0.0.0-20201210144234-2321bbc49cbf // indirect
	golang.org/x/text v0.3.5 // indirect
	golang.org/x/time v0.0.0-20201208040808-7e3f01d25324 // indirect
	google.golang.org/appengine v1.6.7 // indirect
	gopkg.in/yaml.v2 v2.4.0
	gopkg.in/yaml.v3 v3.0.0-20210107192922-496545a6307b
	k8s.io/api v0.20.2
	k8s.io/apiextensions-apiserver v0.20.2 // indirect
	k8s.io/apimachinery v0.20.2
	k8s.io/client-go v0.20.2
	k8s.io/klog/v2 v2.5.0
	k8s.io/kube-openapi v0.0.0-20210113233702-8566a335510f // indirect
	k8s.io/utils v0.0.0-20210111153108-fddb29f9d009
	sigs.k8s.io/controller-runtime v0.8.1
	sigs.k8s.io/controller-tools v0.4.1
)

// this is used by vegeta, but the version they use is older and did not include a licence. we require the licence and so pin this
replace github.com/dgryski/go-gk => github.com/dgryski/go-gk v0.0.0-20200319235926-a69029f61654
