// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package v1beta1

import (
	// "reflect"
	"testing"
	"errors"
	// controllerscheme "github.com/elastic/cloud-on-k8s/pkg/controller/common/scheme"
	
	// "github.com/elastic/cloud-on-k8s/pkg/controller/common/validation"
	// "github.com/stretchr/testify/assert"
	// "github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	// "k8s.io/client-go/kubernetes/scheme"
)

// TODO sabo merge changes in https://github.com/elastic/cloud-on-k8s/commit/dcee46a39bd798413f9e5b38e9f2a9bfc7ea5881#diff-5525b0a5eb4a1f7cd0e1a50a288c54e5

func es(v string) *Elasticsearch {
	return &Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "foo",
		},
		Spec: ElasticsearchSpec{Version: v},
	}
}

func TestValidation_noDowngrades(t *testing.T) {
	// assert.NoError(t, estype.SchemeBuilder.AddToScheme(scheme.Scheme))
	// todo sabo is this a circular import?
	// require.NoError(controllerscheme.SetupScheme())
	type args struct {
		toValidate *Elasticsearch
	}
	tests := []struct {
		name    string
		args    args
		current *Elasticsearch
		want    error
		wantErr bool
	}{
		{
			name: "no validation on create",
			args: args{
				toValidate: es("6.8.0"),
			},
			current: nil,
			want:    nil,
		},
		{
			name: "prevent downgrade",
			args: args{
				toValidate: es("1.0.0"),
			},
			current: es("2.0.0"),
			// want:    validation.Result{Allowed: false, Reason: noDowngradesMsg},
			want: errors.New(""),
		},
		{
			name: "allow upgrades",
			args: args{
				toValidate: es("1.2.0"),
			},
			current: es("1.0.0"),
			want:    nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// ctx, err := NewValidationContext(tt.current, tt.args.toValidate)
			// require.NoError(t, err)
			// got := noDowngrades(tt.current, tt.args.toValidate)
			// todo sabo fix
			// if got.Allowed != tt.want.Allowed || got.Reason != tt.want.Reason || got.Error != nil != tt.wantErr {
			// 	t.Errorf("ValidationHandler.noDowngrades() = %+v, want %+v", got, tt.want)
			// }
		})
	}
}

func Test_validUpgradePath(t *testing.T) {
	type args struct {
		current  *Elasticsearch
		proposed Elasticsearch
	}
	tests := []struct {
		name string
		args args
		want error
	}{
		{
			name: "new cluster validation.OK",
			args: args{
				current:  nil,
				proposed: *es("1.0.0"),
			},
			want: nil,
		},
		{
			name: "unsupported version FAIL",
			args: args{
				current:  es("1.0.0"),
				proposed: *es("2.0.0"),
			},
			// want: validation.Result{Allowed: false, Reason: "unsupported version: 2.0.0"},
			want: errors.New(""),
		},
		{
			name: "too old FAIL",
			args: args{
				current:  es("6.5.0"),
				proposed: *es("7.0.0"),
			},
			// want: validation.Result{Allowed: false, Reason: "unsupported version upgrade from 6.5.0 to 7.0.0"},
			want: errors.New(""),
		},
		{
			name: "too new FAIL",
			args: args{
				current:  es("7.0.0"),
				proposed: *es("6.5.0"),
			},
			// want: validation.Result{Allowed: false, Reason: "unsupported version upgrade from 7.0.0 to 6.5.0"},
			want: errors.New(""),
		},
		{
			name: "in range validation.OK",
			args: args{
				current:  es("6.8.0"),
				proposed: *es("7.1.0"),
			},
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// ctx, err := NewValidationContext(tt.args.current, tt.args.proposed)
			// require.NoError(t, err)
			// TODO sabo make these just all ptrs?
			// got := validUpgradePath(tt.args.current, &tt.args.proposed)
			// if got := validUpgradePath(*ctx); !reflect.DeepEqual(got, tt.want) {
			// 	t.Errorf("validUpgradePath() = %v, want %v", got, tt.want)
			// }
		})
	}
}
