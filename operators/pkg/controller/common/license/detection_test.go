/*
 * Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
 * or more contributor license agreements. Licensed under the Elastic License;
 * you may not use this file except in compliance with the Elastic License.
 */

package license

import (
	"encoding/json"
	"testing"

	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Test_isLicenseType(t *testing.T) {
	trialLicenseBytes, err := json.Marshal(EnterpriseLicense{
		License: LicenseSpec{
			Type: LicenseTypeEnterpriseTrial,
		},
	})
	require.NoError(t, err)
	licenseBytes, err := json.Marshal(EnterpriseLicense{
		License: LicenseSpec{
			Type: LicenseTypeEnterprise,
		},
	})
	require.NoError(t, err)
	type args struct {
		secret corev1.Secret
	}
	tests := []struct {
		name      string
		args      args
		want      bool
		wantTrial bool
	}{
		{
			name: "any secret: no license",
			args: args{
				secret: corev1.Secret{},
			},
			want:      false,
			wantTrial: false,
		},
		{
			name: "label based: trial",
			args: args{
				secret: corev1.Secret{
					ObjectMeta: v1.ObjectMeta{
						Labels: map[string]string{
							common.TypeLabelName: Type,
							LicenseLabelType:     string(LicenseTypeEnterpriseTrial),
						},
					},
				},
			},
			want:      false,
			wantTrial: true,
		},
		{
			name: "label based: enterprise",
			args: args{
				secret: corev1.Secret{
					ObjectMeta: v1.ObjectMeta{
						Labels: map[string]string{
							common.TypeLabelName: Type,
							LicenseLabelType:     string(LicenseTypeEnterprise),
						},
					},
				},
			},
			want:      true,
			wantTrial: false,
		},
		{
			name: "empty license secret: trial",
			args: args{
				secret: corev1.Secret{
					ObjectMeta: v1.ObjectMeta{
						Labels: map[string]string{
							common.TypeLabelName: Type,
						},
					},
				},
			},
			want:      false,
			wantTrial: true,
		},
		{
			name: "non-empty trial license",
			args: args{
				secret: corev1.Secret{
					ObjectMeta: v1.ObjectMeta{
						Labels: map[string]string{
							common.TypeLabelName: Type,
						},
					},
					Data: map[string][]byte{
						FileName: trialLicenseBytes,
					},
				},
			},
			want:      false,
			wantTrial: true,
		},
		{
			name: "non-empty license",
			args: args{
				secret: corev1.Secret{
					ObjectMeta: v1.ObjectMeta{
						Labels: map[string]string{
							common.TypeLabelName: Type,
						},
					},
					Data: map[string][]byte{
						FileName: licenseBytes,
					},
				},
			},
			want:      true,
			wantTrial: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isLicenseType(tt.args.secret, LicenseTypeEnterprise); got != tt.want {
				t.Errorf("isLicenseType() = %v, want %v", got, tt.want)
			}
			if got := isLicenseType(tt.args.secret, LicenseTypeEnterpriseTrial); got != tt.wantTrial {
				t.Errorf("isLicenseType() = %v, wantTrial %v", got, tt.wantTrial)
			}
		})
	}
}
