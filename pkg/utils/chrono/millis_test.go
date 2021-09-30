// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package chrono

import (
	"testing"
	"time"
)

func TestToMillis(t *testing.T) {
	type args struct {
		t time.Time
	}
	tests := []struct {
		name string
		args args
		want int64
	}{
		{
			name: "turns time into unix milliseconds",
			args: args{
				t: time.Date(2019, 01, 22, 0, 0, 0, 0, time.UTC),
			},
			want: 1548115200000,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ToMillis(tt.args.t); got != tt.want {
				t.Errorf("toMillis() = %v, want %v", got, tt.want)
			}
		})
	}
}
