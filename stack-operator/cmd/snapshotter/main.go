package snapshotter

import (
	"crypto/x509"
	"fmt"
	"strings"
	"time"

	"io/ioutil"
	"os"

	esclient "github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/client"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/snapshots"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/pkg/errors"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var (
	log                     = logf.Log.WithName("snapshotter")
	certificateLocationFlag = strings.ToLower(snapshots.CertificateLocationVar)
	userNameFlag            = strings.ToLower(snapshots.UserNameVar)
	userPasswordFlag        = strings.ToLower(snapshots.UserPasswordVar)
	intervalFlag            = strings.ToLower(snapshots.IntervalVar)
	maxFlag                 = strings.ToLower(snapshots.MaxVar)
	esURLFlag               = strings.ToLower(snapshots.EsURLVar)
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

func init() {
	Cmd.Flags().StringP(certificateLocationFlag, "c", "", "Location of cacerts in local filesystem")
	Cmd.Flags().StringP(esURLFlag, "e", "", "Elasticsearch URL")
	Cmd.Flags().StringP(userNameFlag, "u", "", "Elasticsearch user name")
	Cmd.Flags().StringP(userPasswordFlag, "p", "", "Elasticsearch password")
	Cmd.Flags().DurationP(intervalFlag, "d", 30*time.Minute, "Snapshot interval")
	Cmd.Flags().IntP(maxFlag, "m", 100, "Max number of snaphshots retained")

	if err := viper.BindPFlags(Cmd.Flags()); err != nil {
		log.Error(err, "Unexpected error while binding flags")
		os.Exit(1)
	}

	viper.AutomaticEnv()
}

func execute() {

	userName := viper.GetString(userNameFlag)
	userPassword := viper.GetString(userPasswordFlag)
	user := esclient.User{Name: userName, Password: userPassword}

	certCfg := viper.GetString(certificateLocationFlag)
	certPool := x509.NewCertPool()
	if certCfg != "" {
		pemCerts, err := ioutil.ReadFile(certCfg)
		if err != nil {
			unrecoverable(errors.Wrap(err, "Could not read ca certificate"))
		}
		certPool.AppendCertsFromPEM(pemCerts)
	}

	esURL := viper.GetString(esURLFlag)
	if esURL == "" {
		unrecoverable(errors.New(fmt.Sprintf("%s is required", esURLFlag)))
	}
	apiClient := esclient.NewElasticsearchClient(nil, esURL, user, certPool)

	interval := viper.GetDuration(intervalFlag)
	max := viper.GetInt(maxFlag)
	settings := snapshots.Settings{
		Interval:   interval,
		Max:        max,
		Repository: "elastic-snapshots",
	}

	log.Info(fmt.Sprintf("Snapshotter initialised with [%+v]", settings))
	err := snapshots.ExecuteNextPhase(apiClient, settings)
	if err != nil {
		unrecoverable(errors.Wrap(err, "Error during snapshot maintenance"))
	}

}
