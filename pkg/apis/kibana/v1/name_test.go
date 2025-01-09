// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package v1

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestNamers(t *testing.T) {
	tests := []struct {
		name  string
		namer any
		arg   any
		want  string
	}{
		{
			name:  "test httpService namer",
			namer: HTTPService,
			arg:   "sample",
			want:  "sample-kb-http",
		},
		{
			name:  "test deployment namer",
			namer: Deployment,
			arg:   "sample",
			want:  "sample-kb",
		},
		{
			name:  "test scripts configmap namer",
			namer: ScriptsConfigMap,
			arg:   "sample",
			want:  "sample-kb-scripts",
		},
		{
			name:  "test ConfigSecret namer",
			namer: ConfigSecret,
			arg:   Kibana{ObjectMeta: metav1.ObjectMeta{Name: "sample"}},
			want:  "sample-kb-config",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			switch f := tt.namer.(type) {
			case func(string) string:
				arg := tt.arg.(string)
				if got := f(arg); got != tt.want {
					t.Errorf("%s = %v, want %v", tt.name, got, tt.want)
				}
			case func(Kibana) string:
				arg := tt.arg.(Kibana)
				if got := f(arg); got != tt.want {
					t.Errorf("%s = %v, want %v", tt.name, got, tt.want)
				}
			}
		})
	}
}
