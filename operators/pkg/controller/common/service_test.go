// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	v12 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestSetServiceDefaults(t *testing.T) {
	sampleSvc := v1.Service{
		ObjectMeta: v12.ObjectMeta{
			Labels: map[string]string{
				"foo": "bar",
			},
		},
		Spec: v1.ServiceSpec{
			Selector: map[string]string{
				"foo": "bar",
			},
			Ports: []v1.ServicePort{
				{Name: "foo"},
			},
		},
	}

	sampleSvcWith := func(setter func(svc *v1.Service)) *v1.Service {
		svc := sampleSvc.DeepCopy()
		setter(svc)
		return svc
	}

	type args struct {
		svc             *v1.Service
		defaultLabels   map[string]string
		defaultSelector map[string]string
		defaultPorts    []v1.ServicePort
	}
	tests := []struct {
		name string
		args args
		want *v1.Service
	}{
		{
			name: "with empty defaults",
			args: args{
				svc: sampleSvc.DeepCopy(),
			},
			want: &sampleSvc,
		},
		{
			name: "should not overwrite, but add labels",
			args: args{
				svc: sampleSvc.DeepCopy(),
				defaultLabels: map[string]string{
					// this should be ignored
					"foo": "foo",
					// this should be added
					"bar": "baz",
				},
			},
			want: sampleSvcWith(func(svc *v1.Service) {
				svc.Labels["bar"] = "baz"
			}),
		},
		{
			name: "should use default selector",
			args: args{
				svc:             &v1.Service{},
				defaultSelector: map[string]string{"foo": "foo"},
			},
			want: &v1.Service{
				Spec: v1.ServiceSpec{
					Selector: map[string]string{"foo": "foo"},
				},
			},
		},
		{
			name: "should use default ports",
			args: args{
				svc: &v1.Service{},
				defaultPorts: []v1.ServicePort{
					{Name: "bar"},
				},
			},
			want: &v1.Service{
				Spec: v1.ServiceSpec{
					Ports: []v1.ServicePort{
						{Name: "bar"},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SetServiceDefaults(tt.args.svc, tt.args.defaultLabels, tt.args.defaultSelector, tt.args.defaultPorts)
			assert.Equal(t, tt.want, got)
		})
	}
}
