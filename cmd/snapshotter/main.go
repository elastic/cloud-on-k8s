package main

import (
	"crypto/x509"
	"time"

	"io/ioutil"
	"os"

	esclient "github.com/elastic/stack-operators/pkg/controller/stack/elasticsearch/client"
	"github.com/elastic/stack-operators/pkg/controller/stack/elasticsearch/snapshots"

	"github.com/pkg/errors"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

const (
	certificateLocationVar = "CERTIFICATE_LOCATION"
	userNameVar            = "USER"
	userPasswordVar        = "PASSWORD"
	esURLVar               = "ELASTICSEARCH_URL"
)

var (
	log = logf.Log.WithName("main")
)

func unrecoverable(err error) {
	log.Error(err, "unrecoverable error, exiting")
	os.Exit(1)
}

func main() {
	logf.SetLogger(logf.ZapLogger(false))
	certCfg, ok := os.LookupEnv(certificateLocationVar)
	if !ok {
		unrecoverable(errors.New("No certificate config configured")) // TODO should this be actually optional?
	}
	esURL, ok := os.LookupEnv(esURLVar)
	if !ok {
		unrecoverable(errors.New("No Elasticsearch URL configured"))
	}
	userName, ok := os.LookupEnv(userNameVar)
	if !ok {
		unrecoverable(errors.New("No Elasticsearch user configured"))
	}

	userPassword, ok := os.LookupEnv(userPasswordVar)
	if !ok {
		unrecoverable(errors.New("No password for Elasticsearch user configured"))
	}

	user := esclient.User{Name: userName, Password: userPassword}

	pemCerts, err := ioutil.ReadFile(certCfg)
	if err != nil {
		unrecoverable(errors.Wrap(err, "Could not read ca certificate"))
	}
	certPool := x509.NewCertPool()
	certPool.AppendCertsFromPEM(pemCerts)
	apiClient := esclient.NewElasticsearchClient(esURL, user, certPool)

	// TODO read from config
	settings := snapshots.Settings{
		Interval:   30 * time.Minute,
		Max:        100,
		Repository: "elastic-snapshots",
	}

	err = snapshots.Maintain(apiClient, settings)
	if err != nil {
		unrecoverable(errors.Wrap(err, "Error during snapshot maintenance"))
	}

}
