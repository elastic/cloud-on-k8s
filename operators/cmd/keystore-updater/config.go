package keystore

import (
	"fmt"
	"github.com/elastic/k8s-operators/operators/pkg/controller/common/certificates"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/client"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/sidecar"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"io/ioutil"
	"k8s.io/client-go/util/workqueue"
	"os"
	"path"
	"strings"
)

var (
	sourceDirFlag         = envToFlag(sidecar.EnvSourceDir)
	keystoreBinaryFlag    = envToFlag(sidecar.EnvKeystoreBinary)
	keystorePathFlag      = envToFlag(sidecar.EnvKeystorePath)
	reloadCredentialsFlag = envToFlag(sidecar.EnvReloadCredentials)
	usernameFlag          = envToFlag(sidecar.EnvUsername)
	passwordFlag          = envToFlag(sidecar.EnvPassword)
	passwordFileFlag      = envToFlag(sidecar.EnvPasswordFile)
	endpointFlag          = envToFlag(sidecar.EnvEndpoint)
	certPathFlag          = envToFlag(sidecar.EnvCertPath)
)

// Config contains configuration parameters for the keystore updater.
type Config struct {
	// SourceDir is the directory where secrets will appear that need to be added to the keystore.
	SourceDir string
	// KeystoreBinary is the path to the Elasticsearch keystore tool binary.
	KeystoreBinary string
	// KeystorePath is the path to the Elasticsearch keystore file.
	KeystorePath string
	// ReloadCredentials indicates whether the updater should attempt to reload secure settings in Elasticsearch.
	ReloadCredentials bool
	// User is the Elasticsearch user for the reload secure settings API call. Can be empty if ReloadCredentials is false.
	User client.UserAuth
	// Endpoint is the Elasticsearch endpoint for API calls. Can be empty if ReloadCredentials is false.
	Endpoint string
	// CACertsPath points to the CA certificate chain to call the Elasticsearch API.
	CACertsPath string
	// ReloadQueue is a channel to schedule config reload requests
	ReloadQueue workqueue.DelayingInterface
}

// envToFlag reverses viper's autoenv so that we can specify ENV variables as constants and derive flags from them.
func envToFlag(env string) string {
	return strings.Replace(strings.ToLower(env), "_", "-", -1)
}

func parseFlags(cmd *cobra.Command) (error, string) {
	cmd.Flags().StringP(sourceDirFlag, "s", "/volumes/secrets", "directory containing keystore settings source files")
	cmd.Flags().StringP(keystoreBinaryFlag, "b", "/usr/share/elasticsearch/bin/elasticsearch-keystore", "path to keystore binary")
	cmd.Flags().StringP(keystorePathFlag, "k", "/usr/share/elasticsearch/config/elasticsearch.keystore", "path to keystore file")
	cmd.Flags().BoolP(reloadCredentialsFlag, "r", false, "whether or not to trigger a credential reload in Elasticsearch")
	cmd.Flags().StringP(usernameFlag, "u", "", "Elasticsearch username to reload credentials")
	cmd.Flags().StringP(passwordFlag, "p", "", "Elasticsearch password to reload credentials")
	cmd.Flags().StringP(endpointFlag, "e", "https://127.0.0.1:9200", "Elasticsearch endpoint to reload credentials")
	cmd.Flags().StringP(certPathFlag, "c", path.Join("/volume/node-certs", certificates.CAFileName), "Path to the CA certificate to connect to Elasticsearch")

	if err := viper.BindPFlags(cmd.Flags()); err != nil {
		return err, "Unexpected error while binding flags"
	}
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	viper.AutomaticEnv()

	return nil, ""
}

// GetConfig validates the configuration parameters for the keystore-updater and ends execution if invalid.
func NewConfigFromFlags(cmd *cobra.Command) (Config, error, string) {
	if cmd == nil {
		return Config{}, nil, ""
	}

	err, msg := parseFlags(cmd)
	if err != nil {
		return Config{}, err, msg
	}

	sourceDir := viper.GetString(sourceDirFlag)
	_, err = os.Stat(sourceDir)
	if os.IsNotExist(err) {
		return Config{}, err, "source directory does not exist"
	}
	keystoreBinary := viper.GetString(keystoreBinaryFlag)
	_, err = os.Stat(keystoreBinary)
	if os.IsNotExist(err) {
		return Config{}, err, "keystore binary does not exist"
	}
	shouldReload := viper.GetBool(reloadCredentialsFlag)
	config := Config{
		SourceDir:         sourceDir,
		KeystoreBinary:    keystoreBinary,
		KeystorePath:      viper.GetString(keystorePathFlag),
		ReloadCredentials: shouldReload,
		ReloadQueue:       workqueue.NewDelayingQueue(),
	}

	if shouldReload {
		user := viper.GetString(usernameFlag)
		pass := viper.GetString(passwordFlag)

		if pass == "" {
			passwordFile := viper.GetString(passwordFileFlag)
			bytes, err := ioutil.ReadFile(passwordFile)
			if err != nil {
				return Config{}, err, fmt.Sprintf("password file %s could not be read", passwordFile)
			}
			pass = string(bytes)
		}

		if user == "" || pass == "" {
			passwordFeedback := pass
			if pass != "" {
				passwordFeedback = "REDACTED"
			}
			return Config{},
				fmt.Errorf(
					"user and password are required but found username:%s password:%s",
					user,
					passwordFeedback,
				),
				"Invalid config"
		}
		config.User = client.UserAuth{Name: user, Password: pass}

		caCerts := viper.GetString(certPathFlag)
		_, err := loadCerts(caCerts)
		if err != nil {
			return Config{}, err, "CA certificates are required when reloading credentials but could not be read"
		}
		config.CACertsPath = caCerts
		config.Endpoint = viper.GetString(endpointFlag)
	}
	return config, nil, ""
}
