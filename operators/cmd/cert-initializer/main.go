package main

import (
	"os"
	"path"
	"strings"

	"github.com/elastic/k8s-operators/operators/pkg/controller/common/nodecerts"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/initcontainer"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/volume"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

// Flags parsed from command line arguments or environment variables.
const (
	PortFlag           = "port"
	PrivateKeyPathFlag = "private-key-path"
	CertPathFlag       = "cert-path"
	CSRPathFlag        = "csr-path"
)

var log = logf.Log.WithName("certificate-initializer")

// Config for the cert-initializer.
type Config struct {
	Port           int
	PrivateKeyPath string
	CertPath       string
	CSRPath        string
}

func setupCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cert-initializer",
		Short: "Start the certificate initializer",
		Long:  `Start an HTTP server serving a generated CSR`,
		Run: func(cmd *cobra.Command, args []string) {
			config := Config{
				Port:           viper.GetInt(PortFlag),
				PrivateKeyPath: viper.GetString(PrivateKeyPathFlag),
				CertPath:       viper.GetString(CertPathFlag),
				CSRPath:        viper.GetString(CSRPathFlag),
			}
			execute(config)
		},
	}
	cmd.Flags().Int(
		PortFlag,
		initcontainer.CertInitializerPort,
		"HTTP port to listen on",
	)
	cmd.Flags().String(PrivateKeyPathFlag,
		path.Join(initcontainer.PrivateKeySharedVolume.InitContainerMountPath, initcontainer.PrivateKeyFileName),
		"Path to the private key file",
	)
	cmd.Flags().String(CertPathFlag,
		path.Join(volume.NodeCertificatesSecretVolumeMountPath, nodecerts.CertFileName),
		"Path to the cert file",
	)
	cmd.Flags().String(CSRPathFlag,
		path.Join(volume.NodeCertificatesSecretVolumeMountPath, nodecerts.CSRFileName),
		"Path to the CSR file",
	)

	// bind flags to environment variables
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	exitOnErr(viper.BindPFlags(cmd.Flags()))
	viper.AutomaticEnv()

	return cmd
}

func main() {
	logf.SetLogger(logf.ZapLogger(true))
	exitOnErr(setupCmd().Execute())
}

// execute the main program (see README.md for details).
func execute(config Config) {
	if checkExistingOnDisk(config) {
		log.Info("Reusing existing private key, CSR and certificate")
		return
	}

	log.Info("Creating a private key on disk")
	privateKey, err := createAndStorePrivateKey(config.PrivateKeyPath)
	exitOnErr(err)

	log.Info("Generating a CSR from the private key")
	csr, err := createCSR(privateKey)
	exitOnErr(err)

	log.Info("Serving CSR over HTTP", "port", config.Port)
	stopChan := make(chan struct{})
	defer close(stopChan)
	go func() {
		exitOnErr(serveCSR(config.Port, csr, stopChan))
	}()

	log.Info("Watching filesystem for cert update")
	exitOnErr(watchForCertUpdate(config))

	log.Info("Certificate initialization successful")
}

// exitOnErr exits the program if err exists.
func exitOnErr(err error) {
	if err != nil {
		log.Error(err, "Fatal error")
		os.Exit(1)
	}
}
