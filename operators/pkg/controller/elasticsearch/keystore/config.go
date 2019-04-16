// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package keystore

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"strings"

	"github.com/elastic/k8s-operators/operators/pkg/controller/common/certificates"
	"github.com/elastic/k8s-operators/operators/pkg/controller/common/version"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/client"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"k8s.io/client-go/util/workqueue"
)

const (
	KeystoreBinPath = "/usr/share/elasticsearch/bin/elasticsearch-keystore"
)

var (
	sourceDirFlag         = envToFlag(EnvSourceDir)
	keystoreBinaryFlag    = envToFlag(EnvKeystoreBinary)
	keystorePathFlag      = envToFlag(EnvKeystorePath)
	reloadCredentialsFlag = envToFlag(EnvReloadCredentials)
	esUsernameFlag        = envToFlag(EnvEsUsername)
	esPasswordFlag        = envToFlag(EnvEsPassword)
	esPasswordFileFlag    = envToFlag(EnvEsPasswordFile)
	esEndpointFlag        = envToFlag(EnvEsEndpoint)
	esCaCertsPathFlag     = envToFlag(EnvEsCaCertsPath)
	esVersionFlag         = envToFlag(EnvEsVersion)
)

// Config contains configuration parameters for the keystore updater.
type Config struct {
	// SecretsSourceDir is the directory where secrets will appear that need to be added to the keystore.
	SecretsSourceDir string
	// KeystoreBinary is the path to the Elasticsearch keystore tool binary.
	KeystoreBinary string
	// KeystorePath is the path to the Elasticsearch keystore file.
	KeystorePath string
	// ReloadCredentials indicates whether the updater should attempt to reload secure settings in Elasticsearch.
	ReloadCredentials bool
	// EsUsername is the Elasticsearch username for API calls.
	EsUsername string
	// EsPasswordFile is the file for the Elasticsearch password for API calls.
	EsPasswordFile string
	// Endpoint is the Elasticsearch endpoint for API calls. Can be empty if ReloadCredentials is false.
	EsEndpoint string
	// EsVersion is the Elasticsearch version.
	EsVersion version.Version
	// EsCACertsPath points to the CA certificate chain to call the Elasticsearch API.
	EsCACertsPath string
	// EsUser is the Elasticsearch user for the reload secure settings API call. Can be empty if ReloadCredentials is false.
	EsUser client.UserAuth
	// ReloadQueue is a channel to schedule config reload requests
	ReloadQueue workqueue.DelayingInterface
}

// envToFlag reverses viper's autoenv so that we can specify ENV variables as constants and derive flags from them.
func envToFlag(env string) string {
	return strings.Replace(strings.ToLower(env), "_", "-", -1)
}

// BindEnvToFlags binds flags to environment variables.
func BindEnvToFlags(cmd *cobra.Command) error {
	cmd.Flags().StringP(sourceDirFlag, "s", "/volumes/secrets", "directory containing keystore settings source files")
	cmd.Flags().StringP(keystoreBinaryFlag, "b", KeystoreBinPath, "path to keystore binary")
	cmd.Flags().StringP(keystorePathFlag, "k", "/usr/share/elasticsearch/config/elasticsearch.keystore", "path to keystore file")
	cmd.Flags().BoolP(reloadCredentialsFlag, "r", false, "whether or not to trigger a credential reload in Elasticsearch")
	cmd.Flags().StringP(esUsernameFlag, "u", "", "Elasticsearch username to reload credentials")
	cmd.Flags().StringP(esPasswordFlag, "p", "", "Elasticsearch password to reload credentials")
	cmd.Flags().StringP(esEndpointFlag, "e", "https://127.0.0.1:9200", "Elasticsearch endpoint to reload credentials")
	cmd.Flags().String(esVersionFlag, "", "Elasticsearch version")
	cmd.Flags().StringP(esCaCertsPathFlag, "c", path.Join("/volume/node-certs", certificates.CAFileName), "Path to the CA certificate to connect to Elasticsearch")

	if err := viper.BindPFlags(cmd.Flags()); err != nil {
		return err
	}
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	viper.AutomaticEnv()

	return nil
}

// NewConfigFromFlags creates a new configuration from the flags.
func NewConfigFromFlags() (Config, error, string) {
	sourceDir := viper.GetString(sourceDirFlag)
	_, err := os.Stat(sourceDir)
	if os.IsNotExist(err) {
		return Config{}, err, "source directory does not exist"
	}
	keystoreBinary := viper.GetString(keystoreBinaryFlag)
	_, err = os.Stat(keystoreBinary)
	if os.IsNotExist(err) {
		return Config{}, err, "keystore binary does not exist"
	}

	v, err := version.Parse(viper.GetString(esVersionFlag))
	if err != nil {
		return Config{}, err, "no or invalid version"
	}

	shouldReload := viper.GetBool(reloadCredentialsFlag)
	config := Config{
		SecretsSourceDir:  sourceDir,
		KeystoreBinary:    keystoreBinary,
		KeystorePath:      viper.GetString(keystorePathFlag),
		ReloadCredentials: shouldReload,
		ReloadQueue:       workqueue.NewDelayingQueue(),
		EsVersion:         *v,
	}

	if shouldReload {
		user := viper.GetString(esUsernameFlag)
		pass := viper.GetString(esPasswordFlag)

		if pass == "" {
			passwordFile := viper.GetString(esPasswordFileFlag)
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
		config.EsUser = client.UserAuth{Name: user, Password: pass}

		caCerts := viper.GetString(esCaCertsPathFlag)
		_, err := loadCerts(caCerts)
		if err != nil {
			return Config{}, err, "CA certificates are required when reloading credentials but could not be read"
		}
		config.EsCACertsPath = caCerts
		config.EsEndpoint = viper.GetString(esEndpointFlag)
	}
	return config, nil, ""
}
