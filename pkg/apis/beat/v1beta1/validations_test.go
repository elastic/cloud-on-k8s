// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package v1beta1

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_checkBeatType(t *testing.T) {
	for _, tt := range []struct {
		name    string
		typ     string
		wantErr bool
	}{
		{
			name: "official type",
			typ:  "filebeat",
		},
		{
			name: "community type",
			typ:  "apachebeat",
		},
		{
			name: "bad type - space",
			typ:  "file beat",
		},
		{
			name: "bad type - illegal characters",
			typ:  "filebeat-2",
		},
		{
			name: "injection",
			typ:  "filebeat,superuser",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := checkBeatType(&Beat{Spec: BeatSpec{Type: tt.typ}})
			require.Equal(t, tt.wantErr, len(got) > 0)
		})
	}
}
