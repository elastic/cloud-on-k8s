// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

// +build integration

package license

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/license"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/operator"
	esclient "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/pkg/utils/chrono"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/pkg/utils/test"
)

func TestMain(m *testing.M) {
	test.RunWithK8s(m)
}

func TestReconcile(t *testing.T) {
	c, stop := test.StartManager(t, func(mgr manager.Manager, p operator.Parameters) error {
		r := &ReconcileLicenses{
			Client:  k8s.WrapClient(mgr.GetClient()),
			checker: license.MockChecker{},
		}
		c, err := common.NewController(mgr, name, r, p)
		if err != nil {
			return err
		}
		return addWatches(c, r.Client)

	}, operator.Parameters{})
	defer stop()

	thirtyDays := 30 * 24 * time.Hour
	now := time.Now()
	startDate := now.Add(-thirtyDays)
	expiryDate := now.Add(thirtyDays)

	enterpriseLicense := license.EnterpriseLicense{
		License: license.LicenseSpec{
			UID:                "test",
			ExpiryDateInMillis: chrono.ToMillis(expiryDate),
			Type:               "enterprise",
			ClusterLicenses: []license.ElasticsearchLicense{
				{
					License: esclient.License{
						ExpiryDateInMillis: chrono.ToMillis(expiryDate),
						StartDateInMillis:  chrono.ToMillis(startDate),
						Type:               string(esclient.ElasticsearchLicenseTypePlatinum),
						Signature:          "blah",
					},
				},
			},
		},
	}

	// Create the EnterpriseLicense object
	require.NoError(t, CreateEnterpriseLicense(
		c,
		types.NamespacedName{Name: "foo", Namespace: "elastic-system"},
		enterpriseLicense,
	))
	// give the client some time to sync up
	test.RetryUntilSuccess(t, func() error {
		var secs corev1.SecretList
		err := c.List(&secs)
		if err != nil {
			return err
		}
		if len(secs.Items) == 0 {
			return errors.New("no secrets")
		}
		return nil
	})

	cluster := &esv1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "default",
		},
		Spec: esv1.ElasticsearchSpec{
			Version: "7.0.0",
			NodeSets: []esv1.NodeSet{
				{
					Name:  "all",
					Count: 3,
				},
			},
		},
	}
	require.NoError(t, c.Create(cluster))

	// test license assignment and ownership being triggered on cluster create
	test.RetryUntilSuccess(t, func() error {
		var clusterLicense corev1.Secret
		if err := c.Get(types.NamespacedName{Namespace: "default", Name: esv1.LicenseSecretName("foo")}, &clusterLicense); err != nil {
			return err
		}
		return validateOwnerRef(&clusterLicense, cluster.ObjectMeta)
	})

}

func validateOwnerRef(obj runtime.Object, cluster metav1.ObjectMeta) error {
	metaObj, err := meta.Accessor(obj)
	if err != nil {
		return err
	}
	owners := metaObj.GetOwnerReferences()
	if len(owners) != 1 {
		return fmt.Errorf("expected exactly 1 owner, got %d", len(owners))
	}

	ownerName := owners[0].Name
	ownerKind := owners[0].Kind
	expectedKind := "Elasticsearch"
	if ownerName != cluster.Name || ownerKind != expectedKind {
		return fmt.Errorf("expected owner %s (%s), got %s (%s)", cluster.Name, expectedKind, ownerName, ownerKind)
	}
	return nil
}

// purpose of this test is mostly to understand and document the delaying queue behaviour
// can be removed or skipped when it causes trouble in CI because these tests are non-deterministic
func TestDelayingQueueInvariants(t *testing.T) {
	item := types.NamespacedName{Name: "foo", Namespace: "bar"}
	tests := []struct {
		name                 string
		adds                 func(workqueue.DelayingInterface)
		expectedObservations int
		timeout              time.Duration
	}{
		{
			name: "single add",
			adds: func(q workqueue.DelayingInterface) {
				q.Add(item)
			},
			expectedObservations: 1,
			timeout:              10 * time.Millisecond,
		},
		{
			name: "deduplication",
			adds: func(q workqueue.DelayingInterface) {
				q.Add(item)
				q.Add(item)
			},
			expectedObservations: 1,
			timeout:              500 * time.Millisecond,
		},
		{
			name: "no dedup'ing when delaying",
			adds: func(q workqueue.DelayingInterface) {
				q.Add(item)
				q.AddAfter(item, 1*time.Millisecond)
			},
			expectedObservations: 2,
			timeout:              10 * time.Millisecond,
		},
		{
			name: "but dedup's and updates item within the wait queue",
			adds: func(q workqueue.DelayingInterface) {
				q.AddAfter(item, 1*time.Hour)        // schedule for an hour from now
				q.AddAfter(item, 1*time.Millisecond) // update scheduled item for a millisecond from now
			},
			expectedObservations: 1,
			timeout:              10 * time.Millisecond,
		},
		{
			name: "direct add and delayed add are independent",
			adds: func(q workqueue.DelayingInterface) {
				q.AddAfter(item, 10*time.Millisecond)
				q.Add(item) // should work despite one item in the work queue
			},
			expectedObservations: 2,
			timeout:              20 * time.Millisecond,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := workqueue.NewDelayingQueue()
			tt.adds(q)
			results := make(chan int)
			var seen int
			go func() {
				for {
					item, _ := q.Get()
					results <- 1
					q.Done(item)
				}
			}()
			collect := func() {
				for {
					select {
					case r := <-results:
						seen += r
					case <-time.After(tt.timeout):
						return
					}
				}

			}
			collect()
			assert.Equal(t, tt.expectedObservations, seen)
		})
	}
}

// CreateEnterpriseLicense creates an Enterprise license wrapped in a secret.
func CreateEnterpriseLicense(c k8s.Client, key types.NamespacedName, l license.EnterpriseLicense) error {
	bytes, err := json.Marshal(l)
	if err != nil {
		return errors.Wrap(err, "failed to marshal license")
	}
	return c.Create(&corev1.Secret{
		ObjectMeta: v1.ObjectMeta{
			Namespace: key.Namespace,
			Name:      key.Name,
			Labels:    license.LabelsForOperatorScope(l.License.Type),
		},
		Data: map[string][]byte{
			"license": bytes,
		},
	})
}
