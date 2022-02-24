// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package stringsutil

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStringInSlice(t *testing.T) {
	type args struct {
		str  string
		list []string
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			"String in slice returns true",
			args{
				"info",
				[]string{"info", "warn", "debug"},
			},
			true,
		},
		{
			"String not in slice returns false",
			args{
				"error",
				[]string{"info", "warn", "debug"},
			},
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := StringInSlice(tt.args.str, tt.args.list)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestRemoveStringInSlice(t *testing.T) {
	type args struct {
		s     string
		slice []string
	}
	tests := []struct {
		name string
		args args
		want []string
	}{
		{
			name: "removes string from slice",
			args: args{
				s:     "b",
				slice: []string{"a", "b", "c"},
			},
			want: []string{"a", "c"},
		},
		{
			name: "removes string from slice multiple occurrences",
			args: args{
				s:     "b",
				slice: []string{"a", "b", "c", "b"},
			},
			want: []string{"a", "c"},
		},
		{
			name: "noop when string not found",
			args: args{
				s:     "d",
				slice: []string{"a", "b", "c"},
			},
			want: []string{"a", "b", "c"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.ElementsMatch(t, tt.want, RemoveStringInSlice(tt.args.s, tt.args.slice))
		})
	}
}

func TestSliceToMap(t *testing.T) {
	// happy path
	expected := map[string]struct{}{
		"a": {},
		"b": {},
	}
	require.Equal(t, expected, SliceToMap([]string{"a", "b", "b"}))

	// empty input
	require.Equal(t, map[string]struct{}{}, SliceToMap(nil))
}

func Test_sortStringSlice(t *testing.T) {
	slice := []string{"aab", "aac", "aaa", "aab"}
	SortStringSlice(slice)
	require.Equal(t, []string{"aaa", "aab", "aab", "aac"}, slice)
}

func TestDifference(t *testing.T) {
	type args struct {
		a []string
		b []string
	}
	tests := []struct {
		name        string
		args        args
		wantOnlyInA []string
		wantOnlyInB []string
	}{
		{
			name: "Happy path",
			args: args{
				a: []string{"a", "b", "d"},
				b: []string{"a", "c", "d"},
			},
			wantOnlyInA: []string{"b"},
			wantOnlyInB: []string{"c"},
		},
		{
			name: "a is nil",
			args: args{
				a: nil,
				b: []string{"a", "b"},
			},
			wantOnlyInA: nil,
			wantOnlyInB: []string{"a", "b"},
		},
		{
			name: "b is nil",
			args: args{
				a: []string{"a", "b"},
				b: nil,
			},
			wantOnlyInA: []string{"a", "b"},
			wantOnlyInB: nil,
		},
		{
			name: "both nil",
			args: args{
				a: nil,
				b: nil,
			},
			wantOnlyInA: nil,
			wantOnlyInB: nil,
		},
		{
			name: "equals",
			args: args{
				a: []string{"d", "b", "a"},
				b: []string{"a", "b", "d"},
			},
			wantOnlyInA: nil,
			wantOnlyInB: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, got1 := Difference(tt.args.a, tt.args.b)
			if !reflect.DeepEqual(got, tt.wantOnlyInA) {
				t.Errorf("Difference() got = %v, wantOnlyInA %v", got, tt.wantOnlyInA)
			}
			if !reflect.DeepEqual(got1, tt.wantOnlyInB) {
				t.Errorf("Difference() got1 = %v, wantOnlyInB %v", got1, tt.wantOnlyInB)
			}
		})
	}
}
