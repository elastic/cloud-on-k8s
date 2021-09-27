// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package math

import "testing"

func TestRoundUp(t *testing.T) {
	type args struct {
		numToRound int64
		multiple   int64
	}
	tests := []struct {
		name string
		args args
		want int64
	}{
		{
			name: "RoundUp a number to 0 is the identity function",
			args: args{
				numToRound: 42,
				multiple:   0,
			},
			want: 42,
		},
		{
			name: "numToRound and multiple are equal",
			args: args{
				numToRound: 42,
				multiple:   42,
			},
			want: 42,
		},
		{
			name: "numToRound is 0",
			args: args{
				numToRound: 0,
				multiple:   42,
			},
			want: 0,
		},
		{
			name: "numToRound is lower than multiple",
			args: args{
				numToRound: 1,
				multiple:   42,
			},
			want: 42,
		},
		{
			name: "numToRound is greater than multiple",
			args: args{
				numToRound: 43,
				multiple:   42,
			},
			want: 84,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := RoundUp(tt.args.numToRound, tt.args.multiple); got != tt.want {
				t.Errorf("RoundUp() = %v, want %v", got, tt.want)
			}
		})
	}
}
