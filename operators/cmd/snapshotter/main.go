package snapshotter

import (
	"crypto/x509"
	"fmt"
	"strings"
	"time"

	"github.com/elastic/k8s-operators/stack-operator/pkg/controller/common/nodecerts/certutil"

	"io/ioutil"
	"os"

	esclient "github.com/elastic/k8s-operators/stack-operator/pkg/controller/elasticsearch/client"
	"github.com/elastic/k8s-operators/stack-operator/pkg/controller/elasticsearch/snapshot"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/pkg/errors"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var (
	log                     = logf.Log.WithName("snapshotter")
	certificateLocationFlag = strings.ToLower(snapshot.CertificateLocationVar)
	userNameFlag            = strings.ToLower(snapshot.UserNameVar)
	userPasswordFlag        = strings.ToLower(snapshot.UserPasswordVar)
	intervalFlag            = strings.ToLower(snapshot.IntervalVar)
	maxFlag                 = strings.ToLower(snapshot.MaxVar)
	esURLFlag               = strings.ToLower(snapshot.EsURLVar)
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
	Cmd.Flags().IntP(maxFlag, "m", 100, "Max number of snapshots retained")

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
	var certs []*x509.Certificate
	if certCfg != "" {
		pemData, err := ioutil.ReadFile(certCfg)
		if err != nil {
			unrecoverable(errors.Wrap(err, "Could not read ca certificate"))
		}
		certs, err = certutil.ParsePEMCerts(pemData)
		if err != nil {
			unrecoverable(errors.Wrap(err, "Could not parse ca certificate"))
		}
	}

	esURL := viper.GetString(esURLFlag)
	if esURL == "" {
		unrecoverable(errors.New(fmt.Sprintf("%s is required", esURLFlag)))
	}
	apiClient := esclient.NewElasticsearchClient(nil, esURL, user, certs)

	interval := viper.GetDuration(intervalFlag)
	max := viper.GetInt(maxFlag)
	settings := snapshot.Settings{
		Interval:   interval,
		Max:        max,
		Repository: "elastic-snapshots",
	}

	log.Info(fmt.Sprintf("Snapshotter initialised with [%+v]", settings))
	err := snapshot.ExecuteNextPhase(apiClient, settings)
	if err != nil {
		unrecoverable(errors.Wrap(err, "Error during snapshot maintenance"))
	}

}
