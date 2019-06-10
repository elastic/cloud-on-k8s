// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package validation

import (
	"reflect"
	"testing"

	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/license"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/validation"
	v1 "k8s.io/api/core/v1"
	v12 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Test_eulaAccepted(t *testing.T) {
	type args struct {
		ctx Context
	}
	tests := []struct {
		name string
		args args
		want validation.Result
	}{
		{
			name: "No Eula set FAIL",
			args: args{
				ctx: Context{
					Proposed: v1.Secret{
						ObjectMeta: v12.ObjectMeta{
							Labels: map[string]string{
								common.TypeLabelName: license.Type,
							},
						},
					},
				},
			},
			want: validation.Result{Allowed: false, Reason: EULAValidationMsg},
		},
		{
			name: "Other secret OK",
			args: args{
				ctx: Context{
					Proposed: v1.Secret{},
				},
			},
			want: validation.OK,
		},
		{
			name: "Eula accepted OK",
			args: args{
				ctx: Context{
					Proposed: v1.Secret{
						ObjectMeta: v12.ObjectMeta{
							Labels: map[string]string{
								common.TypeLabelName: license.Type,
							},
							Annotations: map[string]string{
								license.EULAAnnotation: license.EULAAcceptedValue,
							},
						},
					},
				},
			},
			want: validation.OK,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := eulaAccepted(tt.args.ctx); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("eulaAccepted() = %v, want %v", got, tt.want)
			}
		})
	}
}
