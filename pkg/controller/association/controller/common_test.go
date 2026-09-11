// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package controller

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	beatv1beta1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/beat/v1beta1"
	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	kbv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/kibana/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

func Test_getElasticsearchFromKibana(t *testing.T) {
	tests := []struct {
		name      string
		assoc     func() commonv1.Association
		objects   []client.Object
		wantFound bool
		wantRef   commonv1.AssociationRef
		wantErr   bool
	}{
		{
			name: "association kibana ref unset",
			assoc: func() commonv1.Association {
				return &beatv1beta1.BeatKibanaAssociation{
					Beat: &beatv1beta1.Beat{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "beat"}},
				}
			},
			wantFound: false,
		},
		{
			name: "kibana not found",
			assoc: func() commonv1.Association {
				return &beatv1beta1.BeatKibanaAssociation{
					Beat: &beatv1beta1.Beat{
						ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "beat"},
						Spec:       beatv1beta1.BeatSpec{KibanaRef: commonv1.ObjectSelector{Name: "kb", Namespace: "ns"}},
					},
				}
			},
			wantFound: false,
		},
		{
			name: "kibana exists but has no elasticsearch ref",
			assoc: func() commonv1.Association {
				return &beatv1beta1.BeatKibanaAssociation{
					Beat: &beatv1beta1.Beat{
						ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "beat"},
						Spec:       beatv1beta1.BeatSpec{KibanaRef: commonv1.ObjectSelector{Name: "kb", Namespace: "ns"}},
					},
				}
			},
			objects:   []client.Object{&kbv1.Kibana{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "kb"}}},
			wantFound: false,
		},
		{
			name: "kibana exists with elasticsearch ref",
			assoc: func() commonv1.Association {
				return &beatv1beta1.BeatKibanaAssociation{
					Beat: &beatv1beta1.Beat{
						ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "beat"},
						Spec:       beatv1beta1.BeatSpec{KibanaRef: commonv1.ObjectSelector{Name: "kb", Namespace: "ns"}},
					},
				}
			},
			objects: []client.Object{&kbv1.Kibana{
				ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "kb"},
				Spec: kbv1.KibanaSpec{
					ElasticsearchRef: commonv1.ElasticsearchSelector{
						ObjectSelector: commonv1.ObjectSelector{Name: "es", Namespace: "ns"},
					},
				},
			}},
			wantFound: true,
			wantRef:   commonv1.ElasticsearchSelector{ObjectSelector: commonv1.ObjectSelector{Name: "es", Namespace: "ns"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := k8s.NewFakeClient(tt.objects...)
			found, ref, err := getElasticsearchFromKibana(context.Background(), c, tt.assoc())
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.wantFound, found)
			if tt.wantFound {
				require.Equal(t, tt.wantRef, ref)
			}
		})
	}
}
