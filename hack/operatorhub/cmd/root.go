package root

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/elastic/cloud-on-k8s/hack/operatorhub/cmd/all"
	"github.com/elastic/cloud-on-k8s/hack/operatorhub/cmd/bundle"
	"github.com/elastic/cloud-on-k8s/hack/operatorhub/cmd/container"
	"github.com/elastic/cloud-on-k8s/hack/operatorhub/cmd/operatorhub"
)

// Root represents the root commmand for redhat operations
var Root = cobra.Command{
	Use:     "redhat",
	Version: "0.4.0",
	Short:   "Manage redhat release operations",
	Long: `Manage redhat release operations, such as pushing operator container to redhat catalog, operator hub release generation, building operator metadata,
and potentially creating pull request to github.com/redhat-openshift-ecosystem/certified-operators repository.`,
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

	Root.AddCommand(all.Command(&Root))
	Root.AddCommand(bundle.Command())
	Root.AddCommand(container.Command())
	Root.AddCommand(operatorhub.Command())
}

func rootPreRunE(cmd *cobra.Command, args []string) error {
	// automatically translate dashes in flags to underscores in environment vars
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))

	if err := viper.BindPFlags(cmd.PersistentFlags()); err != nil {
		return fmt.Errorf("failed to bind flags: %w", err)
	}

	viper.AutomaticEnv()

	if viper.GetString("tag") == "" {
		return fmt.Errorf("tag is required")
	}

	return nil
}
