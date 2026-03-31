// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package nodespec

import (
	"testing"

	"github.com/gkampitakis/go-snaps/snaps"
	"github.com/stretchr/testify/require"
)

func TestRenderPreStopHookScript(t *testing.T) {
	got, err := RenderPreStopHookScript("https://test-es-http.default.svc:9200")
	require.NoError(t, err)
	snaps.MatchSnapshot(t, got)
}
