// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package bundle

import (
	"fmt"
	"path"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/elastic/cloud-on-k8s/hack/operatorhub/pkg/github"
	"github.com/elastic/cloud-on-k8s/hack/operatorhub/pkg/opm"
	"github.com/elastic/cloud-on-k8s/hack/operatorhub/pkg/shared"
)

// Command will return the bundle command
func Command() *cobra.Command {
	bundleCmd := &cobra.Command{
		Use:   "bundle",
		Short: "generate operator bundle metadata, and potentially create pull request for new operator version",
		Long: `Bundle and build operator metadata for publishing on openshift operator hub, and potentially create pull request to
github.com/redhat-openshift-ecosystem/certified-operators repository.`,
		SilenceUsage: true,
		PreRunE:      PreRunE,
	}

	generateCmd := &cobra.Command{
		Use:          "generate",
		Short:        "generate operator bundle metadata",
		Long:         "Bundle and build operator metadata for publishing on openshift operator hub",
		SilenceUsage: true,
		PreRunE:      defaultPreRunE,
		RunE:         DoGenerate,
	}

	createPRCmd := &cobra.Command{
		Use:   "create-pr",
		Short: "create pull request against github.com/redhat-openshift-ecosystem/certified-operators repository",
		Long: `Create pull request using output of 'bundle' command against
github.com/redhat-openshift-ecosystem/certified-operators repository`,
		SilenceUsage: true,
		PreRunE:      defaultPreRunE,
		RunE:         DoCreatePR,
	}

	generateCmd.Flags().StringP(
		"dir",
		"d",
		"",
		"directory containing output from 'operatorhub command' (hack/operatorhub/certified-operators) (DIR)",
	)

	generateCmd.Flags().StringP(
		"supported-openshift-versions",
		"o",
		"v4.6-v4.9",
		"supported openshift versions to be included within annotations (v4.6-v4.9) (SUPPORTED_OPENSHIFT_VERSIONS)",
	)

	createPRCmd.Flags().BoolP(
		"submit-pull-request",
		"P",
		false,
		"attempt to submit PR to https://github.com/redhat-openshift-ecosystem/certified-operators repo? (SUBMIT_PULL_REQUEST)",
	)

	createPRCmd.Flags().StringP(
		"github-token",
		"g",
		"",
		"if 'submit-pull-request' is enabled, user's token to communicate with github.com (GITHUB_TOKEN)",
	)

	createPRCmd.Flags().StringP(
		"github-username",
		"u",
		"",
		"if 'submit-pull-request' is enabled, github username to use to fork repo, and submit PR (GITHUB_USERNAME)",
	)

	createPRCmd.Flags().StringP(
		"github-fullname",
		"f",
		"",
		"if 'submit-pull-request' is enabled, github full name to use to add to commit message (GITHUB_FULLNAME)",
	)

	createPRCmd.Flags().StringP(
		"github-email",
		"e",
		"",
		"if 'submit-pull-request' is enabled, github email to use to add to commit message (GITHUB_EMAIL)",
	)

	createPRCmd.Flags().BoolP(
		"delete-temp-directory",
		"D",
		false,
		"delete git temporary directory after script completes (DELETE_TEMP_DIRECTORY)",
	)

	bundleCmd.AddCommand(generateCmd, createPRCmd)

	return bundleCmd
}

func defaultPreRunE(cmd *cobra.Command, args []string) error {
	if cmd.Parent() != nil && cmd.Parent().PreRunE != nil {
		if err := cmd.Parent().PreRunE(cmd.Parent(), args); err != nil {
			return err
		}
	}
	return nil
}

// PreRunE is pre-run operators for the bundle command
func PreRunE(cmd *cobra.Command, args []string) error {
	if cmd.Parent() != nil && cmd.Parent().PreRunE != nil {
		if err := cmd.Parent().PreRunE(cmd.Parent(), args); err != nil {
			return err
		}
	}

	if err := viper.BindPFlags(cmd.PersistentFlags()); err != nil {
		return fmt.Errorf("failed to bind persistent flags: %w", err)
	}

	if err := viper.BindPFlags(cmd.Flags()); err != nil {
		return fmt.Errorf("failed to bind flags: %w", err)
	}

	if cmd.Name() != "all" && viper.GetString("dir") == "" {
		return fmt.Errorf("directory containing output from operator hub release generator is required (dir)")
	}

	if viper.GetBool("submit-pull-request") {
		if viper.GetString("github-token") == "" {
			return fmt.Errorf("github-token is required if submit-pull-request is enabled")
		}
		if viper.GetString("github-username") == "" {
			return fmt.Errorf("github-username is required if submit-pull-request is enabled")
		}
		if viper.GetString("github-fullname") == "" {
			return fmt.Errorf("github-fullname is required if submit-pull-request is enabled")
		}
		if viper.GetString("github-email") == "" {
			return fmt.Errorf("github-email is required if submit-pull-request is enabled")
		}
	}

	viper.AutomaticEnv()

	return nil
}

// DoGenerate will generate the operator bundle metadata
func DoGenerate(_ *cobra.Command, _ []string) error {
	dir := path.Join(viper.GetString("dir"), viper.GetString("tag"))
	err := opm.GenerateBundle(opm.GenerateConfig{
		LocalDirectory:  dir,
		OutputDirectory: dir,
	})
	if err != nil {
		return err
	}
	err = opm.EnsureAnnotations(path.Join(dir, "metadata", "annotations.yaml"), viper.GetString("supported-openshift-versions"))
	if err != nil {
		return err
	}
	return nil
}

// DoCreatePR will execute a number of local, and potentially remote github operations:
// 1. Clone redhat operatory repository to a temporary directory
// 2. Ensure that the configured github user has forked the repository
// 3. Create a git remote
// 4. Create a new git branch named from the configured git tag
// 5. Checkout the new branch
// 6. Copy the operator manifests, from the configured directory, into the clone directory
// 7. "git add" the new directory to the working tree
// 8. Create a new commit for the new changes
// 9. Push the remote to the fork
// 10. Potentially create a draft pull request in the remote repository
func DoCreatePR(_ *cobra.Command, _ []string) error {
	dir := path.Join(viper.GetString("dir"), viper.GetString("tag"))
	client := github.New(github.Config{
		CreatePullRequest: viper.GetBool("submit-pull-request"),
		GitHubEmail:       viper.GetString("github-email"),
		GitHubFullName:    viper.GetString("github-fullname"),
		GitHubUsername:    viper.GetString("github-username"),
		GitHubToken:       viper.GetString("github-token"),
		GitTag:            viper.GetString("tag"),
		RemoveTempFiles:   shared.PBool(viper.GetBool("delete-temp-directory")),
		PathToNewVersion:  dir,
	})
	return client.CloneRepositoryAndCreatePullRequest()
}
