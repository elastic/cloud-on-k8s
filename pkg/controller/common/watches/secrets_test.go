package watches

import (
	"testing"

	kbv1 "github.com/elastic/cloud-on-k8s/pkg/apis/kibana/v1"

	"k8s.io/apimachinery/pkg/types"

	"sigs.k8s.io/controller-runtime/pkg/handler"

	"github.com/stretchr/testify/require"

	"github.com/elastic/cloud-on-k8s/pkg/controller/common/reconciler"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
)

func Test_reconcileReqForSoftOwner(t *testing.T) {
	kind := esv1.Kind
	toRequestsFunc := reconcileReqForSoftOwner(kind)

	tests := []struct {
		name                  string
		secret                corev1.Secret
		wantReconcileRequests []reconcile.Request
	}{
		{
			name: "watch soft-owned secret",
			secret: corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						reconciler.SoftOwnerKindLabel:      esv1.Kind,
						reconciler.SoftOwnerNamespaceLabel: "ns",
						reconciler.SoftOwnerNameLabel:      "es",
					},
				},
			},
			wantReconcileRequests: []reconcile.Request{
				{NamespacedName: types.NamespacedName{Namespace: "ns", Name: "es"}},
			},
		},
		{
			name: "don't watch secret whose soft owner is a different kind",
			secret: corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						reconciler.SoftOwnerKindLabel:      kbv1.Kind, // Kibana owner instead of Elasticsearch
						reconciler.SoftOwnerNamespaceLabel: "ns",
						reconciler.SoftOwnerNameLabel:      "kb",
					},
				},
			},
			wantReconcileRequests: nil,
		},
		{
			name:                  "don't watch secret with no soft owner labels",
			secret:                corev1.Secret{},
			wantReconcileRequests: nil,
		},
		{
			name: "don't watch secret with corrupted soft owner labels",
			secret: corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						reconciler.SoftOwnerKindLabel:      esv1.Kind,
						reconciler.SoftOwnerNamespaceLabel: "", // no namespace
						reconciler.SoftOwnerNameLabel:      "es",
					},
				}},
			wantReconcileRequests: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mapObject := handler.MapObject{Meta: &tt.secret, Object: &tt.secret}
			requests := toRequestsFunc(mapObject)
			require.Equal(t, tt.wantReconcileRequests, requests)
		})
	}
}

func Test_reconcileReqForSoftOwner1(t *testing.T) {
	type args struct {
		kind string
	}

}
