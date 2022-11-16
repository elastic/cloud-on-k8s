// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package set

import (
	"reflect"
	"sort"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMake(t *testing.T) {
	type args struct {
		strings []string
	}
	tests := []struct {
		name string
		args args
		want StringSet
	}{
		{
			name: "nil makes an empty set",
			args: args{},
			want: StringSet(map[string]struct{}{}),
		},
		{
			name: "creates set from passed strings",
			args: args{
				strings: []string{"a", "b"},
			},
			want: StringSet{"a": {}, "b": {}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Make(tt.args.strings...); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Make() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStringSet_Add(t *testing.T) {
	type args struct {
		s string
	}
	tests := []struct {
		name      string
		set       StringSet
		args      args
		assertion func(s StringSet)
	}{
		{
			name: "add adds the given elements ",
			set:  StringSet{"a": {}},
			args: args{
				s: "b",
			},
			assertion: func(s StringSet) {
				require.True(t, s.Has("b"))
				require.True(t, s.Has("a"))
				require.Equal(t, 2, s.Count())
			},
		},
		{
			name: "add maintains set properties",
			set:  StringSet{"a": {}},
			args: args{
				s: "a",
			},
			assertion: func(s StringSet) {
				require.True(t, s.Has("a"))
				require.Equal(t, 1, s.Count())
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.set.Add(tt.args.s)
			if tt.assertion != nil {
				tt.assertion(tt.set)
			}
		})
	}
}

func TestStringSet_Del(t *testing.T) {
	type args struct {
		s string
	}
	tests := []struct {
		name      string
		set       StringSet
		args      args
		assertion func(StringSet)
	}{
		{
			name: "Del removes elements",
			set:  StringSet{"a": {}},
			args: args{
				s: "a",
			},
			assertion: func(s StringSet) {
				require.False(t, s.Has("a"))
				require.Equal(t, 0, s.Count())
			},
		},
		{
			name: "Del is nil safe",
			set:  nil,
			args: args{
				s: "a",
			},
		},
		{
			name: "Del on empty is a noop",
			set:  StringSet{},
			args: args{
				s: "a",
			},
			assertion: func(s StringSet) {
				require.Equal(t, 0, s.Count())
				require.False(t, s.Has("a"))
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.set.Del(tt.args.s)
			if tt.assertion != nil {
				tt.assertion(tt.set)
			}
		})
	}
}

func TestStringSet_Count(t *testing.T) {
	var tests = []struct {
		name string
		set  StringSet
		want int
	}{
		{
			name: "Count returns num elements",
			set:  StringSet{"a": {}, "b": {}},
			want: 2,
		},
		{
			name: "Count is nil safe",
			set:  nil,
			want: 0,
		},
		{
			name: "Count on empty",
			set:  StringSet{},
			want: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.set.Count(); got != tt.want {
				t.Errorf("StringSet.Count() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStringSet_Has(t *testing.T) {
	type args struct {
		s string
	}
	tests := []struct {
		name       string
		set        StringSet
		args       args
		wantExists bool
	}{
		{
			name: "Has tests set membership: presence",
			set:  StringSet{"a": {}},
			args: args{
				s: "a",
			},
			wantExists: true,
		},
		{
			name: "Has tests set membership: absence",
			set:  StringSet{"b": {}},
			args: args{
				s: "a",
			},
			wantExists: false,
		},
		{
			name: "Has is nil safe",
			set:  nil,
			args: args{
				s: "a",
			},
			wantExists: false,
		},
		{
			name: "Has on empty",
			set:  StringSet{},
			args: args{
				s: "a",
			},
			wantExists: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if gotExists := tt.set.Has(tt.args.s); gotExists != tt.wantExists {
				t.Errorf("StringSet.Has() = %v, want %v", gotExists, tt.wantExists)
			}
		})
	}
}

func TestStringSet_AsSlice(t *testing.T) {
	tests := []struct {
		name string
		set  StringSet
		want sort.StringSlice
	}{
		{
			name: "AsSlice returns the set members as a StringSlice ",
			set: StringSet{
				"a": {}, "b": {},
			},
			want: sort.StringSlice{"a", "b"},
		},
		{
			name: "AsSlice is nil safe",
			set:  nil,
			want: nil,
		},
		{
			name: "AsSlice on empty slice",
			set:  StringSet{},
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.set.AsSlice()
			got.Sort()
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("StringSet.AsSlice() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStringSet_MergeWith(t *testing.T) {
	tests := []struct {
		name  string
		set   StringSet
		other StringSet
		want  StringSet
	}{
		{
			name:  "Merge with nil",
			set:   StringSet{"a": {}, "b": {}},
			other: nil,
			want:  StringSet{"a": {}, "b": {}},
		},
		{
			name:  "Merge with empty",
			set:   StringSet{},
			other: nil,
			want:  StringSet{},
		},
		{
			name:  "Merge with other set containing new and duplicate entries",
			set:   StringSet{"a": {}, "b": {}},
			other: StringSet{"b": {}, "c": {}, "d": {}},
			want:  StringSet{"a": {}, "b": {}, "c": {}, "d": {}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.set.MergeWith(tt.other)
			require.Equal(t, tt.want, tt.set)
		})
	}
}

func TestStringSet_Diff(t *testing.T) {
	tests := []struct {
		name  string
		set   StringSet
		other StringSet
		want  StringSet
	}{
		{
			name:  "both nil",
			set:   nil,
			other: nil,
			want:  StringSet{},
		},
		{
			name:  "other nil",
			set:   StringSet{"a": {}},
			other: nil,
			want:  StringSet{"a": {}},
		},
		{
			name:  "same elements",
			set:   StringSet{"a": {}},
			other: StringSet{"a": {}},
			want:  StringSet{},
		},
		{
			name:  "b not in other",
			set:   StringSet{"a": {}, "b": {}},
			other: StringSet{"a": {}},
			want:  StringSet{"b": {}},
		},
		{
			name:  "b not in set",
			set:   StringSet{"a": {}},
			other: StringSet{"a": {}, "b": {}},
			want:  StringSet{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.set.Diff(tt.other); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Diff() = %v, want %v", got, tt.want)
			}
		})
	}
}
