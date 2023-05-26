// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package beat

import (
	"context"
	"reflect"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/stretchr/testify/require"

	beatv1beta1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/beat/v1beta1"
	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/operator"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
)

func TestReconcileBeat_Reconcile(t *testing.T) {
	defaultBeat := beatv1beta1.Beat{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "testbeat",
			Namespace:  "testing",
			Generation: 2,
		},
		Spec: beatv1beta1.BeatSpec{
			Type:      "filebeat",
			Version:   "7.17.0",
			DaemonSet: &beatv1beta1.DaemonSetSpec{},
		},
		Status: beatv1beta1.BeatStatus{
			ObservedGeneration: 1,
		},
	}
	defaultRequest := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      "testbeat",
			Namespace: "testing",
		},
	}
	tests := []struct {
		name      string
		Client    k8s.Client
		request   reconcile.Request
		want      reconcile.Result
		wantErr   bool
		errString string
		validate  func(*testing.T, k8s.Client)
	}{
		{
			name: "unmanaged beat observedGeneration is not changed",
			Client: k8s.NewFakeClient(
				withAnnotations(defaultBeat, map[string]string{
					common.ManagedAnnotation: "false",
				}),
			),
			request: defaultRequest,
			want:    reconcile.Result{},
			wantErr: false,
			//nolint:thelper
			validate: func(t *testing.T, c k8s.Client) {
				beat := beatv1beta1.Beat{}
				err := c.Get(
					context.Background(),
					types.NamespacedName{
						Name:      "testbeat",
						Namespace: "testing"},
					&beat)
				require.NoError(t, err)
				require.Equal(t, int64(1), beat.Status.ObservedGeneration)
			},
		},
		{
			name: "beat marked for deletion has observedGeneration unchanged",
			Client: k8s.NewFakeClient(
				toBeDeleted(defaultBeat),
			),
			request: defaultRequest,
			want:    reconcile.Result{},
			wantErr: false,
			//nolint:thelper
			validate: func(t *testing.T, c k8s.Client) {
				beat := beatv1beta1.Beat{}
				err := c.Get(
					context.Background(),
					types.NamespacedName{
						Name:      "testbeat",
						Namespace: "testing"},
					&beat)
				require.NoError(t, err)
				require.Equal(t, int64(1), beat.Status.ObservedGeneration)
			},
		},
		{
			name: "has observedGeneration updated",
			Client: k8s.NewFakeClient(
				&defaultBeat,
			),
			request: defaultRequest,
			want:    reconcile.Result{},
			wantErr: false,
			//nolint:thelper
			validate: func(t *testing.T, c k8s.Client) {
				beat := beatv1beta1.Beat{}
				err := c.Get(
					context.Background(),
					types.NamespacedName{
						Name:      "testbeat",
						Namespace: "testing"},
					&beat)
				require.NoError(t, err)
				require.Equal(t, int64(2), beat.Status.ObservedGeneration)
			},
		},
		{
			name: "Elasticsearch association not ready observedGeneration is updated",
			Client: k8s.NewFakeClient(
				withESReference(defaultBeat, commonv1.ObjectSelector{Name: "testes"}),
			),
			request: defaultRequest,
			want:    reconcile.Result{},
			wantErr: false,
			//nolint:thelper
			validate: func(t *testing.T, c k8s.Client) {
				beat := beatv1beta1.Beat{}
				err := c.Get(
					context.Background(),
					types.NamespacedName{
						Name:      "testbeat",
						Namespace: "testing"},
					&beat)
				require.NoError(t, err)
				require.Equal(t, int64(2), beat.Status.ObservedGeneration)
			},
		},
		{
			name: "validation issues return error and observedGeneration is updated",
			Client: k8s.NewFakeClient(
				withName(defaultBeat, "superreallylongbeatsnamecausesvalidationissues"),
			),
			request: reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "superreallylongbeatsnamecausesvalidationissues",
					Namespace: "testing",
				},
			},
			want:      reconcile.Result{},
			wantErr:   true,
			errString: `Beat.beat.k8s.elastic.co "superreallylongbeatsnamecausesvalidationissues" is invalid: metadata.name: Too long: must have at most 36 bytes`,
			//nolint:thelper
			validate: func(t *testing.T, c k8s.Client) {
				beat := beatv1beta1.Beat{}
				err := c.Get(
					context.Background(),
					types.NamespacedName{
						Name:      "superreallylongbeatsnamecausesvalidationissues",
						Namespace: "testing"},
					&beat)
				require.NoError(t, err)
				require.Equal(t, int64(2), beat.Status.ObservedGeneration)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &ReconcileBeat{
				Client:         tt.Client,
				recorder:       record.NewFakeRecorder(100),
				dynamicWatches: watches.NewDynamicWatches(),
				Parameters:     operator.Parameters{},
			}
			got, err := r.Reconcile(context.Background(), tt.request)
			if (err != nil) != tt.wantErr {
				t.Errorf("ReconcileBeat.Reconcile() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				require.EqualError(t, err, tt.errString)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ReconcileBeat.Reconcile() = %v, want %v", got, tt.want)
			}
			tt.validate(t, tt.Client)
		})
	}
}

func withAnnotations(beat beatv1beta1.Beat, annotations map[string]string) *beatv1beta1.Beat {
	if beat.ObjectMeta.Annotations == nil {
		beat.ObjectMeta.Annotations = annotations
		return &beat
	}
	for k, v := range annotations {
		beat.ObjectMeta.Annotations[k] = v
	}
	return &beat
}

func toBeDeleted(beat beatv1beta1.Beat) *beatv1beta1.Beat {
	now := metav1.Now()
	beat.DeletionTimestamp = &now
	beat.Finalizers = []string{"something"}
	return &beat
}

func withESReference(beat beatv1beta1.Beat, selector commonv1.ObjectSelector) *beatv1beta1.Beat {
	obj := beat.DeepCopy()
	obj.Spec.ElasticsearchRef = selector
	return obj
}

func withName(beat beatv1beta1.Beat, name string) *beatv1beta1.Beat {
	beat.ObjectMeta.Name = name
	return &beat
}
