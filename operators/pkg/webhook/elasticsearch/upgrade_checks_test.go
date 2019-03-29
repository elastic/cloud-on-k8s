// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package elasticsearch

import (
	"reflect"
	"testing"

	estype "github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/stretchr/testify/assert"
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

func TestValidation_canUpgrade(t *testing.T) {
	assert.NoError(t, estype.SchemeBuilder.AddToScheme(scheme.Scheme))
	type args struct {
		toValidate estype.Elasticsearch
	}
	tests := []struct {
		name    string
		args    args
		current *estype.Elasticsearch
		want    ValidationResult
		wantErr bool
	}{
		{
			name: "no validation on create",
			args: args{
				toValidate: estype.Elasticsearch{},
			},
			current: nil,
			want:    OK,
		},
		{
			name: "prevent downgrade",
			args: args{
				toValidate: *es("1.0.0"),
			},
			current: es("2.0.0"),
			want:    ValidationResult{Allowed: false, Reason: noDowngradesMsg},
		},
		{
			name: "allow upgrades",
			args: args{
				toValidate: *es("1.2.0"),
			},
			current: es("1.0.0"),
			want:    OK,
		},
		{
			name: "handle corrupt version",
			args: args{
				toValidate: *es("garbage"),
			},
			current: es("1.2.0"),
			want: ValidationResult{
				Allowed: false,
				Reason:  parseVersionErrMsg,
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := noDowngrades(tt.current, &tt.args.toValidate)
			if got.Allowed != tt.want.Allowed || got.Reason != tt.want.Reason || got.Error != nil != tt.wantErr {
				t.Errorf("ValidationHandler.noDowngrades() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func Test_validUpgradePath(t *testing.T) {
	type args struct {
		current  *estype.Elasticsearch
		proposed *estype.Elasticsearch
	}
	tests := []struct {
		name string
		args args
		want ValidationResult
	}{
		{
			name: "new cluster OK",
			args: args{
				current:  nil,
				proposed: es("1.0.0"),
			},
			want: OK,
		},
		{
			name: "unsupported version FAIL",
			args: args{
				current:  es("1.0.0"),
				proposed: es("2.0.0"),
			},
			want: ValidationResult{Allowed: false, Reason: "unsupported version: 2.0.0"},
		},
		{
			name: "too old FAIL",
			args: args{
				current:  es("6.5.0"),
				proposed: es("7.0.0"),
			},
			want: ValidationResult{Allowed: false, Reason: "default/foo has version 6.5.0, which is older than the lowest supported version 6.7.0"},
		},
		{
			name: "too new FAIL",
			args: args{
				current:  es("7.0.0"),
				proposed: es("6.5.0"),
			},
			want: ValidationResult{Allowed: false, Reason: "default/foo has version 7.0.0, which is newer than the highest supported version 6.7.99"},
		},
		{
			name: "in range OK",
			args: args{
				current:  es("6.7.0"),
				proposed: es("7.0.0"),
			},
			want: OK,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := validUpgradePath(tt.args.current, tt.args.proposed); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("validUpgradePath() = %v, want %v", got, tt.want)
			}
		})
	}
}
