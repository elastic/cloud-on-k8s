// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package github

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

var (
	ignoredLabels = map[string]struct{}{
		">non-issue":                 {},
		">refactoring":               {},
		">docs":                      {},
		">test":                      {},
		":ci":                        {},
		"backport":                   {},
		"exclude-from-release-notes": {},
	}

	want = []PullRequest{
		{Number: 3295, Title: "Do not manage keystore if already initialized", Labels: map[string]struct{}{">bug": {}, "v1.2.0": {}}, Issues: []int{3294}},
		{Number: 3285, Title: "Add additional Kibana encryption keys", Labels: map[string]struct{}{">enhancement": {}, "release-highlight": {}, "v1.2.0": {}}, Issues: []int{2279}},
		{Number: 3273, Title: "Only provision Enterprise licenses as of 7.8.1", Labels: map[string]struct{}{">bug": {}, "v1.2.0": {}}, Issues: []int{3272}},
		{Number: 3233, Title: "Name transport service port", Labels: map[string]struct{}{">enhancement": {}, "v1.2.0": {}}, Issues: []int(nil)},
	}
)

func TestDoLoadPullRequests(t *testing.T) {
	firstPayload, err := os.ReadFile("testdata/payload1.json")
	require.NoError(t, err)

	secondPayload, err := os.ReadFile("testdata/payload2.json")
	require.NoError(t, err)

	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Query     string                 `json:"query"`
			Variables map[string]interface{} `json:"variables"`
		}

		reqBytes, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		if err := json.Unmarshal(reqBytes, &req); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// if a cursor is not passed, this is the very first request
		if req.Variables["after"] == nil {
			_, _ = io.Copy(w, bytes.NewReader(firstPayload))
			return
		}

		_, _ = io.Copy(w, bytes.NewReader(secondPayload))
	}))

	defer ts.Close()

	loader := &prLoader{
		apiEndpoint: ts.URL,
		repoName:    "elastic/cloud-on-k8s",
		version:     "1.2.0",
		prp:         newPRProcessor("elastic/cloud-on-k8s", ignoredLabels),
	}

	have, err := loader.loadPullRequests(ts.Client())
	require.NoError(t, err)
	require.Equal(t, want, have)
}
