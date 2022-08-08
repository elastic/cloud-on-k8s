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
	"net/http"
	"os"
	"time"

	"golang.org/x/oauth2"
)

const (
	graphqlEndpoint = "https://api.github.com/graphql"
	contentType     = "application/json; charset=utf-8"
	maxPayloadSize  = 1024 * 1024 * 10 // 10 MiB

	queryBody = `
query ($q: String!, $per_page: Int = 50, $after: String) {
  search(query: $q, type: ISSUE, first: $per_page, after: $after) {
    nodes {
      ... on PullRequest {
        number
        title
        merged
        labels(first: 10) {
          edges {
            node {
             name
            }
          }
        }
		bodyHTML
      }
    }

    pageInfo {
      hasNextPage
      endCursor
    }
  }
}
`
)

type PullRequest struct {
	Number int
	Title  string
	Labels map[string]struct{}
	Issues []int
}

func LoadPullRequests(repoName, version string, ignoredLabels map[string]struct{}) ([]PullRequest, error) {
	client := mkClient()
	loader := &prLoader{
		apiEndpoint: graphqlEndpoint,
		repoName:    repoName,
		version:     version,
		prp:         newPRProcessor(repoName, ignoredLabels),
	}

	return loader.loadPullRequests(client)
}

func mkClient() *http.Client {
	client := &http.Client{
		Timeout: 15 * time.Second,
	}

	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		tokenSource := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
		ctx := context.WithValue(context.Background(), oauth2.HTTPClient, client)

		return oauth2.NewClient(ctx, tokenSource)
	}

	return client
}

type prLoader struct {
	apiEndpoint string
	repoName    string
	version     string
	prp         *prProcessor
}

func (loader *prLoader) loadPullRequests(client *http.Client) ([]PullRequest, error) {
	var pullRequests []PullRequest
	var cursor *string

	for {
		apiResp, err := loader.mkRequest(client, cursor)
		if err != nil {
			return pullRequests, err
		}

		batch := loader.prp.extractPullRequests(apiResp)
		pullRequests = append(pullRequests, batch...)

		if !apiResp.Data.Search.PageInfo.HasNextPage {
			return pullRequests, nil
		}

		cursor = &apiResp.Data.Search.PageInfo.EndCursor
	}
}

func (loader *prLoader) mkRequest(client *http.Client, cursor *string) (*apiResponse, error) {
	req, err := loader.buildRequest(cursor)
	if err != nil {
		return nil, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	defer func() {
		if resp.Body != nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("received status %s from the API", resp.Status)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxPayloadSize))
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var apiResp apiResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &apiResp, nil
}

type apiResponse struct {
	Data struct {
		Search struct {
			Nodes []struct {
				Number   int    `json:"number"`
				Title    string `json:"title"`
				Merged   bool   `json:"merged"`
				BodyHTML string `json:"bodyHTML"`
				Labels   *struct {
					Edges []struct {
						Node struct {
							Name string `json:"name"`
						} `json:"node"`
					} `json:"edges"`
				} `json:"labels"`
			} `json:"nodes"`
			PageInfo struct {
				HasNextPage bool   `json:"hasNextPage"`
				EndCursor   string `json:"endCursor"`
			} `json:"pageInfo"`
		} `json:"search"`
	} `json:"data"`
}

func (loader *prLoader) buildRequest(cursor *string) (*http.Request, error) {
	variables := map[string]interface{}{
		"q":     fmt.Sprintf("repo:%s is:pr is:closed label:v%s", loader.repoName, loader.version),
		"after": cursor,
	}

	body := struct {
		Query     string                 `json:"query"`
		Variables map[string]interface{} `json:"variables"`
	}{
		Variables: variables,
		Query:     queryBody,
	}

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request body: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, loader.apiEndpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}

	req.Header.Add("Content-type", contentType)

	return req, nil
}
