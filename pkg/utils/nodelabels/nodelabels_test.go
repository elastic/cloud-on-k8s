// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package nodelabels

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParse(t *testing.T) {
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
			assert.Equal(t, tt.want, Parse(tt.input))
		})
	}
}

func TestFromAnnotations(t *testing.T) {
	assert.Nil(t, FromAnnotations(nil))
	assert.Nil(t, FromAnnotations(map[string]string{"other": "value"}))
	assert.Equal(
		t,
		[]string{"topology.kubernetes.io/region", "topology.kubernetes.io/zone"},
		FromAnnotations(map[string]string{DownwardNodeLabelsAnnotation: "topology.kubernetes.io/zone,topology.kubernetes.io/region"}),
	)
}
