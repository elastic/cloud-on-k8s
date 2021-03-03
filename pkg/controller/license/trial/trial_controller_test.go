// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package trial

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
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

const (
	testNs           = "ns-1"
	trialLicenseName = "eck-trial" // user's choice but constant simplifies test setup
)

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

func trialStatusSecretSample(t *testing.T, state licensing.TrialState) *corev1.Secret {
	status, err := licensing.ExpectedTrialStatus(testNs, trialLicenseNsn, state)
	require.NoError(t, err)
	return &status
}

func trialLicenseBytes() []byte {
	return []byte(fmt.Sprintf(
		`{"license": {"uid": "x", "type": "enterprise_trial", "issue_date_in_millis": 1, "expiry_date_in_millis": %d, "issued_to": "x", "issuer": "Elastic k8s operator", "start_date_in_millis": 1, "cluster_licenses": null, "Version": 0}}`,
		chrono.ToMillis(time.Now().Add(24*time.Hour)), // simulate a license still valid for 24 hours
	))
}

func trialStateSample(t *testing.T) licensing.TrialState {
	state, err := licensing.NewTrialState()
	require.NoError(t, err)
	return state
}

func runningTrialSample(t *testing.T) licensing.TrialState {
	state := trialStateSample(t)
	state.CompleteTrialActivation()
	return state
}

func simulateLicenseInit(t *testing.T, k k8s.Client, secret corev1.Secret) licensing.TrialState {
	l := licensing.EnterpriseLicense{
		License: licensing.LicenseSpec{
			Type: licensing.LicenseTypeEnterpriseTrial,
		},
	}
	state, err := licensing.NewTrialState()
	require.NoError(t, err)
	err = state.InitTrialLicense(&l)
	require.NoError(t, err)
	require.NoError(t, licensing.UpdateEnterpriseLicense(k, secret, l))
	return state
}

func simulateRunningTrial(t *testing.T, k k8s.Client, secret corev1.Secret) {
	state := simulateLicenseInit(t, k, secret)
	state.CompleteTrialActivation()
	statusSecret := trialStatusSecretSample(t, state)
	require.NoError(t, k.Create(context.Background(), statusSecret))
}

func TestReconcileTrials_Reconcile(t *testing.T) {

	requireValidationMsg := func(msg string) func(c k8s.Client) {
		return func(c k8s.Client) {
			var sec corev1.Secret
			require.NoError(t, c.Get(context.Background(), trialLicenseNsn, &sec))
			err, ok := sec.Annotations[licensing.LicenseInvalidAnnotation]
			require.True(t, ok, "invalid annotation present")
			require.Equal(t, msg, err)
		}
	}

	requireNoValidationMsg := func(c k8s.Client) {
		var sec corev1.Secret
		require.NoError(t, c.Get(context.Background(), trialLicenseNsn, &sec))
		_, ok := sec.Annotations[licensing.LicenseInvalidAnnotation]
		require.False(t, ok, "no invalid annotation expected")
	}

	requireValidTrial := func(c k8s.Client) {
		var sec corev1.Secret
		require.NoError(t, c.Get(context.Background(), types.NamespacedName{
			Namespace: testNs,
			Name:      licensing.TrialStatusSecretKey,
		}, &sec))
		require.NoError(t, c.Get(context.Background(), trialLicenseNsn, &sec))
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
		Client     k8s.Client
		trialState licensing.TrialState
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
				Client: k8s.NewFakeClient(trialLicenseSecretSample(false, nil)),
			},
			wantErr:    false,
			assertions: requireValidationMsg(EULAValidationMsg),
		},
		{
			name: "valid trial secret inits trial",
			fields: fields{
				Client: k8s.NewFakeClient(trialLicenseSecretSample(true, nil)),
			},
			wantErr:    false,
			assertions: requireValidTrial,
		},
		{
			name: "valid trial after operator restart",
			fields: fields{
				Client: func() k8s.Client {
					trialLicense := trialLicenseSecretSample(true, nil)
					client := k8s.NewFakeClient(trialLicense)
					simulateRunningTrial(t, client, *trialLicense)
					return client
				}(),
				trialState: licensing.TrialState{}, // simulating restart
			},
			wantErr:    false,
			assertions: requireValidTrial,
		},
		{
			name: "can start trial after error during trial status retrieval",
			fields: func() fields {
				trialLicense := trialLicenseSecretSample(true, nil)
				client := k8s.NewFakeClient(trialLicense)
				state := simulateLicenseInit(t, client, *trialLicense) // no trial status
				return fields{
					Client:     client,
					trialState: state,
				}
			}(),
			wantErr:    false,
			assertions: requireValidTrial,
		},
		{
			name: "can start trial after error during trial status creation",
			fields: fields{
				Client:     k8s.NewFakeClient(trialLicenseSecretSample(true, nil)), // no trial status
				trialState: trialStateSample(t),
			},
			wantErr:    false,
			assertions: requireValidTrial,
		},
		{
			name: "can start trial after operator crash during trial activation",
			fields: fields{
				Client: func() k8s.Client {
					// simulating operator crash right after trial status has been written
					status, err := licensing.ExpectedTrialStatus(testNs, trialLicenseNsn, trialStateSample(t))
					require.NoError(t, err)
					return k8s.NewFakeClient(trialLicenseSecretSample(true, nil), &status)
				}(),
			},
			wantErr:    false,
			assertions: requireValidTrial,
		},
		{
			name: "invalid: external trial status modification is not allowed once trial is running",
			fields: fields{
				Client: k8s.NewFakeClient(
					trialLicenseSecretSample(true, nil),
					trialStatusSecretSample(t, trialStateSample(t)), // simulate a different key
				),
				trialState: runningTrialSample(t),
			},
			wantErr:    false,
			assertions: requireValidationMsg(trialOnlyOnceMsg),
		},
		{
			name: "external trial status modification is compensated while trial is being activated",
			fields: fields{
				Client: k8s.NewFakeClient(
					trialLicenseSecretSample(true, nil),
					trialStatusSecretSample(t, trialStateSample(t)), // simulating a different key
				),
				trialState: trialStateSample(t),
			},
			wantErr:    false,
			assertions: requireValidTrial,
		},
		{
			name: "invalid: trial running but no status secret",
			fields: fields{
				Client:     k8s.NewFakeClient(trialLicenseSecretSample(true, nil)),
				trialState: runningTrialSample(t),
			},
			wantErr:    false,
			assertions: requireValidationMsg(trialOnlyOnceMsg),
		},
		{
			name: "invalid: restarting a running trial",
			fields: fields{
				Client: k8s.NewFakeClient(
					// user creates a new trial secret
					trialLicenseSecretSample(true, nil),
					trialStatusSecretSample(t, trialStateSample(t)),
				),
				trialState: runningTrialSample(t), // but trial is already running
			},
			wantErr:    false,
			assertions: requireValidationMsg(trialOnlyOnceMsg),
		},
		{
			name: "invalid: license signature",
			fields: fields{
				Client: k8s.NewFakeClient(trialLicenseSecretSample(true, map[string][]byte{
					"license": trialLicenseBytes(),
				}), &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      licensing.TrialStatusSecretKey,
						Namespace: testNs,
					},
				}),
				trialState: runningTrialSample(t),
			},
			wantErr:    false,
			assertions: requireValidationMsg("trial license signature invalid"),
		},
		{
			name: "invalid: trial license but no trial running",
			fields: fields{
				Client: k8s.NewFakeClient(trialLicenseSecretSample(true, map[string][]byte{
					"license": trialLicenseBytes(),
				})),
			},
			wantErr:    false,
			assertions: requireValidationMsg("trial license signature invalid"),
		},
		{
			name: "externally generated licenses are ignored",
			fields: fields{
				Client: k8s.NewFakeClient(trialLicenseSecretSample(true, map[string][]byte{
					"license": []byte(strings.ReplaceAll(string(trialLicenseBytes()), licensing.ECKLicenseIssuer, "Some other issuer")),
				})),
			},
			wantErr:    false,
			assertions: requireNoValidationMsg,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			r := &ReconcileTrials{
				Client:            tt.fields.Client,
				recorder:          record.NewFakeRecorder(10),
				trialState:        tt.fields.trialState,
				operatorNamespace: testNs,
			}
			_, err := r.Reconcile(context.Background(), reconcile.Request{
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
	var licenseSample licensing.EnterpriseLicense
	require.NoError(t, json.Unmarshal(trialLicenseBytes(), &licenseSample))

	assertTrialActivationState := func(r *ReconcileTrials, status corev1.Secret) {
		require.False(t, r.trialState.IsTrialStarted())
		require.Equal(t, status.Data[licensing.TrialActivationKey], []byte("true"))
		require.Contains(t, status.Data, licensing.TrialPubkeyKey)
	}

	assertTrialRunningState := func(r *ReconcileTrials, status corev1.Secret) {
		require.True(t, r.trialState.IsTrialStarted())
		require.NotContains(t, status.Data, licensing.TrialActivationKey)
		require.Contains(t, status.Data, licensing.TrialPubkeyKey)
	}

	type fields struct {
		Client     k8s.Client
		trialState licensing.TrialState
		license    licensing.EnterpriseLicense
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
				Client: k8s.NewFakeClient(),
			},
			wantErr:    false,
			assertions: assertTrialActivationState,
		},
		{
			name: "recreates missing trial status during trial activation",
			fields: fields{
				Client:     k8s.NewFakeClient(),
				trialState: trialStateSample(t),
			},
			wantErr:    false,
			assertions: assertTrialActivationState,
		},
		{
			name: "recreates missing trial status after trial activation",
			fields: fields{
				Client:     k8s.NewFakeClient(),
				trialState: runningTrialSample(t),
			},
			wantErr:    false,
			assertions: assertTrialRunningState,
		},

		{
			name: "restore trial status memory on operator restart during activation: ok",
			fields: fields{
				Client: k8s.NewFakeClient(trialStatusSecretSample(t, trialStateSample(t))),
			},
			wantErr:    false,
			assertions: assertTrialActivationState,
		},
		{
			name: "restore trial status memory on operator restart during trial: ok",
			fields: fields{
				Client: k8s.NewFakeClient(trialStatusSecretSample(t, runningTrialSample(t))),
			},
			wantErr:    false,
			assertions: assertTrialRunningState,
		},
		{
			name: "restore trial status memory on operator restart: fail",
			fields: fields{
				Client: k8s.NewFakeClient(&corev1.Secret{
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
			name: "don't go back to activation state if a populated license exists",
			fields: fields{
				Client:  k8s.NewFakeClient(trialStatusSecretSample(t, trialStateSample(t))), // status still in activation phase
				license: licenseSample,
			},
			wantErr:    false,
			assertions: assertTrialRunningState,
		},
		{
			name: "update trial status from memory",
			fields: fields{
				Client:     k8s.NewFakeClient(trialStatusSecretSample(t, trialStateSample(t))),
				trialState: runningTrialSample(t),
			},
			wantErr:    false,
			assertions: assertTrialRunningState,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &ReconcileTrials{
				Client:            tt.fields.Client,
				recorder:          record.NewFakeRecorder(10),
				trialState:        tt.fields.trialState,
				operatorNamespace: testNs,
			}
			if err := r.reconcileTrialStatus(trialLicenseNsn, tt.fields.license); (err != nil) != tt.wantErr {
				t.Errorf("reconcileTrialStatus() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.assertions != nil {
				var sec corev1.Secret
				require.NoError(t, r.Get(context.Background(), types.NamespacedName{
					Namespace: testNs,
					Name:      licensing.TrialStatusSecretKey,
				}, &sec))
				tt.assertions(r, sec)
			}
		})
	}
}
