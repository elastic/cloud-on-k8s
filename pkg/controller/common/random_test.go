// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package common

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/sethvargo/go-password/password"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/license"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/operator"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

func TestRandomBytesRespectingLicense(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name           string
		client         k8s.Client
		initK8sObjects func(t *testing.T, client k8s.Client)
		params         operator.PasswordGeneratorParams
		expectedLen    int
		expectedError  error
		validate       func(t *testing.T, out []byte, params operator.PasswordGeneratorParams, expectedLen int)
	}{
		{
			name:   "basic license: fixed length alphanumeric",
			client: k8s.NewFakeClient(),
			initK8sObjects: func(t *testing.T, client k8s.Client) {
			},
			// basic license should ignore the provided character sets and length
			params: operator.PasswordGeneratorParams{Length: 12},
			// basic license should return a fixed length of 24
			expectedLen:   24,
			expectedError: nil,
			validate: func(t *testing.T, out []byte, params operator.PasswordGeneratorParams, expectedLen int) {
				require.Equal(t, expectedLen, len(out))
				for _, r := range string(out) {
					// basic license only allows alphanumeric characters
					require.True(t, strings.ContainsRune(password.LowerLetters+password.UpperLetters+password.Digits, r))
				}
			},
		},
		{
			name:   "enterprise trial: respects provided character sets and length",
			client: k8s.NewFakeClient(),
			initK8sObjects: func(t *testing.T, client k8s.Client) {
				operatorNS := "elastic-system"
				trialState, err := license.NewTrialState()
				require.NoError(t, err)
				trialLic := license.EnterpriseLicense{License: license.LicenseSpec{Type: license.LicenseTypeEnterpriseTrial}}
				require.NoError(t, trialState.InitTrialLicense(ctx, &trialLic))
				licBytes, err := json.Marshal(trialLic)
				require.NoError(t, err)
				licenseSecret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: operatorNS,
						Name:      "test-enterprise-trial-license",
						Labels:    license.LabelsForOperatorScope(trialLic.License.Type),
					},
					Data: map[string][]byte{
						license.FileName: licBytes,
					},
				}
				statusSecret, err := license.ExpectedTrialStatus(operatorNS, types.NamespacedName{}, trialState)
				require.NoError(t, err)
				require.NoError(t, client.Create(ctx, licenseSecret))
				require.NoError(t, client.Create(ctx, &statusSecret))
			},
			params: operator.PasswordGeneratorParams{
				LowerLetters: "ab",
				UpperLetters: "XY",
				Digits:       "12",
				Symbols:      "@#",
				Length:       40,
			},
			expectedLen:   40,
			expectedError: nil,
			validate: func(t *testing.T, out []byte, params operator.PasswordGeneratorParams, expectedLen int) {
				require.Equal(t, expectedLen, len(out))
				for _, r := range string(out) {
					require.True(t, strings.ContainsRune(params.LowerLetters+params.UpperLetters+params.Digits+params.Symbols, r))
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.initK8sObjects(t, tt.client)
			out, err := RandomBytesRespectingLicense(ctx, tt.client, "elastic-system", tt.params)
			require.Equal(t, tt.expectedError, err)
			tt.validate(t, out, tt.params, tt.expectedLen)
		})
	}
}
