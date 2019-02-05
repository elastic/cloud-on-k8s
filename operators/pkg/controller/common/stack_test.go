// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package common

import (
	"testing"

	"github.com/stretchr/testify/assert"

	deploymentsv1alpha1 "github.com/elastic/k8s-operators/operators/pkg/apis/deployments/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestStackID(t *testing.T) {
	type args struct {
		s deploymentsv1alpha1.Stack
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			args: args{s: deploymentsv1alpha1.Stack{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-stack",
					Namespace: "default",
				},
			}},
			want: "default-my-stack",
		},
		{
			args: args{s: deploymentsv1alpha1.Stack{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-other-stack",
					Namespace: "default",
				},
			}},
			want: "default-my-other-stack",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := StackID(tt.args.s)
			assert.Equal(t, tt.want, got)
		})
	}
}
