// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package settings

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func Test_OperatorPrivilegesSettingsContent(t *testing.T) {
	OperatorPrivilegesSettingsBytes := []byte(`operator:
    - usernames:
        - elastic
        - elastic-internal
      realm_type: file
      auth_type: realm
`)

	// unmarshal into OperatorPrivilegesSettings
	r := OperatorPrivilegesSettings{
		Operator: []OperatorPrivilegesSetting{
			{
				Usernames: []string{"elastic", "elastic-internal"},
				RealmType: "file",
				AuthType:  "realm",
			},
		},
	}
	asBytes, err := yaml.Marshal(&r)
	require.NoError(t, err)
	require.Equal(t, OperatorPrivilegesSettingsBytes, asBytes)
}

func TestNewOperatorPrivilegesSettings(t *testing.T) {
	type args struct {
		operatorUsernames []OperatorAccount
		tokenNames        []string
	}
	tests := []struct {
		name    string
		args    args
		want    OperatorPrivilegesSettings
		wantErr error
	}{
		{
			name: "Operator Privileges with only file realm users",
			args: args{
				operatorUsernames: []OperatorAccount{
					{
						Names:     []string{"elastic"},
						RealmType: OperatorRealmTypeFile,
					},
					{
						Names:     []string{"elastic-internal"},
						RealmType: OperatorRealmTypeFile,
					},
				},
			},
			want: OperatorPrivilegesSettings{
				Operator: []OperatorPrivilegesSetting{
					{
						Usernames: []string{"elastic"},
						RealmType: OperatorRealmTypeFile,
						AuthType:  RealmAuthType,
					},
					{
						Usernames: []string{"elastic-internal"},
						RealmType: OperatorRealmTypeFile,
						AuthType:  RealmAuthType,
					},
				},
			},
		},
		{
			name: "Operator Privileges with file realm users and service accounts but missing service tokens",
			args: args{
				operatorUsernames: []OperatorAccount{
					{
						Names:     []string{"elastic"},
						RealmType: OperatorRealmTypeFile,
					},
					{
						Names:     []string{"elastic-internal"},
						RealmType: OperatorRealmTypeFile,
					},
				},
			},
			want: OperatorPrivilegesSettings{
				Operator: []OperatorPrivilegesSetting{
					{
						Usernames: []string{"elastic"},
						RealmType: OperatorRealmTypeFile,
						AuthType:  RealmAuthType,
					},
					{
						Usernames: []string{"elastic-internal"},
						RealmType: OperatorRealmTypeFile,
						AuthType:  RealmAuthType,
					},
				},
			},
		},
		{
			name: "Operator Privileges with file realm users and service accounts",
			args: args{
				operatorUsernames: []OperatorAccount{
					{
						Names:     []string{"elastic"},
						RealmType: OperatorRealmTypeFile,
					},
					{
						Names:     []string{"elastic/auto-ops", "elastic/fleet-server", "elastic/kibana"},
						RealmType: OperatorRealmTypeServiceAccount,
					},
					{
						Names:     []string{"elastic-internal"},
						RealmType: OperatorRealmTypeFile,
					},
				},
				tokenNames: []string{
					"elastic/auto-ops/token1",
					"elastic/fleet-server/token1",
					"elastic/kibana/token1",
				},
			},
			want: OperatorPrivilegesSettings{
				Operator: []OperatorPrivilegesSetting{
					{
						Usernames: []string{"elastic"},
						RealmType: OperatorRealmTypeFile,
						AuthType:  RealmAuthType,
					},
					{
						Usernames:   []string{"elastic/auto-ops"},
						RealmType:   OperatorRealmTypeServiceAccount,
						AuthType:    TokenAuthType,
						TokenSource: "file",
						TokenNames:  []string{"token1"},
					},
					{
						Usernames:   []string{"elastic/fleet-server"},
						RealmType:   OperatorRealmTypeServiceAccount,
						AuthType:    TokenAuthType,
						TokenSource: "file",
						TokenNames:  []string{"token1"},
					},
					{
						Usernames:   []string{"elastic/kibana"},
						RealmType:   OperatorRealmTypeServiceAccount,
						AuthType:    TokenAuthType,
						TokenSource: "file",
						TokenNames:  []string{"token1"},
					},
					{
						Usernames: []string{"elastic-internal"},
						RealmType: OperatorRealmTypeFile,
						AuthType:  RealmAuthType,
					},
				},
			},
		},
		{
			name: "Operator Privileges with jwt realm users",
			args: args{
				operatorUsernames: []OperatorAccount{
					{
						Names:     []string{"platform-operator"},
						RealmType: OperatorRealmTypeJWT,
					},
				},
			},
			want: OperatorPrivilegesSettings{
				Operator: []OperatorPrivilegesSetting{
					{
						Usernames: []string{"platform-operator"},
						RealmType: OperatorRealmTypeJWT,
						RealmName: "jwt1",
					},
				},
			},
		},
		{
			name: "Operator Privileges with invalid realmtype",
			args: args{
				operatorUsernames: []OperatorAccount{
					{
						Names:     []string{"test"},
						RealmType: "test",
					},
				},
			},
			want:    OperatorPrivilegesSettings{},
			wantErr: fmt.Errorf("unknown realm type test, known realm types are _service_account and file"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewOperatorPrivilegesSettings(tt.args.operatorUsernames, tt.args.tokenNames)
			if tt.wantErr == nil {
				assert.NoError(t, err)
			} else {
				assert.Equal(t, tt.wantErr, err)
			}
			assert.Equal(t, tt.want, got)
		})
	}
}
