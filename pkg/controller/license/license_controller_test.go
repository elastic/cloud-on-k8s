// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package license

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	commonlicense "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/license"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/chrono"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
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
		want time.Duration
	}{
		{
			name: "no expiry found, retry after",
			args: args{
				expiry: time.Time{},
				safety: 30 * 24 * time.Hour,
			},
			want: minimumRetryInterval,
		},
		{
			name: "remaining time too short: requeue after default 10s interval",
			args: args{
				expiry: chrono.MustParseTime("2019-02-02"),
				safety: 30 * 24 * time.Hour,
			},
			want: reconciler.DefaultRequeue,
		},
		{
			name: "default: requeue after expiry - safety/2",
			args: args{
				expiry: chrono.MustParseTime("2019-02-03"),
				safety: 48 * time.Hour,
			},
			want: 24 * time.Hour,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := nextReconcileRelativeTo(now, tt.args.expiry, tt.args.safety); got != tt.want {
				t.Errorf("nextReconcileRelativeTo() = %v, want %v", got, tt.want)
			}
		})
	}
}

var cluster = &esv1.Elasticsearch{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "cluster",
		Namespace: "namespace",
	},
	Spec: esv1.ElasticsearchSpec{
		Version: "8.0.0",
	},
}

func TestReconcileLicenses_reconcileInternal(t *testing.T) {
	tests := []struct {
		name               string
		cluster            *esv1.Elasticsearch
		k8sResources       []crclient.Object
		wantErr            string
		wantClusterLicense bool
		wantRequeueAfter   bool
	}{
		{
			name:               "no existing license: nothing to do",
			cluster:            cluster,
			k8sResources:       []crclient.Object{cluster},
			wantErr:            "",
			wantClusterLicense: false,
			wantRequeueAfter:   false,
		},
		{
			name:    "no existing license but cluster license exists: delete cluster license",
			cluster: cluster,
			k8sResources: []crclient.Object{cluster, &corev1.Secret{ObjectMeta: metav1.ObjectMeta{
				Name:      esv1.LicenseSecretName("cluster"),
				Namespace: "namespace",
			}}},
			wantErr:            "",
			wantClusterLicense: false,
			wantRequeueAfter:   false,
		},
		{
			name:    "existing gold matching license",
			cluster: cluster,
			k8sResources: []crclient.Object{
				FakeEnterpriseLicense(t, client.ElasticsearchLicenseTypeGold, 1, false),
				cluster,
			},
			wantErr:            "",
			wantClusterLicense: true,
			wantRequeueAfter:   true,
		},
		{
			name:    "existing platinum matching license",
			cluster: cluster,
			k8sResources: []crclient.Object{
				FakeEnterpriseLicense(t, client.ElasticsearchLicenseTypePlatinum, 1, false),
				cluster,
			},
			wantErr:            "",
			wantClusterLicense: true,
			wantRequeueAfter:   true,
		},
		{
			name:    "existing license expired",
			cluster: cluster,
			k8sResources: []crclient.Object{
				FakeEnterpriseLicense(t, client.ElasticsearchLicenseTypePlatinum, 1, true),
				cluster,
			},
			wantErr:            "",
			wantClusterLicense: false,
			wantRequeueAfter:   false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := k8s.NewFakeClient(tt.k8sResources...)
			r := &ReconcileLicenses{
				Client:  client,
				checker: commonlicense.MockLicenseChecker{EnterpriseEnabled: true},
			}
			nsn := k8s.ExtractNamespacedName(tt.cluster)
			res, err := r.reconcileInternal(context.Background(), reconcile.Request{NamespacedName: nsn}).Aggregate()
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)

			if tt.wantRequeueAfter {
				require.NotZero(t, res.RequeueAfter)
			}
			// verify that a cluster license was created
			// following the es naming convention
			licenseNsn := nsn
			licenseNsn.Name = esv1.LicenseSecretName(licenseNsn.Name)
			var license corev1.Secret
			err = client.Get(context.Background(), licenseNsn, &license)
			if !tt.wantClusterLicense {
				require.True(t, apierrors.IsNotFound(err))
			} else {
				require.NoError(t, err)
				require.NotEmpty(t, license.Data)
			}
		})
	}
}
