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
	const svcURL = "https://test-es-http.default.svc:9200"

	tests := []struct {
		name                         string
		clientAuthenticationRequired bool
	}{
		{
			name:                         "without client authentication",
			clientAuthenticationRequired: false,
		},
		{
			name:                         "with client authentication",
			clientAuthenticationRequired: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := RenderPreStopHookScript(svcURL, tt.clientAuthenticationRequired)
			require.NoError(t, err)
			snaps.MatchSnapshot(t, got)
		})
	}
}
