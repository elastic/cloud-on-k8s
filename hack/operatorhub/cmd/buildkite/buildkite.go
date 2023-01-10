// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package buildkite

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
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
		&flags.PreviousVersion,
		flags.PrevVersionFlag,
		"V",
		"",
		"Previous version of the operator to use during release (OHUB_PREV_VERSION)",
	)

	cmd.Flags().StringVarP(
		&flags.StackVersion,
		flags.StackVersionFlag,
		"s",
		"",
		"Stack version of Elastic stack to use during release (OHUB_STACK_VERSION)",
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
		"",
		"Git branch to use when running release (OHUB_BUILDKITE_BRANCH)",
	)

	cmd.Flags().StringVarP(
		&flags.BuildkiteCommit,
		flags.BuildkiteCommitFlag,
		"c",
		"",
		"Git commit SHA to use when running release (OHUB_BUILDKITE_COMMIT)",
	)

	cmd.Flags().StringVarP(
		&flags.BuildkitePRRepository,
		flags.BuildkitePRRepositoryFlag,
		"r",
		"",
		"Git pull request repository to use when running release (OHUB_BUILDKITE_PR_REPOSITORY)",
	)

	cmd.Flags().StringVarP(
		&flags.BuildkitePRID,
		flags.BuildkitePRIDFlag,
		"i",
		"",
		"Git pull request id to use when running release (OHUB_BUILDKITE_PR_ID)",
	)

	return cmd
}

// preRunE are the pre-run operations for the buildkite command
func preRunE(cmd *cobra.Command, args []string) error {
	// disable vault integration for this operation
	flags.EnableVault = false

	if flags.Tag == "" {
		return fmt.Errorf(flags.RequiredErrFmt, flags.TagFlag)
	}

	if flags.PreviousVersion == "" {
		return fmt.Errorf(flags.RequiredErrFmt, flags.PreviousVersion)
	}

	if flags.StackVersion == "" {
		return fmt.Errorf(flags.RequiredErrFmt, flags.StackVersion)
	}

	if flags.BuildkiteToken == "" {
		return fmt.Errorf(flags.RequiredErrFmt, flags.BuildkiteToken)
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
			"OHUB_TAG":           flags.Tag,
			"OHUB_PREV_VERSION":  flags.PreviousVersion,
			"OHUB_STACK_VERSION": flags.StackVersion,
		},
	}

	if flags.BuildkiteBranch != "" {
		reqBody.Branch = &flags.BuildkiteBranch
	}
	if flags.BuildkiteCommit != "" {
		reqBody.Commit = &flags.BuildkiteCommit
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
	bodyBytes, err = ioutil.ReadAll(res.Body)
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
	Message               string
	Env                   map[string]string
	Commit                *string
	Branch                *string
	PullRequestRepository *string
	PullRequestID         *string
}
