// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package reconciler

import (
	"reflect"
	"testing"

	"github.com/elastic/cloud-on-k8s/pkg/controller/common/comparison"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
)

func withoutControllerRef(obj runtime.Object) runtime.Object {
	copy := obj.DeepCopyObject()
	copy.(metav1.Object).SetOwnerReferences(nil)
	return copy
}

func noopModifier() {}

type wrong struct{}

func (w wrong) GetObjectKind() schema.ObjectKind {
	return nil
}

func (w wrong) DeepCopyObject() runtime.Object {
	return w
}

var _ runtime.Object = wrong{}

func TestReconcileResource(t *testing.T) {

	type args struct {
		Expected         runtime.Object
		Reconciled       runtime.Object
		NeedsUpdate      func() bool
		UpdateReconciled func()
	}

	objectKey := types.NamespacedName{Name: "test", Namespace: "foo"}
	obj := &corev1.Secret{ObjectMeta: k8s.ToObjectMeta(objectKey)}
	secretData := map[string][]byte{"bar": []byte("shush")}

	tests := []struct {
		name            string
		args            func() args
		initialObjects  []runtime.Object
		argAssertion    func(args args)
		errorAssertion  func(err error)
		clientAssertion func(c k8s.Client)
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
			errorAssertion: func(err error) {
				assert.Contains(t, err.Error(), "Expected must not be nil")
			},
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
			errorAssertion: func(err error) {
				assert.Contains(t, err.Error(), "NeedsUpdate must not be nil")
			},
		},
		{
			name: "Error: Reconcile must not be nil",
			args: func() args {
				return args{
					Expected: obj.DeepCopy(),
				}
			},
			errorAssertion: func(err error) {
				assert.Contains(t, err.Error(), "Reconciled must not be nil")
			},
		},
		{
			name: "Error: UpdateReconciled must be defined",
			args: func() args {
				return args{
					Expected:   obj.DeepCopy(),
					Reconciled: &corev1.Secret{},
				}
			},
			errorAssertion: func(err error) {
				assert.Contains(t, err.Error(), "UpdateReconciled must not be nil")
			},
		},
		{
			name: "Error: Expected needs to implement runtime.Object and meta.Object",
			args: func() args {
				return args{
					Expected:         &wrong{},
					Reconciled:       &corev1.Secret{},
					UpdateReconciled: noopModifier,
					NeedsUpdate: func() bool {
						return false
					},
				}
			},
			errorAssertion: func(err error) {
				assert.Contains(t, err.Error(), "object does not implement the Object interfaces")
			},
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
			clientAssertion: func(c k8s.Client) {
				var found corev1.Secret
				assert.NoError(t, c.Get(objectKey, &found))
				diff := comparison.Diff(obj, withoutControllerRef(&found))
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
			clientAssertion: func(c k8s.Client) {
				var found corev1.Secret
				assert.NoError(t, c.Get(objectKey, &found))
				assert.Equal(t, "be quiet", string(found.Data["bar"]))
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
			clientAssertion: func(c k8s.Client) {
				var found corev1.Secret
				assert.NoError(t, c.Get(objectKey, &found))
				// should be unchanged as it is ignored by the custom differ
				assert.Equal(t, "other", found.Labels["label"])
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			client := k8s.WrappedFakeClient(tt.initialObjects...)
			args := tt.args()
			p := Params{
				Client:           client,
				Owner:            &appsv1.Deployment{}, //just a dummy
				Expected:         args.Expected,
				Reconciled:       args.Reconciled,
				NeedsUpdate:      args.NeedsUpdate,
				UpdateReconciled: args.UpdateReconciled,
			}

			err := ReconcileResource(p)
			if (err != nil) != (tt.errorAssertion != nil) {
				t.Errorf("ReconcileResource() error = %v, wantErr %v", err, tt.errorAssertion != nil)
				return
			}
			if tt.errorAssertion != nil {
				tt.errorAssertion(err)
			}
			if tt.clientAssertion != nil {
				tt.clientAssertion(client)
			}
			if tt.argAssertion != nil {
				tt.argAssertion(args)
			}
		})
	}
}
