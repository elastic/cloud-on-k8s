// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package filerealm

import (
	"reflect"
	"testing"
)

func Test_usersPasswordHashes_mergeWith(t *testing.T) {
	tests := []struct {
		name  string
		u     usersPasswordHashes
		other usersPasswordHashes
		want  usersPasswordHashes
	}{
		{
			name:  "merge with nil",
			u:     usersPasswordHashes{"user1": []byte("hash1"), "user2": []byte("hash2")},
			other: nil,
			want:  usersPasswordHashes{"user1": []byte("hash1"), "user2": []byte("hash2")},
		},
		{
			name:  "merge from nil",
			u:     nil,
			other: usersPasswordHashes{"user1": []byte("hash1"), "user2": []byte("hash2")},
			want:  usersPasswordHashes{"user1": []byte("hash1"), "user2": []byte("hash2")},
		},
		{
			name:  "merge distinct users",
			u:     usersPasswordHashes{"user1": []byte("hash1"), "user2": []byte("hash2")},
			other: usersPasswordHashes{"user3": []byte("hash3"), "user4": []byte("hash4")},
			want:  usersPasswordHashes{"user1": []byte("hash1"), "user2": []byte("hash2"), "user3": []byte("hash3"), "user4": []byte("hash4")},
		},
		{
			name:  "merge duplicate users: gives priority to other",
			u:     usersPasswordHashes{"user1": []byte("hash1"), "user2": []byte("hash2")},
			other: usersPasswordHashes{"user1": []byte("anotherhash1"), "user3": []byte("hash3")},
			want:  usersPasswordHashes{"user1": []byte("anotherhash1"), "user2": []byte("hash2"), "user3": []byte("hash3")},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.u.mergeWith(tt.other); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("mergeWith() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_usersPasswordHashes_fileBytes(t *testing.T) {
	tests := []struct {
		name string
		u    usersPasswordHashes
		want []byte
	}{
		{
			name: "nil case",
			u:    nil,
			want: []byte("\n"), // final empty line is always added
		},
		{
			name: "empty case",
			u:    nil,
			want: []byte("\n"), // final empty line is always added
		},
		{
			name: "standard case",
			u:    usersPasswordHashes{"user1": []byte("hash1"), "user2": []byte("hash2")},
			want: []byte("user1:hash1\nuser2:hash2\n"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.u.fileBytes(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("fileBytes() = %v, want %v", got, tt.want)
			}
		})
	}
}
