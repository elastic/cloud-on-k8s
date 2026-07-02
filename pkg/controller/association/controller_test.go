// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package association

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	apmv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/apm/v1"
	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

func Test_namespaceFlipRequests(t *testing.T) {
	apm := func(name, namespace string, esRef, kbRef commonv1.ObjectSelector) *apmv1.ApmServer {
		return &apmv1.ApmServer{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
			Spec: apmv1.ApmServerSpec{
				ElasticsearchRef: commonv1.ElasticsearchSelector{ObjectSelector: esRef},
				KibanaRef:        kbRef,
			},
		}
	}
	noRef := commonv1.ObjectSelector{}

	// "descoped" is the namespace whose match state just changed, "scoped" is a namespace
	// that remains in scope.
	objs := []client.Object{
		// lives in the descoped namespace: enqueued regardless of refs
		apm("in-descoped", "descoped", noRef, noRef),
		// in a scoped namespace, references an ES in the descoped namespace: enqueued
		apm("cross-ref", "scoped", commonv1.ObjectSelector{Name: "es", Namespace: "descoped"}, noRef),
		// in the descoped namespace with an implicit same-namespace ref: enqueued exactly once
		apm("default-ns-ref", "descoped", commonv1.ObjectSelector{Name: "es"}, noRef),
		// references an ES in an unrelated namespace: not enqueued
		apm("unrelated", "scoped", commonv1.ObjectSelector{Name: "es", Namespace: "elsewhere"}, noRef),
		// references the descoped namespace, but through an association of another type: not enqueued
		apm("wrong-type", "scoped", noRef, commonv1.ObjectSelector{Name: "kb", Namespace: "descoped"}),
	}

	r := &Reconciler{
		AssociationInfo: AssociationInfo{
			AssociationType:           commonv1.ElasticsearchAssociationType,
			AssociatedObjListTemplate: func() client.ObjectList { return &apmv1.ApmServerList{} },
		},
	}

	reqs := namespaceFlipRequests(k8s.NewFakeClient(objs...), r)(
		context.Background(),
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "descoped"}},
	)

	require.ElementsMatch(t, []reconcile.Request{
		{NamespacedName: types.NamespacedName{Namespace: "descoped", Name: "in-descoped"}},
		{NamespacedName: types.NamespacedName{Namespace: "scoped", Name: "cross-ref"}},
		{NamespacedName: types.NamespacedName{Namespace: "descoped", Name: "default-ns-ref"}},
	}, reqs)
}
