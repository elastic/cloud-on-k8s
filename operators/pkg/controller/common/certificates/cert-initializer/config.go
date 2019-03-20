package certinitializer

import (
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/initcontainer"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/nodecerts"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/volume"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"path"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
)

const (
	PortFlag           = "csr-port"
	PrivateKeyPathFlag = "csr-private-key-path"
	CertPathFlag       = "csr-cert-path"
	CSRPathFlag        = "csr-csr-path"

	EnvPort           = "CSR_HTTP_PORT"
	EnvPrivateKeyPath = "CSR_PRIVATE_KEY_PATH"
	EnvCertPath       = "CSR_CERT_PATH"
	EnvCSRPath        = "CSR_PATH"
)

// Config for the cert-initializer.
type Config struct {
	Port           int
	PrivateKeyPath string
	CertPath       string
	CSRPath        string
}

func BindEnv(cmd *cobra.Command) error {
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
	viper.AutomaticEnv()

	return viper.BindPFlags(cmd.Flags())
}

func NewConfig() Config {
	return Config{
		Port:           viper.GetInt(PortFlag),
		PrivateKeyPath: viper.GetString(PrivateKeyPathFlag),
		CertPath:       viper.GetString(CertPathFlag),
		CSRPath:        viper.GetString(CSRPathFlag),
	}
}

func NewEnvVars() []corev1.EnvVar {
	return []corev1.EnvVar{
		{Name: EnvPort, Value: strconv.Itoa(initcontainer.CertInitializerPort)},
		{Name: EnvPrivateKeyPath, Value: path.Join(initcontainer.PrivateKeySharedVolume.InitContainerMountPath, initcontainer.PrivateKeyFileName)},
		{Name: EnvCertPath, Value: path.Join(volume.NodeCertificatesSecretVolumeMountPath, nodecerts.CertFileName)},
		{Name: EnvCSRPath, Value: path.Join(volume.NodeCertificatesSecretVolumeMountPath, nodecerts.CSRFileName)},
	}
}
