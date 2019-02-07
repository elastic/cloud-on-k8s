// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package version

import (
	"testing"

	"k8s.io/apimachinery/pkg/api/resource"
)

func Test_quantityToMegabytes(t *testing.T) {
	type args struct {
		q resource.Quantity
	}
	tests := []struct {
		name string
		args args
		want int
	}{
		{name: "simple", args: args{q: resource.MustParse("2Gi")}, want: 2 * 1024},
		{name: "large", args: args{q: resource.MustParse("9Ti")}, want: 9 * 1024 * 1024},
		{name: "small", args: args{q: resource.MustParse("0.25Gi")}, want: 256},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := quantityToMegabytes(tt.args.q); got != tt.want {
				t.Errorf("quantityToMegabytes() = %v, want %v", got, tt.want)
			}
		})
	}
}
