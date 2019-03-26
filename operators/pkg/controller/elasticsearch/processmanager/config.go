// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package processmanager

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	procNameFlag        = envToFlag(EnvProcName)
	procCmdFlag         = envToFlag(EnvProcCmd)
	reaperFlag          = envToFlag(EnvReaper)
	tlsFlag             = envToFlag(EnvTLS)
	certPathFlag        = envToFlag(EnvCertPath)
	keyPathFlag         = envToFlag(EnvKeyPath)
	keystoreUpdaterFlag = envToFlag(EnvKeystoreUpdater)
	expVarsFlag         = envToFlag(EnvExpVars)
	profilerFlag        = envToFlag(EnvProfiler)
)

// Config contains configuration parameters for the process manager.
type Config struct {
	ProcessName  string
	ProcessCmd   string
	EnableReaper bool

	EnableTLS bool
	CertPath  string
	KeyPath   string

	EnableKeystoreUpdater bool

	EnableExpVars  bool
	EnableProfiler bool
}

// BindFlagsToEnv binds flags to environment variables.
func BindFlagsToEnv(cmd *cobra.Command) error {
	cmd.Flags().StringP(procNameFlag, "", "", "process name to manage")
	cmd.Flags().StringP(procCmdFlag, "", "", "process command to manage")
	cmd.Flags().BoolP(reaperFlag, "", true, "enable the child processes reaper")
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

// NewConfigFromFlags creates a Config from the flags.
func NewConfigFromFlags() (Config, error) {
	procName := viper.GetString(procNameFlag)
	if procName == "" {
		return Config{}, flagRequiredError(procNameFlag)
	}

	procCmd := viper.GetString(procCmdFlag)
	if procCmd == "" {
		return Config{}, flagRequiredError(procCmdFlag)
	}

	reaper := viper.GetBool(reaperFlag)

	tls := viper.GetBool(tlsFlag)
	certPath := viper.GetString(certPathFlag)
	keyPath := viper.GetString(keyPathFlag)
	if tls {
		if certPath == "" {
			return Config{}, flagRequiredError(certPathFlag)
		}

		if keyPath == "" {
			return Config{}, flagRequiredError(keyPathFlag)
		}
	}

	keystoreUpdater := viper.GetBool(keystoreUpdaterFlag)
	profiler := viper.GetBool(profilerFlag)
	expVars := viper.GetBool(expVarsFlag)

	return Config{
		ProcessName:           procName,
		ProcessCmd:            procCmd,
		EnableReaper:          reaper,
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
