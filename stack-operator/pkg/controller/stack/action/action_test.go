package action

import (
	"context"
	"reflect"
	"testing"

	"github.com/elastic/stack-operators/stack-operator/pkg/controller/stack/state"
	"github.com/stretchr/testify/assert"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func Test_apply(t *testing.T) {

	mockService := corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: "default"}}

	fakeClient := fake.NewFakeClient()
	ctx := Context{Client: fakeClient}
	tests := []struct {
		name         string
		args         []Interface
		want         state.ReconcileState
		wantErr      bool
		clientAssert func(c client.Client)
	}{
		{
			name:         "nothing todo means return the original state",
			args:         []Interface{},
			want:         ctx.State,
			wantErr:      false,
			clientAssert: func(c client.Client) {},
		},
		{
			name:    "actions are executed during apply",
			args:    []Interface{Create{Obj: &mockService}},
			want:    ctx.State,
			wantErr: false,
			clientAssert: func(c client.Client) {
				var found corev1.Service
				c.Get(context.TODO(), types.NamespacedName{Namespace: "default", Name: "a"}, &found)
				assert.Equal(t, mockService, found)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := apply(ctx, tt.args)
			if (err != nil) != tt.wantErr {
				t.Errorf("apply() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			tt.clientAssert(fakeClient)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("apply() = %v, want %v", got, tt.want)
			}
		})
	}
}
