/*
 * Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
 * or more contributor license agreements. Licensed under the Elastic License;
 * you may not use this file except in compliance with the Elastic License.
 */

package chrono

import (
	"testing"
	"time"
)

func Test_toMillis(t *testing.T) {
	type args struct {
		t time.Time
	}
	tests := []struct {
		name string
		args args
		want int64
	}{
		{
			name: "turnes time into unix milliseconds",
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
