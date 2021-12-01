// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package controller

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/association"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

var (
	sampleES = esv1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:       "esns",
			Name:            "esname",
			ResourceVersion: "42"},
		Status: esv1.ElasticsearchStatus{Version: "7.15.0"},
	}
)

// Test_EsMonitoringReconciler_NoAssociation tests that an Elasticsearch resource is not updated by the EsMonitoring
// reconciler when there is no EsMonitoring association. Covers the bug https://github.com/elastic/cloud-on-k8s/issues/4985.
func Test_EsMonitoringReconciler_NoAssociation(t *testing.T) {
	es := sampleES
	resourceVersion := es.ResourceVersion
	r := association.NewTestAssociationReconciler(esMonitoringAssociationInfo(), &es)
	_, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: k8s.ExtractNamespacedName(&es)})
	require.NoError(t, err)
	// should not update the Elasticsearch resource
	var updatedEs esv1.Elasticsearch
	err = r.Get(context.Background(), k8s.ExtractNamespacedName(&es), &updatedEs)
	require.NoError(t, err)
	// resource version should not have changed
	require.Equal(t, resourceVersion, updatedEs.ResourceVersion)
}
