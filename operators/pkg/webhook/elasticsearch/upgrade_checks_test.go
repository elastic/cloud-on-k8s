// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package elasticsearch

import (
	"context"
	"testing"

	estype "github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestValidation_canUpgrade(t *testing.T) {
	assert.NoError(t, estype.SchemeBuilder.AddToScheme(scheme.Scheme))
	type args struct {
		toValidate estype.Elasticsearch
	}
	tests := []struct {
		name           string
		args           args
		initialObjects []runtime.Object
		want           ValidationResult
		wantErr        bool
	}{
		{
			name: "no validation on create",
			args: args{
				toValidate: estype.Elasticsearch{},
			},
			initialObjects: nil,
			want:           ValidationResult{Allowed: true},
		},
		{
			name: "prevent major upgrades",
			args: args{
				toValidate: estype.Elasticsearch{
					Spec: estype.ElasticsearchSpec{Version: "1.0.0"},
				},
			},
			initialObjects: []runtime.Object{&estype.Elasticsearch{Spec: estype.ElasticsearchSpec{Version: "2.0.0"}}},
			want:           ValidationResult{Allowed: false, Reason: notMajorVersionUpgradeMsg},
		},
		{
			name: "allow minor upgrades",
			args: args{
				toValidate: estype.Elasticsearch{
					Spec: estype.ElasticsearchSpec{Version: "1.0.0"},
				},
			},
			initialObjects: []runtime.Object{&estype.Elasticsearch{Spec: estype.ElasticsearchSpec{Version: "1.2.0"}}},
			want:           ValidationResult{Allowed: true},
		},
		{
			name: "handle corrupt version",
			args: args{
				toValidate: estype.Elasticsearch{
					Spec: estype.ElasticsearchSpec{Version: "garbage"},
				},
			},
			initialObjects: []runtime.Object{&estype.Elasticsearch{Spec: estype.ElasticsearchSpec{Version: "1.2.0"}}},
			want: ValidationResult{
				Allowed: false,
				Reason:  parseVersionErrMsg,
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := fake.NewFakeClient(tt.initialObjects...)
			v := &Validation{
				client: client,
			}
			got := v.canUpgrade(context.TODO(), tt.args.toValidate)
			if got.Allowed != tt.want.Allowed || got.Reason != tt.want.Reason || got.Error != nil != tt.wantErr {
				t.Errorf("Validation.canUpgrade() = %+v, want %+v", got, tt.want)
			}
		})
	}
}
