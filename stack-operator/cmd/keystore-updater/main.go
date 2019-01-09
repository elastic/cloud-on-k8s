package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/elastic/stack-operators/stack-operator/pkg/controller/common/nodecerts/certutil"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/client"

	"github.com/fsnotify/fsnotify"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var (
	log                   = logf.Log.WithName("keystore-updater")
	sourceDirFlag         = "source-dir"
	keystoreBinaryFlag    = "keystore-binary"
	keystorePathFlag      = "keystore-path"
	reloadCredentialsFlag = "reload-credentials"
	usernameFlag          = "username"
	passwordFlag          = "password"
	endpointFlag          = "endpoint"
	certPathFlag          = "certificates-path"

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
	// CACerts contains the CA certificate chain to call the Elasticsearch API. Can be empty if ReloadCredentials is false.
	CACerts []byte
}

func init() {
	cmd.Flags().StringP(sourceDirFlag, "s", "/volumes/secrets", "directory containing keystore settings source files")
	cmd.Flags().StringP(keystoreBinaryFlag, "b", "/usr/share/elasticsearch/bin/elasticsearch-keystore", "path to keystore binary")
	cmd.Flags().StringP(keystorePathFlag, "k", "/usr/share/elasticsearch/config/elasticsearch.keystore", "path to keystore file")
	cmd.Flags().BoolP(reloadCredentialsFlag, "r", false, "whether or not to trigger a credential reload in Elasticsearch")
	cmd.Flags().StringP(usernameFlag, "u", "", "Elasticsearch username to reload credentials")
	cmd.Flags().StringP(passwordFlag, "p", "", "Elasticsearch password to reload credentials")
	cmd.Flags().StringP(endpointFlag, "e", "https://127.0.0.1:9200", "Elasticsearch endpoint to reload credentials")
	cmd.Flags().StringP(certPathFlag, "c", "/volume/node-certs/ca.pem", "Path to CA certificate to connect to Elasticsearch")

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

// reloadCredentials tries to make an API call to the reload_secure_credentials API
// to reload reloadable settings after the keystore has been updated.
func reloadCredentials(cfg Config) error {
	caCerts, err := certutil.ParsePEMCerts(cfg.CACerts)
	if err != nil {
		fatal(err, "Cannot create Elasticsearch client from CA cert")
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
		err := reloadCredentials(cfg)
		if err != nil {
			log.Error(err, "Error reloading credentials. Continuing.")
		}
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
	user := viper.GetString(usernameFlag)
	pass := viper.GetString(passwordFlag)
	endpoint := viper.GetString(endpointFlag)
	log.Info("should reload", "?", shouldReload)
	caCerts := viper.GetString(certPathFlag)
	if shouldReload && (user == "" || pass == "") {
		fatal(
			fmt.Errorf("user and password are required but found username: %s password:%s", user, strings.Repeat("*", len(pass))),
			"Invalid config",
		)
	}

	var certificates []byte
	if shouldReload {
		certificates, err = ioutil.ReadFile(caCerts)
		if err != nil {
			fatal(err, "CA certificates are required when reloading credentials but could not be read")
		}
	}

	return Config{
		SourceDir:         sourceDir,
		KeystoreBinary:    keystoreBinary,
		KeystorePath:      viper.GetString(keystorePathFlag),
		ReloadCredentials: shouldReload,
		Endpoint:          endpoint,
		CACerts:           certificates,
		User: client.User{
			Name:     user,
			Password: pass,
		},
	}

}

// execute updates the keystore once and then starts a watcher on source dir to update again on file changes.
func execute() {
	config := validateConfig()

	//initial update
	updateKeystore(config)

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		fatal(err, "Failed to create watcher")
	}
	defer watcher.Close()

	done := make(chan bool)
	go func() {
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				log.Info("Observed:", "event", event)
				updateKeystore(config)
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				log.Error(err, "watcher error")
			}
		}
	}()

	err = watcher.Add(config.SourceDir)
	if err != nil {
		fatal(err, fmt.Sprintf("failed to add watch on %s", config.SourceDir))
	}
	<-done

}

func main() {
	logf.SetLogger(logf.ZapLogger(false))
	if err := cmd.Execute(); err != nil {
		log.Error(err, "Unexpected error while running command")
	}
}
