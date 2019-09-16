// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package validation

import (
	"reflect"
	"testing"

	estype "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/validation"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
)

func es(v string) *estype.Elasticsearch {
	return &estype.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "foo",
		},
		Spec: estype.ElasticsearchSpec{Version: v},
	}
}

func TestValidation_noDowngrades(t *testing.T) {
	assert.NoError(t, estype.SchemeBuilder.AddToScheme(scheme.Scheme))
	type args struct {
		toValidate estype.Elasticsearch
	}
	tests := []struct {
		name    string
		args    args
		current *estype.Elasticsearch
		want    validation.Result
		wantErr bool
	}{
		{
			name: "no validation on create",
			args: args{
				toValidate: *es("6.8.0"),
			},
			current: nil,
			want:    validation.OK,
		},
		{
			name: "prevent downgrade",
			args: args{
				toValidate: *es("1.0.0"),
			},
			current: es("2.0.0"),
			want:    validation.Result{Allowed: false, Reason: noDowngradesMsg},
		},
		{
			name: "allow upgrades",
			args: args{
				toValidate: *es("1.2.0"),
			},
			current: es("1.0.0"),
			want:    validation.OK,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, err := NewValidationContext(tt.current, tt.args.toValidate)
			require.NoError(t, err)
			got := noDowngrades(*ctx)
			if got.Allowed != tt.want.Allowed || got.Reason != tt.want.Reason || got.Error != nil != tt.wantErr {
				t.Errorf("ValidationHandler.noDowngrades() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func Test_validUpgradePath(t *testing.T) {
	type args struct {
		current  *estype.Elasticsearch
		proposed estype.Elasticsearch
	}
	tests := []struct {
		name string
		args args
		want validation.Result
	}{
		{
			name: "new cluster validation.OK",
			args: args{
				current:  nil,
				proposed: *es("1.0.0"),
			},
			want: validation.OK,
		},
		{
			name: "unsupported version FAIL",
			args: args{
				current:  es("1.0.0"),
				proposed: *es("2.0.0"),
			},
			want: validation.Result{Allowed: false, Reason: "unsupported version: 2.0.0"},
		},
		{
			name: "too old FAIL",
			args: args{
				current:  es("6.5.0"),
				proposed: *es("7.0.0"),
			},
			want: validation.Result{Allowed: false, Reason: "unsupported version upgrade from 6.5.0 to 7.0.0"},
		},
		{
			name: "too new FAIL",
			args: args{
				current:  es("7.0.0"),
				proposed: *es("6.5.0"),
			},
			want: validation.Result{Allowed: false, Reason: "unsupported version upgrade from 7.0.0 to 6.5.0"},
		},
		{
			name: "in range validation.OK",
			args: args{
				current:  es("6.8.0"),
				proposed: *es("7.1.0"),
			},
			want: validation.OK,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, err := NewValidationContext(tt.args.current, tt.args.proposed)
			require.NoError(t, err)
			if got := validUpgradePath(*ctx); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("validUpgradePath() = %v, want %v", got, tt.want)
			}
		})
	}
}
