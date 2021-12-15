// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"strings"
	"time"
)

// GithubV3JSONMediaType is the github content type to be included in all github api requests
const GithubV3JSONMediaType = "application/vnd.github.v3+json"

var (
	// GithubAPIURL is the URL to communicate with github's api
	GithubAPIURL = "https://api.github.com"
	// RedhatOpenshiftEcosystemOperatorsRepo is the organization/repository for the redhat openshift operators
	RedhatOpenshiftEcosystemOperatorsRepo = "redhat-openshift-ecosystem/certified-operators"
)

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

func (c *Client) ensureFork() error {
	exists, err := c.forkExists()
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	err = c.createFork()
	if err != nil {
		return err
	}
	return c.waitOnForkCreation()
}

func (c *Client) createRequest(method, url string, body io.Reader) (*http.Request, context.CancelFunc, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, cancel, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Add("Accept", GithubV3JSONMediaType)
	req.SetBasicAuth(c.GitHubUsername, c.GitHubToken)
	return req, cancel, nil
}

func (c *Client) createFork() error {
	req, cancel, err := c.createRequest(http.MethodPost, fmt.Sprintf("%s/repos/%s/forks", GithubAPIURL, RedhatOpenshiftEcosystemOperatorsRepo), nil)
	defer cancel()
	if err != nil {
		return err
	}
	log.Printf("executing request: %v", req)
	var res *http.Response
	res, err = c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("github request to create fork failed: %w", err)
	}
	defer res.Body.Close()
	var bodyBytes []byte
	bodyBytes, err = ioutil.ReadAll(res.Body)
	if err != nil {
		return fmt.Errorf("failed to read create fork response body: %w", err)
	}
	if res.StatusCode < 200 || res.StatusCode > 299 {
		return fmt.Errorf("github request to create fork failed, code: %d, body: %s", res.StatusCode, string(bodyBytes))
	}
	return nil
}

func (c *Client) waitOnForkCreation() error {
	ticker := time.NewTicker(10 * time.Second)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	for {
		select {
		case <-ticker.C:
			req, cancel, err := c.createRequest(http.MethodGet, fmt.Sprintf("%s/repos/%s/%s", GithubAPIURL, c.GitHubUsername, strings.Split(RedhatOpenshiftEcosystemOperatorsRepo, "/")[1]), nil)
			defer cancel()
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
				bodyBytes, err = ioutil.ReadAll(res.Body)
				log.Printf("failed to execute request to check if fork exists, body: %s: %s", string(bodyBytes), err)
			}
		case <-ctx.Done():
			return fmt.Errorf("github fork creation not completed within timeout of 5m")
		}
	}
}

func (c *Client) forkExists() (bool, error) {
	req, cancel, err := c.createRequest(http.MethodGet, fmt.Sprintf("%s/repos/%s/forks", GithubAPIURL, RedhatOpenshiftEcosystemOperatorsRepo), nil)
	defer cancel()
	var res *http.Response
	res, err = c.HTTPClient.Do(req)
	if err != nil {
		return false, fmt.Errorf("github request to ensure fork exists failed: %w", err)
	}
	defer res.Body.Close()
	var bodyBytes []byte
	bodyBytes, err = ioutil.ReadAll(res.Body)
	if err != nil {
		return false, fmt.Errorf("failed to read get forks response body: %w", err)
	}
	if res.StatusCode < 200 || res.StatusCode > 299 {
		return false, fmt.Errorf("github request to ensure fork exists failed, code: %d, body: %s", res.StatusCode, string(bodyBytes))
	}
	var forksResponse []githubFork
	err = json.NewDecoder(bytes.NewReader(bodyBytes)).Decode(&forksResponse)
	if err != nil {
		return false, fmt.Errorf("failed to decode get forks response into json, body: %s: %w", string(bodyBytes), err)
	}
	return userInForks(c.GitHubUsername, forksResponse), nil
}

func userInForks(user string, forks []githubFork) bool {
	for _, fork := range forks {
		if fork.Owner.Login == user {
			return true
		}
	}
	return false
}
