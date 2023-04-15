// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package bundle

import (
	"fmt"
	"path"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/elastic/cloud-on-k8s/v2/hack/operatorhub/cmd/flags"
	"github.com/elastic/cloud-on-k8s/v2/hack/operatorhub/internal/github"
	"github.com/elastic/cloud-on-k8s/v2/hack/operatorhub/internal/opm"
)

// Command will return the bundle command
func Command() *cobra.Command {
	bundleCmd := &cobra.Command{
		Use:   "bundle",
		Short: "generate operator bundle metadata/create pull requests for new operator versions",
		Long: `Bundle and build operator metadata for publishing on openshift operator hub, and create pull requests to
certified-operators, and community-operators repositories.`,
		SilenceUsage: true,
	}

	generateCmd := &cobra.Command{
		Use:          "generate",
		Short:        "generate operator bundle metadata",
		Long:         "Bundle and build operator metadata for publishing on openshift operator hub",
		SilenceUsage: true,
		PreRunE:      generateCmdPreRunE,
		RunE:         doGenerate,
	}

	createPRCmd := &cobra.Command{
		Use:   "create-pr",
		Short: "create pull requests against community and certified repositories",
		Long: `Create pull requests using output of 'bundle' command against
certified-operators and community-operators repositories.`,
		SilenceUsage: true,
		PreRunE:      createPRPreRunE,
		RunE:         doCreatePR,
	}

	bundleCmd.PersistentFlags().StringVarP(
		&flags.Dir,
		flags.DirFlag,
		"d",
		"./",
		"directory containing output from 'operatorhub command' which contains 'certified-operators', and 'community-operators' subdirectories. (OHUB_DIR)",
	)

	createPRCmd.Flags().StringVarP(
		&flags.GithubToken,
		flags.GithubTokenFlag,
		"g",
		"",
		"if 'dry-run' isn't set, user's token to communicate with github.com (OHUB_GITHUB_TOKEN)",
	)

	createPRCmd.Flags().StringVarP(
		&flags.GithubUsername,
		flags.GithubUsernameFlag,
		"u",
		"",
		"if 'dry-run' isn't set, github username to use to fork repositories, and submit PRs (OHUB_GITHUB_USERNAME)",
	)

	createPRCmd.Flags().StringVarP(
		&flags.GithubFullname,
		flags.GithubFullnameFlag,
		"f",
		"",
		"if 'dry-run' isn't set, github full name to use to add to commit message (OHUB_GITHUB_FULLNAME)",
	)

	createPRCmd.Flags().StringVarP(
		&flags.GithubEmail,
		flags.GithubEmailFlag,
		"e",
		"",
		"if 'dry-run' isn't set, github email to use to add to commit message (OHUB_GITHUB_EMAIL)",
	)

	createPRCmd.Flags().BoolVarP(
		&flags.DeleteTempDirectory,
		flags.DeleteTempDirectoryFlag,
		"D",
		true,
		"delete git temporary directory after script completes (OHUB_DELETE_TEMP_DIRECTORY)",
	)

	bundleCmd.AddCommand(generateCmd, createPRCmd)

	return bundleCmd
}

// generateCmdPreRunE are pre-run operations for the generate command
func generateCmdPreRunE(cmd *cobra.Command, args []string) error {
	if flags.Dir == "" {
		return fmt.Errorf("directory containing output from operator hub release generator is required (%s)", flags.DirFlag)
	}
	if flags.Conf.MinSupportedOpenshiftVersion == "" {
		return fmt.Errorf("minimum supported openshift version is required in configuration file")
	} else if strings.Contains(flags.Conf.MinSupportedOpenshiftVersion, "-") {
		return fmt.Errorf("minimum supported openshift version in configuration file should not be a range")
	}
	return nil
}

// createPRPreRunE are pre-run operations for the create pull request command
func createPRPreRunE(cmd *cobra.Command, args []string) error {
	if flags.Dir == "" {
		return fmt.Errorf("directory containing output from operator hub release generator is required (%s)", flags.DirFlag)
	}

	if flags.GithubToken == "" {
		return fmt.Errorf(flags.RequiredErrFmt, flags.GithubTokenFlag)
	}
	if flags.GithubUsername == "" {
		return fmt.Errorf(flags.RequiredErrFmt, flags.GithubUsernameFlag)
	}
	if flags.GithubFullname == "" {
		return fmt.Errorf(flags.RequiredErrFmt, flags.GithubFullnameFlag)
	}
	if flags.GithubEmail == "" {
		return fmt.Errorf(flags.RequiredErrFmt, flags.GithubEmailFlag)
	}

	return nil
}

// doGenerate will generate the operator bundle metadata
func doGenerate(_ *cobra.Command, _ []string) error {
	dir := filepath.Join(flags.Dir, "certified-operators", flags.Conf.NewVersion)
	err := opm.GenerateBundle(opm.GenerateConfig{
		LocalDirectory:  dir,
		OutputDirectory: dir,
	})
	if err != nil {
		return err
	}
	err = opm.EnsureAnnotations(path.Join(dir, "metadata", "annotations.yaml"), flags.Conf.MinSupportedOpenshiftVersion)
	if err != nil {
		return err
	}
	return opm.EnsureLabels(path.Join(flags.Dir, "bundle.Dockerfile"), flags.Conf.MinSupportedOpenshiftVersion)
}

// doCreatePR will execute a number of local, and potentially remote github operations
// for each of the certified, and community github repositories.
// 1. Clone the repository to a temporary directory
// 2. Ensure that the configured github user has forked the repository
// 3. Create a git remote
// 4. Create a new git branch named from the configured git tag
// 5. Checkout the new branch
// 6. Copy the operator manifests, from the configured directory, into the clone directory
// 7. "git add" the new directory to the working tree
// 8. Create a new commit for the new changes
// 9. Push the remote to the fork
// 10. Potentially create a draft pull request in the remote repository
func doCreatePR(_ *cobra.Command, _ []string) error {
	client := github.New(github.Config{
		DryRun:           flags.DryRun,
		GitHubEmail:      flags.GithubEmail,
		GitHubFullName:   flags.GithubFullname,
		GitHubUsername:   flags.GithubUsername,
		GitHubToken:      flags.GithubToken,
		GitTag:           flags.Conf.NewVersion,
		KeepTempFiles:    !flags.DeleteTempDirectory,
		PathToNewVersion: flags.Dir,
	})
	return client.CloneRepositoryAndCreatePullRequest()
}
