// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package validation

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/util/validation/field"

	lsv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/logstash/v1alpha1"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
)

func TestCheckNameLength(t *testing.T) {
	testCases := []struct {
		name         string
		logstashName string
		wantErr      bool
		wantErrMsg   string
	}{
		{
			name:         "valid configuration",
			logstashName: "test-logstash",
			wantErr:      false,
		},
		{
			name:         "long Logstash name",
			logstashName: "extremely-long-winded-and-unnecessary-name-for-logstash",
			wantErr:      true,
			wantErrMsg:   "name exceeds maximum allowed length",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ls := lsv1alpha1.Logstash{
				ObjectMeta: metav1.ObjectMeta{
					Name:      tc.logstashName,
					Namespace: "test",
				},
				Spec: lsv1alpha1.LogstashSpec{},
			}

			errList := checkNameLength(&ls)
			assert.Equal(t, tc.wantErr, len(errList) > 0)
		})
	}
}

func TestCheckNoUnknownFields(t *testing.T) {
	type args struct {
		prev *lsv1alpha1.Logstash
		curr *lsv1alpha1.Logstash
	}
	tests := []struct {
		name string
		args args
		want field.ErrorList
	}{
		{
			name: "No downgrade",
			args: args{
				prev: &lsv1alpha1.Logstash{Spec: lsv1alpha1.LogstashSpec{Version: "7.17.0"}},
				curr: &lsv1alpha1.Logstash{Spec: lsv1alpha1.LogstashSpec{Version: "8.6.1"}},
			},
			want: nil,
		},
		{
			name: "Downgrade NOK",
			args: args{
				prev: &lsv1alpha1.Logstash{Spec: lsv1alpha1.LogstashSpec{Version: "8.6.1"}},
				curr: &lsv1alpha1.Logstash{Spec: lsv1alpha1.LogstashSpec{Version: "8.5.0"}},
			},
			want: field.ErrorList{&field.Error{Type: field.ErrorTypeForbidden, Field: "spec.version", BadValue: "", Detail: "Version downgrades are not supported"}},
		},
		{
			name: "Downgrade with override OK",
			args: args{
				prev: &lsv1alpha1.Logstash{Spec: lsv1alpha1.LogstashSpec{Version: "8.6.1"}},
				curr: &lsv1alpha1.Logstash{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{
					commonv1.DisableDowngradeValidationAnnotation: "true",
				}}, Spec: lsv1alpha1.LogstashSpec{Version: "8.5.0"}},
			},
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equalf(t, tt.want, checkNoDowngrade(tt.args.prev, tt.args.curr), "checkNoDowngrade(%v, %v)", tt.args.prev, tt.args.curr)
		})
	}
}

func Test_checkSingleConfigSource(t *testing.T) {
	tests := []struct {
		name     string
		logstash lsv1alpha1.Logstash
		wantErr  bool
	}{
		{
			name: "configRef absent, config present",
			logstash: lsv1alpha1.Logstash{
				Spec: lsv1alpha1.LogstashSpec{
					Config: &commonv1.Config{},
				},
			},
			wantErr: false,
		},
		{
			name: "config absent, configRef present",
			logstash: lsv1alpha1.Logstash{
				Spec: lsv1alpha1.LogstashSpec{
					ConfigRef: &commonv1.ConfigSource{},
				},
			},
			wantErr: false,
		},
		{
			name: "neither present",
			logstash: lsv1alpha1.Logstash{
				Spec: lsv1alpha1.LogstashSpec{},
			},
			wantErr: false,
		},
		{
			name: "both present",
			logstash: lsv1alpha1.Logstash{
				Spec: lsv1alpha1.LogstashSpec{
					Config:    &commonv1.Config{},
					ConfigRef: &commonv1.ConfigSource{},
				},
			},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := checkSingleConfigSource(&tc.logstash)
			assert.Equal(t, tc.wantErr, len(got) > 0)
		})
	}
}

func Test_checkSinglePipelineSource(t *testing.T) {
	tests := []struct {
		name     string
		logstash lsv1alpha1.Logstash
		wantErr  bool
	}{
		{
			name: "pipelinesRef absent, pipelines present",
			logstash: lsv1alpha1.Logstash{
				Spec: lsv1alpha1.LogstashSpec{
					Pipelines: []commonv1.Config{},
				},
			},
			wantErr: false,
		},
		{
			name: "pipelines absent, pipelinesRef present",
			logstash: lsv1alpha1.Logstash{
				Spec: lsv1alpha1.LogstashSpec{
					PipelinesRef: &commonv1.ConfigMapOrSecretSource{},
				},
			},
			wantErr: false,
		},
		{
			name: "neither present",
			logstash: lsv1alpha1.Logstash{
				Spec: lsv1alpha1.LogstashSpec{},
			},
			wantErr: false,
		},
		{
			name: "both present",
			logstash: lsv1alpha1.Logstash{
				Spec: lsv1alpha1.LogstashSpec{
					Pipelines:    []commonv1.Config{},
					PipelinesRef: &commonv1.ConfigMapOrSecretSource{},
				},
			},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := checkSinglePipelineSource(&tc.logstash)
			assert.Equal(t, tc.wantErr, len(got) > 0)
		})
	}
}

func Test_checkPipelinesRefSource(t *testing.T) {
	tests := []struct {
		name     string
		logstash lsv1alpha1.Logstash
		wantErr  bool
	}{
		{
			name:    "nil pipelinesRef",
			wantErr: false,
		},
		{
			name: "secretName only",
			logstash: lsv1alpha1.Logstash{
				Spec: lsv1alpha1.LogstashSpec{
					PipelinesRef: &commonv1.ConfigMapOrSecretSource{
						SecretRef: commonv1.SecretRef{SecretName: "my-secret"},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "configMapName only",
			logstash: lsv1alpha1.Logstash{
				Spec: lsv1alpha1.LogstashSpec{
					PipelinesRef: &commonv1.ConfigMapOrSecretSource{
						ConfigMapRef: commonv1.ConfigMapRef{ConfigMapName: "my-cm"},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "both secretName and configMapName",
			logstash: lsv1alpha1.Logstash{
				Spec: lsv1alpha1.LogstashSpec{
					PipelinesRef: &commonv1.ConfigMapOrSecretSource{
						SecretRef:    commonv1.SecretRef{SecretName: "my-secret"},
						ConfigMapRef: commonv1.ConfigMapRef{ConfigMapName: "my-cm"},
					},
				},
			},
			wantErr: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := checkPipelinesRefSource(&tc.logstash)
			assert.Equal(t, tc.wantErr, len(got) > 0)
		})
	}
}

func Test_checkSupportedVersion(t *testing.T) {
	for _, tt := range []struct {
		name    string
		version string
		wantErr bool
	}{
		{
			name:    "below min supported",
			version: "8.5.0",
			wantErr: true,
		},
		{
			name:    "above max supported",
			version: "42.0.0",
			wantErr: true,
		},
		{
			name:    "above min supported",
			version: "8.12.0",
			wantErr: false,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			a := lsv1alpha1.Logstash{
				Spec: lsv1alpha1.LogstashSpec{
					Version: tt.version,
				},
			}
			got := checkSupportedVersion(&a)
			assert.Equal(t, tt.wantErr, len(got) > 0)
		})
	}
}

func Test_checkEsRefsAssociations(t *testing.T) {
	type args struct {
		b *lsv1alpha1.Logstash
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "no ref: OK",
			args: args{
				b: &lsv1alpha1.Logstash{},
			},
			wantErr: false,
		},
		{
			name: "mix secret named and named refs: OK",
			args: args{
				b: &lsv1alpha1.Logstash{
					Spec: lsv1alpha1.LogstashSpec{
						ElasticsearchRefs: []lsv1alpha1.ElasticsearchCluster{
							{
								ElasticsearchSelector: commonv1.ElasticsearchSelector{ObjectSelector: commonv1.ObjectSelector{SecretName: "bla"}},
								ClusterName:           "test",
							},
							{
								ElasticsearchSelector: commonv1.ElasticsearchSelector{ObjectSelector: commonv1.ObjectSelector{Name: "bla", Namespace: "blub"}},
								ClusterName:           "test2",
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "secret named ref with a name: NOK",
			args: args{
				b: &lsv1alpha1.Logstash{
					Spec: lsv1alpha1.LogstashSpec{
						ElasticsearchRefs: []lsv1alpha1.ElasticsearchCluster{
							{
								ElasticsearchSelector: commonv1.ElasticsearchSelector{ObjectSelector: commonv1.ObjectSelector{SecretName: "bla", Name: "bla"}},
								ClusterName:           "test",
							},
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "no name or secret name with namespace: NOK",
			args: args{
				b: &lsv1alpha1.Logstash{
					Spec: lsv1alpha1.LogstashSpec{
						ElasticsearchRefs: []lsv1alpha1.ElasticsearchCluster{
							{
								ElasticsearchSelector: commonv1.ElasticsearchSelector{ObjectSelector: commonv1.ObjectSelector{Namespace: "blub"}},
								ClusterName:           "test",
							},
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "no name or secret name with serviceName: NOK",
			args: args{
				b: &lsv1alpha1.Logstash{
					Spec: lsv1alpha1.LogstashSpec{
						ElasticsearchRefs: []lsv1alpha1.ElasticsearchCluster{
							{
								ElasticsearchSelector: commonv1.ElasticsearchSelector{ObjectSelector: commonv1.ObjectSelector{ServiceName: "ble"}},
								ClusterName:           "test",
							},
						},
					},
				},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := checkAssociations(tt.args.b)
			assert.Equal(t, tt.wantErr, len(got) > 0)
		})
	}
}

func Test_checkESRefsNamed(t *testing.T) {
	type args struct {
		b *lsv1alpha1.Logstash
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "no ref: OK",
			args: args{
				b: &lsv1alpha1.Logstash{},
			},
			wantErr: false,
		},
		{
			name: "one ref, missing clusterName: NOK",
			args: args{
				b: &lsv1alpha1.Logstash{
					Spec: lsv1alpha1.LogstashSpec{
						ElasticsearchRefs: []lsv1alpha1.ElasticsearchCluster{
							{
								ElasticsearchSelector: commonv1.ElasticsearchSelector{ObjectSelector: commonv1.ObjectSelector{Name: "bla", Namespace: "blub"}},
							},
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "multiple refs, each with clusterName: OK",
			args: args{
				b: &lsv1alpha1.Logstash{
					Spec: lsv1alpha1.LogstashSpec{
						ElasticsearchRefs: []lsv1alpha1.ElasticsearchCluster{
							{
								ElasticsearchSelector: commonv1.ElasticsearchSelector{ObjectSelector: commonv1.ObjectSelector{Name: "bla", Namespace: "blub"}},
								ClusterName:           "bla",
							},
							{
								ElasticsearchSelector: commonv1.ElasticsearchSelector{ObjectSelector: commonv1.ObjectSelector{Name: "bla", Namespace: "blub"}},
								ClusterName:           "blub",
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "multiple refs, one missing clusterName: NOK",
			args: args{
				b: &lsv1alpha1.Logstash{
					Spec: lsv1alpha1.LogstashSpec{
						ElasticsearchRefs: []lsv1alpha1.ElasticsearchCluster{
							{
								ElasticsearchSelector: commonv1.ElasticsearchSelector{ObjectSelector: commonv1.ObjectSelector{Name: "bla", Namespace: "blub"}},
								ClusterName:           "",
							},
							{
								ElasticsearchSelector: commonv1.ElasticsearchSelector{ObjectSelector: commonv1.ObjectSelector{Name: "bla", Namespace: "blub"}},
								ClusterName:           "default",
							},
						},
					},
				},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := checkESRefsNamed(tt.args.b)
			assert.Equal(t, tt.wantErr, len(got) > 0)
		})
	}
}

func Test_checkPauseOrchestrationAnnotation(t *testing.T) {
	testCases := []struct {
		name    string
		ls      *lsv1alpha1.Logstash
		wantErr bool
	}{
		{
			name: "pause-orchestration false",
			ls: &lsv1alpha1.Logstash{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{commonv1.PauseOrchestrationAnnotation: "false"},
				},
			},
			wantErr: false,
		},
		{
			name: "pause-orchestration true",
			ls: &lsv1alpha1.Logstash{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{commonv1.PauseOrchestrationAnnotation: "true"},
				},
			},
			wantErr: false,
		},
		{
			name: "pause-orchestration invalid",
			ls: &lsv1alpha1.Logstash{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{commonv1.PauseOrchestrationAnnotation: "True"},
				},
			},
			wantErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			errList := commonv1.CheckPauseOrchestrationAnnotation(tc.ls)
			assert.Equal(t, tc.wantErr, len(errList) > 0)
		})
	}
}

func Test_checkESRefsRole(t *testing.T) {
	rolesField := func(i int) *field.Path {
		return field.NewPath("spec").Child("elasticsearchRefs").Index(i).Child("userRoles")
	}
	forbiddenMsg := "userRoles cannot be used with secretName: no file-realm user is created for external Elasticsearch references"

	tests := []struct {
		name string
		l    *lsv1alpha1.Logstash
		want field.ErrorList
	}{
		{
			name: "no refs: OK",
			l:    &lsv1alpha1.Logstash{},
			want: nil,
		},
		{
			name: "ref without roles: OK",
			l: &lsv1alpha1.Logstash{Spec: lsv1alpha1.LogstashSpec{
				ElasticsearchRefs: []lsv1alpha1.ElasticsearchCluster{
					{
						ElasticsearchSelector: commonv1.ElasticsearchSelector{ObjectSelector: commonv1.ObjectSelector{Name: "es"}},
						ClusterName:           "es",
					},
				},
			}},
			want: nil,
		},
		{
			name: "named ref with valid roles: OK",
			l: &lsv1alpha1.Logstash{Spec: lsv1alpha1.LogstashSpec{
				ElasticsearchRefs: []lsv1alpha1.ElasticsearchCluster{
					{
						ElasticsearchSelector: commonv1.ElasticsearchSelector{ObjectSelector: commonv1.ObjectSelector{Name: "es"}},
						ClusterName:           "es",
						UserRolesOverrideSpec: commonv1.UserRolesOverrideSpec{UserRoles: []string{"custom_role", "another-role", "Role123"}},
					},
				},
			}},
			want: nil,
		},
		{
			name: "role with invalid characters: invalid",
			l: &lsv1alpha1.Logstash{Spec: lsv1alpha1.LogstashSpec{
				ElasticsearchRefs: []lsv1alpha1.ElasticsearchCluster{
					{
						ElasticsearchSelector: commonv1.ElasticsearchSelector{ObjectSelector: commonv1.ObjectSelector{Name: "es"}},
						ClusterName:           "es",
						UserRolesOverrideSpec: commonv1.UserRolesOverrideSpec{UserRoles: []string{"bad role!"}},
					},
				},
			}},
			want: field.ErrorList{
				field.Invalid(rolesField(0).Index(0), "bad role!", "invalid user role"),
			},
		},
		{
			name: "empty role string: invalid",
			l: &lsv1alpha1.Logstash{Spec: lsv1alpha1.LogstashSpec{
				ElasticsearchRefs: []lsv1alpha1.ElasticsearchCluster{
					{
						ElasticsearchSelector: commonv1.ElasticsearchSelector{ObjectSelector: commonv1.ObjectSelector{Name: "es"}},
						ClusterName:           "es",
						UserRolesOverrideSpec: commonv1.UserRolesOverrideSpec{UserRoles: []string{""}},
					},
				},
			}},
			want: field.ErrorList{
				field.Invalid(rolesField(0).Index(0), "", "invalid user role"),
			},
		},
		{
			name: "mix of valid and invalid roles: only invalid ones error, with their index",
			l: &lsv1alpha1.Logstash{Spec: lsv1alpha1.LogstashSpec{
				ElasticsearchRefs: []lsv1alpha1.ElasticsearchCluster{
					{
						ElasticsearchSelector: commonv1.ElasticsearchSelector{ObjectSelector: commonv1.ObjectSelector{Name: "es"}},
						ClusterName:           "es",
						UserRolesOverrideSpec: commonv1.UserRolesOverrideSpec{UserRoles: []string{"valid_role", "with space", "also-valid", "comma,role"}},
					},
				},
			}},
			want: field.ErrorList{
				field.Invalid(rolesField(0).Index(1), "with space", "invalid user role"),
				field.Invalid(rolesField(0).Index(3), "comma,role", "invalid user role"),
			},
		},
		{
			name: "secretName ref with roles: forbidden",
			l: &lsv1alpha1.Logstash{Spec: lsv1alpha1.LogstashSpec{
				ElasticsearchRefs: []lsv1alpha1.ElasticsearchCluster{
					{
						ElasticsearchSelector: commonv1.ElasticsearchSelector{ObjectSelector: commonv1.ObjectSelector{SecretName: "ext-secret"}},
						ClusterName:           "ext",
						UserRolesOverrideSpec: commonv1.UserRolesOverrideSpec{UserRoles: []string{"custom_role"}},
					},
				},
			}},
			want: field.ErrorList{
				field.Forbidden(rolesField(0), forbiddenMsg),
			},
		},
		{
			name: "secretName ref with invalid role: both invalid and forbidden errors",
			l: &lsv1alpha1.Logstash{Spec: lsv1alpha1.LogstashSpec{
				ElasticsearchRefs: []lsv1alpha1.ElasticsearchCluster{
					{
						ElasticsearchSelector: commonv1.ElasticsearchSelector{ObjectSelector: commonv1.ObjectSelector{SecretName: "ext-secret"}},
						ClusterName:           "ext",
						UserRolesOverrideSpec: commonv1.UserRolesOverrideSpec{UserRoles: []string{"bad role!"}},
					},
				},
			}},
			want: field.ErrorList{
				field.Invalid(rolesField(0).Index(0), "bad role!", "invalid user role"),
				field.Forbidden(rolesField(0), forbiddenMsg),
			},
		},
		{
			name: "multiple refs, only one with secretName and roles: only that ref errors",
			l: &lsv1alpha1.Logstash{Spec: lsv1alpha1.LogstashSpec{
				ElasticsearchRefs: []lsv1alpha1.ElasticsearchCluster{
					{
						ElasticsearchSelector: commonv1.ElasticsearchSelector{ObjectSelector: commonv1.ObjectSelector{Name: "es1"}},
						ClusterName:           "first",
						UserRolesOverrideSpec: commonv1.UserRolesOverrideSpec{UserRoles: []string{"valid_role"}},
					},
					{
						ElasticsearchSelector: commonv1.ElasticsearchSelector{ObjectSelector: commonv1.ObjectSelector{SecretName: "ext-secret"}},
						ClusterName:           "second",
						UserRolesOverrideSpec: commonv1.UserRolesOverrideSpec{UserRoles: []string{"custom_role"}},
					},
				},
			}},
			want: field.ErrorList{
				field.Forbidden(rolesField(1), forbiddenMsg),
			},
		},
		{
			name: "multiple refs with secretName and roles: both error",
			l: &lsv1alpha1.Logstash{Spec: lsv1alpha1.LogstashSpec{
				ElasticsearchRefs: []lsv1alpha1.ElasticsearchCluster{
					{
						ElasticsearchSelector: commonv1.ElasticsearchSelector{ObjectSelector: commonv1.ObjectSelector{SecretName: "ext-a"}},
						ClusterName:           "first",
						UserRolesOverrideSpec: commonv1.UserRolesOverrideSpec{UserRoles: []string{"role_a"}},
					},
					{
						ElasticsearchSelector: commonv1.ElasticsearchSelector{ObjectSelector: commonv1.ObjectSelector{SecretName: "ext-b"}},
						ClusterName:           "second",
						UserRolesOverrideSpec: commonv1.UserRolesOverrideSpec{UserRoles: []string{"role_b"}},
					},
				},
			}},
			want: field.ErrorList{
				field.Forbidden(rolesField(0), forbiddenMsg),
				field.Forbidden(rolesField(1), forbiddenMsg),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, checkESRefsRole(tt.l))
		})
	}
}
