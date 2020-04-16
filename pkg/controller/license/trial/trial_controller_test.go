// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package trial

import (
	"crypto/rsa"
	"crypto/x509"
	"fmt"
	"testing"
	"time"

	licensing "github.com/elastic/cloud-on-k8s/pkg/controller/common/license"
	"github.com/elastic/cloud-on-k8s/pkg/utils/chrono"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const testNs = "ns-1"
const trialLicenseName = "eck-trial" // user's choice but constant simplifies test setup

var trialLicenseNsn = types.NamespacedName{
	Namespace: testNs,
	Name:      trialLicenseName,
}

func trialLicenseSecretSample(annotated bool, data map[string][]byte) *corev1.Secret {
	sec := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      trialLicenseName,
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

func trialStatusSecretSample(t *testing.T, key *rsa.PrivateKey, includePK bool) *corev1.Secret {
	var status corev1.Secret
	var err error
	keys := licensing.TrialKeys{
		PublicKey: &key.PublicKey,
	}
	if includePK {
		keys.PrivateKey = key
	}
	status, err = licensing.ExpectedTrialStatus(testNs, trialLicenseNsn, keys)
	require.NoError(t, err)
	return &status
}

func trialLicenseBytes() []byte {
	return []byte(fmt.Sprintf(
		`{"license": {"uid": "x", "type": "enterprise-trial", "issue_date_in_millis": 1, "expiry_date_in_millis": %d, "issued_to": "x", "issuer": "x", "start_date_in_millis": 1, "cluster_licenses": null, "Version": 0}}`,
		chrono.ToMillis(time.Now().Add(24*time.Hour)), // simulate a license still valid for 24 hours
	))

}

func testTrialKey(t *testing.T) *rsa.PrivateKey {
	key, err := licensing.NewTrialKey()
	require.NoError(t, err)
	return key
}

func simulateRunningTrial(t *testing.T, k k8s.Client, secret corev1.Secret) []byte {
	l := licensing.EnterpriseLicense{
		License: licensing.LicenseSpec{
			Type: licensing.LicenseTypeEnterpriseTrial,
		},
	}
	key := testTrialKey(t)
	status := trialStatusSecretSample(t, key, false)
	require.NoError(t, k.Create(status))
	err := licensing.InitTrial(key, &l)
	require.NoError(t, err)
	require.NoError(t, licensing.UpdateEnterpriseLicense(k, secret, l))
	keyBytes, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	require.NoError(t, err)
	return keyBytes
}

func TestReconcileTrials_Reconcile(t *testing.T) {
	trialKeySample := testTrialKey(t)

	requireValidationMsg := func(msg string) func(c k8s.Client) {
		return func(c k8s.Client) {
			var sec corev1.Secret
			require.NoError(t, c.Get(trialLicenseNsn, &sec))
			err, ok := sec.Annotations[licensing.LicenseInvalidAnnotation]
			require.True(t, ok, "invalid annotation present")
			require.Equal(t, msg, err)
		}
	}

	requireNoValidationMsg := func(c k8s.Client) {
		var sec corev1.Secret
		require.NoError(t, c.Get(trialLicenseNsn, &sec))
		_, ok := sec.Annotations[licensing.LicenseInvalidAnnotation]
		require.False(t, ok, "no invalid annotation expected")
	}

	requireValidTrial := func(c k8s.Client) {
		var sec corev1.Secret
		require.NoError(t, c.Get(types.NamespacedName{
			Namespace: testNs,
			Name:      licensing.TrialStatusSecretKey,
		}, &sec))
		require.NoError(t, c.Get(trialLicenseNsn, &sec))
		pubKeyBytes := sec.Data[licensing.TrialPubkeyKey]
		key, err := licensing.ParsePubKey(pubKeyBytes)
		require.NoError(t, err)
		_, lic, err := licensing.TrialLicense(c, trialLicenseNsn)
		require.NoError(t, err)
		require.NoError(t, lic.IsMissingFields())
		verifier := licensing.Verifier{PublicKey: key}
		require.NoError(t, verifier.ValidSignature(lic))
		requireNoValidationMsg(c)
	}

	type fields struct {
		Client          k8s.Client
		trialPubKey     *rsa.PublicKey
		trialPrivateKey *rsa.PrivateKey
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
				Client: k8s.WrappedFakeClient(trialLicenseSecretSample(false, nil)),
			},
			wantErr:    false,
			assertions: requireValidationMsg(EULAValidationMsg),
		},
		{
			name: "valid trial secret inits trial",
			fields: fields{
				Client: k8s.WrappedFakeClient(trialLicenseSecretSample(true, nil)),
			},
			wantErr:    false,
			assertions: requireValidTrial,
		},
		{
			name: "valid trial after operator restart",
			fields: fields{
				Client: func() k8s.Client {
					trialLicense := trialLicenseSecretSample(true, nil)
					client := k8s.WrappedFakeClient(
						trialLicense,
					)
					simulateRunningTrial(t, client, *trialLicense)
					return client
				}(),
				trialPubKey: nil, // simulating restart
			},
			wantErr:    false,
			assertions: requireValidTrial,
		},
		{
			name: "can start trial after error during trial status creation",
			fields: fields{
				Client:          k8s.WrappedFakeClient(trialLicenseSecretSample(true, nil)), // no trial status
				trialPubKey:     &trialKeySample.PublicKey,                                  // but trial being activated
				trialPrivateKey: trialKeySample,
			},
			wantErr:    false,
			assertions: requireValidTrial,
		},
		{
			name: "can start trial after operator crash during trial activation",
			fields: fields{
				Client: func() k8s.Client {
					// simulating operator crash right after trial status has been written
					status, err := licensing.ExpectedTrialStatus(testNs, trialLicenseNsn, licensing.TrialKeys{
						PrivateKey: trialKeySample,
						PublicKey:  &trialKeySample.PublicKey,
					})
					require.NoError(t, err)
					return k8s.WrappedFakeClient(trialLicenseSecretSample(true, nil), &status)
				}(),
				trialPubKey:     nil,
				trialPrivateKey: nil,
			},
			wantErr:    false,
			assertions: requireValidTrial,
		},
		{
			name: "invalid: external trial status modification is not allowed once trial is running",
			fields: fields{
				Client: k8s.WrappedFakeClient(
					trialLicenseSecretSample(true, nil),
					trialStatusSecretSample(t, testTrialKey(t), true), // simulate a different key
				),
				trialPubKey:     &trialKeySample.PublicKey,
				trialPrivateKey: nil,
			},
			wantErr:    false,
			assertions: requireValidationMsg(trialOnlyOnceMsg),
		},
		{
			name: "external trial status modification is compensated while trial is being activated",
			fields: fields{
				Client: k8s.WrappedFakeClient(
					trialLicenseSecretSample(true, nil),
					trialStatusSecretSample(t, testTrialKey(t), true), // simulating a different key
				),
				trialPubKey:     &trialKeySample.PublicKey,
				trialPrivateKey: trialKeySample,
			},
			wantErr:    false,
			assertions: requireValidTrial,
		},
		{
			name: "invalid: trial running but no status secret",
			fields: fields{
				Client:      k8s.WrappedFakeClient(trialLicenseSecretSample(true, nil)),
				trialPubKey: &trialKeySample.PublicKey,
			},
			wantErr:    false,
			assertions: requireValidationMsg(trialOnlyOnceMsg),
		},
		{
			name: "invalid: restarting a running trial",
			fields: fields{
				Client: k8s.WrappedFakeClient(
					// user creates a new trial secret
					trialLicenseSecretSample(true, nil),
					trialStatusSecretSample(t, trialKeySample, false),
				),
				trialPubKey: &trialKeySample.PublicKey, // but trial is already running
			},
			wantErr:    false,
			assertions: requireValidationMsg(trialOnlyOnceMsg),
		},
		{
			name: "invalid: license signature",
			fields: fields{
				Client: k8s.WrappedFakeClient(trialLicenseSecretSample(true, map[string][]byte{
					"license": trialLicenseBytes(),
				}), &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      licensing.TrialStatusSecretKey,
						Namespace: testNs,
					},
				}),
				trialPubKey: &trialKeySample.PublicKey,
			},
			wantErr:    false,
			assertions: requireValidationMsg("trial license signature invalid"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			r := &ReconcileTrials{
				Client:   tt.fields.Client,
				recorder: record.NewFakeRecorder(10),
				trialKeys: licensing.TrialKeys{
					PrivateKey: tt.fields.trialPrivateKey,
					PublicKey:  tt.fields.trialPubKey,
				},
				operatorNamespace: testNs,
			}
			_, err := r.Reconcile(reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: testNs,
					Name:      trialLicenseName,
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

func TestReconcileTrials_reconcileTrialStatus(t *testing.T) {
	keySample := testTrialKey(t)

	assertTrialActivationState := func(r *ReconcileTrials, status corev1.Secret) {
		require.True(t, r.trialKeys.IsTrialActivationInProgress())
		require.Contains(t, status.Data, licensing.TrialPrivateKey)
		require.Contains(t, status.Data, licensing.TrialPubkeyKey)
	}

	assertTrialRunningState := func(r *ReconcileTrials, status corev1.Secret) {
		require.True(t, r.trialKeys.IsTrialRunning())
		require.NotContains(t, status.Data, licensing.TrialPrivateKey)
		require.Contains(t, status.Data, licensing.TrialPubkeyKey)
	}

	type fields struct {
		Client          k8s.Client
		trialPubKey     *rsa.PublicKey
		trialPrivateKey *rsa.PrivateKey
	}
	tests := []struct {
		name       string
		fields     fields
		wantErr    bool
		assertions func(*ReconcileTrials, corev1.Secret)
	}{
		{
			name: "starts trial activation and creates status",
			fields: fields{
				Client: k8s.WrappedFakeClient(),
			},
			wantErr:    false,
			assertions: assertTrialActivationState,
		},
		{
			name: "recreates missing trial status during trial activation",
			fields: fields{
				Client:          k8s.WrappedFakeClient(),
				trialPubKey:     &keySample.PublicKey,
				trialPrivateKey: keySample,
			},
			wantErr:    false,
			assertions: assertTrialActivationState,
		},
		{
			name: "recreates missing trial status after trial activation",
			fields: fields{
				Client:          k8s.WrappedFakeClient(),
				trialPubKey:     &keySample.PublicKey,
				trialPrivateKey: nil,
			},
			wantErr:    false,
			assertions: assertTrialRunningState,
		},

		{
			name: "restore trial status memory on operator restart during activation: ok",
			fields: fields{
				Client: k8s.WrappedFakeClient(trialStatusSecretSample(t, keySample, true)),
			},
			wantErr:    false,
			assertions: assertTrialActivationState,
		},
		{
			name: "restore trial status memory on operator restart during trial: ok",
			fields: fields{
				Client: k8s.WrappedFakeClient(trialStatusSecretSample(t, keySample, false)),
			},
			wantErr:    false,
			assertions: assertTrialRunningState,
		},
		{
			name: "restore trial status memory on operator restart: fail",
			fields: fields{
				Client: k8s.WrappedFakeClient(&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      licensing.TrialStatusSecretKey,
						Namespace: testNs,
					},
					Data: map[string][]byte{
						licensing.TrialPubkeyKey: []byte("garbage"),
					},
				}),
			},
			wantErr: true,
		},
		{
			name: "update trial status from memory",
			fields: fields{
				Client:      k8s.WrappedFakeClient(trialStatusSecretSample(t, testTrialKey(t), true)),
				trialPubKey: &keySample.PublicKey,
			},
			wantErr:    false,
			assertions: assertTrialRunningState, // never restore the private key
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &ReconcileTrials{
				Client:   tt.fields.Client,
				recorder: record.NewFakeRecorder(10),
				trialKeys: licensing.TrialKeys{
					PrivateKey: tt.fields.trialPrivateKey,
					PublicKey:  tt.fields.trialPubKey,
				},
				operatorNamespace: testNs,
			}
			if err := r.reconcileTrialStatus(trialLicenseNsn); (err != nil) != tt.wantErr {
				t.Errorf("reconcileTrialStatus() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.assertions != nil {
				var sec corev1.Secret
				require.NoError(t, r.Get(types.NamespacedName{
					Namespace: testNs,
					Name:      licensing.TrialStatusSecretKey,
				}, &sec))
				tt.assertions(r, sec)
			}
		})
	}
}
