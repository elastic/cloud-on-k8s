// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package license

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	commonlicense "github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/license"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/chrono"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func Test_nextReconcileRelativeTo(t *testing.T) {
	now := chrono.MustParseTime("2019-02-01")
	type args struct {
		expiry time.Time
		safety time.Duration
	}
	tests := []struct {
		name string
		args args
		want reconcile.Result
	}{
		{
			name: "no expiry found, retry after ",
			args: args{
				expiry: time.Time{},
				safety: 30 * 24 * time.Hour,
			},
			want: reconcile.Result{
				Requeue:      true,
				RequeueAfter: minimumRetryInternval,
			},
		},
		{
			name: "remaining time too short: requeue immediately ",
			args: args{
				expiry: chrono.MustParseTime("2019-02-02"),
				safety: 30 * 24 * time.Hour,
			},
			want: reconcile.Result{Requeue: true},
		},
		{
			name: "default: requeue after expiry - safety/2 ",
			args: args{
				expiry: chrono.MustParseTime("2019-02-03"),
				safety: 48 * time.Hour,
			},
			want: reconcile.Result{RequeueAfter: 24 * time.Hour},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := nextReconcileRelativeTo(now, tt.args.expiry, tt.args.safety); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("nextReconcileRelativeTo() = %v, want %v", got, tt.want)
			}
		})
	}
}

var cluster = &v1alpha1.Elasticsearch{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "cluster",
		Namespace: "namespace",
	},
}

func enterpriseLicense(t *testing.T, licenseType v1alpha1.LicenseType, maxNodes int, expired bool) *corev1.Secret {
	expiry := time.Now().Add(31 * 24 * time.Hour)
	if expired {
		expiry = time.Now().Add(-24 * time.Hour)
	}
	license := commonlicense.EnterpriseLicense{
		License: commonlicense.LicenseSpec{
			ExpiryDateInMillis: expiry.Unix() * 1000,
			StartDateInMillis:  time.Now().Add(-1*time.Minute).Unix() * 1000,
			ClusterLicenses: []commonlicense.ElasticsearchLicense{
				{
					License: client.License{
						ExpiryDateInMillis: expiry.Unix() * 1000,
						StartDateInMillis:  time.Now().Add(-1*time.Minute).Unix() * 1000,
						Type:               string(licenseType),
						MaxNodes:           maxNodes,
						Signature:          "blah",
					},
				},
			},
		},
	}
	bytes, err := json.Marshal(license)
	require.NoError(t, err)
	return &corev1.Secret{
		Data: map[string][]byte{
			"_": bytes,
		},
	}
}

func TestReconcileLicenses_reconcileInternal(t *testing.T) {
	tests := []struct {
		name             string
		cluster          *v1alpha1.Elasticsearch
		k8sResources     []runtime.Object
		wantErr          string
		wantNewLicense   bool
		wantRequeue      bool
		wantRequeueAfter bool
	}{
		{
			name:             "no existing license: nothing to do",
			cluster:          cluster,
			k8sResources:     []runtime.Object{cluster},
			wantErr:          "",
			wantNewLicense:   false,
			wantRequeue:      false,
			wantRequeueAfter: false,
		},
		{
			name:    "existing gold matching license",
			cluster: cluster,
			k8sResources: []runtime.Object{
				enterpriseLicense(t, v1alpha1.LicenseTypeGold, 1, false),
				cluster,
			},
			wantErr:          "",
			wantNewLicense:   true,
			wantRequeue:      false,
			wantRequeueAfter: true,
		},
		{
			name:    "existing platinum matching license",
			cluster: cluster,
			k8sResources: []runtime.Object{
				enterpriseLicense(t, v1alpha1.LicenseTypePlatinum, 1, false),
				cluster,
			},
			wantErr:          "",
			wantNewLicense:   true,
			wantRequeue:      false,
			wantRequeueAfter: true,
		},
		{
			name:    "existing license expired",
			cluster: cluster,
			k8sResources: []runtime.Object{
				enterpriseLicense(t, v1alpha1.LicenseTypePlatinum, 1, true),
				cluster,
			},
			wantErr:          "no matching license found",
			wantNewLicense:   false,
			wantRequeue:      false,
			wantRequeueAfter: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.NoError(t, v1alpha1.AddToScheme(scheme.Scheme))
			client := k8s.WrapClient(fake.NewFakeClient(tt.k8sResources...))
			r := &ReconcileLicenses{
				Client:  client,
				scheme:  scheme.Scheme,
				checker: commonlicense.MockChecker{},
			}
			nsn := k8s.ExtractNamespacedName(tt.cluster)
			res, err := r.reconcileInternal(reconcile.Request{NamespacedName: nsn})
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			if tt.wantRequeue {
				require.True(t, res.Requeue)
				require.Zero(t, res.RequeueAfter)
			}
			if tt.wantRequeueAfter {
				require.False(t, res.Requeue)
				require.NotZero(t, res.RequeueAfter)
			}
			// verify that a cluster license was created
			// with the same name as the cluster
			var license v1alpha1.ClusterLicense
			err = client.Get(nsn, &license)
			if !tt.wantNewLicense {
				require.True(t, apierrors.IsNotFound(err))
			} else {
				require.NoError(t, err)
				require.NotEmpty(t, license.Spec.Type)
			}
		})
	}
}
