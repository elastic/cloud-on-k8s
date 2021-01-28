// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package common

import (
	"context"
	"testing"

	beatv1beta1 "github.com/elastic/cloud-on-k8s/pkg/apis/beat/v1beta1"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/pkg/utils/maps"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func Test_reconcileDaemonSet(t *testing.T) {
	spec := corev1.PodSpec{
		DNSPolicy:          "ClusterFirstWithHostNet",
		ServiceAccountName: "my-service-account",
		HostNetwork:        true,
	}
	int10 := intstr.FromInt(10)
	tests := []struct {
		name      string
		args      ReconciliationParams
		assertion func(daemonSet appsv1.DaemonSet, beat beatv1beta1.Beat)
		wantErr   bool
	}{
		{
			name: "propagates strategy",
			args: ReconciliationParams{
				client: k8s.NewFakeClient(),
				beat: beatv1beta1.Beat{
					Spec: beatv1beta1.BeatSpec{
						DaemonSet: &beatv1beta1.DaemonSetSpec{
							UpdateStrategy: appsv1.DaemonSetUpdateStrategy{
								Type: appsv1.RollingUpdateDaemonSetStrategyType,
								RollingUpdate: &appsv1.RollingUpdateDaemonSet{
									MaxUnavailable: &int10,
								},
							},
						},
					},
				},
			},
			assertion: func(daemonSet appsv1.DaemonSet, _ beatv1beta1.Beat) {
				require.Equal(t, daemonSet.Spec.UpdateStrategy.Type, appsv1.RollingUpdateDaemonSetStrategyType)
				require.Equal(t, daemonSet.Spec.UpdateStrategy.RollingUpdate.MaxUnavailable, &int10)
			},
			wantErr: false,
		},
		{
			name: "propagates pod template",
			args: ReconciliationParams{
				client: k8s.NewFakeClient(),
				beat: beatv1beta1.Beat{
					Spec: beatv1beta1.BeatSpec{
						DaemonSet: &beatv1beta1.DaemonSetSpec{},
					},
				},
				podTemplate: corev1.PodTemplateSpec{
					Spec: spec,
				},
			},
			assertion: func(daemonSet appsv1.DaemonSet, _ beatv1beta1.Beat) {
				require.Equal(t, daemonSet.Spec.Template.Spec, spec)
				require.Equal(t, 1, len(daemonSet.OwnerReferences))
			},
			wantErr: false,
		},
		{
			name: "propagates labels and owner",
			args: ReconciliationParams{
				client: k8s.NewFakeClient(),
				beat: beatv1beta1.Beat{
					ObjectMeta: metav1.ObjectMeta{Name: "my-beat", Namespace: "my-namespace"},
					Spec: beatv1beta1.BeatSpec{
						DaemonSet: &beatv1beta1.DaemonSetSpec{},
					},
				},
			},
			assertion: func(daemonSet appsv1.DaemonSet, beat beatv1beta1.Beat) {
				require.True(t, maps.IsSubset(beat.Labels, daemonSet.Labels))
				require.True(t, maps.IsSubset(beat.Labels, daemonSet.Spec.Selector.MatchLabels))
				require.Equal(t, 1, len(daemonSet.OwnerReferences))
				require.Equal(t, beat.Name, daemonSet.OwnerReferences[0].Name)

			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := reconcileDaemonSet(tt.args)
			if (err != nil) != tt.wantErr {
				t.Errorf("reconcileDaemonSet() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			var list appsv1.DaemonSetList
			err = tt.args.client.List(context.Background(), &list)
			require.NoError(t, err)
			require.Equal(t, 1, len(list.Items))
			daemonSet := list.Items[0]
			tt.assertion(daemonSet, tt.args.beat)
		})
	}
}
