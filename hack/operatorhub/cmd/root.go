package root

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/elastic/cloud-on-k8s/v2/hack/operatorhub/cmd/all"
	"github.com/elastic/cloud-on-k8s/v2/hack/operatorhub/cmd/bundle"
	"github.com/elastic/cloud-on-k8s/v2/hack/operatorhub/cmd/container"
	"github.com/elastic/cloud-on-k8s/v2/hack/operatorhub/cmd/operatorhub"
)

// Root represents the root commmand for redhat operations
var Root = cobra.Command{
	Use:     "redhat",
	Version: "0.4.0",
	Short:   "Manage redhat release operations",
	Long: `Manage redhat release operations, such as pushing operator container to redhat catalog, operator hub release generation, building operator metadata,
and potentially creating pull requests to community/certified operator repositories.`,
	PreRunE: rootPreRunE,
}

func init() {
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))

	Root.PersistentFlags().StringP(
		"tag",
		"t",
		"",
		"tag/new version of operator (TAG)",
	)

	Root.PersistentFlags().BoolP(
		"dry-run",
		"Y",
		true,
		"Run dry run of all operations. Default: true. To un-set --dry-run=false (DRY_RUN)",
	)

	Root.PersistentFlags().Bool(
		"enable-vault",
		true,
		"Enable vault functionality to try and automatically read from given vault keys (uses VAULT_* environment variables) (ENABLE_VAULT)",
	)

	Root.PersistentFlags().String(
		"vault-addr",
		"",
		"Vault address to use when enable-vault is set",
	)

	Root.PersistentFlags().String(
		"vault-token",
		"",
		"Vault token to use when enable-vault is set",
	)

	Root.PersistentFlags().String(
		"redhat-vault-secret",
		"",
		`When --enable-vault is set, attempts to read the following flags from a given vault secret:
		* container sub-command flags concerning redhat interactions:
		** registry-password
		** project-id
		** api-key
		`,
	)

	Root.PersistentFlags().String(
		"github-vault-secret",
		"",
		`When --enable-vault is set, attempts to read the following flags from a given vault secret:
		* bundle sub-command flags concerning generating operator bundle and creating PRs:
		** github-token
		** github-username
		** github-fullname
		** github-email
		`,
	)

	Root.AddCommand(all.Command(&Root))
	Root.AddCommand(bundle.Command())
	Root.AddCommand(container.Command())
	Root.AddCommand(operatorhub.Command())
}

func rootPreRunE(cmd *cobra.Command, args []string) error {
	viper.SetEnvPrefix("OHUB")
	// automatically translate dashes in flags to underscores in environment vars
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))

	if err := viper.BindPFlags(cmd.PersistentFlags()); err != nil {
		return fmt.Errorf("failed to bind flags: %w", err)
	}

	// Vault environment variables need to also support not having OHUB prefix.
	viper.BindEnv("vault-addr", "VAULT_ADDR")
	viper.BindEnv("vault-token", "VAULT_TOKEN")

	viper.AutomaticEnv()

	if viper.GetString("tag") == "" {
		return fmt.Errorf("tag is required")
	}

	if viper.GetBool("enable-vault") {
		for _, key := range []string{"vault-addr", "vault-token", "redhat-vault-secret", "github-vault-secret"} {
			if viper.GetString(key) == "" {
				return fmt.Errorf("%s is required when enable-vault is set", key)
			}
		}
		return readSecretsFromVault()
	}

	return nil
}
