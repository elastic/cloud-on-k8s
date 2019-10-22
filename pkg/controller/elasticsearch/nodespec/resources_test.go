// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package nodespec

import (
	"reflect"
	"testing"

	"github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1beta1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/sset"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestResourcesList_MasterNodesNames(t *testing.T) {
	tests := []struct {
		name string
		l    ResourcesList
		want []string
	}{
		{
			name: "no nodes",
			l:    nil,
			want: nil,
		},
		{
			name: "3 master-only nodes, 3 master-data nodes, 3 data nodes",
			l: ResourcesList{
				{StatefulSet: sset.TestSset{Name: "master-only", Version: "7.2.0", Replicas: 3, Master: true, Data: false}.Build()},
				{StatefulSet: sset.TestSset{Name: "master-data", Version: "7.2.0", Replicas: 3, Master: true, Data: true}.Build()},
				{StatefulSet: sset.TestSset{Name: "data-only", Version: "7.2.0", Replicas: 3, Master: false, Data: true}.Build()},
			},
			want: []string{
				"master-only-0", "master-only-1", "master-only-2",
				"master-data-0", "master-data-1", "master-data-2",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.l.MasterNodesNames(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ResourcesList.MasterNodesNames() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSetVolumeClaimsControllerReference(t *testing.T) {
	es := v1beta1.Elasticsearch{
		ObjectMeta: v1.ObjectMeta{
			Name:      "es1",
			Namespace: "default",
			UID:       "ABCDEF",
		},
	}
	type args struct {
		volumeClaims []corev1.PersistentVolumeClaim
	}
	tests := []struct {
		name    string
		args    args
		want    []string
		wantErr bool
	}{
		{
			name: "Simple test case",
			args: args{
				volumeClaims: []corev1.PersistentVolumeClaim{
					{ObjectMeta: v1.ObjectMeta{Name: "elasticsearch-data"}},
				},
			},
			want: []string{"elasticsearch-data"},
		},
		{
			name: "With a user volume",
			args: args{
				volumeClaims: []corev1.PersistentVolumeClaim{
					{ObjectMeta: v1.ObjectMeta{Name: "elasticsearch-data"}},
					{ObjectMeta: v1.ObjectMeta{Name: "user-volume"}},
				},
			},
			want: []string{"elasticsearch-data", "user-volume"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := setVolumeClaimsControllerReference(tt.args.volumeClaims, es, k8s.Scheme())
			if (err != nil) != tt.wantErr {
				t.Errorf("BuildExpectedResources() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			assert.Equal(t, len(tt.want), len(got))

			// Extract PVC names
			actualPVCs := make([]string, len(got))
			for i := range got {
				actualPVCs[i] = got[i].Name
			}
			// Check the number of PVCs we got
			assert.ElementsMatch(t, tt.want, actualPVCs)

			// Check that VolumeClaimTemplates have an owner with the right settings
			for _, pvc := range got {
				assert.Equal(t, 1, len(pvc.OwnerReferences))
				ownerRef := pvc.OwnerReferences[0]
				require.False(t, *ownerRef.BlockOwnerDeletion)
				assert.Equal(t, es.UID, ownerRef.UID)
			}
		})
	}
}
