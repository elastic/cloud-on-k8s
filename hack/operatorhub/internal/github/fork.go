// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package github

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	git_http "github.com/go-git/go-git/v5/plumbing/transport/http"
)

const (
	// githubAPIURL is the URL to communicate with github's api
	githubAPIURL = "https://api.github.com"
	// githubV3JSONMediaType is the github content type to be included in all github api requests
	githubV3JSONMediaType    = "application/vnd.github.v3+json"
	githubRepoForksURLFormat = "%s/repos/%s/forks"
	httpAcceptHeader         = "Accept"
)

// githubFork is the format of a github fork as returned by the github API.
type githubFork struct {
	ID       uint   `json:"id"`
	Name     string `json:"name"`
	FullName string `json:"full_name"`
	Owner    struct {
		Login string `json:"login"`
	} `json:"owner"`
	Private       bool   `json:"private"`
	HTMLURL       string `json:"html_url"`
	Fork          bool   `json:"fork"`
	URL           string `json:"url"`
	DefaultBranch string `json:"default_branch"`
	Visibility    string `json:"visibility"`
}

// ensureFork will ensure that a given organization/repository
// is forked by the Client.GitHubUsername by checking if the fork
// already exists, and if it does not, creating the fork and waiting
// for the fork creation to complete.
func (c *Client) ensureFork(orgRepo string) error {
	exists, err := c.forkExists(orgRepo)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	err = c.createFork(orgRepo)
	if err != nil {
		return err
	}
	return c.waitOnForkCreation(orgRepo)
}

// forkExists will check if a given organization/repository
// is forked by the Client.GitHubUsername and will return
// a bool and any errors encountered.
func (c *Client) forkExists(orgRepo string) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()
	req, err := c.createRequest(ctx, http.MethodGet, fmt.Sprintf(githubRepoForksURLFormat, githubAPIURL, orgRepo), nil)
	if err != nil {
		return false, fmt.Errorf("while creating github request to ensure fork exists: %w", err)
	}
	var res *http.Response
	res, err = c.HTTPClient.Do(req)
	if err != nil {
		return false, fmt.Errorf("while executing github request to ensure fork exists: %w", err)
	}
	defer res.Body.Close()
	var bodyBytes []byte
	bodyBytes, err = io.ReadAll(res.Body)
	if err != nil {
		return false, fmt.Errorf("while reading get forks response body: %w", err)
	}
	if res.StatusCode < 200 || res.StatusCode > 299 {
		return false, fmt.Errorf("invalid status code for github request to ensure fork exists, code: %d, body: %s", res.StatusCode, string(bodyBytes))
	}
	var forksResponse []githubFork
	err = json.NewDecoder(bytes.NewReader(bodyBytes)).Decode(&forksResponse)
	if err != nil {
		return false, fmt.Errorf("while decoding get forks response into json, body: %s: %w", string(bodyBytes), err)
	}
	return userInForks(c.GitHubUsername, forksResponse), nil
}

// createRequest will create an HTTP request to the given URL, with supplied HTTP method and body,
// using the given context, setting required headers and returning the request, and any errors encountered.
func (c *Client) createRequest(ctx context.Context, method, url string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, fmt.Errorf("while creating request: %w", err)
	}
	req.Header.Add(httpAcceptHeader, githubV3JSONMediaType)
	req.SetBasicAuth(c.GitHubUsername, c.GitHubToken)
	return req, nil
}

// createFork will fork a given organization/repository for the Client.GitHubUsername.
func (c *Client) createFork(orgRepo string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()
	req, err := c.createRequest(ctx, http.MethodPost, fmt.Sprintf(githubRepoForksURLFormat, githubAPIURL, orgRepo), nil)
	if err != nil {
		return err
	}
	var res *http.Response
	res, err = c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("while executing github request to create fork: %w", err)
	}
	defer res.Body.Close()
	var bodyBytes []byte
	bodyBytes, err = io.ReadAll(res.Body)
	if err != nil {
		return fmt.Errorf("while reading create fork response body: %w", err)
	}
	if res.StatusCode < 200 || res.StatusCode > 299 {
		return fmt.Errorf("invalid response code for github request to create fork, code: %d, body: %s", res.StatusCode, string(bodyBytes))
	}
	return nil
}

func (c *Client) syncFork(orgRepo string, repository *git.Repository, remote *git.Remote) error {
	err := repository.Fetch(&git.FetchOptions{
		RemoteName: "fork",
	})
	if err != nil {
		return fmt.Errorf("while fetching fork: %w", err)
	}
	w, err := repository.Worktree()
	if err != nil {
		return fmt.Errorf("while retrieving a working tree from the git filesystem: %w", err)
	}
	err = w.Checkout(&git.CheckoutOptions{Branch: "refs/remotes/fork/main", Create: false})
	if err != nil {
		return fmt.Errorf("while checking out (%s) branch (main): %w", orgRepo, err)
	}
	err = w.Pull(&git.PullOptions{RemoteName: "origin", ReferenceName: plumbing.NewBranchReferenceName("main")})
	if err != nil && !errors.Is(err, git.NoErrAlreadyUpToDate) {
		return fmt.Errorf("while merging upstream changes from upstream/main into fork/main: %w", err)
	}
	refSpec := "+refs/heads/main:refs/heads/main"
	err = repository.Push(&git.PushOptions{
		Auth: &git_http.BasicAuth{
			Username: c.GitHubToken,
		},
		RemoteName: "fork",
		RefSpecs: []config.RefSpec{
			config.RefSpec(refSpec),
		},
	})
	if err != nil && !errors.Is(err, git.NoErrAlreadyUpToDate) {
		return fmt.Errorf("while pushing merge of fork/main: %w", err)
	}
	return nil
}

// waitOnForkCreation will check if a given organization/repository has completed the fork
// process for the Client.GitHubUsername by requesting the GitHubUsername/repository URL
// from the Github API, looking for an eventual HTTP 200 response, timing out in 5 minutes.
func (c *Client) waitOnForkCreation(orgRepo string) error {
	ticker := time.NewTicker(10 * time.Second)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	for {
		select {
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
			defer cancel()
			req, err := c.createRequest(ctx, http.MethodGet, fmt.Sprintf("%s/repos/%s/%s", githubAPIURL, c.GitHubUsername, strings.Split(orgRepo, "/")[1]), nil)
			if err != nil {
				log.Printf("failed to create request to check if fork exists: %s", err)
				continue
			}
			var res *http.Response
			res, err = c.HTTPClient.Do(req)
			if err != nil {
				log.Printf("failed to execute request to check if fork exists: %s", err)
				continue
			}
			switch res.StatusCode {
			case 200:
				return nil
			default:
				var bodyBytes []byte
				bodyBytes, err = io.ReadAll(res.Body)
				log.Printf("failed to execute request to check if fork exists, body: %s: %s", string(bodyBytes), err)
			}
		case <-ctx.Done():
			return fmt.Errorf("github fork creation not completed within timeout of 5m")
		}
	}
}

// userInForks checks if a given user is the owner of any forks in the given slice of github forks.
func userInForks(user string, forks []githubFork) bool {
	for _, fork := range forks {
		if fork.Owner.Login == user {
			return true
		}
	}
	return false
}
