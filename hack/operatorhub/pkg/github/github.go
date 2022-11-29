// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package github

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"log"
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
)

const (
	certifiedOperatorOrganization = "redhat-openshift-ecosystem"
	certifiedOperatorRepository   = "certified-operators"
	communityOperatorOrganization = "k8s-operatorhub"
	communityOperatorRepository   = "community-operators"

	// certifiedOperatorsRepositoryMainBranchName is the name of the default branch of the certified operators git repository.
	certifiedOperatorsRepositoryMainBranchName = "main"
	// communityOperatorsRepositoryMainBranchName is the name of the default branch of the community operators git repository.
	communityOperatorsRepositoryMainBranchName = "main"

	certifiedOperatorDirectoryName = "elasticsearch-eck-operator-certified"
	communityOperatorDirectoryName = "elastic-cloud-eck"
)

var (
	// certifiedOperatorsFQDN is the FQDN to use for cloning, and submitting PRs against certified operators repository.
	certifiedOperatorsFQDN = fmt.Sprintf("https://github.com/%s/%s", certifiedOperatorOrganization, certifiedOperatorRepository)
	// communityOperatorsFQDN is the FQDN to use for cloning, and submitting PRs against community operators repository.
	communityOperatorsFQDN = fmt.Sprintf("https://github.com/%s/%s", communityOperatorOrganization, communityOperatorRepository)
)

// Config is the configuration for the github package
type Config struct {
	DryRun                                                   bool
	GitHubFullName, GitHubEmail, GitHubUsername, GitHubToken string
	HTTPClient                                               *http.Client
	GitTag                                                   string
	KeepTempFiles                                            bool
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
	if c.HTTPClient == nil {
		c.HTTPClient = &http.Client{
			Timeout: 10 * time.Second,
		}
	}
	return c
}

type githubRepository struct {
	organization   string
	repository     string
	mainBranchName string
	directoryName  string
	url            string
	tempDir        string
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
	log.Printf("Creating temporarily directory for git operations ")
	tempDir, err := os.MkdirTemp(os.TempDir(), "eck-redhat-operations")
	if err != nil {
		return fmt.Errorf("failed to create temporary directory for operations: %w", err)
	}
	log.Println(fmt.Sprintf("(%s): ✓", tempDir))

	defer func() {
		if !c.KeepTempFiles {
			os.RemoveAll(tempDir)
		}
	}()

	for _, repository := range []githubRepository{
		{
			organization:   communityOperatorOrganization,
			repository:     communityOperatorRepository,
			url:            communityOperatorsFQDN,
			directoryName:  communityOperatorDirectoryName,
			mainBranchName: communityOperatorsRepositoryMainBranchName,
			tempDir:        tempDir,
		},
		{
			organization:   certifiedOperatorOrganization,
			repository:     certifiedOperatorRepository,
			url:            certifiedOperatorsFQDN,
			directoryName:  certifiedOperatorDirectoryName,
			mainBranchName: certifiedOperatorsRepositoryMainBranchName,
			tempDir:        tempDir,
		},
	} {
		if err := c.cloneAndCreate(repository); err != nil {
			return err
		}
	}

	return nil
}

func (c *Client) cloneAndCreate(repo githubRepository) error {
	orgRepo := fmt.Sprintf("%s/%s", repo.organization, repo.repository)
	localTempDir := filepath.Join(repo.tempDir, repo.repository)

	log.Printf("Cloning (%s) repository to temporary directory ", orgRepo)
	r, err := git.PlainClone(localTempDir, false, &git.CloneOptions{
		URL: repo.url,
		Auth: &git_http.BasicAuth{
			Username: c.GitHubUsername,
			Password: c.GitHubToken,
		},
	})
	if err != nil {
		return fmt.Errorf("cloning (%s): %w", orgRepo, err)
	}
	log.Println("✓")

	log.Printf("Ensuring that (%s) repository has been forked ", orgRepo)
	err = c.ensureFork()
	if err != nil {
		log.Println("ⅹ")
		return fmt.Errorf("failed to ensure fork exists: %w", err)
	}
	log.Println("✓")

	log.Printf("Creating git remote for (%s) ", orgRepo)
	remote, err := r.CreateRemote(&config.RemoteConfig{
		Name: "fork",
		URLs: []string{
			fmt.Sprintf("https://github.com/%s/%s", c.GitHubUsername, repo.repository),
		},
	})
	if err != nil {
		log.Println("ⅹ")
		return fmt.Errorf("failed to create git remote: %w", err)
	}
	log.Println("✓")

	branchName := fmt.Sprintf("eck-%s-%s", repo.repository, c.GitTag)
	log.Printf("Creating git branch (%s) for (%s) ", branchName, orgRepo)
	err = r.CreateBranch(&config.Branch{
		Name:   branchName,
		Remote: "fork",
		Merge:  plumbing.NewBranchReferenceName(branchName),
	})
	if err != nil {
		log.Println("ⅹ")
		return fmt.Errorf("failed to create git branch: %w", err)
	}
	log.Println("✓")

	log.Printf("Checking out new branch (%s) ", branchName)
	var w *git.Worktree
	w, err = r.Worktree()
	if err != nil {
		log.Println("ⅹ")
		return fmt.Errorf("failed to retrieve a working tree from the git filesystem: %w", err)
	}

	err = w.Checkout(&git.CheckoutOptions{
		Branch: plumbing.NewBranchReferenceName(branchName),
		Create: true,
	})
	if err != nil {
		log.Println("ⅹ")
		return fmt.Errorf("Unable to checkout branch (%s): %w", branchName, err)
	}
	log.Println("✓")

	log.Printf("Adding new data to git working tree ")
	newVersion := strings.Split(c.PathToNewVersion, string(os.PathSeparator))[len(strings.Split(c.PathToNewVersion, string(os.PathSeparator)))-1]
	destDir := filepath.Join(localTempDir, "operators", repo.directoryName, newVersion)
	err = copy.Copy(c.PathToNewVersion, destDir)
	if err != nil {
		log.Println("ⅹ")
		return fmt.Errorf("failed to copy source dir (%s) to new git cloned dir (%s): %w", c.PathToNewVersion, destDir, err)
	}

	_, err = w.Add(filepath.Join("operators", repo.directoryName, newVersion))
	if err != nil {
		log.Println("ⅹ")
		return fmt.Errorf("failed to add destination directory (%s) to git working tree: %w", destDir, err)
	}
	log.Println("✓")

	log.Printf("Creating git commit ")
	_, err = w.Commit(fmt.Sprintf(`version %s of eck operator\n\nSigned-off-by: %s <%s>`, newVersion, c.GitHubFullName, c.GitHubEmail), &git.CommitOptions{
		Author: &object.Signature{
			Name:  c.GitHubFullName,
			Email: c.GitHubEmail,
			When:  time.Now(),
		},
	})
	if err != nil {
		log.Println("ⅹ")
		return fmt.Errorf("failed to commit changes to git working tree: %w", err)
	}
	log.Println("✓")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	err = remote.PushContext(ctx, &git.PushOptions{
		RemoteName: "fork",
		Auth: &git_http.TokenAuth{
			Token: c.GitHubToken,
		},
		RefSpecs: []config.RefSpec{
			config.RefSpec(branchName),
		},
	})
	if err != nil {
		return fmt.Errorf("failed to push branch (%s) to remote: %w", branchName, err)
	}

	if c.CreatePullRequest {
		return c.createPullRequest(repo, branchName)
	}
	log.Printf("Not creating pull request for (%s)\n", orgRepo)
	return nil
}

func (c *Client) createPullRequest(repo githubRepository, branchName string) error {
	log.Printf("Creating pull request for (%s) ", repo.repository)
	var body = []byte(
		fmt.Sprintf(`{"title": "operator %s (%s)", "head": "%s:%s", "base": "%s", "draft": true}`,
			repo.directoryName, c.GitTag, c.GitHubUsername, branchName, repo.mainBranchName))
	req, cancel, err := c.createRequest(http.MethodPost, fmt.Sprintf("%s/repos/%s/%s/pulls", GithubAPIURL, repo.organization, repo.repository), bytes.NewBuffer(body))
	defer cancel()
	if err != nil {
		log.Println("ⅹ")
		return fmt.Errorf("while creating request to create pr for (%s): %w", repo.repository, err)
	}
	if !c.DryRun {
		var res *http.Response
		if res, err = c.HTTPClient.Do(req); err != nil {
			log.Println("ⅹ")
			return fmt.Errorf("while creating pr for (%s): %w", repo.repository, err)
		}
		if res.StatusCode > 299 {
			log.Println("ⅹ")
			if bodyBytes, err := ioutil.ReadAll(res.Body); err != nil {
				return fmt.Errorf("while creating pr for (%s), body: %s, code: %d", repo.repository, string(bodyBytes), res.StatusCode)
			}
			return fmt.Errorf("while creating pr for (%s), code: %d", repo.repository, res.StatusCode)
		}
		log.Println("✓")
	} else {
		log.Println("Not creating pull request as dry-run is set")
	}
	return nil
}
