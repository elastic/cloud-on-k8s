package watches

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/handler"
)

type fakeHandler struct {
	name    string
	handler handler.EventHandler
}

func (t fakeHandler) Key() string {
	return t.name
}

func (t fakeHandler) EventHandler() handler.EventHandler {
	return t.handler
}

var _ HandlerRegistration = &fakeHandler{}

func TestDynamicEnqueueRequest_AddWatch(t *testing.T) {
	tests := []struct {
		name               string
		setup              func(handler *DynamicEnqueueRequest)
		args               HandlerRegistration
		wantErr            bool
		registeredHandlers int
	}{
		{
			name:    "fail on uninitialized handler",
			args:    &fakeHandler{},
			wantErr: true,
		},
		{
			name: "succeed on initialized handler",
			args: &fakeHandler{},
			setup: func(handler *DynamicEnqueueRequest) {
				handler.InjectScheme(scheme.Scheme)
			},
			wantErr:            false,
			registeredHandlers: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := NewDynamicEnqueueRequest()
			if tt.setup != nil {
				tt.setup(d)
			}
			if err := d.AddHandler(tt.args); (err != nil) != tt.wantErr {
				t.Errorf("DynamicEnqueueRequest.AddHandler() error = %v, wantErr %v", err, tt.wantErr)
			}
			assert.Equal(t, len(d.registrations), tt.registeredHandlers)
		})
	}
}

func TestDynamicEnqueueRequest_RemoveWatch(t *testing.T) {
	tests := []struct {
		name               string
		setup              func(handler *DynamicEnqueueRequest)
		args               HandlerRegistration
		registeredHandlers int
	}{
		{
			name: "removal on empty handler is a NOOP",
			args: &fakeHandler{},
		},
		{
			name: "succeed on initialized handler",
			args: &fakeHandler{},
			setup: func(handler *DynamicEnqueueRequest) {
				handler.InjectScheme(scheme.Scheme)
				handler.AddHandler(&fakeHandler{})
				assert.Equal(t, len(handler.registrations), 1)
			},
			registeredHandlers: 0,
		},
		{
			name: "uses key to identify transformer",
			args: &fakeHandler{
				name: "bar",
			},
			setup: func(handler *DynamicEnqueueRequest) {
				handler.InjectScheme(scheme.Scheme)
				assert.NoError(t, handler.AddHandler(&fakeHandler{
					name: "foo",
				}))
				assert.Equal(t, len(handler.registrations), 1)
			},
			registeredHandlers: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := NewDynamicEnqueueRequest()
			if tt.setup != nil {
				tt.setup(d)
			}
			d.RemoveHandler(tt.args)
			assert.Equal(t, len(d.registrations), tt.registeredHandlers)
		})
	}
}
