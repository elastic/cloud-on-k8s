// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package trial

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"fmt"
	"testing"
	"time"

	licensing "github.com/elastic/cloud-on-k8s/pkg/controller/common/license"
	"github.com/elastic/cloud-on-k8s/pkg/utils/chrono"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const testNs = "ns-1"
const trialSecretName = "eck-trial" // user's choice but constant simplifies test setup

var trialSecretNsn = types.NamespacedName{
	Namespace: testNs,
	Name:      trialSecretName,
}

func trialSecretSample(annotated bool, data map[string][]byte) *v1.Secret {
	sec := v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      trialSecretName,
			Namespace: testNs,
			Labels: map[string]string{
				licensing.LicenseLabelType: "enterprise-trial", // assume always there otherwise watch would not trigger
			},
			Annotations: map[string]string{},
		},
		Data: data,
	}
	if annotated {
		sec.Annotations[licensing.EULAAnnotation] = licensing.EULAAcceptedValue
	}
	return &sec
}

func trialLicenseBytes() []byte {
	return []byte(fmt.Sprintf(
		`{"license": {"uid": "x", "type": "enterprise-trial", "issue_date_in_millis": 1, "expiry_date_in_millis": %d, "issued_to": "x", "issuer": "x", "start_date_in_millis": 1, "cluster_licenses": null, "Version": 0}}`,
		chrono.ToMillis(time.Now().Add(24*time.Hour)), // simulate a license still valid for 24 hours
	))

}

func testPubkey(t *testing.T) *rsa.PublicKey {
	rnd := rand.Reader
	key, err := rsa.GenerateKey(rnd, 2048)
	require.NoError(t, err)
	return &key.PublicKey
}

func simulateRunningTrial(t *testing.T, k k8s.Client, secret v1.Secret) []byte {
	l := licensing.EnterpriseLicense{
		License: licensing.LicenseSpec{
			Type: licensing.LicenseTypeEnterpriseTrial,
		},
	}
	trialKey, err := licensing.InitTrial(k, testNs, secret, &l)
	require.NoError(t, err)
	keyBytes, err := x509.MarshalPKIXPublicKey(trialKey)
	require.NoError(t, err)
	return keyBytes
}

func TestReconcileTrials_Reconcile(t *testing.T) {
	requireValidationMsg := func(msg string) func(c k8s.Client) {
		return func(c k8s.Client) {
			var sec v1.Secret
			require.NoError(t, c.Get(trialSecretNsn, &sec))
			err, ok := sec.Annotations[licensing.LicenseInvalidAnnotation]
			require.True(t, ok, "invalid annotation present")
			require.Equal(t, msg, err)
		}
	}
	requireNoValidationMsg := func(c k8s.Client) {
		var sec v1.Secret
		require.NoError(t, c.Get(trialSecretNsn, &sec))
		_, ok := sec.Annotations[licensing.LicenseInvalidAnnotation]
		require.False(t, ok, "no invalid annotation expected")
	}

	type fields struct {
		Client      k8s.Client
		trialPubKey *rsa.PublicKey
	}
	tests := []struct {
		name       string
		fields     fields
		wantErr    bool
		assertions func(c k8s.Client)
	}{
		{
			name: "trial secret needs accepted EULA",
			fields: fields{
				Client: k8s.WrappedFakeClient(trialSecretSample(false, nil)),
			},
			wantErr:    false,
			assertions: requireValidationMsg(EULAValidationMsg),
		},
		{
			name: "valid trial secret inits trial",
			fields: fields{
				Client: k8s.WrappedFakeClient(trialSecretSample(true, nil)),
			},
			wantErr: false,
			assertions: func(c k8s.Client) {
				var sec v1.Secret
				require.NoError(t, c.Get(types.NamespacedName{
					Namespace: testNs,
					Name:      licensing.TrialStatusSecretKey,
				}, &sec))
				require.NoError(t, c.Get(trialSecretNsn, &sec))
				_, lic, err := licensing.TrialLicense(c, trialSecretNsn)
				require.NoError(t, err)
				require.NoError(t, lic.IsMissingFields())
			},
		},
		{
			name: "valid trial after operator restart",
			fields: fields{
				Client: func() k8s.Client {
					trialLicense := trialSecretSample(
						true,
						map[string][]byte{
							"license": trialLicenseBytes(),
						})
					client := k8s.WrappedFakeClient(
						trialLicense,
					)
					simulateRunningTrial(t, client, *trialLicense)
					return client
				}(),
				trialPubKey: nil, // simulating restart
			},
			wantErr:    false,
			assertions: requireNoValidationMsg,
		},
		{
			name: "invalid: trial running but no status secret",
			fields: fields{
				Client:      k8s.WrappedFakeClient(trialSecretSample(true, nil)),
				trialPubKey: testPubkey(t),
			},
			wantErr:    false,
			assertions: requireValidationMsg(trialOnlyOnceMsg),
		},
		{
			name: "invalid: restarting a running trial",
			fields: fields{
				Client: k8s.WrappedFakeClient(
					// user creates a new trial secret
					trialSecretSample(true, nil),
					&v1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name:      licensing.TrialStatusSecretKey,
							Namespace: testNs,
						},
					}),
				trialPubKey: testPubkey(t), // but trial is already running
			},
			wantErr:    false,
			assertions: requireValidationMsg(trialOnlyOnceMsg),
		},
		{
			name: "invalid: license signature",
			fields: fields{
				Client: k8s.WrappedFakeClient(trialSecretSample(true, map[string][]byte{
					"license": trialLicenseBytes(),
				}), &v1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      licensing.TrialStatusSecretKey,
						Namespace: testNs,
					},
				}),
				trialPubKey: testPubkey(t),
			},
			wantErr:    false,
			assertions: requireValidationMsg("trial license signature invalid"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			r := &ReconcileTrials{
				Client:            tt.fields.Client,
				recorder:          record.NewFakeRecorder(10),
				trialPubKey:       tt.fields.trialPubKey,
				operatorNamespace: testNs,
			}
			_, err := r.Reconcile(reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: testNs,
					Name:      trialSecretName,
				},
			})
			if (err != nil) != tt.wantErr {
				t.Errorf("Reconcile() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.assertions != nil {
				tt.assertions(tt.fields.Client)
			}
		})
	}
}
