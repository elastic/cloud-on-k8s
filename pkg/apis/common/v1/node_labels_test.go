// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package v1

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseDownwardNodeLabels(t *testing.T) {
	tests := []struct {
		name, input string
		want        []string
	}{
		{name: "empty", input: "", want: nil},
		{name: "whitespace only", input: " , , ", want: nil},
		{name: "single", input: "topology.kubernetes.io/zone", want: []string{"topology.kubernetes.io/zone"}},
		{
			name:  "trimmed deduplicated and sorted",
			input: " zeta.io/rack ,alpha.io/zone,alpha.io/zone, ",
			want:  []string{"alpha.io/zone", "zeta.io/rack"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, ParseDownwardNodeLabels(tt.input))
		})
	}
}

func TestDownwardNodeLabelsFromAnnotations(t *testing.T) {
	assert.Nil(t, DownwardNodeLabelsFromAnnotations(nil))
	assert.Nil(t, DownwardNodeLabelsFromAnnotations(map[string]string{"other": "value"}))
	assert.Equal(
		t,
		[]string{"topology.kubernetes.io/region", "topology.kubernetes.io/zone"},
		DownwardNodeLabelsFromAnnotations(map[string]string{DownwardNodeLabelsAnnotation: "topology.kubernetes.io/zone,topology.kubernetes.io/region"}),
	)
}
