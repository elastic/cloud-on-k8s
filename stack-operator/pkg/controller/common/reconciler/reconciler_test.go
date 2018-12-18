package reconciler

import (
	"context"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func withoutControllerRef(obj runtime.Object) runtime.Object {
	copy := obj.DeepCopyObject()
	copy.(metav1.Object).SetOwnerReferences(nil)
	return copy
}

func noopModifier(_, _ *corev1.Secret) {}

func TestReconcileResource(t *testing.T) {

	type args struct {
		Object runtime.Object
		// Differ is a generic function of the type func(expected, found T) bool where T is a runtime.Object
		Differ interface{}
		// Modifier is generic function of the type func(expected, found T) where T is runtime Object
		Modifier interface{}
	}

	objectKey := types.NamespacedName{Name: "test", Namespace: "foo"}
	obj := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: objectKey.Name, Namespace: objectKey.Namespace}}
	secretData := map[string][]byte{"bar": []byte("shush")}

	tests := []struct {
		name            string
		args            args
		initialObjects  []runtime.Object
		argAssertion    func(args args)
		errorAssertion  func(err error)
		clientAssertion func(c client.Client)
		panics          bool
	}{
		{
			name: "Error: not a metadata object",
			args: args{},
			errorAssertion: func(err error) {
				assert.Contains(t, err.Error(), "not a k8s metadata Object")
			},
		},
		{
			name: "Create resource if not found",
			args: args{Object: obj.DeepCopy()},
			clientAssertion: func(c client.Client) {
				var found corev1.Secret
				assert.NoError(t, c.Get(context.TODO(), objectKey, &found))
				assert.Equal(t, obj, withoutControllerRef(&found))
			},
		},
		{
			name: "Returns server state via in/out param",
			args: args{
				Object:   obj.DeepCopy(),
				Modifier: noopModifier,
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
				assert.Equal(t, "baz", args.Object.(*corev1.Secret).Labels["label"])
			},
		},
		{
			name: "Updates server state, if in param differs from remote",
			args: args{
				Object: &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      objectKey.Name,
						Namespace: objectKey.Namespace,
					},
					Data: map[string][]byte{
						"bar": []byte("be quiet"),
					},
				},
				Modifier: func(expected, found *corev1.Secret) {
					found.Data = expected.Data
				},
			},
			initialObjects: []runtime.Object{obj},
			argAssertion: func(args args) {
				// should be unchanged
				assert.Equal(t, "be quiet", string(args.Object.(*corev1.Secret).Data["bar"]))
			},
			clientAssertion: func(c client.Client) {
				var found corev1.Secret
				assert.NoError(t, c.Get(context.TODO(), objectKey, &found))
				assert.Equal(t, "be quiet", string(found.Data["bar"]))
			},
		},
		{
			name: "Can optionally use custom differ",
			args: args{
				Object: &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      objectKey.Name,
						Namespace: objectKey.Namespace,
						Labels: map[string]string{
							"label": "baz",
						},
					},
					Data: secretData,
				},
				Differ: func(expected, found *corev1.Secret) bool {
					return !reflect.DeepEqual(expected.Data, found.Data)
				},
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
				assert.Equal(t, "other", string(args.Object.(*corev1.Secret).Labels["label"]))
			},
			clientAssertion: func(c client.Client) {
				var found corev1.Secret
				assert.NoError(t, c.Get(context.TODO(), objectKey, &found))
				// should be unchanged as it is ignored by the custom differ
				assert.Equal(t, "other", string(found.Labels["label"]))
				// but we don't panic even though I haven't specified the modifier function
				// because the differ should not flap up an update
			},
		},
		{
			name: "Validates differ function",
			args: args{
				Object: obj,
				Differ: func(x, y int) int {
					return 0
				},
			},
			initialObjects: []runtime.Object{obj},
			panics:         true,
		},
		{
			name: "Validates modifier function",
			args: args{
				Object: obj,
				Differ: func(x, y *corev1.Secret) bool {
					return true
				},
			},
			initialObjects: []runtime.Object{obj},
			panics:         true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			client := fake.NewFakeClient(tt.initialObjects...)
			p := Params{
				Client:   client,
				Scheme:   scheme.Scheme,
				Owner:    &appsv1.Deployment{}, //just a dummy
				Object:   tt.args.Object,
				Differ:   tt.args.Differ,
				Modifier: tt.args.Modifier,
			}

			if tt.panics {
				defer func() {
					if r := recover(); r == nil {
						t.Errorf("The call did not panic, but it should")
					}
				}()
			}
			err := ReconcileResource(p)
			if (err != nil) != (tt.errorAssertion != nil) {
				t.Errorf("ReconcileResource() error = %v, wantErr %v", err, tt.errorAssertion != nil)
			}
			if tt.errorAssertion != nil {
				tt.errorAssertion(err)
			}
			if tt.clientAssertion != nil {
				tt.clientAssertion(client)
			}
			if tt.argAssertion != nil {
				tt.argAssertion(tt.args)
			}
		})
	}
}
