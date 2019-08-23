// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package observer

import (
	"testing"

	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
)

func Test_hasHealthChanged(t *testing.T) {
	tests := []struct {
		name     string
		previous State
		new      State
		want     bool
	}{
		{
			name:     "both nil",
			previous: State{},
			new:      State{},
			want:     false,
		},
		{
			name:     "previous nil",
			previous: State{},
			new:      State{ClusterHealth: &client.Health{Status: "green"}},
			want:     true,
		},
		{
			name:     "new nil",
			previous: State{ClusterHealth: &client.Health{Status: "green"}},
			new:      State{},
			want:     true,
		},
		{
			name:     "different values",
			previous: State{ClusterHealth: &client.Health{Status: "green"}},
			new:      State{ClusterHealth: &client.Health{Status: "red"}},
			want:     true,
		},
		{
			name:     "same values",
			previous: State{ClusterHealth: &client.Health{Status: "green"}},
			new:      State{ClusterHealth: &client.Health{Status: "green"}},
			want:     false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := hasHealthChanged(tt.previous, tt.new); got != tt.want {
				t.Errorf("hasHealthChanged() = %v, want %v", got, tt.want)
			}
		})
	}
}
