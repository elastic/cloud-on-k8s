// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package sset

import (
	"testing"

	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestPodName(t *testing.T) {
	require.Equal(t, "sset-2", PodName("sset", 2))
}

func TestPodNames(t *testing.T) {
	require.Equal(t,
		[]string{"sset-0", "sset-1", "sset-2"},
		PodNames(appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "ns",
				Name:      "sset",
			},
			Spec: appsv1.StatefulSetSpec{
				Replicas: common.Int32(3),
			},
		}),
	)
}

func TestStatefulSetName(t *testing.T) {
	type args struct {
		podName string
	}
	tests := []struct {
		name         string
		args         args
		wantSsetName string
		wantOrdinal  int32
		wantErr      bool
	}{
		{
			name:         "Get the name of the StatefulSet from a Pod",
			args:         args{podName: "foo-14"},
			wantErr:      false,
			wantOrdinal:  14,
			wantSsetName: "foo",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotSsetName, gotOrdinal, err := StatefulSetName(tt.args.podName)
			if (err != nil) != tt.wantErr {
				t.Errorf("StatefulSetName() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if gotSsetName != tt.wantSsetName {
				t.Errorf("StatefulSetName() gotSsetName = %v, want %v", gotSsetName, tt.wantSsetName)
			}
			if gotOrdinal != tt.wantOrdinal {
				t.Errorf("StatefulSetName() gotOrdinal = %v, want %v", gotOrdinal, tt.wantOrdinal)
			}
		})
	}
}
