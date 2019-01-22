package watches

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/handler"
)

type fakeTransformer struct {
	name    string
	handler handler.EventHandler
}

func (t fakeTransformer) Key() string {
	return t.name
}

func (t fakeTransformer) EventHandler() handler.EventHandler {
	return t.handler
}

var _ ToReconcileRequestTransformer = &fakeTransformer{}

func TestDynamicEnqueueRequest_AddWatch(t *testing.T) {
	tests := []struct {
		name                   string
		setup                  func(handler *DynamicEnqueueRequest)
		args                   ToReconcileRequestTransformer
		wantErr                bool
		registeredTransformers int
	}{
		{
			name:    "fail on unitialized handler",
			args:    &fakeTransformer{},
			wantErr: true,
		},
		{
			name: "succeed on initialized handler",
			args: &fakeTransformer{},
			setup: func(handler *DynamicEnqueueRequest) {
				handler.InjectScheme(scheme.Scheme)
			},
			wantErr:                false,
			registeredTransformers: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := NewDynamicEnqueueRequest()
			if tt.setup != nil {
				tt.setup(d)
			}
			if err := d.AddWatch(tt.args); (err != nil) != tt.wantErr {
				t.Errorf("DynamicEnqueueRequest.AddWatch() error = %v, wantErr %v", err, tt.wantErr)
			}
			assert.Equal(t, len(d.transformers), tt.registeredTransformers)
		})
	}
}

func TestDynamicEnqueueRequest_RemoveWatch(t *testing.T) {
	tests := []struct {
		name                   string
		setup                  func(handler *DynamicEnqueueRequest)
		args                   ToReconcileRequestTransformer
		registeredTransformers int
	}{
		{
			name: "removal on empty handler is a NOOP",
			args: &fakeTransformer{},
		},
		{
			name: "succeed on initialized handler",
			args: &fakeTransformer{},
			setup: func(handler *DynamicEnqueueRequest) {
				handler.InjectScheme(scheme.Scheme)
				handler.AddWatch(&fakeTransformer{})
				assert.Equal(t, len(handler.transformers), 1)
			},
			registeredTransformers: 0,
		},
		{
			name: "uses key to identify transformer",
			args: &fakeTransformer{
				name: "bar",
			},
			setup: func(handler *DynamicEnqueueRequest) {
				handler.InjectScheme(scheme.Scheme)
				assert.NoError(t, handler.AddWatch(&fakeTransformer{
					name: "foo",
				}))
				assert.Equal(t, len(handler.transformers), 1)
			},
			registeredTransformers: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := NewDynamicEnqueueRequest()
			if tt.setup != nil {
				tt.setup(d)
			}
			d.RemoveWatch(tt.args)
			assert.Equal(t, len(d.transformers), tt.registeredTransformers)
		})
	}
}
