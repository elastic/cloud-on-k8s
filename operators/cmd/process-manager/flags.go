// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package main

import (
	"fmt"
	"strings"

	pm "github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/processmanager"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	procNameFlag        = envToFlag(pm.EnvProcName)
	procCmdFlag         = envToFlag(pm.EnvProcCmd)
	reaperFlag          = envToFlag(pm.EnvReaper)
	HTTPPortFlag        = envToFlag(pm.EnvHTTPPort)
	tlsFlag             = envToFlag(pm.EnvTLS)
	certPathFlag        = envToFlag(pm.EnvCertPath)
	keyPathFlag         = envToFlag(pm.EnvKeyPath)
	keystoreUpdaterFlag = envToFlag(pm.EnvKeystoreUpdater)
	expVarsFlag         = envToFlag(pm.EnvExpVars)
	profilerFlag        = envToFlag(pm.EnvProfiler)
)

// BindFlagsToEnv binds flags to environment variables.
func BindFlagsToEnv(cmd *cobra.Command) error {
	cmd.Flags().StringP(procNameFlag, "", "", "process name to manage")
	cmd.Flags().StringP(procCmdFlag, "", "", "process command to manage")
	cmd.Flags().BoolP(reaperFlag, "", true, "enable the child processes reaper")
	cmd.Flags().IntP(HTTPPortFlag, "", pm.DefaultPort, "HTTP server port")
	cmd.Flags().BoolP(tlsFlag, "", false, "secure the HTTP server using TLS")
	cmd.Flags().StringP(certPathFlag, "", "", "path to the certificate file used to secure the HTTP server")
	cmd.Flags().StringP(keyPathFlag, "", "", "path to the private key file used to secure the HTTP server")
	cmd.Flags().BoolP(keystoreUpdaterFlag, "", true, "enable the keystore updater")
	cmd.Flags().BoolP(expVarsFlag, "", false, "enable exported variables (basic memory metrics)")
	cmd.Flags().BoolP(profilerFlag, "", false, "enable the pprof go profiler")

	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	viper.AutomaticEnv()

	return viper.BindPFlags(cmd.Flags())
}

// NewConfigFromFlags creates a new Config from the flags.
func NewConfigFromFlags() (*pm.Config, error) {
	procName := viper.GetString(procNameFlag)
	if procName == "" {
		return nil, flagRequiredError(procNameFlag)
	}

	procCmd := viper.GetString(procCmdFlag)
	if procCmd == "" {
		return nil, flagRequiredError(procCmdFlag)
	}

	reaper := viper.GetBool(reaperFlag)

	HTTPPort := viper.GetInt(HTTPPortFlag)

	tls := viper.GetBool(tlsFlag)
	certPath := viper.GetString(certPathFlag)
	keyPath := viper.GetString(keyPathFlag)
	if tls {
		if certPath == "" {
			return nil, flagRequiredError(certPathFlag)
		}

		if keyPath == "" {
			return nil, flagRequiredError(keyPathFlag)
		}
	}

	keystoreUpdater := viper.GetBool(keystoreUpdaterFlag)
	profiler := viper.GetBool(profilerFlag)
	expVars := viper.GetBool(expVarsFlag)

	return &pm.Config{
		ProcessName:           procName,
		ProcessCmd:            procCmd,
		EnableReaper:          reaper,
		HTTPPort:              HTTPPort,
		EnableTLS:             tls,
		CertPath:              certPath,
		KeyPath:               keyPath,
		EnableKeystoreUpdater: keystoreUpdater,
		EnableProfiler:        profiler,
		EnableExpVars:         expVars,
	}, nil
}

func flagRequiredError(flagName string) error {
	return fmt.Errorf("flag --%s is required", flagName)
}

func envToFlag(env string) string {
	return strings.Replace(strings.ToLower(env), "_", "-", -1)
}
