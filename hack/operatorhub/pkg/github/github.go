// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package github

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	git_http "github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/otiai10/copy"
	"github.com/pterm/pterm"
)

var (
	// OpenshiftOperatorsRepository is the repository to use for cloning, and submitting PRs against.
	OpenshiftOperatorsRepository = "https://github.com/redhat-openshift-ecosystem/certified-operators"
	// OpenshiftOperatorsRepositoryMainBranchName is the name of the default branch of the git repository
	OpenshiftOperatorsRepositoryMainBranchName = "main"
)

// Config is the configuration for the github package
type Config struct {
	GitHubFullName, GitHubEmail, GitHubUsername, GitHubToken string
	HTTPClient                                               *http.Client
	GitTag                                                   string
	RemoveTempFiles                                          *bool
	PathToNewVersion                                         string
	CreatePullRequest                                        bool
}

// Client is the client for the github package
type Client struct {
	Config
}

type operatorHubConfig struct {
	NewVersion   string                   `json:"newVersion"`
	PrevVersion  string                   `json:"prevVersion"`
	StackVersion string                   `json:"stackVersion"`
	CRDs         []map[string]interface{} `json:"crds"`
	Packages     []map[string]interface{} `json:"packages"`
}

// New returns a new github client
func New(config Config) *Client {
	c := &Client{
		config,
	}
	if c.RemoveTempFiles == nil {
		c.RemoveTempFiles = config.RemoveTempFiles
	}
	if c.HTTPClient == nil {
		c.HTTPClient = &http.Client{
			Timeout: 10 * time.Second,
		}
	}
	return c
}

// CloneRepositoryAndCreatePullRequest will execute a number of local, and potentially remote github operations:
// 1. Clone redhat operatory repository to a temporary directory
// 2. Ensure that the configured github user has forked the repository
// 3. Create a git remote
// 4. Create a new git branch named from the configured git tag
// 5. Checkout the new branch
// 6. Copy the operator manifests, from the configured directory, into the clone directory
// 7. "git add" the new directory to the working tree
// 8. Create a new commit for the new changes
// 9. Push the remote to the fork
// 10. Create a draft pull request in the remote repository
func (c *Client) CloneRepositoryAndCreatePullRequest() error {
	pterm.Printf("Creating temporarily directory for git operations ")
	tempDir, err := os.MkdirTemp(os.TempDir(), "eck-redhat-operations")
	if err != nil {
		pterm.Println(pterm.Red("ⅹ"))
		return fmt.Errorf("failed to create temporary directory for operations: %w", err)
	}
	pterm.Println(pterm.Green(fmt.Sprintf("(%s): ✓", tempDir)))

	pterm.Printf("Cloning Openshift Operator repository to temporary directory ")
	r, err := git.PlainClone(tempDir, false, &git.CloneOptions{
		URL: OpenshiftOperatorsRepository,
		Auth: &git_http.BasicAuth{
			Username: c.GitHubUsername,
			Password: c.GitHubToken,
		},
	})
	if err != nil {
		pterm.Println(pterm.Red("ⅹ"))
		return fmt.Errorf("cloning git repository: %w", err)
	}
	pterm.Println(pterm.Green("✓"))

	defer func() {
		if c.RemoveTempFiles == nil || (c.RemoveTempFiles != nil && *c.RemoveTempFiles) {
			os.RemoveAll(tempDir)
		}
	}()

	pterm.Printf("Ensuring that Openshift Operator repository has been forked ")
	err = c.ensureFork()
	if err != nil {
		pterm.Println(pterm.Red("ⅹ"))
		return fmt.Errorf("failed to ensure fork exists: %w", err)
	}
	pterm.Println(pterm.Green("✓"))

	pterm.Printf("Creating git remote ")
	remote, err := r.CreateRemote(&config.RemoteConfig{
		Name: "fork",
		URLs: []string{
			fmt.Sprintf("https://github.com/%s/certified-oprators", c.GitHubUsername),
		},
	})
	if err != nil {
		pterm.Println(pterm.Red("ⅹ"))
		return fmt.Errorf("failed to create git remote: %w", err)
	}
	pterm.Println(pterm.Green("✓"))

	pterm.Printf("Creating git branch ")
	err = r.CreateBranch(&config.Branch{
		Name:   fmt.Sprintf("eck-operator-certified-%s", c.GitTag),
		Remote: "fork",
		Merge:  plumbing.NewBranchReferenceName(fmt.Sprintf("eck-operator-certified-%s", c.GitTag)),
	})
	if err != nil {
		pterm.Println(pterm.Red("ⅹ"))
		return fmt.Errorf("failed to create git branch: %w", err)
	}
	pterm.Println(pterm.Green("✓"))

	pterm.Printf("Checking out new branch (%s) ", fmt.Sprintf("eck-operator-certified-%s", c.GitTag))
	var w *git.Worktree
	w, err = r.Worktree()
	if err != nil {
		pterm.Println(pterm.Red("ⅹ"))
		return fmt.Errorf("failed to retrieve a working tree from the git filesystem: %w", err)
	}

	err = w.Checkout(&git.CheckoutOptions{
		Branch: plumbing.NewBranchReferenceName(fmt.Sprintf("eck-operator-certified-%s", c.GitTag)),
		Create: true,
	})
	if err != nil {
		pterm.Println(pterm.Red("ⅹ"))
		return fmt.Errorf("Unable to checkout branch: %w", err)
	}
	pterm.Println(pterm.Green("✓"))

	pterm.Printf("Adding new data to git working tree ")
	newVersion := strings.Split(c.PathToNewVersion, string(os.PathSeparator))[len(strings.Split(c.PathToNewVersion, string(os.PathSeparator)))-1]
	destDir := filepath.Join(tempDir, "operators", "elasticsearch-eck-operator-certified", newVersion)
	err = copy.Copy(c.PathToNewVersion, destDir)
	if err != nil {
		pterm.Println(pterm.Red("ⅹ"))
		return fmt.Errorf("failed to copy source dir (%s) to new git cloned dir (%s): %w", c.PathToNewVersion, destDir, err)
	}

	_, err = w.Add(filepath.Join("operators", "elasticsearch-eck-operator-certified", newVersion))
	if err != nil {
		pterm.Println(pterm.Red("ⅹ"))
		return fmt.Errorf("failed to add destination directory (%s) to git working tree: %w", destDir, err)
	}
	pterm.Println(pterm.Green("✓"))

	pterm.Printf("Creating git commit ")
	_, err = w.Commit(fmt.Sprintf("version %s of eck operator", newVersion), &git.CommitOptions{
		Author: &object.Signature{
			Name:  c.GitHubFullName,
			Email: c.GitHubEmail,
			When:  time.Now(),
		},
	})
	if err != nil {
		pterm.Println(pterm.Red("ⅹ"))
		return fmt.Errorf("failed to commit changes to git working tree: %w", err)
	}
	pterm.Println(pterm.Green("✓"))

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	err = remote.PushContext(ctx, &git.PushOptions{
		RemoteName: "fork",
		Auth: &git_http.TokenAuth{
			Token: c.GitHubToken,
		},
		RefSpecs: []config.RefSpec{
			config.RefSpec(fmt.Sprintf("eck-operator-certified-%s", c.GitTag)),
		},
	})
	if err != nil {
		return fmt.Errorf("failed to push branch (%s) to remote: %w", fmt.Sprintf("eck-operator-certified-%s", c.GitTag), err)
	}

	if c.CreatePullRequest {
		return c.createPullRequest()
	}
	pterm.Println(pterm.Yellow("Not creating pull request"))
	return nil
}

func (c *Client) createPullRequest() error {
	pterm.Printf("Creating pull request ")
	var body = []byte(
		fmt.Sprintf(`{"title": "operator elasticsearch-eck-operator-certified (%s)", "head": "%s:%s", "base": "%s", "draft": true}`,
			c.GitTag, c.GitHubUsername, fmt.Sprintf("eck-operator-certified-%s", c.GitTag), OpenshiftOperatorsRepositoryMainBranchName))
	req, cancel, err := c.createRequest(http.MethodPost, fmt.Sprintf("%s/repos/%s/pulls", GithubAPIURL, RedhatOpenshiftEcosystemOperatorsRepo), bytes.NewBuffer(body))
	defer cancel()
	if err != nil {
		pterm.Println(pterm.Red("ⅹ"))
		return fmt.Errorf("failed to create request to creating pr: %w", err)
	}
	var res *http.Response
	if res, err = c.HTTPClient.Do(req); err != nil {
		pterm.Println(pterm.Red("ⅹ"))
		return fmt.Errorf("failed request to create pr: %w", err)
	}
	if res.StatusCode > 299 {
		pterm.Println(pterm.Red("ⅹ"))
		if bodyBytes, err := ioutil.ReadAll(res.Body); err != nil {
			return fmt.Errorf("failed request to create pr, body: %s, code: %d", string(bodyBytes), res.StatusCode)
		}
		return fmt.Errorf("failed request to create pr, code: %d", res.StatusCode)
	}
	pterm.Println(pterm.Green("✓"))
	return nil
}
