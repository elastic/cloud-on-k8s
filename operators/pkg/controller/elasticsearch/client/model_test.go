// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package client

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestModel_RemoteCluster(t *testing.T) {
	tests := []struct {
		name string
		arg  Settings
		want string
	}{
		{
			name: "Simple remote cluster",
			arg: Settings{
				PersistentSettings: &SettingsGroup{
					Cluster: Cluster{
						RemoteClusters: map[string]RemoteCluster{
							"leader": {
								Seeds: []string{"127.0.0.1:9300"},
							},
						},
					},
				},
			},
			want: `{"persistent":{"cluster":{"remote":{"leader":{"seeds":["127.0.0.1:9300"]}}}}}`,
		},
		{
			name: "Deleted remote cluster",
			arg: Settings{
				PersistentSettings: &SettingsGroup{
					Cluster: Cluster{
						RemoteClusters: map[string]RemoteCluster{
							"leader": {
								Seeds: nil,
							},
						},
					},
				},
			},
			want: `{"persistent":{"cluster":{"remote":{"leader":{"seeds":null}}}}}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			json, err := json.Marshal(tt.arg)
			assert.NoError(t, err)
			assert.Equal(t, tt.want, string(json))
		})
	}
}
