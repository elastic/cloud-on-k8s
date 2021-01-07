// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package reconciler

import (
	"context"
	"reflect"
	"testing"

	"github.com/elastic/cloud-on-k8s/pkg/controller/common/comparison"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func withoutControllerRef(obj runtime.Object) runtime.Object {
	copied := obj.DeepCopyObject()
	copied.(metav1.Object).SetOwnerReferences(nil)
	return copied
}

func noopModifier() {}

func TestReconcileResource(t *testing.T) {
	true := true

	objectKey := types.NamespacedName{Name: "test", Namespace: "foo"}
	obj := &corev1.Secret{ObjectMeta: k8s.ToObjectMeta(objectKey)}

	type args struct {
		Expected         client.Object
		Reconciled       client.Object
		Owner            client.Object
		NeedsUpdate      func() bool
		UpdateReconciled func()
	}

	secretData := map[string][]byte{"bar": []byte("shush")}

	tests := []struct {
		name                 string
		args                 func() args
		initialObjects       []runtime.Object
		argAssertion         func(args args)
		exptectedErrorMsg    string
		serverStateAssertion func(serverState corev1.Secret)
	}{
		{
			name: "Error: Expected must not be nil",
			args: func() args {
				return args{
					Reconciled:       &corev1.Secret{},
					UpdateReconciled: noopModifier,
					NeedsUpdate: func() bool {
						return false
					},
				}
			},
			exptectedErrorMsg: "Expected must not be nil",
		},
		{
			name: "Error: NeedsUpdate must not be nil",
			args: func() args {
				return args{
					Expected:         obj.DeepCopy(),
					Reconciled:       &corev1.Secret{},
					UpdateReconciled: noopModifier,
				}
			},
			exptectedErrorMsg: "NeedsUpdate must not be nil",
		},
		{
			name: "Error: Reconcile must not be nil",
			args: func() args {
				return args{
					Expected: obj.DeepCopy(),
				}
			},
			exptectedErrorMsg: "Reconciled must not be nil",
		},
		{
			name: "Error: UpdateReconciled must be defined",
			args: func() args {
				return args{
					Expected:   obj.DeepCopy(),
					Reconciled: &corev1.Secret{},
				}
			},
			exptectedErrorMsg: "UpdateReconciled must not be nil",
		},
		{
			name: "Create resource if not found",
			args: func() args {
				reconciled := &corev1.Secret{}
				return args{
					Expected:         obj.DeepCopy(),
					Reconciled:       reconciled,
					UpdateReconciled: noopModifier,
					NeedsUpdate: func() bool {
						return false
					},
				}
			},
			serverStateAssertion: func(serverState corev1.Secret) {
				diff := comparison.Diff(obj, withoutControllerRef(&serverState))
				assert.Empty(t, diff)
			},
			argAssertion: func(args args) {
				// reconciled should be updated to the expected
				diff := comparison.Diff(args.Expected, args.Reconciled)
				assert.Empty(t, diff)
			},
		},
		{
			name: "Returns server state via in/out param",
			args: func() args {
				reconciled := &corev1.Secret{}
				return args{
					Expected:         obj.DeepCopy(),
					Reconciled:       reconciled,
					UpdateReconciled: noopModifier,
					NeedsUpdate: func() bool {
						return false
					},
				}
			},
			initialObjects: []runtime.Object{&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      objectKey.Name,
					Namespace: objectKey.Namespace,
					Labels: map[string]string{
						"label": "baz",
					},
				},
			}},
			argAssertion: func(args args) {
				assert.Equal(t, "baz", args.Reconciled.(*corev1.Secret).Labels["label"])
			},
		},
		{
			name: "Updates server state, if in param differs from remote",
			args: func() args {
				expected := &corev1.Secret{
					ObjectMeta: k8s.ToObjectMeta(objectKey),
					Data: map[string][]byte{
						"bar": []byte("be quiet"),
					},
				}
				reconciled := &corev1.Secret{}
				return args{
					Expected:   expected,
					Reconciled: reconciled,
					UpdateReconciled: func() {
						reconciled.Data = expected.Data
					},
					NeedsUpdate: func() bool {
						return !reflect.DeepEqual(expected, reconciled)
					},
				}
			},
			initialObjects: []runtime.Object{obj},
			argAssertion: func(args args) {
				// should be unchanged
				assert.Equal(t, "be quiet", string(args.Expected.(*corev1.Secret).Data["bar"]))
			},
			serverStateAssertion: func(serverState corev1.Secret) {
				assert.Equal(t, "be quiet", string(serverState.Data["bar"]))
			},
		},
		{
			name: "NeedsUpdate can ignore parts of the resource",
			args: func() args {
				expected := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      objectKey.Name,
						Namespace: objectKey.Namespace,
						Labels: map[string]string{
							"label": "baz",
						},
					},
					Data: secretData,
				}
				reconciled := &corev1.Secret{}
				return args{
					Expected:   expected,
					Reconciled: reconciled,
					NeedsUpdate: func() bool {
						return !reflect.DeepEqual(expected.Data, reconciled.Data)
					},
					UpdateReconciled: noopModifier,
				}
			},
			initialObjects: []runtime.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      objectKey.Name,
						Namespace: objectKey.Namespace,
						Labels: map[string]string{
							"label": "other",
						},
					},
					Data: secretData,
				},
			},
			argAssertion: func(args args) {
				// should be updated to the server state
				assert.Equal(t, "other", args.Reconciled.(*corev1.Secret).Labels["label"])
			},
			serverStateAssertion: func(serverState corev1.Secret) {
				// should be unchanged as it is ignored by the custom differ
				assert.Equal(t, "other", serverState.Labels["label"])
			},
		},
		{
			name: "Update owner reference if changed",
			args: func() args {
				return args{
					Expected:   obj.DeepCopy(),
					Reconciled: &corev1.Secret{},
					Owner: &appsv1.Deployment{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: objectKey.Namespace,
							Name:      "newOwner",
						},
					},
					NeedsUpdate: func() bool {
						return true
					},
					// owner reference update should happen automatically, so noop is OK
					UpdateReconciled: noopModifier,
				}
			},
			initialObjects: []runtime.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:       objectKey.Namespace,
						Name:            objectKey.Name,
						OwnerReferences: []metav1.OwnerReference{{Name: "oldOwner", Controller: &true}},
					},
				},
			},
			argAssertion: func(args args) {
				accessor, err := meta.Accessor(args.Reconciled)
				require.NoError(t, err)
				require.Equal(t, "newOwner", accessor.GetOwnerReferences()[0].Name)
			},
			serverStateAssertion: func(serverState corev1.Secret) {
				require.Equal(t, "newOwner", serverState.OwnerReferences[0].Name)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			client := k8s.NewFakeClient(tt.initialObjects...)
			args := tt.args()
			p := Params{
				Client:           client,
				Owner:            args.Owner,
				Expected:         args.Expected,
				Reconciled:       args.Reconciled,
				NeedsUpdate:      args.NeedsUpdate,
				UpdateReconciled: args.UpdateReconciled,
			}

			err := ReconcileResource(p)
			if (err != nil) != (tt.exptectedErrorMsg != "") {
				t.Errorf("ReconcileResource() error = %v, wantErr %v", err, tt.exptectedErrorMsg != "")
				return
			}
			if tt.exptectedErrorMsg != "" {
				assert.Contains(t, err.Error(), tt.exptectedErrorMsg)
			}
			if tt.serverStateAssertion != nil {
				var serverState corev1.Secret
				require.NoError(t, client.Get(context.Background(), objectKey, &serverState))
				tt.serverStateAssertion(serverState)
			}
			if tt.argAssertion != nil {
				tt.argAssertion(args)
			}
		})
	}
}
