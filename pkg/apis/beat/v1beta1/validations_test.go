// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package v1beta1

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"

	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
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
			name:    "bad type - space",
			typ:     "file beat",
			wantErr: true,
		},
		{
			name:    "bad type - illegal characters",
			typ:     "filebeat$2",
			wantErr: true,
		},
		{
			name:    "injection",
			typ:     "filebeat,superuser",
			wantErr: true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := checkBeatType(&Beat{Spec: BeatSpec{Type: tt.typ}})
			require.Equal(t, tt.wantErr, len(got) > 0)
		})
	}
}

func Test_checkSpec(t *testing.T) {
	tests := []struct {
		name    string
		beat    Beat
		wantErr bool
	}{
		{
			name: "deployment absent, dset present",
			beat: Beat{
				Spec: BeatSpec{
					DaemonSet: &DaemonSetSpec{},
				},
			},
			wantErr: false,
		},
		{
			name: "deployment present, dset absent",
			beat: Beat{
				Spec: BeatSpec{
					Deployment: &DeploymentSpec{},
				},
			},
			wantErr: false,
		},
		{
			name: "neither present",
			beat: Beat{
				Spec: BeatSpec{},
			},
			wantErr: true,
		},
		{
			name: "both present",
			beat: Beat{
				Spec: BeatSpec{
					Deployment: &DeploymentSpec{},
					DaemonSet:  &DaemonSetSpec{},
				},
			},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := checkSpec(&tc.beat)
			assert.Equal(t, tc.wantErr, len(got) > 0)
		})
	}
}

func Test_checkAssociations(t *testing.T) {
	type args struct {
		b *Beat
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "no ref: OK",
			args: args{
				b: &Beat{},
			},
			wantErr: false,
		},
		{
			name: "mix secret named and named refs: OK",
			args: args{
				b: &Beat{
					Spec: BeatSpec{
						ElasticsearchRef: commonv1.ObjectSelector{Name: "bla", Namespace: "blub"},
						KibanaRef:        commonv1.ObjectSelector{SecretName: "bli"},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "secret named ref with a name: NOK",
			args: args{
				b: &Beat{
					Spec: BeatSpec{
						ElasticsearchRef: commonv1.ObjectSelector{SecretName: "bla", Name: "bla"},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "secret named ref with a namespace: NOK",
			args: args{
				b: &Beat{
					Spec: BeatSpec{
						KibanaRef: commonv1.ObjectSelector{SecretName: "bli", Namespace: "blub"},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "invalid metrics stackmon ref with name and secretname: NOK",
			args: args{
				b: &Beat{
					Spec: BeatSpec{
						Monitoring: commonv1.Monitoring{
							Metrics: commonv1.MetricsMonitoring{
								ElasticsearchRefs: []commonv1.ObjectSelector{{SecretName: "bli", Namespace: "blub"}},
							},
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "invalid logs stackmon ref with name and secretname: NOK",
			args: args{
				b: &Beat{
					Spec: BeatSpec{
						Monitoring: commonv1.Monitoring{
							Logs: commonv1.LogsMonitoring{
								ElasticsearchRefs: []commonv1.ObjectSelector{{SecretName: "bli", Namespace: "blub"}},
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
			errs := checkAssociations(tt.args.b)
			if (len(errs) != 0) != tt.wantErr {
				t.Errorf("checkAssociationst() errors = %v, wantErr %v", errs, tt.wantErr)
			}
		})
	}
}

func Test_checkNoDowngrade(t *testing.T) {
	type args struct {
		prev *Beat
		curr *Beat
	}
	tests := []struct {
		name string
		args args
		want field.ErrorList
	}{
		{
			name: "No downgrade",
			args: args{
				prev: &Beat{Spec: BeatSpec{Version: "7.17.0"}},
				curr: &Beat{Spec: BeatSpec{Version: "8.2.0"}},
			},
			want: nil,
		},
		{
			name: "Downgrade NOK",
			args: args{
				prev: &Beat{Spec: BeatSpec{Version: "8.2.0"}},
				curr: &Beat{Spec: BeatSpec{Version: "8.1.0"}},
			},
			want: field.ErrorList{&field.Error{Type: field.ErrorTypeForbidden, Field: "spec.version", BadValue: "", Detail: "Version downgrades are not supported"}},
		},
		{
			name: "Downgrade with override OK",
			args: args{
				prev: &Beat{Spec: BeatSpec{Version: "8.2.0"}},
				curr: &Beat{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{
					commonv1.DisableDowngradeValidationAnnotation: "true",
				}}, Spec: BeatSpec{Version: "8.1.0"}},
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

func Test_checkMonitoring(t *testing.T) {
	tests := []struct {
		name string
		beat *Beat
		want field.ErrorList
	}{
		{
			name: "stack monitoring not enabled returns nil",
			beat: &Beat{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "testbeat",
					Namespace: "test",
				},
				Spec: BeatSpec{
					Type:      "filebeat",
					Version:   "8.2.3",
					DaemonSet: &DaemonSetSpec{},
				},
			},
			want: nil,
		},
		{
			name: "stack monitoring enabled with only metrics ref is valid",
			beat: &Beat{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "testbeat",
					Namespace: "test",
				},
				Spec: BeatSpec{
					Type:      "filebeat",
					Version:   "8.2.3",
					DaemonSet: &DaemonSetSpec{},
					Monitoring: commonv1.Monitoring{
						Metrics: commonv1.MetricsMonitoring{
							ElasticsearchRefs: []commonv1.ObjectSelector{
								{
									Name:      "es",
									Namespace: "test",
								},
							},
						},
					},
				},
			},
			want: nil,
		},
		{
			name: "stack monitoring enabled with only logs ref is valid",
			beat: &Beat{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "testbeat",
					Namespace: "test",
				},
				Spec: BeatSpec{
					Type:      "filebeat",
					Version:   "8.2.3",
					DaemonSet: &DaemonSetSpec{},
					Monitoring: commonv1.Monitoring{
						Logs: commonv1.LogsMonitoring{
							ElasticsearchRefs: []commonv1.ObjectSelector{
								{
									Name:      "es",
									Namespace: "test",
								},
							},
						},
					},
				},
			},
			want: nil,
		},
		{
			name: "stack monitoring enabled with both logs and metrics ref is valid",
			beat: &Beat{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "testbeat",
					Namespace: "test",
				},
				Spec: BeatSpec{
					Type:      "filebeat",
					Version:   "8.2.3",
					DaemonSet: &DaemonSetSpec{},
					Monitoring: commonv1.Monitoring{
						Logs: commonv1.LogsMonitoring{
							ElasticsearchRefs: []commonv1.ObjectSelector{
								{
									Name:      "es",
									Namespace: "test",
								},
							},
						},
						Metrics: commonv1.MetricsMonitoring{
							ElasticsearchRefs: []commonv1.ObjectSelector{
								{
									Name:      "es",
									Namespace: "test",
								},
							},
						},
					},
				},
			},
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := checkMonitoring(tt.beat); !cmp.Equal(got, tt.want) {
				t.Errorf("checkMonitoring() = diff: %s", cmp.Diff(got, tt.want))
			}
		})
	}
}
