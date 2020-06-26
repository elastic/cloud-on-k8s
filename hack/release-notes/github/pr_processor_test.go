// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package github

import (
	"io/ioutil"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExtractIssues(t *testing.T) {
	issueBody, err := ioutil.ReadFile("testdata/issue_body.txt")
	require.NoError(t, err)

	want := []int{3040, 3042, 3133, 3134}

	prp := newPRProcessor("elastic/cloud-on-k8s", map[string]struct{}{})
	have := prp.extractIssues(string(issueBody))
	require.Equal(t, want, have)
}
