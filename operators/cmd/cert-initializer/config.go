// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package main

import (
	"path"
	"strings"

	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/initcontainer"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/volume"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const (
	PortFlag           = "port"
	PrivateKeyPathFlag = "private-key-path"
	CertPathFlag       = "cert-path"
	CSRPathFlag        = "csr-path"
)

// Config contains configuration parameters for the cert initializer.
type Config struct {
	Port           int
	PrivateKeyPath string
	CertPath       string
	CSRPath        string
}

// BindEnvFromFlags binds flags to environment variables.
func BindEnvFromFlags(cmd *cobra.Command) error {
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
		path.Join(volume.TransportCertificatesSecretVolumeMountPath, certificates.CertFileName),
		"Path to the cert file",
	)
	cmd.Flags().String(CSRPathFlag,
		path.Join(volume.TransportCertificatesSecretVolumeMountPath, certificates.CSRFileName),
		"Path to the CSR file",
	)

	// bind flags to environment variables
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	viper.AutomaticEnv()

	return viper.BindPFlags(cmd.Flags())
}

// NewConfigFromFlags creates a new configuration from the flags.
func NewConfigFromFlags() Config {
	return Config{
		Port:           viper.GetInt(PortFlag),
		PrivateKeyPath: viper.GetString(PrivateKeyPathFlag),
		CertPath:       viper.GetString(CertPathFlag),
		CSRPath:        viper.GetString(CSRPathFlag),
	}
}
