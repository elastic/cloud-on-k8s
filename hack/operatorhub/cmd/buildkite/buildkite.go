// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package buildkite

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/spf13/cobra"

	"github.com/elastic/cloud-on-k8s/v2/hack/operatorhub/cmd/flags"
)

const (
	url = "https://api.buildkite.com/v2/organizations/elastic/pipelines/cloud-on-k8s-operator/builds"
)

// Command will return the buildkite command
func Command() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "buildkite",
		Short:        "Start operatorhub release operation within Buildkite",
		Long:         "Start operatorhub release operation within Buildkite.",
		PreRunE:      preRunE,
		SilenceUsage: true,
		RunE:         doRun,
	}

	cmd.Flags().StringVarP(
		&flags.SupportedOpenshiftVersions,
		flags.SupportedOpenshiftVersionsFlag,
		"o",
		"v4.6",
		"supported openshift versions to be included within annotations. should *not* be a range. (v4.6) (OHUB_SUPPORTED_OPENSHIFT_VERSIONS)",
	)

	cmd.Flags().StringVarP(
		&flags.BuildkiteToken,
		flags.BuildkiteTokenFlag,
		"b",
		"",
		"Buildkite token to communicate with Buildkite API (OHUB_BUILDKITE_TOKEN)",
	)

	cmd.Flags().StringVarP(
		&flags.BuildkiteBranch,
		flags.BuildkiteBranchFlag,
		"B",
		"main",
		"Git branch with operatorhub tooling to use when running release (Should typically be `main` or same as `git-branch` flag; Could be PR branch if changes are not merged) (OHUB_BUILDKITE_BRANCH)",
	)

	cmd.Flags().StringVarP(
		&flags.BuildkiteCommit,
		flags.BuildkiteCommitFlag,
		"c",
		"HEAD",
		"Git commit SHA to use when running release (OHUB_BUILDKITE_COMMIT)",
	)

	cmd.Flags().StringVarP(
		&flags.BuildkitePRRepository,
		flags.BuildkitePRRepositoryFlag,
		"r",
		"",
		"Git pull request repository (format git://github.com/org/repo) to use when running release. (Only required when cli tooling changes aren't merged to main) (OHUB_BUILDKITE_PR_REPOSITORY)",
	)

	cmd.Flags().StringVarP(
		&flags.BuildkitePRID,
		flags.BuildkitePRIDFlag,
		"i",
		"",
		"Git pull request id to use when running release (Only required when cli tooling changes aren't merged to main) (OHUB_BUILDKITE_PR_ID)",
	)

	return cmd
}

// preRunE are the pre-run operations for the buildkite command
func preRunE(cmd *cobra.Command, args []string) error {
	// disable vault integration for this operation
	flags.EnableVault = false

	if flags.Conf.NewVersion == "" {
		return fmt.Errorf(flags.RequiredErrFmt, "newVersion")
	}

	if flags.BuildkiteToken == "" {
		return fmt.Errorf(flags.RequiredErrFmt, flags.BuildkiteTokenFlag)
	}

	if flags.BuildkiteBranch == "" {
		return fmt.Errorf(flags.RequiredErrFmt, flags.BuildkiteBranchFlag)
	}

	if flags.BuildkiteCommit == "" {
		return fmt.Errorf(flags.RequiredErrFmt, flags.BuildkiteCommitFlag)
	}

	return nil
}

// doRun will start the operatorhub release operation within buildkite
func doRun(_ *cobra.Command, _ []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()
	reqBody := body{
		Message: "run operatorhub release",
		Env: map[string]string{
			"OHUB_DRY_RUN":                      fmt.Sprintf("%t", flags.DryRun),
			"OHUB_TAG":                          flags.Conf.NewVersion,
			"OHUB_SUPPORTED_OPENSHIFT_VERSIONS": flags.SupportedOpenshiftVersions,
		},
		Branch: flags.BuildkiteBranch,
		Commit: flags.BuildkiteCommit,
	}

	if flags.BuildkitePRRepository != "" {
		reqBody.PullRequestRepository = &flags.BuildkitePRRepository
	}
	if flags.BuildkitePRID != "" {
		reqBody.PullRequestID = &flags.BuildkitePRID
	}
	b, err := json.Marshal(&reqBody)
	if err != nil {
		return fmt.Errorf("while marshaling body into json: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		return fmt.Errorf("while generating new http request: %w", err)
	}
	req.Header.Add("authorization", fmt.Sprintf("Bearer %s", flags.BuildkiteToken))
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("while executing buildkite request: %w", err)
	}
	defer res.Body.Close()
	var bodyBytes []byte
	bodyBytes, err = io.ReadAll(res.Body)
	if err != nil {
		return fmt.Errorf("failed to read buildkite response body: %w", err)
	}
	if res.StatusCode < 200 || res.StatusCode > 299 {
		return fmt.Errorf("request to start buildkite build failed, code: %d, body: %s", res.StatusCode, string(bodyBytes))
	}
	log.Println("Buildkite build submitted successfully")
	return nil
}

// body is the HTTP body for submitting a buildkite request to start a new build
type body struct {
	Message               string            `json:"message"`
	Env                   map[string]string `json:"env"`
	Commit                string            `json:"commit"`
	Branch                string            `json:"branch"`
	PullRequestRepository *string           `json:"pull_request_repository"`
	PullRequestID         *string           `json:"pull_request_id"`
}
