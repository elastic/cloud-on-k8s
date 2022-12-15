package root

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"

	"github.com/elastic/cloud-on-k8s/v2/hack/operatorhub/cmd/all"
	"github.com/elastic/cloud-on-k8s/v2/hack/operatorhub/cmd/bundle"
	"github.com/elastic/cloud-on-k8s/v2/hack/operatorhub/cmd/container"
	"github.com/elastic/cloud-on-k8s/v2/hack/operatorhub/cmd/flags"
	"github.com/elastic/cloud-on-k8s/v2/hack/operatorhub/cmd/operatorhub"
)

// Root represents the root commmand for redhat operations
var Root = cobra.Command{
	Use:     "operatorhub",
	Version: "0.4.0",
	Short:   "Manage operatorhub release operations",
	Long: `Manage oepratorhub release operations, such as pushing operator container to quay.io, operator hub release generation, building operator metadata,
and potentially creating pull requests to community/certified operator repositories.`,
	// use persistent PreRunE here to ensure that all sub-commands
	// also get this function to execute prior to
	PersistentPreRunE: rootPersistentPreRunE,
}

func init() {
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))

	Root.PersistentFlags().StringVarP(
		&flags.Tag,
		flags.TagFlag,
		"t",
		"",
		"tag/new version of operator (OHUB_TAG)",
	)

	Root.PersistentFlags().BoolVarP(
		&flags.DryRun,
		flags.DryRunFlag,
		"Y",
		true,
		"Run dry run of all operations. Default: true. To un-set --dry-run=false (OHUB_DRY_RUN)",
	)

	Root.PersistentFlags().BoolVar(
		&flags.EnableVault,
		flags.EnableVaultFlag,
		true,
		"Enable vault functionality to try and automatically read from given vault keys (uses VAULT_* environment variables) (OHUB_ENABLE_VAULT)",
	)

	Root.PersistentFlags().StringVar(
		&flags.VaultAddress,
		flags.VaultAddressFlag,
		"",
		"Vault address to use when enable-vault is set (VAULT_ADDR)",
	)

	Root.PersistentFlags().StringVar(
		&flags.VaultToken,
		flags.VaultTokenFlag,
		"",
		"Vault token to use when enable-vault is set (VAULT_TOKEN)",
	)

	Root.PersistentFlags().StringVar(
		&flags.RedhatVaultSecret,
		flags.RedhatVaultSecretFlag,
		"",
		`When --enable-vault is set, attempts to read the following flags from a given vault secret:
* container sub-command flags concerning redhat interactions:
** registry-password
** project-id
** api-key
(OHUB_REDHAT_VAULT_SECRET)`,
	)

	Root.PersistentFlags().StringVar(
		&flags.GithubVaultSecret,
		flags.GithubVaultSecretFlag,
		"",
		`When --enable-vault is set, attempts to read the following flags from a given vault secret:
* bundle sub-command flags concerning generating operator bundle and creating PRs:
** github-token
** github-username
** github-fullname
** github-email
(OHUB_GITHUB_VAULT_SECRET)`,
	)

	Root.AddCommand(all.Command(&Root))
	Root.AddCommand(bundle.Command())
	Root.AddCommand(container.Command())
	Root.AddCommand(operatorhub.Command())
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

	// vault environment variables need to also support not
	// having OHUB prefix, as they exist in CI/Buildkite.
	viper.BindEnv(flags.VaultAddressFlag, "VAULT_ADDR")
	viper.BindEnv(flags.VaultTokenFlag, "VAULT_TOKEN")

	viper.AutomaticEnv()

	if viper.GetString(flags.TagFlag) == "" {
		return fmt.Errorf("%s is required", flags.TagFlag)
	}

	if flags.EnableVault {
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
		if err := readSecretsFromVault(); err != nil {
			return err
		}
	}

	// set all flag variables with what's set within viper prior to running
	// to allow commands to use the variables directly without calling viper.
	bindFlags(cmd, viper.GetViper())

	return nil
}

func bindFlags(cmd *cobra.Command, v *viper.Viper) {
	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		// Apply the viper config value to the flag when the flag is not set and viper has a value
		if !f.Changed && v.IsSet(f.Name) {
			val := v.Get(f.Name)
			cmd.Flags().Set(f.Name, fmt.Sprintf("%v", val))
		}
	})
}
