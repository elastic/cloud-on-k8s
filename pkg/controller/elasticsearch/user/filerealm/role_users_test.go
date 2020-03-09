// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package filerealm

import (
	"reflect"
	"testing"
)

func Test_roleUsersMapping_mergeWith(t *testing.T) {
	tests := []struct {
		name  string
		r     roleUsersMapping
		other roleUsersMapping
		want  roleUsersMapping
	}{
		{
			name:  "merge with nil",
			r:     roleUsersMapping{"role1": []string{"user1"}, "role2": []string{"user1", "user2"}},
			other: nil,
			want:  roleUsersMapping{"role1": []string{"user1"}, "role2": []string{"user1", "user2"}},
		},
		{
			name:  "merge from nil",
			r:     nil,
			other: roleUsersMapping{"role1": []string{"user1"}, "role2": []string{"user1", "user2"}},
			want:  roleUsersMapping{"role1": []string{"user1"}, "role2": []string{"user1", "user2"}},
		},
		{
			name:  "merge distinct roles",
			r:     roleUsersMapping{"role1": []string{"user1"}, "role2": []string{"user1", "user2"}},
			other: roleUsersMapping{"role3": []string{"user1"}, "role4": nil},
			want:  roleUsersMapping{"role1": []string{"user1"}, "role2": []string{"user1", "user2"}, "role3": []string{"user1"}, "role4": nil},
		},
		{
			name:  "merge duplicate roles with different users",
			r:     roleUsersMapping{"role1": []string{"user1"}, "role2": []string{"user1", "user2"}},
			other: roleUsersMapping{"role1": []string{"user2"}, "role2": []string{"user1"}},
			want:  roleUsersMapping{"role1": []string{"user1", "user2"}, "role2": []string{"user1", "user2"}},
		},
		{
			name:  "merged users should be sorted",
			r:     roleUsersMapping{"role1": []string{"user1", "user3", "user5"}},
			other: roleUsersMapping{"role1": []string{"user1", "user2", "user4"}},
			want:  roleUsersMapping{"role1": []string{"user1", "user2", "user3", "user4", "user5"}},
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

func Test_roleUsersMapping_fileBytes(t *testing.T) {
	tests := []struct {
		name string
		r    roleUsersMapping
		want []byte
	}{
		{
			name: "nil case",
			r:    nil,
			want: []byte("\n"), // final empty line is always added
		},
		{
			name: "empty case",
			r:    roleUsersMapping{},
			want: []byte("\n"), // final empty line is always added
		},
		{
			name: "standard case",
			r:    roleUsersMapping{"role1": []string{"user1", "user2"}, "role2": []string{"user1"}},
			want: []byte("role1:user1,user2\nrole2:user1\n"),
		},
		{
			name: "role with no user",
			r:    roleUsersMapping{"role1": nil},
			want: []byte("role1:\n"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.r.fileBytes(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("fileBytes() = %v, want %v", got, tt.want)
			}
		})
	}
}
