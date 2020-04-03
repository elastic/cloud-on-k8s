module github.com/elastic/cloud-on-k8s

go 1.13

require (
	github.com/Masterminds/goutils v1.1.0 // indirect
	github.com/Masterminds/semver v1.4.2 // indirect
	github.com/Masterminds/sprig v2.20.0+incompatible
	github.com/bmizerany/perks v0.0.0-20141205001514-d9a9656a3a4b // indirect
	github.com/davecgh/go-spew v1.1.1
	github.com/dgryski/go-gk v0.0.0-20140819190930-201884a44051 // indirect
	github.com/docker/spdystream v0.0.0-20181023171402-6480d4af844c // indirect
	github.com/elastic/go-ucfg v0.7.0
	github.com/elazarl/goproxy v0.0.0-20190711103511-473e67f1d7d2 // indirect
	github.com/elazarl/goproxy/ext v0.0.0-20190711103511-473e67f1d7d2 // indirect
	github.com/ghodss/yaml v1.0.0
	github.com/go-logr/logr v0.1.0
	github.com/go-test/deep v1.0.3
	github.com/gobuffalo/flect v0.2.0
	github.com/gogo/protobuf v1.3.1 // indirect
	github.com/golang/groupcache v0.0.0-20191002201903-404acd9df4cc // indirect
	github.com/google/go-cmp v0.3.1
	github.com/hashicorp/go-multierror v1.0.0
	github.com/hashicorp/golang-lru v0.5.3 // indirect
	github.com/hashicorp/vault/api v1.0.4
	github.com/huandu/xstrings v1.2.0 // indirect
	github.com/imdario/mergo v0.3.8
	github.com/influxdata/tdigest v0.0.1 // indirect
	github.com/magiconair/properties v1.8.1
	github.com/pelletier/go-toml v1.4.0 // indirect
	github.com/pkg/errors v0.8.1
	github.com/prometheus/client_golang v1.1.0 // indirect
	github.com/prometheus/client_model v0.0.0-20190812154241-14fe0d1b01d4 // indirect
	github.com/prometheus/common v0.7.0 // indirect
	github.com/prometheus/procfs v0.0.5 // indirect
	github.com/sethvargo/go-password v0.1.3
	github.com/spf13/cobra v0.0.5
	github.com/spf13/jwalterweatherman v1.1.0 // indirect
	github.com/spf13/pflag v1.0.5
	github.com/spf13/viper v1.4.0
	github.com/streadway/quantile v0.0.0-20150917103942-b0c588724d25 // indirect
	github.com/stretchr/testify v1.4.0
	github.com/tsenart/vegeta v12.7.0+incompatible
	go.elastic.co/apm v1.7.0
	go.elastic.co/apm/module/apmelasticsearch v1.7.0
	go.uber.org/automaxprocs v1.3.0
	go.uber.org/zap v1.12.0
	golang.org/x/crypto v0.0.0-20191011191535-87dc89f01550
	golang.org/x/net v0.0.0-20191011234655-491137f69257 // indirect
	golang.org/x/time v0.0.0-20190921001708-c4c64cad1fd0 // indirect
	golang.org/x/xerrors v0.0.0-20191011141410-1b5146add898 // indirect
	google.golang.org/appengine v1.6.5 // indirect
	gopkg.in/yaml.v2 v2.2.5
	gopkg.in/yaml.v3 v3.0.0-20191120175047-4206685974f2
	gotest.tools v2.2.0+incompatible
	k8s.io/api v0.17.2
	k8s.io/apimachinery v0.17.2
	k8s.io/client-go v0.17.2
	k8s.io/klog v1.0.0
	sigs.k8s.io/controller-runtime v0.5.0
	sigs.k8s.io/controller-tools v0.2.5
)

// this is used by vegeta, but the version they use is older and did not include a licence. we require the licence and so pin this
replace github.com/dgryski/go-gk => github.com/dgryski/go-gk v0.0.0-20200319235926-a69029f61654
