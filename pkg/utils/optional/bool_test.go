// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package optional_test

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/optional"
)

func TestBool_UnmarshalJSON(t *testing.T) {
	type testStruct struct {
		OptionalBoolean *optional.Bool `json:"optional_boolean,omitempty"`
	}
	type args struct {
		data []byte
	}
	tests := []struct {
		name      string
		args      args
		wantErr   bool
		want      *optional.Bool
		wantIsSet bool
		wantTrue  bool
		wantFalse bool
	}{
		{
			name: "true",
			args: args{
				data: []byte(`{ "optional_boolean": true }`),
			},
			want:      optional.NewBool(true),
			wantIsSet: true,
			wantTrue:  true,
			wantFalse: false,
		},
		{
			name: "false",
			args: args{
				data: []byte(`{ "optional_boolean": false }`),
			},
			want:      optional.NewBool(false),
			wantIsSet: true,
			wantTrue:  false,
			wantFalse: true,
		},
		{
			name: "empty",
			args: args{
				data: []byte(`{ }`),
			},
			want:      nil,
			wantIsSet: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testStruct := testStruct{}
			if err := json.Unmarshal(tt.args.data, &testStruct); (err != nil) != tt.wantErr {
				t.Errorf("Bool.UnmarshalJSON() error = %v, wantErr %v", err, tt.wantErr)
			}
			assert.Equal(t, tt.want, testStruct.OptionalBoolean)
			assert.Equal(t, tt.wantIsSet, testStruct.OptionalBoolean.IsSet())
			if tt.wantIsSet {
				assert.Equal(t, tt.wantTrue, testStruct.OptionalBoolean.IsTrue())
				assert.Equal(t, tt.wantFalse, testStruct.OptionalBoolean.IsFalse())
			}
		})
	}
}

func TestBool_MarshalJSON(t *testing.T) {
	type args struct {
		optionalBool *optional.Bool
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
		want    []byte
	}{
		{
			name: "true",
			args: args{
				optionalBool: optional.NewBool(true),
			},
			want: []byte("true"),
		},
		{
			name: "empty",
			args: args{
				optionalBool: nil,
			},
			want: []byte("null"),
		},
		{
			name: "false",
			args: args{
				optionalBool: optional.NewBool(false),
			},
			want: []byte("false"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := json.Marshal(tt.args.optionalBool)
			if (err != nil) != tt.wantErr {
				t.Errorf("Bool.MarshalJSON() error = %v, wantErr %v", err, tt.wantErr)
			}
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestBool_Or(t *testing.T) {
	type fields struct {
		receiver *optional.Bool
	}
	type args struct {
		other *optional.Bool
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   *optional.Bool
	}{
		{
			fields: fields{receiver: nil},
			args:   args{other: nil},
			want:   nil,
		},
		{
			fields: fields{receiver: optional.NewBool(true)},
			args:   args{other: nil},
			want:   optional.NewBool(true),
		},
		{
			fields: fields{receiver: optional.NewBool(true)},
			args:   args{other: optional.NewBool(false)},
			want:   optional.NewBool(true),
		},
		{
			fields: fields{receiver: nil},
			args:   args{other: optional.NewBool(false)},
			want:   optional.NewBool(false),
		},
		{
			fields: fields{receiver: nil},
			args:   args{other: optional.NewBool(true)},
			want:   optional.NewBool(true),
		},
		{
			fields: fields{receiver: optional.NewBool(false)},
			args:   args{other: optional.NewBool(true)},
			want:   optional.NewBool(true),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.fields.receiver.Or(tt.args.other); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Bool.Or() = %v, want %v", got, tt.want)
			}
		})
	}
}
