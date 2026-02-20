// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package client

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModel_RemoteCluster(t *testing.T) {
	tests := []struct {
		name string
		arg  RemoteClustersSettings
		want string
	}{
		{
			name: "Simple remote cluster",
			arg: RemoteClustersSettings{
				PersistentSettings: &SettingsGroup{
					Cluster: RemoteClusters{
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
			arg: RemoteClustersSettings{
				PersistentSettings: &SettingsGroup{
					Cluster: RemoteClusters{
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

func TestModel_License(t *testing.T) {
	tests := []struct {
		name    string
		license License
		want    string
	}{
		{
			name: "platinum",
			license: License{
				UID:                "304d04fe-c2d2-8774-cd34-7a71a4cc8c4d",
				Type:               "platinum",
				IssueDateInMillis:  1576000000000,
				ExpiryDateInMillis: 1590000000000,
				MaxNodes:           100,
				IssuedTo:           "ECK Test",
				Issuer:             "ECK Tester",
				StartDateInMillis:  1576000000000,
				Signature:          "AAA...xyz",
			},
			want: `{"uid":"304d04fe-c2d2-8774-cd34-7a71a4cc8c4d","type":"platinum","issue_date_in_millis":1576000000000,"expiry_date_in_millis":1590000000000,"max_nodes":100,"issued_to":"ECK Test","issuer":"ECK Tester","start_date_in_millis":1576000000000,"signature":"AAA...xyz"}`,
		},
		{
			name: "enterprise",
			license: License{
				UID:                "304d04fe-c2d2-8774-cd34-7a71a4cc8c4d",
				Type:               "enterprise",
				IssueDateInMillis:  1576000000000,
				ExpiryDateInMillis: 1590000000000,
				MaxResourceUnits:   100,
				IssuedTo:           "ECK Test",
				Issuer:             "ECK Tester",
				StartDateInMillis:  1576000000000,
				Signature:          "AAA...xyz",
			},
			want: `{"uid":"304d04fe-c2d2-8774-cd34-7a71a4cc8c4d","type":"enterprise","issue_date_in_millis":1576000000000,"expiry_date_in_millis":1590000000000,"max_resource_units":100,"issued_to":"ECK Test","issuer":"ECK Tester","start_date_in_millis":1576000000000,"signature":"AAA...xyz"}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			json, err := json.Marshal(tt.license)
			assert.NoError(t, err)
			assert.Equal(t, tt.want, string(json))
		})
	}
}

func TestClusterRoutingAllocation(t *testing.T) {
	clusterSettingsSample := `{"persistent":{},"transient":{"cluster":{"routing":{"allocation":{"enable":"none","exclude":{"_name":"excluded"}}}}}}`
	expected := ClusterRoutingAllocation{Transient: AllocationSettings{Cluster: ClusterRoutingSettings{Routing: RoutingSettings{Allocation: RoutingAllocationSettings{Enable: "none", Exclude: AllocationExclude{Name: "excluded"}}}}}}

	var settings ClusterRoutingAllocation
	require.NoError(t, json.Unmarshal([]byte(clusterSettingsSample), &settings))
	require.Equal(t, expected, settings)
	require.Equal(t, false, settings.Transient.IsShardsAllocationEnabled())
}

func TestLicenseUpdateResponse_IsSuccess(t *testing.T) {
	type fields struct {
		Acknowledged  bool
		LicenseStatus string
	}
	tests := []struct {
		name   string
		fields fields
		want   bool
	}{
		{
			name: "success: valid & ack'ed",
			fields: fields{
				Acknowledged:  true,
				LicenseStatus: "valid",
			},
			want: true,
		},
		{
			name: "no success: not valid",
			fields: fields{
				Acknowledged:  true,
				LicenseStatus: "invalid",
			},
			want: false,
		},
		{
			name: "no success: not ack'ed",
			fields: fields{
				Acknowledged:  false,
				LicenseStatus: "valid",
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lr := LicenseUpdateResponse{
				Acknowledged:  tt.fields.Acknowledged,
				LicenseStatus: tt.fields.LicenseStatus,
			}
			if got := lr.IsSuccess(); got != tt.want {
				t.Errorf("IsSuccess() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDuration_MarshalJSON(t *testing.T) {
	tests := []struct {
		name string
		d    Duration
		want string
	}{
		{
			name: "zero duration",
			d:    Duration(0),
			want: `"0s"`,
		},
		{
			name: "5 minutes",
			d:    Duration(5 * time.Minute),
			want: `"5m"`,
		},
		{
			name: "1 hour 30 minutes",
			d:    Duration(1*time.Hour + 30*time.Minute),
			want: `"90m"`,
		},
		{
			name: "500 milliseconds",
			d:    Duration(500 * time.Millisecond),
			want: `"500ms"`,
		},
		{
			name: "negative duration",
			d:    Duration(-10 * time.Second),
			want: `"-10s"`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := json.Marshal(tt.d)
			require.NoError(t, err)
			assert.Equal(t, tt.want, string(got))
		})
	}
}

func TestDuration_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    Duration
		wantErr bool
	}{
		{
			name:  "zero duration",
			input: `"0s"`,
			want:  Duration(0),
		},
		{
			name:  "5 minutes",
			input: `"5m"`,
			want:  Duration(5 * time.Minute),
		},
		{
			name:  "1 hour 30 minutes",
			input: `"90m"`,
			want:  Duration(1*time.Hour + 30*time.Minute),
		},
		{
			name:  "500 milliseconds",
			input: `"500ms"`,
			want:  Duration(500 * time.Millisecond),
		},
		{
			name:  "negative duration",
			input: `"-10s"`,
			want:  Duration(-10 * time.Second),
		},
		{
			name:    "invalid duration string",
			input:   `"not_a_duration"`,
			wantErr: true,
		},
		{
			name:    "not a string",
			input:   `12345`,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got Duration
			err := json.Unmarshal([]byte(tt.input), &got)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestDuration_ShutdownRequest(t *testing.T) {
	tests := []struct {
		name         string
		request      ShutdownRequest
		expectedJSON string
	}{
		{
			name: "non empty",
			request: ShutdownRequest{
				Type:            Restart,
				Reason:          "rolling upgrade",
				AllocationDelay: ptr(Duration(20 * time.Minute)),
			},
			expectedJSON: `{"type":"restart","reason":"rolling upgrade","allocation_delay":"20m"}`,
		},
		{
			name: "empty",
			request: ShutdownRequest{
				Type:   Remove,
				Reason: "decommission",
			},
			expectedJSON: `{"type":"remove","reason":"decommission"}`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			data, err := json.Marshal(tc.request)
			require.NoError(t, err)
			assert.JSONEq(t, tc.expectedJSON, string(data))

			var restored ShutdownRequest
			require.NoError(t, json.Unmarshal(data, &restored))
			assert.Equal(t, tc.request, restored)
		})
	}
}

func TestShutdownResponse(t *testing.T) {
	nodeShudownSample := `{
	"nodes": [{
		"node_id": "PQKHA4xCQd2xErO2fUK-hg",
		"type": "REMOVE",
		"reason": "2331481",
		"shutdown_startedmillis": 1643648932189,
		"status": "IN_PROGRESS",
		"shard_migration": {
			"status": "IN_PROGRESS",
			"shard_migrations_remaining": 1
		},
		"persistent_tasks": {
			"status": "COMPLETE"
		},
		"plugins": {
			"status": "COMPLETE"
		}
	}]
}`
	expected := ShutdownResponse{
		Nodes: []NodeShutdown{
			{
				NodeID:                "PQKHA4xCQd2xErO2fUK-hg",
				Type:                  "REMOVE",
				Reason:                "2331481",
				ShutdownStartedMillis: 1643648932189,
				Status:                ShutdownInProgress,
				ShardMigration: ShardMigration{
					Status:                   ShutdownInProgress,
					ShardMigrationsRemaining: 1,
					Explanation:              "",
				},
				PersistentTasks: PersistentTasks{
					Status: ShutdownComplete,
				},
				Plugins: Plugins{
					Status: ShutdownComplete,
				},
			},
		},
	}

	var actual ShutdownResponse
	require.NoError(t, json.Unmarshal([]byte(nodeShudownSample), &actual))
	require.Equal(t, expected, actual)
}

func TestFormatTimeValue(t *testing.T) {
	tests := []struct {
		name string
		d    time.Duration
		want string
	}{
		{
			name: "zero",
			d:    0,
			want: "0s",
		},
		{
			name: "exact days",
			d:    3 * 24 * time.Hour,
			want: "3d",
		},
		{
			name: "one day",
			d:    24 * time.Hour,
			want: "1d",
		},
		{
			name: "exact hours",
			d:    5 * time.Hour,
			want: "5h",
		},
		{
			name: "exact minutes",
			d:    15 * time.Minute,
			want: "15m",
		},
		{
			name: "exact seconds",
			d:    42 * time.Second,
			want: "42s",
		},
		{
			name: "exact milliseconds",
			d:    750 * time.Millisecond,
			want: "750ms",
		},
		{
			name: "exact microseconds",
			d:    300 * time.Microsecond,
			want: "300micros",
		},
		{
			name: "nanoseconds",
			d:    123 * time.Nanosecond,
			want: "123nanos",
		},
		{
			name: "one nanosecond",
			d:    1 * time.Nanosecond,
			want: "1nanos",
		},
		{
			name: "hours not evenly divisible into days",
			d:    25 * time.Hour,
			want: "25h",
		},
		{
			name: "minutes not evenly divisible into hours",
			d:    90 * time.Minute,
			want: "90m",
		},
		{
			name: "seconds not evenly divisible into minutes",
			d:    90 * time.Second,
			want: "90s",
		},
		{
			name: "mixed hours and minutes falls back to minutes",
			d:    2*time.Hour + 30*time.Minute,
			want: "150m",
		},
		{
			name: "mixed minutes and seconds falls back to seconds",
			d:    1*time.Minute + 30*time.Second,
			want: "90s",
		},
		{
			name: "mixed seconds and milliseconds falls back to milliseconds",
			d:    1*time.Second + 500*time.Millisecond,
			want: "1500ms",
		},
		{
			name: "mixed milliseconds and microseconds falls back to microseconds",
			d:    1*time.Millisecond + 500*time.Microsecond,
			want: "1500micros",
		},
		{
			name: "mixed microseconds and nanoseconds falls back to nanoseconds",
			d:    1*time.Microsecond + 1*time.Nanosecond,
			want: "1001nanos",
		},
		{
			name: "negative duration in seconds",
			d:    -10 * time.Second,
			want: "-10s",
		},
		{
			name: "negative duration in hours",
			d:    -2 * time.Hour,
			want: "-2h",
		},
		{
			name: "large number of days",
			d:    365 * 24 * time.Hour,
			want: "365d",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatTimeValue(tt.d)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestParseTimeValue(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    time.Duration
		wantErr bool
	}{
		{
			name:  "days",
			input: "3d",
			want:  3 * 24 * time.Hour,
		},
		{
			name:  "one day",
			input: "1d",
			want:  24 * time.Hour,
		},
		{
			name:  "hours",
			input: "5h",
			want:  5 * time.Hour,
		},
		{
			name:  "minutes",
			input: "15m",
			want:  15 * time.Minute,
		},
		{
			name:  "seconds",
			input: "42s",
			want:  42 * time.Second,
		},
		{
			name:  "zero seconds",
			input: "0s",
			want:  0,
		},
		{
			name:  "milliseconds",
			input: "750ms",
			want:  750 * time.Millisecond,
		},
		{
			name:  "microseconds",
			input: "300micros",
			want:  300 * time.Microsecond,
		},
		{
			name:  "nanoseconds",
			input: "123nanos",
			want:  123 * time.Nanosecond,
		},
		{
			name:  "one nanosecond",
			input: "1nanos",
			want:  time.Nanosecond,
		},
		{
			name:  "negative seconds",
			input: "-10s",
			want:  -10 * time.Second,
		},
		{
			name:  "negative hours",
			input: "-2h",
			want:  -2 * time.Hour,
		},
		{
			name:  "large days",
			input: "365d",
			want:  365 * 24 * time.Hour,
		},
		{
			name:    "empty string",
			input:   "",
			wantErr: true,
		},
		{
			name:    "just numbers",
			input:   "123",
			wantErr: true,
		},
		{
			name:    "no unit",
			input:   "42",
			wantErr: true,
		},
		{
			name:    "no number",
			input:   "ms",
			wantErr: true,
		},
		{
			name:    "only minus",
			input:   "-s",
			wantErr: true,
		},
		{
			name:    "unknown unit",
			input:   "5w",
			wantErr: true,
		},
		{
			name:    "unknown long unit",
			input:   "5seconds",
			wantErr: true,
		},
		{
			name:    "decimal number",
			input:   "1.5s",
			wantErr: true,
		},
		{
			name:    "spaces",
			input:   "5 s",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseTimeValue(tt.input)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func ptr[T any](t T) *T { return &t }
