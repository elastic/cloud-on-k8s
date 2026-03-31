// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package hints

import (
	"reflect"
	"testing"
)

func TestNewFromAnnotations(t *testing.T) {
	type args struct {
		ann map[string]string
	}
	tests := []struct {
		name    string
		args    args
		want    OrchestrationsHints
		wantErr bool
	}{
		{
			name: "OK, valid annotations",
			args: args{
				ann: map[string]string{
					OrchestrationsHintsAnnotation: `{"no_transient_settings": true}`,
				},
			},
			want:    OrchestrationsHints{NoTransientSettings: true},
			wantErr: false,
		},
		{
			name:    "OK, no annotation",
			args:    args{},
			want:    OrchestrationsHints{NoTransientSettings: false},
			wantErr: false,
		},
		{
			name: "NOK, invalid annotation",
			args: args{
				ann: map[string]string{OrchestrationsHintsAnnotation: `not json`},
			},
			want:    OrchestrationsHints{NoTransientSettings: false},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewFromAnnotations(tt.args.ann)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewFromAnnotations() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NewFromAnnotations() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestOrchestrationsHints_Merge(t *testing.T) {
	type fields struct {
		NoTransientSettings bool
	}
	type args struct {
		other OrchestrationsHints
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   OrchestrationsHints
	}{
		{
			name: "f|f",
			fields: fields{
				NoTransientSettings: false,
			},
			args: args{
				other: OrchestrationsHints{NoTransientSettings: false},
			},
			want: OrchestrationsHints{NoTransientSettings: false},
		},
		{
			name: "t|f",
			fields: fields{
				NoTransientSettings: true,
			},
			args: args{
				other: OrchestrationsHints{NoTransientSettings: false},
			},
			want: OrchestrationsHints{NoTransientSettings: true},
		}, {
			name: "f|t",
			fields: fields{
				NoTransientSettings: false,
			},
			args: args{
				other: OrchestrationsHints{NoTransientSettings: true},
			},
			want: OrchestrationsHints{NoTransientSettings: true},
		},
		{
			name: "t|t",
			fields: fields{
				NoTransientSettings: true,
			},
			args: args{
				other: OrchestrationsHints{NoTransientSettings: true},
			},
			want: OrchestrationsHints{NoTransientSettings: true},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			oh := OrchestrationsHints{
				NoTransientSettings: tt.fields.NoTransientSettings,
			}
			if got := oh.Merge(tt.args.other); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Merge() = %v, want %v", got, tt.want)
			}
		})
	}
}
