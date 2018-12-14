package portforward

import (
	"context"
	"net"
	"reflect"
	"testing"
)

type stubForwarder struct {
	network, addr string
	onRun         func(ctx context.Context) error
	onDialContext func(ctx context.Context) (net.Conn, error)
}

func (f *stubForwarder) Run(ctx context.Context) error {
	return f.onRun(ctx)
}

func (f *stubForwarder) DialContext(ctx context.Context) (net.Conn, error) {
	return f.onDialContext(ctx)
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
				dialer.forwarderFactory = func(network, addr string) Forwarder {
					return &stubForwarder{network: network, addr: addr}
				}
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
