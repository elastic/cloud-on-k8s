// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package main

import (
	"context"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/elastic/k8s-operators/operators/pkg/controller/common/nodecerts"
	"github.com/elastic/k8s-operators/operators/pkg/controller/common/nodecerts/certutil"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/client"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/sidecar"
	"github.com/elastic/k8s-operators/operators/pkg/utils/fs"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"k8s.io/client-go/util/workqueue"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var (
	log                   = logf.Log.WithName("keystore-updater")
	sourceDirFlag         = envToFlag(sidecar.EnvSourceDir)
	keystoreBinaryFlag    = envToFlag(sidecar.EnvKeystoreBinary)
	keystorePathFlag      = envToFlag(sidecar.EnvKeystorePath)
	reloadCredentialsFlag = envToFlag(sidecar.EnvReloadCredentials)
	usernameFlag          = envToFlag(sidecar.EnvUsername)
	passwordFlag          = envToFlag(sidecar.EnvPassword)
	passwordFileFlag      = envToFlag(sidecar.EnvPasswordFile)
	endpointFlag          = envToFlag(sidecar.EnvEndpoint)
	certPathFlag          = envToFlag(sidecar.EnvCertPath)
	attemptReload         = "attempt-reload"

	cmd = &cobra.Command{
		Use: "keystore-updater",
		Run: func(cmd *cobra.Command, args []string) {
			execute()
		},
	}
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
	User client.User
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

func init() {
	cmd.Flags().StringP(sourceDirFlag, "s", "/volumes/secrets", "directory containing keystore settings source files")
	cmd.Flags().StringP(keystoreBinaryFlag, "b", "/usr/share/elasticsearch/bin/elasticsearch-keystore", "path to keystore binary")
	cmd.Flags().StringP(keystorePathFlag, "k", "/usr/share/elasticsearch/config/elasticsearch.keystore", "path to keystore file")
	cmd.Flags().BoolP(reloadCredentialsFlag, "r", false, "whether or not to trigger a credential reload in Elasticsearch")
	cmd.Flags().StringP(usernameFlag, "u", "", "Elasticsearch username to reload credentials")
	cmd.Flags().StringP(passwordFlag, "p", "", "Elasticsearch password to reload credentials")
	cmd.Flags().StringP(endpointFlag, "e", "https://127.0.0.1:9200", "Elasticsearch endpoint to reload credentials")
	cmd.Flags().StringP(certPathFlag, "c", path.Join("/volume/node-certs", nodecerts.CAFileName), "Path to the CA certificate to connect to Elasticsearch")

	if err := viper.BindPFlags(cmd.Flags()); err != nil {
		fatal(err, "Unexpected error while binding flags")
	}
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	viper.AutomaticEnv()

}

func fatal(err error, msg string) {
	log.Error(err, msg)
	os.Exit(1)
}

// coalescingRetry attempts to reload the keystore coalescing subsequent requests into one when retrying.
func coalescingRetry(cfg Config) {
	shutdown := false
	var item interface{}
	for !shutdown {
		item, shutdown = cfg.ReloadQueue.Get()
		err := reloadCredentials(cfg)
		if err != nil {
			log.Error(err, "Error reloading credentials. Continuing.")
			cfg.ReloadQueue.AddAfter(item, 5*time.Second) // TODO exp. backoff w/ jitter
		} else {
			log.Info("Successfully reloaded credentials")
		}
		cfg.ReloadQueue.Done(item)
	}
}

func loadCerts(caCertPath string) ([]*x509.Certificate, error) {
	bytes, err := ioutil.ReadFile(caCertPath)
	if err != nil {
		return nil, err
	}
	return certutil.ParsePEMCerts(bytes)
}

// reloadCredentials tries to make an API call to the reload_secure_credentials API
// to reload reloadable settings after the keystore has been updated.
func reloadCredentials(cfg Config) error {
	caCerts, err := loadCerts(cfg.CACertsPath)
	if err != nil {
		fatal(err, "Cannot create Elasticsearch client with CA certs")
	}
	api := client.NewElasticsearchClient(nil, cfg.Endpoint, cfg.User, caCerts)
	// TODO this is problematic as this call is supposed to happen only when all nodes have the updated
	// keystore which is something we cannot guarantee from this process. Also this call will be issued
	// on each node which is redundant and might be problematic as well.
	return api.ReloadSecureSettings(context.Background())
}

// updateKeystore reconciles the source directory with Elasticsearch keystores by recreating the
// keystore and adding a setting for each file in the source directory.
func updateKeystore(cfg Config) {
	// delete existing keystore (TODO can we do that to a running cluster?)
	_, err := os.Stat(cfg.KeystorePath)
	if !os.IsNotExist(err) {
		log.Info("Removing keystore", "keystore-path", cfg.KeystorePath)
		err := os.Remove(cfg.KeystorePath)
		if err != nil {
			fatal(err, "could not delete keystore file")
		}
	}

	log.Info("Creating keystore", "keystore-path", cfg.KeystorePath)
	create := exec.Command(cfg.KeystoreBinary, "create", "--silent")
	create.Dir = filepath.Dir(cfg.KeystorePath)
	err = create.Run()
	if err != nil {
		fatal(err, "could not create new keystore")
	}

	fileInfos, err := ioutil.ReadDir(cfg.SourceDir)
	if err != nil {
		fatal(err, "could not read source directory")
	}
	for _, file := range fileInfos {
		if strings.HasPrefix(file.Name(), ".") {
			log.Info(fmt.Sprintf("Ignoring %s", file.Name()))
			continue
		}
		log.Info("Adding setting to keystore", "file", file.Name())
		add := exec.Command(cfg.KeystoreBinary, "add-file", file.Name(), path.Join(cfg.SourceDir, file.Name()))
		err := add.Run()
		if err != nil {
			fatal(err, fmt.Sprintf("could not add setting %s", file.Name()))
		}
	}

	list := exec.Command(cfg.KeystoreBinary, "list")
	bytes, err := list.Output()
	if err != nil {
		fatal(err, "error during listing keystore settings")
	}

	re := regexp.MustCompile(`\r?\n`)
	input := re.ReplaceAllString(string(bytes), " ")
	log.Info("keystore updated", "settings", input)
	if cfg.ReloadCredentials {
		cfg.ReloadQueue.Add(attemptReload)
	}
}

// validateConfig validates the configuration parameters for the keystore-updater and ends execution if invalid.
func validateConfig() Config {
	sourceDir := viper.GetString(sourceDirFlag)
	_, err := os.Stat(sourceDir)
	if os.IsNotExist(err) {
		fatal(err, "source directory does not exist")
	}
	keystoreBinary := viper.GetString(keystoreBinaryFlag)
	_, err = os.Stat(keystoreBinary)
	if os.IsNotExist(err) {
		fatal(err, "keystore binary does not exist")
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
				fatal(err, fmt.Sprintf("password file %s could not be read", passwordFile))
			}
			pass = string(bytes)
		}

		if user == "" || pass == "" {
			passwordFeedback := pass
			if pass != "" {
				passwordFeedback = "REDACTED"
			}
			fatal(
				fmt.Errorf(
					"user and password are required but found username: %s password:%s",
					user,
					passwordFeedback,
				),
				"Invalid config",
			)
		}
		config.User = client.User{
			Name:     user,
			Password: pass,
		}

		caCerts := viper.GetString(certPathFlag)
		_, err := loadCerts(caCerts)
		if err != nil {
			fatal(err, "CA certificates are required when reloading credentials but could not be read")
		}
		config.CACertsPath = caCerts
		config.Endpoint = viper.GetString(endpointFlag)
	}
	return config
}

// execute updates the keystore once and then starts a watcher on source dir to update again on file changes.
func execute() {
	config := validateConfig()

	if config.ReloadCredentials {
		go coalescingRetry(config)
	}

	// on each filesystem event for config.SourceDir, update the keystore
	onEvent := func() (stop bool, err error) {
		updateKeystore(config)
		return false, nil // run forever
	}
	if err := fs.WatchPath(config.SourceDir, onEvent, log); err != nil {
		log.Error(err, "Cannot watch filesystem", "path", config.SourceDir)
	}
}

func main() {
	logf.SetLogger(logf.ZapLogger(false))
	if err := cmd.Execute(); err != nil {
		log.Error(err, "Unexpected error while running command")
	}
}
