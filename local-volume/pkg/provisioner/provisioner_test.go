// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package provisioner

import (
	"testing"

	"github.com/elastic/k8s-operators/local-volume/pkg/driver/protocol"
	"github.com/elastic/k8s-operators/local-volume/pkg/provider"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/kubernetes-sigs/sig-storage-lib-external-provisioner/controller"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Test_flexProvisioner_Provision(t *testing.T) {
	type args struct {
		options controller.VolumeOptions
	}
	tests := []struct {
		name string
		args args
		want *v1.PersistentVolume
		err  error
	}{
		{
			name: "Provisons a new volume",
			args: args{options: controller.VolumeOptions{
				PVName: "A-pvname",
				PVC: &v1.PersistentVolumeClaim{
					Spec: v1.PersistentVolumeClaimSpec{
						Resources: v1.ResourceRequirements{
							Requests: v1.ResourceList{
								v1.ResourceName(v1.ResourceStorage): *resource.NewQuantity(5000, resource.BinarySI),
							},
						},
					},
				},
			}},
			want: &v1.PersistentVolume{
				ObjectMeta: metav1.ObjectMeta{
					Name: "A-pvname",
				},
				Spec: v1.PersistentVolumeSpec{
					Capacity: v1.ResourceList{
						v1.ResourceName(v1.ResourceStorage): *resource.NewQuantity(5000, resource.BinarySI),
					},
					PersistentVolumeSource: v1.PersistentVolumeSource{
						FlexVolume: &v1.FlexPersistentVolumeSource{
							Driver: provider.Name,
							Options: protocol.MountOptions{
								SizeBytes: 5000,
							}.AsStrMap(),
						},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := flexProvisioner{}
			got, err := p.Provision(tt.args.options)
			assert.Equal(t, tt.err, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
