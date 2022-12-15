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
	"github.com/elastic/cloud-on-k8s/v2/hack/operatorhub/pkg/github"
	"github.com/elastic/cloud-on-k8s/v2/hack/operatorhub/pkg/opm"
)

// Command will return the bundle command
func Command() *cobra.Command {
	bundleCmd := &cobra.Command{
		Use:   "bundle",
		Short: "generate operator bundle metadata, and potentially create pull request for new operator version",
		Long: `Bundle and build operator metadata for publishing on openshift operator hub, and potentially create pull request to
github.com/redhat-openshift-ecosystem/certified-operators repository.`,
		SilenceUsage: true,
	}

	generateCmd := &cobra.Command{
		Use:          "generate",
		Short:        "generate operator bundle metadata",
		Long:         "Bundle and build operator metadata for publishing on openshift operator hub",
		SilenceUsage: true,
		PreRunE:      generateCmdPreRunE,
		RunE:         DoGenerate,
	}

	createPRCmd := &cobra.Command{
		Use:   "create-pr",
		Short: "create pull request against github.com/redhat-openshift-ecosystem/certified-operators repository",
		Long: `Create pull request using output of 'bundle' command against
github.com/redhat-openshift-ecosystem/certified-operators repository`,
		SilenceUsage: true,
		PreRunE:      CreatePRPreRunE,
		RunE:         DoCreatePR,
	}

	generateCmd.Flags().StringP(
		flags.DirFlag,
		"d",
		"",
		"directory containing output from 'operatorhub command' (hack/operatorhub/certified-operators) (OHUB_DIR)",
	)

	generateCmd.Flags().StringVarP(
		&flags.SupportedOpenshiftVersions,
		flags.SupportedOpenshiftVersionsFlag,
		"o",
		"v4.6",
		"supported openshift versions to be included within annotations. should *not* be a range. (v4.6) (OHUB_SUPPORTED_OPENSHIFT_VERSIONS)",
	)

	createPRCmd.Flags().BoolVarP(
		&flags.SubmitPullRequest,
		flags.SubmitPullRequestFlag,
		"P",
		false,
		"attempt to submit PR to https://github.com/redhat-openshift-ecosystem/certified-operators repo? (OHUB_SUBMIT_PULL_REQUEST)",
	)

	createPRCmd.Flags().StringVarP(
		&flags.GithubToken,
		flags.GithubTokenFlag,
		"g",
		"",
		"if 'submit-pull-request' is enabled, user's token to communicate with github.com (OHUB_GITHUB_TOKEN)",
	)

	createPRCmd.Flags().StringVarP(
		&flags.GithubUsername,
		flags.GithubUsernameFlag,
		"u",
		"",
		"if 'submit-pull-request' is enabled, github username to use to fork repo, and submit PR (OHUB_GITHUB_USERNAME)",
	)

	createPRCmd.Flags().StringVarP(
		&flags.GithubFullname,
		flags.GithubFullnameFlag,
		"f",
		"",
		"if 'submit-pull-request' is enabled, github full name to use to add to commit message (OHUB_GITHUB_FULLNAME)",
	)

	createPRCmd.Flags().StringVarP(
		&flags.GithubEmail,
		flags.GithubEmailFlag,
		"e",
		"",
		"if 'submit-pull-request' is enabled, github email to use to add to commit message (OHUB_GITHUB_EMAIL)",
	)

	createPRCmd.Flags().BoolVarP(
		&flags.DeleteTempDirectory,
		flags.DeleteTempDirectoryFlag,
		"D",
		false,
		"delete git temporary directory after script completes (OHUB_DELETE_TEMP_DIRECTORY)",
	)

	bundleCmd.AddCommand(generateCmd, createPRCmd)

	return bundleCmd
}

func generateCmdPreRunE(cmd *cobra.Command, args []string) error {
	if flags.Dir == "" {
		return fmt.Errorf("directory containing output from operator hub release generator is required (%s)", flags.DirFlag)
	}
	if flags.SupportedOpenshiftVersions == "" {
		return fmt.Errorf("supported openshift versions flag (%s) is required", flags.SupportedOpenshiftVersionsFlag)
	} else if strings.Contains(flags.SupportedOpenshiftVersions, "-") {
		return fmt.Errorf("supported openshift versions flag (%s) should not be a range", flags.SupportedOpenshiftVersionsFlag)
	}
	return nil
}

// CreatePRPreRunE are pre-run operations for the create pull request command
func CreatePRPreRunE(cmd *cobra.Command, args []string) error {
	// TODO is this really needed?
	if cmd.Name() != "all" && flags.Dir == "" {
		return fmt.Errorf("directory containing output from operator hub release generator is required (%s)", flags.DirFlag)
	}

	if flags.SubmitPullRequest {
		if flags.GithubToken == "" {
			return fmt.Errorf(flags.ErrRequiredIfEnabled, flags.GithubTokenFlag, flags.SubmitPullRequestFlag)
		}
		if flags.GithubUsername == "" {
			return fmt.Errorf(flags.ErrRequiredIfEnabled, flags.GithubUsernameFlag, flags.SubmitPullRequestFlag)
		}
		if flags.GithubFullname == "" {
			return fmt.Errorf(flags.ErrRequiredIfEnabled, flags.GithubFullnameFlag, flags.SubmitPullRequestFlag)
		}
		if flags.GithubEmail == "" {
			return fmt.Errorf(flags.ErrRequiredIfEnabled, flags.GithubEmailFlag, flags.SubmitPullRequestFlag)
		}
	}

	return nil
}

// DoGenerate will generate the operator bundle metadata
func DoGenerate(_ *cobra.Command, _ []string) error {
	dir := path.Join(flags.Dir, flags.Tag)
	err := opm.GenerateBundle(opm.GenerateConfig{
		LocalDirectory:  dir,
		OutputDirectory: dir,
	})
	if err != nil {
		return err
	}
	err = opm.EnsureAnnotations(path.Join(dir, "metadata", "annotations.yaml"), flags.SupportedOpenshiftVersions)
	if err != nil {
		return err
	}
	return nil
}

// DoCreatePR will execute a number of local, and potentially remote github operations
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
func DoCreatePR(_ *cobra.Command, _ []string) error {
	dir := filepath.Join(flags.Dir, flags.Tag)
	client := github.New(github.Config{
		CreatePullRequest: flags.SubmitPullRequest,
		DryRun:            flags.DryRun,
		GitHubEmail:       flags.GithubEmail,
		GitHubFullName:    flags.GithubFullname,
		GitHubUsername:    flags.GithubUsername,
		GitHubToken:       flags.GithubToken,
		GitTag:            flags.Tag,
		KeepTempFiles:     !flags.DeleteTempDirectory,
		PathToNewVersion:  dir,
	})
	return client.CloneRepositoryAndCreatePullRequest()
}
