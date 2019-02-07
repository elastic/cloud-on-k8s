// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package pvgc

import (
	"context"
	"fmt"
	"github.com/elastic/k8s-operators/local-volume/pkg/driver/daemon/drivers"
	"github.com/elastic/k8s-operators/local-volume/pkg/driver/flex"
	"github.com/elastic/k8s-operators/local-volume/pkg/driver/protocol"
	"github.com/elastic/k8s-operators/local-volume/pkg/utils/test"
	"github.com/stretchr/testify/require"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	cachetesting "k8s.io/client-go/tools/cache/testing"
	"reflect"
	"testing"
)

// stubDriver is a driver stub for our tests
type stubDriver struct {
	ListVolumesResponse []string
	PurgeVolumeResponse error
	PurgedVolumes       []string
}

func (d *stubDriver) Info() string {
	panic("implement me")
}

func (d *stubDriver) Init() flex.Response {
	panic("implement me")
}

func (d *stubDriver) Mount(params protocol.MountRequest) flex.Response {
	panic("implement me")
}

func (d *stubDriver) Unmount(params protocol.UnmountRequest) flex.Response {
	panic("implement me")
}

func (d *stubDriver) ListVolumes() ([]string, error) {
	return d.ListVolumesResponse, nil
}

func (d *stubDriver) PurgeVolume(volumeName string) error {
	d.PurgedVolumes = append(d.PurgedVolumes, volumeName)
	return d.PurgeVolumeResponse
}

var _ drivers.Driver = &stubDriver{}

const testVolumeName = "test-volume-name"

func TestController(t *testing.T) {
	controllerSourceWithFoo := cachetesting.NewFakeControllerSource()
	controllerSourceWithFoo.Add(&v1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: testVolumeName,
		},
	})

	type args struct {
		cp ControllerParams
	}
	tests := []struct {
		name                  string
		args                  args
		wantPurgedVolumes     []string
		wantReconciledVolumes []reconcileVolumeArgs
	}{
		{
			name: "Driver has volume not in API",
			args: args{
				cp: ControllerParams{
					NodeName: "",
					Driver: &stubDriver{
						ListVolumesResponse: []string{testVolumeName},
					},
					testWatcher: cachetesting.NewFakeControllerSource(),
				},
			},
			wantReconciledVolumes: []reconcileVolumeArgs{{key: testVolumeName, exists: false}},
			wantPurgedVolumes:     []string{testVolumeName},
		},
		{
			name: "API and Driver knows about same volume",
			args: args{
				cp: ControllerParams{
					NodeName: "",
					Driver: &stubDriver{
						ListVolumesResponse: []string{testVolumeName},
					},
					testWatcher: controllerSourceWithFoo,
				},
			},
			wantReconciledVolumes: []reconcileVolumeArgs{{key: testVolumeName, exists: true}},
		},
		{
			name: "API has volume not known to Driver",
			args: args{
				cp: ControllerParams{
					NodeName:    "",
					Driver:      &stubDriver{},
					testWatcher: controllerSourceWithFoo,
				},
			},
			wantReconciledVolumes: []reconcileVolumeArgs{{key: testVolumeName, exists: true}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var reconciledVolumes []reconcileVolumeArgs
			tt.args.cp.testOnReconcile = func(reconcileArgs reconcileVolumeArgs) {
				reconciledVolumes = append(reconciledVolumes, reconcileArgs)
			}

			c, err := NewController(tt.args.cp)
			require.NoError(t, err)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			go func() {
				err := c.Run(ctx)
				require.NoError(t, err)
			}()

			sd := tt.args.cp.Driver.(*stubDriver)

			for _, wantReconciledVolume := range tt.wantReconciledVolumes {
				test.RetryUntilSuccess(t, func() error {
					found := false
					for _, reconciledVolume := range reconciledVolumes {
						if reflect.DeepEqual(reconciledVolume, wantReconciledVolume) {
							found = true
							break
						}
					}

					if !found {
						return fmt.Errorf(
							"%s not found in reconciled volumes %v",
							wantReconciledVolume.key,
							reconciledVolumes,
						)
					}
					return nil
				})
			}

			for _, wantPurgedVolumeName := range tt.wantPurgedVolumes {
				test.RetryUntilSuccess(t, func() error {
					found := false
					for _, purgedVolumeName := range sd.PurgedVolumes {
						if purgedVolumeName == wantPurgedVolumeName {
							found = true
							break
						}
					}

					if !found {
						return fmt.Errorf(
							"%s not found in purged volumes %v",
							wantPurgedVolumeName,
							sd.PurgedVolumes,
						)
					}
					return nil
				})
			}
		})
	}
}
