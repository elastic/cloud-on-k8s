// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package portforward

import (
	"context"
	"errors"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/types"
)

type capturingDialer struct {
	addresses []string
}

func (d *capturingDialer) DialContext(_ context.Context, _, address string) (net.Conn, error) {
	d.addresses = append(d.addresses, address)
	return nil, nil
}

func NewPodForwarderWithTest(t *testing.T, network, addr string) *PodForwarder {
	t.Helper()
	fwd, err := NewPodForwarder(context.Background(), network, addr, nil)
	require.NoError(t, err)
	return fwd
}

type stubPortForwarder struct {
	ctx context.Context
}

func (c *stubPortForwarder) ForwardPorts() error {
	<-c.ctx.Done()
	return nil
}

func Test_podForwarder_DialContext(t *testing.T) {
	type args struct {
		ctx context.Context
	}
	tests := []struct {
		name         string
		forwarder    *PodForwarder
		tweaks       func(t *testing.T, f *PodForwarder)
		args         args
		wantDialArgs []string
		wantErr      bool
	}{
		{
			name:      "pod should be forwarded",
			forwarder: NewPodForwarderWithTest(t, "tcp", "foo.bar.pod:9200"),
			tweaks: func(t *testing.T, f *PodForwarder) {
				t.Helper()
				f.ephemeralPortFinder = func() (string, error) {
					return "12345", nil
				}
				f.portForwarderFactory = PortForwarderFactory(func(
					ctx context.Context,
					namespace, podName string,
					ports []string,
					readyChan chan struct{},
				) (PortForwarder, error) {
					assert.Equal(t, "bar", namespace)
					assert.Equal(t, "foo", podName)
					assert.Equal(t, []string{"12345:9200"}, ports)

					// closing the readyChan to pretend we're ready
					close(readyChan)

					return &stubPortForwarder{ctx: ctx}, nil
				})
			},
			wantDialArgs: []string{"127.0.0.1:12345"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dialer := &capturingDialer{}
			tt.forwarder.dialerFunc = dialer.DialContext

			if tt.args.ctx == nil {
				tt.args.ctx = context.Background()
			}

			// wait for our goroutines to finish before returning
			wg := sync.WaitGroup{}
			defer wg.Wait()

			ctx, canceller := context.WithTimeout(tt.args.ctx, 5*time.Second)
			defer canceller()

			if tt.tweaks != nil {
				tt.tweaks(t, tt.forwarder)
			}

			wg.Add(1)
			currentTest := tt
			go func() {
				defer wg.Done()
				err := currentTest.forwarder.Run(ctx)
				if !currentTest.wantErr {
					assert.NoError(t, err)
				}
			}()

			_, err := tt.forwarder.DialContext(ctx)

			if (err != nil) != tt.wantErr {
				t.Errorf("podForwarder.DialContext() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			assert.Equal(t, tt.wantDialArgs, dialer.addresses)
		})
	}
}

func Test_parsePodAddr(t *testing.T) {
	type args struct {
		addr string
	}
	tests := []struct {
		name    string
		args    args
		want    types.NamespacedName
		wantErr error
	}{
		{
			name: "pod DNS without subdomain",
			args: args{addr: "foo.bar.pod:1234"},
			want: types.NamespacedName{Namespace: "bar", Name: "foo"},
		},
		{
			name: "pod DNS with pod and namespace only",
			args: args{addr: "foopod.barnamespace:1234"},
			want: types.NamespacedName{Namespace: "barnamespace", Name: "foopod"},
		},
		{
			name: "pod DNS with pod, subdomain and namespace",
			args: args{addr: "foopod.foosubdomain.barnamespace:1234"},
			want: types.NamespacedName{Namespace: "barnamespace", Name: "foopod"},
		},
		{
			name:    "invalid",
			args:    args{addr: "foobar:1234"},
			wantErr: errors.New("unsupported pod address format: foobar"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parsePodAddr(context.Background(), tt.args.addr, nil)

			if tt.wantErr != nil {
				assert.Equal(t, tt.wantErr, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, *got)
		})
	}
}

func Test_podIPv4Regex(t *testing.T) {
	tests := []struct {
		name string
		addr string
		want bool
	}{
		{
			name: "valid ipv4",
			addr: "10.0.0.2",
			want: true,
		},
		{
			name: "invalid ipv4 still correctly parsed",
			addr: "666.666.666.666",
			want: true,
		},
		{
			name: "empty string",
			addr: "",
			want: false,
		},
		{
			name: "dns",
			addr: "name.namespace.pod",
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, podIPv4Regex.MatchString(tt.addr))
		})
	}
}
