// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package filerealm

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_usersRoles_mergeWith(t *testing.T) {
	tests := []struct {
		name  string
		r     usersRoles
		other usersRoles
		want  usersRoles
	}{
		{
			name:  "merge with nil",
			r:     usersRoles{"role1": []string{"user1"}, "role2": []string{"user1", "user2"}},
			other: nil,
			want:  usersRoles{"role1": []string{"user1"}, "role2": []string{"user1", "user2"}},
		},
		{
			name:  "merge from nil",
			r:     nil,
			other: usersRoles{"role1": []string{"user1"}, "role2": []string{"user1", "user2"}},
			want:  usersRoles{"role1": []string{"user1"}, "role2": []string{"user1", "user2"}},
		},
		{
			name:  "merge distinct roles",
			r:     usersRoles{"role1": []string{"user1"}, "role2": []string{"user1", "user2"}},
			other: usersRoles{"role3": []string{"user1"}, "role4": nil},
			want:  usersRoles{"role1": []string{"user1"}, "role2": []string{"user1", "user2"}, "role3": []string{"user1"}, "role4": nil},
		},
		{
			name:  "merge duplicate roles with different users",
			r:     usersRoles{"role1": []string{"user1"}, "role2": []string{"user1", "user2"}},
			other: usersRoles{"role1": []string{"user2"}, "role2": []string{"user1"}},
			want:  usersRoles{"role1": []string{"user1", "user2"}, "role2": []string{"user1", "user2"}},
		},
		{
			name:  "merged users should be sorted",
			r:     usersRoles{"role1": []string{"user1", "user3", "user5"}},
			other: usersRoles{"role1": []string{"user1", "user2", "user4"}},
			want:  usersRoles{"role1": []string{"user1", "user2", "user3", "user4", "user5"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.r.mergeWith(tt.other); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("mergeWith() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_usersRoles_fileBytes(t *testing.T) {
	tests := []struct {
		name string
		r    usersRoles
		want []byte
	}{
		{
			name: "nil case",
			r:    nil,
			want: []byte("\n"), // final empty line is always added
		},
		{
			name: "empty case",
			r:    usersRoles{},
			want: []byte("\n"), // final empty line is always added
		},
		{
			name: "standard case",
			r:    usersRoles{"role1": []string{"user1", "user2"}, "role2": []string{"user1"}},
			want: []byte("role1:user1,user2\nrole2:user1\n"),
		},
		{
			name: "role with no user",
			r:    usersRoles{"role1": nil},
			want: []byte("role1:\n"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, tt.r.fileBytes())
		})
	}
}

func Test_parseUsersRoles(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want usersRoles
	}{
		{
			name: "nil data",
			data: nil,
			want: usersRoles{},
		},
		{
			name: "standard case",
			data: []byte("role1:user1,user2\nrole2:user1\n"),
			want: usersRoles{"role1": []string{"user1", "user2"}, "role2": []string{"user1"}},
		},
		{
			name: "users should be sorted in the internal representation",
			data: []byte("role1:user3,user1,user2\nrole2:user2,user1\n"),
			want: usersRoles{"role1": []string{"user1", "user2", "user3"}, "role2": []string{"user1", "user2"}},
		},
		{
			name: "role with no user",
			data: []byte("role1:\n"),
			want: usersRoles{"role1": nil},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseUsersRoles(tt.data)
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}
