package snapshots

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/elastic/stack-operators/stack-operator/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/client"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
)

func TestValidateSnapshotCredentials(t *testing.T) {
	tests := []struct {
		name string
		args map[string][]byte
		want error
	}{
		{
			name: "complete key file no error",
			args: map[string][]byte{
				"service-account.json": []byte(
					`{
                      "type": "service_account",
                      "project_id": "your-project-id",
                      "private_key_id": "...",
                      "private_key": "-----BEGIN PRIVATE KEY-----\n...\n-----END PRIVATE KEY-----\n",
                      "client_email": "service-account-for-your-repository@your-project-id.iam.gserviceaccount.com",
                      "client_id": "...",
                      "auth_uri": "https://accounts.google.com/o/oauth2/auth",
                      "token_uri": "https://accounts.google.com/o/oauth2/token",
                      "auth_provider_x509_cert_url": "https://www.googleapis.com/oauth2/v1/certs",
                      "client_x509_cert_url": "https://www.googleapis.com/robot/v1/metadata/x509/your-bucket@your-project-id.iam.gserviceaccount.com"
                    }`),
			},
			want: nil,
		},
		{
			name: "missing keys cause an error",
			args: map[string][]byte{
				"something-missing": []byte(`{"type": "foo"}`),
			},
			want: errors.New("Expected keys [project_id private_key_id private_key client_email client_id auth_uri token_uri auth_provider_x509_cert_url client_x509_cert_url] not found in something-missing gcs credential file"),
		},
		{
			name: "non-Json data is an error",
			args: map[string][]byte{
				"something-missing": []byte("~?~"),
			},
			want: errors.New("gcs secrets need to be JSON, PKCS12 is not supported: invalid character '~' looking for beginning of value"),
		},
	}

	for _, tt := range tests {
		actual := ValidateSnapshotCredentials(v1alpha1.SnapshotRepositoryTypeGCS, tt.args)
		if tt.want != nil {
			assert.EqualError(t, actual, tt.want.Error())
		} else {
			assert.NoError(t, actual)
		}
	}
}

func TestRepositoryCredentialsKey(t *testing.T) {

	tests := []struct {
		name string
		args v1alpha1.SnapshotRepository
		want string
	}{
		{
			name: "gcs is currently the only one",
			args: v1alpha1.SnapshotRepository{
				Type: v1alpha1.SnapshotRepositoryTypeGCS,
			},
			want: "gcs.client.elastic-internal.credentials_file",
		},
		{
			name: "empty string is the default",
			args: v1alpha1.SnapshotRepository{},
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := RepositoryCredentialsKey(tt.args); got != tt.want {
				t.Errorf("RepositoryCredentialsKey() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSettings_NextPhase(t *testing.T) {
	now := time.Date(2018, 11, 17, 0, 9, 0, 0, time.UTC)
	settings := Settings{
		Interval: 30 * time.Minute,
	}

	tests := []struct {
		name string
		args []client.Snapshot
		want Phase
	}{
		{
			name: "no snapshots means take one",
			args: []client.Snapshot{},
			want: PhaseTake,
		},
		{
			name: "last snapshot too old means take a new one",
			args: []client.Snapshot{
				client.Snapshot{
					State:   client.SnapshotStateSuccess,
					EndTime: now.Add(-1 * time.Hour),
				},
				client.Snapshot{
					State:   client.SnapshotStateSuccess,
					EndTime: now.Add(-2 * time.Hour),
				},
			},
			want: PhaseTake,
		},
		{
			name: "last snapshot recent enough means purge",
			args: []client.Snapshot{
				client.Snapshot{
					State:   client.SnapshotStateSuccess,
					EndTime: now.Add(-29 * time.Minute),
				},
				client.Snapshot{
					State:   client.SnapshotStateSuccess,
					EndTime: now.Add(-1 * time.Hour),
				},
			},
			want: PhasePurge,
		},
		{
			name: "recent enough includes failures",
			args: []client.Snapshot{
				client.Snapshot{
					State:   client.SnapshotStateFailed,
					EndTime: now.Add(-29 * time.Minute),
				},
			},
			want: PhasePurge,
		},
		{
			name: "last snapshot still running means wait",
			args: []client.Snapshot{
				client.Snapshot{
					State: client.SnapshotStateInProgress,
				},
			},
			want: PhaseWait,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := settings.nextPhase(tt.args, now); got != tt.want {
				t.Errorf("nextPhase was %v want %v", got, tt.want)
			}
		})
	}
}

type mockClient struct {
	Snapshots client.SnapshotsList
	Deleted   []string
	Taken     []string
}

func (m *mockClient) GetAllSnapshots(ctx context.Context, repo string) (client.SnapshotsList, error) {
	return m.Snapshots, nil
}

func (m *mockClient) TakeSnapshot(ctx context.Context, repo string, snapshot string) error {
	m.Taken = append(m.Taken, snapshot)
	return nil
}

func (m *mockClient) DeleteSnapshot(ctx context.Context, repo string, snapshot string) error {
	m.Deleted = append(m.Deleted, snapshot)
	return nil
}

type clientAssertion func(m mockClient)

func TestMaintain(t *testing.T) {
	now := time.Now()
	settings := Settings{
		Repository: "test-repo",
		Max:        2,
		Interval:   30 * time.Minute,
	}

	tests := []struct {
		name string
		args func() *mockClient
		want clientAssertion
	}{
		{
			name: "no snapshots exist take one",
			args: func() *mockClient {
				return new(mockClient)
			},
			want: func(m mockClient) {
				assert.Empty(t, m.Deleted)
				assert.Len(t, m.Taken, 1)
			},
		},
		{
			name: "most recent snapshot too old take a new one",
			args: func() *mockClient {
				m := new(mockClient)
				m.Snapshots = client.SnapshotsList{
					Snapshots: []client.Snapshot{
						client.Snapshot{
							State:     client.SnapshotStateSuccess,
							StartTime: now.Add(-120 * time.Minute),
							EndTime:   now.Add(-115 * time.Minute),
						},
						client.Snapshot{
							State:     client.SnapshotStateSuccess,
							StartTime: now.Add(-60 * time.Minute),
							EndTime:   now.Add(-55 * time.Minute),
						},
					},
				}
				return m
			},
			want: func(m mockClient) {
				assert.Empty(t, m.Deleted)
				assert.Len(t, m.Taken, 1)

			},
		},
		{
			name: "most recent snapshot new enough, purge",
			args: func() *mockClient {
				m := new(mockClient)
				m.Snapshots = client.SnapshotsList{
					// Purposely out of order to test sorting as well
					Snapshots: []client.Snapshot{
						client.Snapshot{
							State:     client.SnapshotStateSuccess,
							StartTime: now.Add(-60 * time.Minute),
							EndTime:   now.Add(-55 * time.Minute),
						},
						client.Snapshot{
							Snapshot:  "delete-me",
							State:     client.SnapshotStateSuccess,
							StartTime: now.Add(-150 * time.Minute),
							EndTime:   now.Add(-145 * time.Minute),
						},
						client.Snapshot{
							Snapshot:  "delete-me-too-just-not-yet",
							State:     client.SnapshotStateSuccess,
							StartTime: now.Add(-120 * time.Minute),
							EndTime:   now.Add(-115 * time.Minute),
						},
						client.Snapshot{
							State:     client.SnapshotStateSuccess,
							StartTime: now.Add(-20 * time.Minute),
							EndTime:   now.Add(-15 * time.Minute),
						},
					},
				}
				return m
			},
			want: func(m mockClient) {
				assert.Empty(t, m.Taken)
				assert.Equal(t, []string{"delete-me"}, m.Deleted)
			},
		},
		{
			name: "ongoing snapshot just wait",
			args: func() *mockClient {
				m := new(mockClient)
				m.Snapshots = client.SnapshotsList{
					Snapshots: []client.Snapshot{
						client.Snapshot{
							State:     client.SnapshotStateInProgress,
							StartTime: now.Add(-30 * time.Minute),
						},
						client.Snapshot{
							State:     client.SnapshotStateSuccess,
							StartTime: now.Add(-60 * time.Minute),
							EndTime:   now.Add(-55 * time.Minute),
						},
					},
				}
				return m
			},
			want: func(m mockClient) {
				assert.Empty(t, m.Taken)
				assert.Empty(t, m.Deleted)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mocked := tt.args()
			if err := ExecuteNextPhase(mocked, settings); err != nil {
				t.Errorf("Maintain() error = %v", err)
			}
			tt.want(*mocked)
		})
	}
}

func TestSettings_snapshotsToPurge(t *testing.T) {
	settings := Settings{
		Max: 2,
	}

	tests := []struct {
		name string
		args []client.Snapshot
		want []client.Snapshot
	}{
		{
			name: "no snapshots: nothing to purge",
			args: []client.Snapshot{},
			want: nil,
		},
		{
			name: "less than max snapshots: nothing to purge",
			args: []client.Snapshot{
				client.Snapshot{
					State: client.SnapshotStateSuccess,
				},
			},
			want: nil,
		},
		{
			name: "exceeding max, delete extra snapshots",
			args: []client.Snapshot{
				client.Snapshot{
					State: client.SnapshotStateSuccess,
				},
				client.Snapshot{
					State: client.SnapshotStateSuccess,
				},
				client.Snapshot{
					Snapshot: "to-delete-1",
					State:    client.SnapshotStateSuccess,
				},

				client.Snapshot{
					Snapshot: "to-delete-2",
					State:    client.SnapshotStateSuccess,
				},
			},
			want: []client.Snapshot{
				client.Snapshot{
					Snapshot: "to-delete-1",
					State:    client.SnapshotStateSuccess,
				},

				client.Snapshot{
					Snapshot: "to-delete-2",
					State:    client.SnapshotStateSuccess,
				},
			},
		},
		{
			name: "exceeding max, but failures don't count",
			args: []client.Snapshot{
				client.Snapshot{
					State: client.SnapshotStateSuccess,
				},
				client.Snapshot{
					Snapshot: "to-delete-1",
					State:    client.SnapshotStateFailed,
				},

				client.Snapshot{
					Snapshot: "to-delete-2",
					State:    client.SnapshotStatePartial,
				},

				client.Snapshot{
					State: client.SnapshotStateSuccess,
				},
			},
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := snapshotsToPurge(tt.args, settings); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Settings.snapshotsToPurge() = %v, want %v", got, tt.want)
			}
		})
	}
}
