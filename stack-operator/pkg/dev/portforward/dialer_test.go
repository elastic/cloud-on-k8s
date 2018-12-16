package portforward

import (
	"context"
	"net"
	"reflect"
	"testing"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

type stubForwarder struct {
	network, addr string
	onRun         func(ctx context.Context) error
	onDialContext func(ctx context.Context) (net.Conn, error)
}

func (f *stubForwarder) Run(ctx context.Context) error {
	if f.onRun != nil {
		return f.onRun(ctx)
	}
	<-ctx.Done()
	return nil
}

func (f *stubForwarder) DialContext(ctx context.Context) (net.Conn, error) {
	if f.onDialContext != nil {
		return f.onDialContext(ctx)
	}
	return nil, nil
}

func TestForwardingDialer_DialContext(t *testing.T) {
	type args struct {
		ctx     context.Context
		network string
		addr    string
	}
	tests := []struct {
		name            string
		tweaks          func(dialer *ForwardingDialer)
		args            args
		want            net.Conn
		wantErr         bool
		extraAssertions func(t *testing.T, dialer *ForwardingDialer)
	}{
		{
			name: "sample",
			tweaks: func(dialer *ForwardingDialer) {
				dialer.forwarderFactory = ForwarderFactoryFunc(
					func(_ client.Client, network, addr string) (Forwarder, error) {
						return &stubForwarder{
							network: network, addr: addr,
						}, nil
					},
				)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := NewForwardingDialer()

			if tt.tweaks != nil {
				tt.tweaks(d)
			}

			got, err := d.DialContext(tt.args.ctx, tt.args.network, tt.args.addr)
			if (err != nil) != tt.wantErr {
				t.Errorf("ForwardingDialer.DialContext() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ForwardingDialer.DialContext() = %v, want %v", got, tt.want)
			}

			if tt.extraAssertions != nil {
				tt.extraAssertions(t, d)
			}
		})
	}
}
