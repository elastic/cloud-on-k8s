// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package root

import (
	"fmt"
	"os"
	"strings"

	gyaml "github.com/ghodss/yaml"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"

	"github.com/elastic/cloud-on-k8s/v2/hack/operatorhub/cmd/bundle"
	"github.com/elastic/cloud-on-k8s/v2/hack/operatorhub/cmd/container"
	"github.com/elastic/cloud-on-k8s/v2/hack/operatorhub/cmd/flags"
	"github.com/elastic/cloud-on-k8s/v2/hack/operatorhub/cmd/manifests"
)

// Cmd represents the root command for Red Hat/operatorhub release operations
var Cmd = cobra.Command{
	Use:     "operatorhub",
	Version: "0.5.0",
	Short:   "Manage operatorhub release operations",
	Long: `Manage operatorhub release operations, such as pushing operator container to quay.io, operator hub release generation, building operator metadata,
and potentially creating pull requests to community/certified operator repositories.`,
	// PersistentPreRunE is used here to ensure that all sub-commands
	// run these initialization steps, and can properly use both the vault
	// integration and the flag variables defined in the cmd/flags package.
	PersistentPreRunE: rootPersistentPreRunE,
}

func init() {
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))

	Cmd.PersistentFlags().StringVarP(
		&flags.ConfigPath,
		flags.ConfFlag,
		"c",
		"./config.yaml",
		"Path to configuration file (OHUB_CONF)",
	)

	Cmd.PersistentFlags().StringVarP(
		&flags.APIKey,
		flags.APIKeyFlags,
		"a",
		"",
		"API key to use when communicating with Red Hat certification API. Used in both the bundle, and generate-manifests sub-commands. (OHUB_API_KEY)",
	)

	Cmd.PersistentFlags().StringVarP(
		&flags.ProjectID,
		flags.ProjectIDFlag,
		"p",
		"",
		"Red Hat project id within the Red Hat technology portal (OHUB_PROJECT_ID)",
	)

	Cmd.PersistentFlags().BoolVarP(
		&flags.DryRun,
		flags.DryRunFlag,
		"Y",
		true,
		"Run dry run of all operations. Default: true. To un-set --dry-run=false (OHUB_DRY_RUN)",
	)

	Cmd.PersistentFlags().BoolVar(
		&flags.EnableVault,
		flags.EnableVaultFlag,
		true,
		"Enable vault functionality to try and automatically read from given vault keys (uses VAULT_* environment variables) (OHUB_ENABLE_VAULT)",
	)

	Cmd.PersistentFlags().StringVar(
		&flags.VaultAddress,
		flags.VaultAddressFlag,
		"",
		"Vault address to use when enable-vault is set (VAULT_ADDR)",
	)

	Cmd.PersistentFlags().StringVar(
		&flags.VaultToken,
		flags.VaultTokenFlag,
		"",
		"Vault token to use when enable-vault is set (VAULT_TOKEN)",
	)

	Cmd.PersistentFlags().StringVar(
		&flags.RedhatVaultSecret,
		flags.RedhatVaultSecretFlag,
		"secret/ci/elastic-cloud-on-k8s/operatorhub-release-redhat",
		`When --enable-vault is set, attempts to read the following flags from a given vault secret:
	* container sub-command flags concerning redhat interactions:
		** registry-password
		** project-id
		** api-key
(OHUB_REDHAT_VAULT_SECRET)`,
	)

	Cmd.PersistentFlags().StringVar(
		&flags.GithubVaultSecret,
		flags.GithubVaultSecretFlag,
		"secret/ci/elastic-cloud-on-k8s/operatorhub-release-github",
		`When --enable-vault is set, attempts to read the following flags from a given vault secret:
	* bundle sub-command flags concerning generating operator bundle and creating PRs:
		** github-token
		** github-username
		** github-fullname
		** github-email
(OHUB_GITHUB_VAULT_SECRET)`,
	)

	Cmd.AddCommand(
		bundle.Command(),
		container.Command(),
		manifests.Command(),
	)
}

func rootPersistentPreRunE(cmd *cobra.Command, args []string) error {
	// prefix all environment variables with "OHUB_"
	viper.SetEnvPrefix("OHUB")
	// automatically translate dashes in flags to underscores in environment vars
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))

	if err := viper.BindPFlags(cmd.PersistentFlags()); err != nil {
		return fmt.Errorf("failed to bind persistent flags: %w", err)
	}

	if err := viper.BindPFlags(cmd.Flags()); err != nil {
		return fmt.Errorf("failed to bind flags: %w", err)
	}

	if err := setVariablesFromConfigurationFile(); err != nil {
		return err
	}

	// vault environment variables need to also support not
	// having OHUB prefix, as they exist in CI/Buildkite.
	viper.BindEnv(flags.VaultAddressFlag, "VAULT_ADDR")
	viper.BindEnv(flags.VaultTokenFlag, "VAULT_TOKEN")

	viper.AutomaticEnv()

	if viper.GetBool(flags.EnableVaultFlag) {
		// ensure that the flag variables are set using what's current in viper configuration prior to calling
		// command to read secrets from vault, as the flags are exclusively used, not viper directly.
		for _, flag := range []string{
			flags.VaultAddressFlag,
			flags.VaultTokenFlag,
			flags.RedhatVaultSecretFlag,
			flags.GithubVaultSecretFlag} {
			if viper.GetString(flag) == "" {
				return fmt.Errorf("%s is required when %s is set", flag, flags.EnableVaultFlag)
			}
			cmd.Flags().Set(flag, viper.GetString(flag))
		}
		if err := readAllSecretsFromVault(); err != nil {
			return err
		}
	}

	// set all flag variables with what's set within viper prior to running to allow
	// (sub)commands to use the variables in cmd/flags directly without calling viper.
	bindFlags(cmd, viper.GetViper())

	return nil
}

func setVariablesFromConfigurationFile() error {
	confFilePath := viper.GetString(flags.ConfFlag)
	b, err := os.ReadFile(confFilePath)
	if err != nil {
		return fmt.Errorf("while reading configuration file %s: %w", confFilePath, err)
	}
	err = gyaml.Unmarshal(b, &flags.Conf)
	if err != nil {
		return fmt.Errorf("while unmarshaling config file into configuration struct: %w", err)
	}
	if flags.Conf == nil {
		return fmt.Errorf("configuration from config file empty after yaml unmarshal")
	}
	for k, v := range map[string]string{"newVersion": flags.Conf.NewVersion, "prevVersion": flags.Conf.PrevVersion, "stackVersion": flags.Conf.StackVersion} {
		if v == "" {
			return fmt.Errorf("%s must be defined in %s and not be empty", k, confFilePath)
		}
	}
	return nil
}

// bindFlags will be called last prior to running all (sub)commands to set all cobra flags
// to what is set within viper so that all subsequent operations can use cmd/flags variables
// directly without calling viper.
func bindFlags(cmd *cobra.Command, v *viper.Viper) {
	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		// Apply the viper config value to the flag when the flag has not been set by the user and viper has a value
		if !f.Changed && v.IsSet(f.Name) {
			val := v.Get(f.Name)
			cmd.Flags().Set(f.Name, fmt.Sprintf("%v", val))
		}
	})
}
