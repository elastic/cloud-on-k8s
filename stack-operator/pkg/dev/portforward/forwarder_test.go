package portforward

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/ioutil"
	"net"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

func init() {
	logf.SetLogger(logf.ZapLogger(true))
}

func Test_findLocalForwardingFromComponent(t *testing.T) {
	type args struct {
		line string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "example ipv4",
			args: args{line: "Forwarding from 127.0.0.1:34479 -> 9200"},
			want: "127.0.0.1:34479",
		},
		{
			// Forwarding from [::1]:34479 -> 9200
			name: "example ipv4 with newline",
			args: args{line: "Forwarding from 127.0.0.1:34479 -> 9200\n"},
			want: "127.0.0.1:34479",
		},
		{
			name: "ipv6 should be ignored",
			args: args{line: "Forwarding from [::1]:34479 -> 9200\n"},
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := findForwardedFromLocalAddress(tt.args.line); got != tt.want {
				t.Errorf("findForwardedFromLocalAddress() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_parseAddrType(t *testing.T) {
	type args struct {
		addr string
	}
	tests := []struct {
		name    string
		args    args
		want    addrType
		wantErr bool
	}{
		{
			name: "sample service",
			args: args{addr: "stack-sample-es-public.default.svc.cluster.local"},
			want: addrType{
				type_:     "service",
				name:      "stack-sample-es-public",
				namespace: "default",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseAddrType(tt.args.addr)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseAddrType() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parseAddrType() = %v, want %v", got, tt.want)
			}
		})
	}
}

type capturingDialer struct {
	addresses []string
}

func (d *capturingDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	d.addresses = append(d.addresses, address)
	return nil, nil
}

type testingCommand struct {
	onStdoutPipe func() (io.ReadCloser, error)
	onStart      func() error
	onWait       func() error
}

func (c *testingCommand) StdoutPipe() (io.ReadCloser, error) {
	return c.onStdoutPipe()
}

func (c *testingCommand) Start() error {
	if c.onStart == nil {
		return nil
	}
	return c.onStart()
}

func (c *testingCommand) Wait() error {
	if c.onWait == nil {
		return nil
	}
	return c.onWait()
}

func Test_forwarder_DialContext(t *testing.T) {
	type args struct {
		ctx context.Context
	}
	tests := []struct {
		name         string
		forwarder    *forwarder
		tweaks       func(*forwarder)
		args         args
		wantDialArgs []string
		wantErr      bool
	}{
		{
			name:      "service should be forwarded",
			forwarder: NewForwarder("tcp", "foo.bar.svc.cluster.local:9200"),
			tweaks: func(f *forwarder) {
				f.commandFactory = func(ctx context.Context, name string, arg ...string) command {
					return &testingCommand{
						onStdoutPipe: func() (io.ReadCloser, error) {
							return ioutil.NopCloser(bytes.NewBufferString(
								"Forwarding from 127.0.0.1:34479 -> 9200\n",
							)), nil
						},
					}
				}
			},
			wantDialArgs: []string{"127.0.0.1:34479"},
		},
		{
			name:      "unsupported address should result in error",
			forwarder: NewForwarder("tcp", "example.com"),
			wantErr:   true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dialer := &capturingDialer{}
			tt.forwarder.dialerFunc = dialer.DialContext

			if tt.args.ctx == nil {
				tt.args.ctx = context.TODO()
			}

			ctx, canceller := context.WithTimeout(tt.args.ctx, 5*time.Second)
			defer canceller()

			if tt.tweaks != nil {
				tt.tweaks(tt.forwarder)
			}

			go func() {
				err := tt.forwarder.Run(ctx)
				if !tt.wantErr {
					assert.NoError(t, err)
				}
			}()

			_, err := tt.forwarder.DialContext(ctx)

			if (err != nil) != tt.wantErr {
				t.Errorf("forwarder.DialContext() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			assert.Equal(t, tt.wantDialArgs, dialer.addresses)
		})
	}
}

func Test_forwarder_runPortForwardProcess(t *testing.T) {
	exitError := errors.New("exit error: 1")

	type args struct {
		ctx context.Context
		out chan string
	}
	tests := []struct {
		name      string
		forwarder *forwarder
		tweaks    func(*forwarder)
		args      args
		wantOut   []string
		wantErr   error
	}{
		{
			name:      "example",
			forwarder: NewForwarder("tcp", "foo.bar.svc.cluster.local:9200"),
			tweaks: func(f *forwarder) {
				f.commandFactory = func(ctx context.Context, name string, arg ...string) command {
					return &testingCommand{
						onStdoutPipe: func() (io.ReadCloser, error) {
							return ioutil.NopCloser(bytes.NewBufferString(
								"Forwarding from 127.0.0.1:34479 -> 9200\n",
							)), nil
						},
					}
				}
			},
			wantOut: []string{"127.0.0.1:34479"},
		},
		{
			name:      "error propagated when process exits",
			forwarder: NewForwarder("tcp", "example.com:9200"),
			tweaks: func(f *forwarder) {
				f.commandFactory = func(ctx context.Context, name string, arg ...string) command {
					return &testingCommand{
						onStdoutPipe: func() (io.ReadCloser, error) {
							return ioutil.NopCloser(bytes.NewBufferString("")), nil
						},
						onWait: func() error {
							return exitError
						},
					}
				}
			},
			wantErr: exitError,
		},
		{
			name:      "error propagated when process unable to start",
			forwarder: NewForwarder("tcp", "example.com:9200"),
			tweaks: func(f *forwarder) {
				f.commandFactory = func(ctx context.Context, name string, arg ...string) command {
					return &testingCommand{
						onStdoutPipe: func() (io.ReadCloser, error) {
							return ioutil.NopCloser(bytes.NewBufferString("")), nil
						},
						onStart: func() error {
							return exitError
						},
					}
				}
			},
			wantErr: exitError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.args.ctx == nil {
				tt.args.ctx = context.TODO()
			}
			if tt.args.out == nil {
				tt.args.out = make(chan string)
			}

			ctx, canceller := context.WithTimeout(tt.args.ctx, 5*time.Second)
			defer canceller()

			if tt.tweaks != nil {
				tt.tweaks(tt.forwarder)
			}

			go func() {
				err := tt.forwarder.runPortForwardProcess(ctx, tt.args.out)

				if tt.wantErr != nil {
					assert.Equal(t, tt.wantErr, err)
				} else if err != nil {
					assert.NoError(t, err)
				}

				// if we got an error, close the out channel so wantOut assertions can finish
				if err != nil {
					close(tt.args.out)
				}
			}()

			for _, wantOut := range tt.wantOut {
				gotOut := <-tt.args.out
				assert.Equal(t, wantOut, gotOut)
			}
		})
	}
}
