// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package github

import (
	"bytes"
	"context"
	"fmt"
	"io"
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

	// githubURL is the URL to communicate with github
	githubURL = "https://github.com"
)

var (
	// certifiedOperatorsFQDN is the FQDN to use for cloning, and submitting of PRs against certified operators repository.
	certifiedOperatorsFQDN = fmt.Sprintf("%s/%s/%s", githubURL, certifiedOperatorOrganization, certifiedOperatorRepository)
	// communityOperatorsFQDN is the FQDN to use for cloning, and submitting of PRs against community operators repository.
	communityOperatorsFQDN = fmt.Sprintf("%s/%s/%s", githubURL, communityOperatorOrganization, communityOperatorRepository)
)

// Config is the configuration for the github package
type Config struct {
	DryRun                                                   bool
	GitHubFullName, GitHubEmail, GitHubUsername, GitHubToken string
	HTTPClient                                               *http.Client
	GitTag                                                   string
	KeepTempFiles                                            bool
	PathToNewVersion                                         string
	ContainerImageSHA                                        string
}

// Client is the client for the github package
type Client struct {
	Config
}

// New returns a new github client, using
// a default HTTP client with a timeout of 10 seconds
// if one isn't supplied within the config.
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
	organization           string
	repository             string
	mainBranchName         string
	directoryName          string
	url                    string
	tempDir                string
	extraSteps             func() error
	additionalChangedFiles []string
}

// CloneRepositoryAndCreatePullRequest will execute a number of local, and potentially remote github operations
// for each of the certified and community operators github repositories:
// 1. Clone the repository to a temporary directory
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
	log.Printf("(%s): ✓\n", tempDir)

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
			// Each repository (community/certified) has a set of steps
			// that differ slightly between each repo, and this function
			// contains those steps to be run prior to adding the data
			// to the git repository, creating the commit, and submitting
			// the pull request.
			//
			// This step simply copies the 'elastic-cloud-eck.package.yaml'
			// file that already exists within the community-operators
			// directory into the directory to be added to the git commit.
			extraSteps: func() error {
				fileName := "elastic-cloud-eck.package.yaml"
				fileSrc := filepath.Join(c.PathToNewVersion, communityOperatorRepository, fileName)
				fileDst := filepath.Join(tempDir, communityOperatorRepository, "operators", communityOperatorDirectoryName, fileName)
				log.Printf("copying (%s) to (%s)", fileSrc, fileDst)
				err := copy.Copy(fileSrc, fileDst)
				if err != nil {
					return fmt.Errorf("while copying (%s) to (%s)", fileSrc, fileDst)
				}
				return nil
			},
			additionalChangedFiles: []string{
				filepath.Join("operators", communityOperatorDirectoryName, "elastic-cloud-eck.package.yaml"),
			},
		},
		{
			organization:   certifiedOperatorOrganization,
			repository:     certifiedOperatorRepository,
			url:            certifiedOperatorsFQDN,
			directoryName:  certifiedOperatorDirectoryName,
			mainBranchName: certifiedOperatorsRepositoryMainBranchName,
			tempDir:        tempDir,
			// Each repository (community/certified) has a set of steps
			// that differ slightly between each repo, and this function
			// contains those steps to be run prior to adding the data
			// to the git repository, creating the commit, and submitting
			// the pull request.
			//
			// This step:
			// 1. renames the elasticsearch-eck-operator-certified.{TAG}.clusterserviceversion.yaml to
			//    elasticsearch-eck-operator-certified.clusterserviceversion.yaml.
			// 2. replaces the container tag with the image SHA within the CSV.yaml.
			extraSteps: func() error {
				// ensure git tag has a preceeding 'v'.
				gitTag := c.GitTag
				if !strings.HasPrefix(gitTag, "v") {
					gitTag = fmt.Sprintf("v%s", gitTag)
				}
				fileSrc := filepath.Join(tempDir, certifiedOperatorRepository, "operators", certifiedOperatorDirectoryName, c.GitTag, "manifests", fmt.Sprintf("elasticsearch-eck-operator-certified.%s.clusterserviceversion.yaml", gitTag))
				fileDst := filepath.Join(tempDir, certifiedOperatorRepository, "operators", certifiedOperatorDirectoryName, c.GitTag, "manifests", "elasticsearch-eck-operator-certified.clusterserviceversion.yaml")
				log.Printf("copying (%s) to (%s)", fileSrc, fileDst)
				if err := copy.Copy(fileSrc, fileDst); err != nil {
					return fmt.Errorf("while copying (%s) to (%s)", fileSrc, fileDst)
				}
				log.Printf("removing file (%s)", fileSrc)
				if err := os.Remove(fileSrc); err != nil {
					return fmt.Errorf("while removing file (%s)", fileSrc)
				}

				log.Printf("reading (%s) to replace git tag with container image SHA", fileDst)
				input, err := os.ReadFile(fileDst)
				if err != nil {
					return fmt.Errorf("while reading file (%s)", fileDst)
				}

				lines := strings.Split(string(input), "\n")

				find := fmt.Sprintf(`registry.connect.redhat.com/elastic/eck-operator:%s`, c.GitTag)
				replace := fmt.Sprintf(`registry.connect.redhat.com/elastic/eck-operator@%s`, c.ContainerImageSHA)
				for i, line := range lines {
					lines[i] = strings.ReplaceAll(line, find, replace)
				}
				output := strings.Join(lines, "\n")
				log.Printf("rewriting (%s) with container image SHA", fileDst)
				err = os.WriteFile(fileDst, []byte(output), 0644)
				if err != nil {
					return fmt.Errorf("while writing updated file (%s)", fileDst)
				}
				return nil
			},
		},
	} {
		if err := c.cloneAndCreate(repository); err != nil {
			return err
		}
	}

	return nil
}

// cloneAndCreate does the following steps
// 1. clones the given repository to a temporary directory.
// 2. ensure the repository has been forked by the Client.GitHubUsername
// 3. configures a git remote for the fork.
// 4. creates a git branch within the fork.
// 5. copies the data from the 'generate-manifests' command into the git working tree.
// 6. runs the defined extra steps.
// 7. creates a git commit for the new version.
// 8. pushes the new branch to the remote fork.
// 9. potentially creates a draft pull request.
func (c *Client) cloneAndCreate(repo githubRepository) error {
	orgRepo := fmt.Sprintf("%s/%s", repo.organization, repo.repository)
	localTempDir := filepath.Join(repo.tempDir, repo.repository)

	log.Printf("Cloning (%s) repository to temporary directory ", repo.url)
	r, err := git.PlainClone(localTempDir, false, &git.CloneOptions{
		URL: repo.url,
		Auth: &git_http.BasicAuth{
			Username: c.GitHubToken,
		},
	})
	if err != nil {
		return fmt.Errorf("cloning (%s): %w", orgRepo, err)
	}
	log.Println("✓")

	log.Printf("Ensuring that (%s) repository has been forked ", orgRepo)
	err = c.ensureFork(orgRepo)
	if err != nil {
		log.Println("ⅹ")
		return fmt.Errorf("while ensuring fork exists: %w", err)
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
		return fmt.Errorf("while creating git remote: %w", err)
	}
	log.Println("✓")

	err = c.syncFork(orgRepo, r, remote)
	if err != nil {
		return fmt.Errorf("while syncing fork with upstream: %w", err)
	}

	branchName := fmt.Sprintf("eck-%s-%s", repo.repository, c.GitTag)
	log.Printf("Creating git branch (%s) for (%s) ", branchName, orgRepo)
	err = r.CreateBranch(&config.Branch{
		Name:   branchName,
		Remote: "fork",
		Merge:  plumbing.NewBranchReferenceName(branchName),
	})
	if err != nil {
		log.Println("ⅹ")
		return fmt.Errorf("while creating git branch: %w", err)
	}
	log.Println("✓")

	log.Printf("Checking out new branch (%s) ", branchName)
	var w *git.Worktree
	w, err = r.Worktree()
	if err != nil {
		log.Println("ⅹ")
		return fmt.Errorf("while retrieving a working tree from the git filesystem: %w", err)
	}

	err = w.Checkout(&git.CheckoutOptions{
		Branch: plumbing.NewBranchReferenceName(branchName),
		Create: true,
	})
	if err != nil {
		log.Println("ⅹ")
		return fmt.Errorf("while checking out branch (%s): %w", branchName, err)
	}
	log.Println("✓")

	destDir := filepath.Join(localTempDir, "operators", repo.directoryName, c.GitTag)
	srcDir := filepath.Join(c.PathToNewVersion, repo.repository, c.GitTag)
	log.Printf("copying (%s) to (%s)", srcDir, destDir)
	err = copy.Copy(srcDir, destDir)
	if err != nil {
		log.Println("ⅹ")
		return fmt.Errorf("while copying source dir (%s) to new git cloned dir (%s): %w", srcDir, destDir, err)
	}

	if err := repo.extraSteps(); err != nil {
		return err
	}

	log.Printf("Adding new data to git working tree ")
	pathToAdd := filepath.Join("operators", repo.directoryName, c.GitTag)

	for _, file := range append([]string{pathToAdd}, repo.additionalChangedFiles...) {
		_, err = w.Add(file)
		if err != nil {
			log.Println("ⅹ")
			return fmt.Errorf("while adding destination directory (%s) to git working tree: %w", file, err)
		}
	}
	log.Println("✓")

	log.Printf("Creating git commit ")
	_, err = w.Commit(fmt.Sprintf("Update ECK to the latest version `%s`\n\nSigned-off-by: %s <%s>", c.GitTag, c.GitHubFullName, c.GitHubEmail), &git.CommitOptions{
		Author: &object.Signature{
			Name:  c.GitHubFullName,
			Email: c.GitHubEmail,
			When:  time.Now(),
		},
	})
	if err != nil {
		log.Println("ⅹ")
		return fmt.Errorf("while commiting changes to git working tree: %w", err)
	}
	log.Println("✓")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	refSpec := fmt.Sprintf("+refs/heads/%[1]s:refs/heads/%[1]s", branchName)
	err = remote.PushContext(ctx, &git.PushOptions{
		RemoteName: "fork",
		Auth: &git_http.BasicAuth{
			Username: c.GitHubToken,
		},
		RefSpecs: []config.RefSpec{
			config.RefSpec(refSpec),
		},
	})
	if err != nil {
		return fmt.Errorf("while pushing git refspec (%s) to remote: %w", refSpec, err)
	}

	return c.createPullRequest(repo, branchName)
}

// createPullRequest will create a draft pull request for the given github repository
// unless dry-run is set.
func (c *Client) createPullRequest(repo githubRepository, branchName string) error {
	if c.DryRun {
		log.Println("Not creating draft pull request as dry-run is set")
		return nil
	}
	log.Printf("Creating draft pull request for (%s) ", repo.repository)

	prBody := fmt.Sprintf("Update the ECK operator to the latest version `%s`.", c.GitTag)
	var body = []byte(
		fmt.Sprintf(`{"title": "operator %s (%s)", "head": "%s:%s", "base": "%s", "draft": true, "body": "%s"}`,
			repo.directoryName, c.GitTag, c.GitHubUsername, branchName, repo.mainBranchName, prBody))
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()
	req, err := c.createRequest(ctx, http.MethodPost, fmt.Sprintf("%s/repos/%s/%s/pulls", githubAPIURL, repo.organization, repo.repository), bytes.NewBuffer(body))
	if err != nil {
		log.Println("ⅹ")
		return fmt.Errorf("while creating request to create draft pr for (%s): %w", repo.repository, err)
	}
	var res *http.Response
	if res, err = c.HTTPClient.Do(req); err != nil {
		log.Println("ⅹ")
		return fmt.Errorf("while creating draft pr for (%s): %w", repo.repository, err)
	}
	if res.StatusCode < 200 || res.StatusCode > 299 {
		log.Println("ⅹ")
		if bodyBytes, err := io.ReadAll(res.Body); err != nil {
			return fmt.Errorf("while creating draft pr for (%s), body: %s, code: %d", repo.repository, string(bodyBytes), res.StatusCode)
		}
		return fmt.Errorf("while creating draft pr for (%s), code: %d", repo.repository, res.StatusCode)
	}
	log.Println("✓")
	return nil
}
