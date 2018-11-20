package snapshotter

import (
	"crypto/x509"
	"fmt"
	"strconv"
	"time"

	"io/ioutil"
	"os"

	esclient "github.com/elastic/stack-operators/pkg/controller/stack/elasticsearch/client"
	"github.com/elastic/stack-operators/pkg/controller/stack/elasticsearch/snapshots"
	"github.com/spf13/cobra"

	"github.com/pkg/errors"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var (
	log = logf.Log.WithName("snapshotter")
	// Cmd is the cobra command to start a snapshotter run
	Cmd = &cobra.Command{
		Use:   "snapshotter",
		Short: "Start a run of the snapshotter",
		Long: `snapshotter starts a run of the snapshotter process.
This should typically be run in the context of some form of scheduler.`,
		Run: func(cmd *cobra.Command, args []string) {
			execute()
		},
	}
)

func unrecoverable(err error) {
	log.Error(err, "unrecoverable error, exiting")
	os.Exit(1)
}

func execute() {
	certCfg, ok := os.LookupEnv(snapshots.CertificateLocationVar)
	if !ok {
		unrecoverable(errors.New("No certificate config configured")) // TODO should this be actually optional?
	}
	esURL, ok := os.LookupEnv(snapshots.EsURLVar)
	if !ok {
		unrecoverable(errors.New("No Elasticsearch URL configured"))
	}
	userName, ok := os.LookupEnv(snapshots.UserNameVar)
	if !ok {
		unrecoverable(errors.New("No Elasticsearch user configured"))
	}

	userPassword, ok := os.LookupEnv(snapshots.UserPasswordVar)
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

	interval := 30 * time.Minute
	intervalStr, _ := os.LookupEnv(snapshots.IntervalVar)
	if intervalStr != "" {
		parsed, err := time.ParseDuration(intervalStr)
		if err != nil {
			log.Error(err, "could not parse interval: "+intervalStr)
		}
		interval = parsed
	}

	max := 100
	maxStr, _ := os.LookupEnv(snapshots.MaxVar)
	if maxStr != "" {
		parsed, err := strconv.Atoi(maxStr)
		if err != nil {
			log.Error(err, "could not parse max: "+maxStr)
		}
		max = parsed
	}

	settings := snapshots.Settings{
		Interval:   interval,
		Max:        max,
		Repository: "elastic-snapshots",
	}

	log.Info(fmt.Sprintf("Snapshotter initialised with interval: [%v], max snapshots: [%d], repository: [%s]",
		settings.Interval, settings.Max, settings.Repository,
	))
	err = snapshots.Maintain(apiClient, settings)
	if err != nil {
		unrecoverable(errors.Wrap(err, "Error during snapshot maintenance"))
	}

}
