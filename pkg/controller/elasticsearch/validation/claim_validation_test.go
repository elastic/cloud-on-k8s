// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package validation

import (
	"errors"
	"reflect"
	"testing"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

func Test_noIllegalVolumeClaimDeletePolicyChange(t *testing.T) {

	esWithVolumeClaimPolicy := func(policy esv1.VolumeClaimDeletePolicy) esv1.Elasticsearch {
		return esv1.Elasticsearch{
			ObjectMeta: metav1.ObjectMeta{Name: "es", Namespace: "ns"},
			Spec: esv1.ElasticsearchSpec{
				VolumeClaimDeletePolicy: policy,
			},
		}
	}

	buildSsetWithClaims := func(name string, withOwner bool, claims ...string) *appsv1.StatefulSet {
		s := appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "ns",
				Name:      name,
				Labels: map[string]string{
					label.ClusterNameLabelName: "es",
				},
			},
			Spec: appsv1.StatefulSetSpec{},
		}
		for _, claim := range claims {
			pvc := corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name: claim,
				},
			}
			if withOwner {
				pvc.OwnerReferences = append(pvc.OwnerReferences, metav1.OwnerReference{Name: "es"})
			}
			s.Spec.VolumeClaimTemplates = append(s.Spec.VolumeClaimTemplates, pvc)
		}
		return &s
	}

	type args struct {
		c  k8s.Client
		es esv1.Elasticsearch
	}
	tests := []struct {
		name string
		args args
		want field.ErrorList
	}{
		{
			name: "Be lenient in case of k8s API errors",
			args: args{
				c:  k8s.NewFailingClient(errors.New("boom")),
				es: esWithVolumeClaimPolicy(esv1.RemoveOnClusterDeletionPolicy),
			},
			want: nil,
		},
		{
			name: "Be lenient in case statefulsets don't exist yet",
			args: args{
				c:  k8s.NewFakeClient(),
				es: esWithVolumeClaimPolicy(esv1.RetainPolicy),
			},
			want: nil,
		},
		{
			name: "Disallow change to retain policy if previous policy was delete",
			args: args{
				c:  k8s.NewFakeClient(buildSsetWithClaims("sset1", true, "claim1")),
				es: esWithVolumeClaimPolicy(esv1.RetainPolicy),
			},
			want: []*field.Error{
				field.Forbidden(field.NewPath("spec").Child("volumeClaimDeletePolicy"), forbiddenPolicyChgMsg),
			},
		},
		{
			name: "Disallow change to remove policy if previous policy was retain",
			args: args{
				c:  k8s.NewFakeClient(buildSsetWithClaims("sset1", false, "claim1")),
				es: esWithVolumeClaimPolicy(esv1.RemoveOnScaleDownPolicy),
			},
			want: []*field.Error{
				field.Forbidden(field.NewPath("spec").Child("volumeClaimDeletePolicy"), forbiddenPolicyChgMsg),
			},
		},
		{
			name: "Allow when no change in policy",
			args: args{
				c:  k8s.NewFakeClient(buildSsetWithClaims("sset1", true, "claim1")),
				es: esWithVolumeClaimPolicy(esv1.RemoveOnScaleDownPolicy),
			},
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := noIllegalVolumeClaimDeletePolicyChange(tt.args.c, tt.args.es); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("noIllegalVolumeClaimDeletePolicyChange() = %v, want %v", got, tt.want)
			}
		})
	}
}
