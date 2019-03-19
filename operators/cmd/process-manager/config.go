package main

import (
	"fmt"
	"github.com/kelseyhightower/envconfig"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

type Config struct {
	ProcessName string `envconfig:"NAME" required:"true"`
	ProcessCmd  string `envconfig:"CMD" required:"true"`
}

func NewConfigFromEnv() (Config, error) {
	var cfg Config
	err := envconfig.Process("PROC", &cfg)
	if err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func SetFlags(cmd *cobra.Command) {
	cmd.Flags().StringP(procNameFlag, "n", "", "process name to manage")
	cmd.Flags().StringP(procCmdFlag, "m", "", "process command to manage")
}

func NewConfigFromFlags() (Config, error) {
	procName := viper.GetString(procNameFlag)
	procCmd := viper.GetString(procCmdFlag)

	if procName == "" {
		return Config{}, fmt.Errorf("flag --%s not provided", procNameFlag)
	}
	if procName == "" {
		return Config{}, fmt.Errorf("flag --%s not provided", procCmdFlag)
	}

	return Config{procName, procCmd}, nil
}
