// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package association

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
)

func TestServiceURL(t *testing.T) {
	type args struct {
		c          k8s.Client
		serviceNSN types.NamespacedName
		protocol   string
		basePath   string
	}
	svcName := types.NamespacedName{Namespace: "a", Name: "b"}
	svcFixture := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Namespace: "a", Name: "b"},
		Spec: corev1.ServiceSpec{Ports: []corev1.ServicePort{
			{
				Name: "https",
				Port: 9200,
			},
		}},
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "happy path",
			args: args{
				c:          k8s.NewFakeClient(svcFixture),
				serviceNSN: svcName,
				protocol:   "https",
			},
			want:    "https://b.a.svc:9200",
			wantErr: false,
		},
		{
			name: "service does not exist",
			args: args{
				c:          k8s.NewFakeClient(),
				serviceNSN: svcName,
			},
			want:    "",
			wantErr: true,
		},
		{
			name: "no port for protocol",
			args: args{
				c:          k8s.NewFakeClient(svcFixture),
				serviceNSN: svcName,
				protocol:   "http",
			},
			want:    "",
			wantErr: true,
		},
		{
			name: "happy path with base path",
			args: args{
				c:          k8s.NewFakeClient(svcFixture),
				serviceNSN: svcName,
				protocol:   "https",
				basePath:   "/monitoring/kibana",
			},
			want:    "https://b.a.svc:9200/monitoring/kibana",
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ServiceURL(tt.args.c, tt.args.serviceNSN, tt.args.protocol, tt.args.basePath)
			if (err != nil) != tt.wantErr {
				t.Errorf("ServiceURL() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ServiceURL() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_findPortFor(t *testing.T) {
	svcFixture := corev1.Service{
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{Name: "a", Port: 1},
				{Name: "b", Port: 2},
			},
		},
	}
	type args struct {
		protocol string
		svc      corev1.Service
	}
	tests := []struct {
		name    string
		args    args
		want    int32
		wantErr bool
	}{
		{
			name: "finds existing port",
			args: args{
				protocol: "a",
				svc:      svcFixture,
			},
			want:    1,
			wantErr: false,
		},
		{
			name: "err on non-existing port",
			args: args{
				protocol: "http",
				svc:      svcFixture,
			},
			want:    -1,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := findPortFor(tt.args.protocol, tt.args.svc)
			if (err != nil) != tt.wantErr {
				t.Errorf("findPortFor() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("findPortFor() got = %v, want %v", got, tt.want)
			}
		})
	}
}
