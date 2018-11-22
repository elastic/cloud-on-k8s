package action

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/elastic/stack-operators/stack-operator/pkg/controller/stack/state"
	"github.com/stretchr/testify/assert"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type testAction struct {
	Result *reconcile.Result
}

func (a testAction) Name() string {
	return "test"
}

func (a testAction) Execute(ctx Context) (*reconcile.Result, error) {
	return a.Result, nil
}

var noClientInteraction = func(c client.Client) {}

func Test_apply(t *testing.T) {

	mockService := corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: "default"}}
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
			want:         state.ReconcileState{},
			wantErr:      false,
			clientAssert: noClientInteraction,
		},
		{
			name:    "actions are executed during apply",
			args:    []Interface{Create{Obj: &mockService}},
			want:    state.ReconcileState{},
			wantErr: false,
			clientAssert: func(c client.Client) {
				var found corev1.Service
				c.Get(context.TODO(), types.NamespacedName{Namespace: "default", Name: "a"}, &found)
				assert.Equal(t, mockService, found)
			},
		},
		{
			name:    "errors short circuit the execution (e.g. update before create)",
			args:    []Interface{Update{Obj: &mockService}, Create{Obj: &mockService}},
			want:    state.ReconcileState{},
			wantErr: true,
			clientAssert: func(c client.Client) {
				var found corev1.Service
				err := c.Get(context.TODO(), types.NamespacedName{Namespace: "default", Name: "a"}, &found)
				assert.True(t, errors.IsNotFound(err))
			},
		},
		{
			name: "results are aggregated",
			args: []Interface{
				testAction{Result: &reconcile.Result{RequeueAfter: 1 * time.Hour}},
				testAction{Result: &reconcile.Result{RequeueAfter: 1 * time.Minute}},
				testAction{Result: &reconcile.Result{}},
			},
			want:         state.ReconcileState{Result: reconcile.Result{RequeueAfter: 1 * time.Minute}},
			wantErr:      false,
			clientAssert: noClientInteraction,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeClient := fake.NewFakeClient()
			ctx := Context{Client: fakeClient}
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
