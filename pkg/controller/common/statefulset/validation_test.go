// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package statefulset

import (
	"context"
	"fmt"
	"net/http"
	"reflect"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
)

func Test_ValidatePodTemplate(t *testing.T) {
	es := esv1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns",
			Name:      "es",
		},
	}
	ssetSample := appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: es.Namespace,
			Name:      "sset",
		},
	}
	type args struct {
		c      k8s.Client
		parent metav1.Object
		sset   appsv1.StatefulSet
	}
	tests := []struct {
		name    string
		args    args
		wantErr error
	}{
		{
			name: "Client returns a validation error",
			args: args{
				c: k8s.NewFailingClient(&errors.StatusError{
					ErrStatus: metav1.Status{
						Status:  metav1.StatusFailure,
						Message: "Pod dummy is invalid",
						Reason:  metav1.StatusReasonInvalid,
						Details: &metav1.StatusDetails{
							Name: "es-sample-dummy-vd4kq",
							Kind: "Pod",
							Causes: []metav1.StatusCause{
								{
									Type:    metav1.CauseTypeFieldValueInvalid,
									Message: "Invalid value: \"6\": must be less than or equl to cpu limit",
									Field:   "spec.containers[0].resources.requests",
								},
							},
						},
					},
				}),
				parent: &es,
				sset:   ssetSample,
			},
			wantErr: &PodTemplateError{
				Parent:      &es,
				StatefulSet: ssetSample,
				Causes: []metav1.StatusCause{
					{
						Type:    metav1.CauseTypeFieldValueInvalid,
						Message: "Invalid value: \"6\": must be less than or equl to cpu limit",
						Field:   "spec.containers[0].resources.requests",
					},
				},
			},
		},
		{
			name: "Skip BadRequest error from Openshift 3.11 or K8S 1.12",
			args: args{
				c:      k8s.NewFailingClient(errors.NewBadRequest("not supported yet")),
				parent: &es,
				sset:   ssetSample,
			},
			wantErr: nil,
		},
		{
			name: "Client returns a Kubernetes error which is not a validation error",
			args: args{
				c: k8s.NewFailingClient(&errors.StatusError{
					ErrStatus: metav1.Status{
						Status:  metav1.StatusFailure,
						Message: "Pod dummy is invalid",
						Reason:  metav1.StatusReasonInternalError,
						Code:    http.StatusInternalServerError,
						Details: &metav1.StatusDetails{
							Name: "es-sample-dummy-vd4kq",
							Kind: "Pod",
							Causes: []metav1.StatusCause{
								{
									Type:    metav1.CauseTypeFieldValueInvalid,
									Message: "Internal error occurred: admission webhook \"pod-ready.common-webhooks.networking.gke.io\" does not support dry run",
									Field:   "spec.containers[0].resources.requests",
								},
							},
						},
					},
				}),
				parent: &es,
				sset:   ssetSample,
			},
			wantErr: nil, // Do not block reconciliation loop on unknown error
		},
		{
			name: "Return non StatusError as is",
			args: args{
				c:      k8s.NewFailingClient(fmt.Errorf("foo")),
				parent: &es,
				sset:   ssetSample,
			},
			wantErr: fmt.Errorf("foo"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePodTemplate(context.Background(), tt.args.c, tt.args.parent, tt.args.sset)
			if !reflect.DeepEqual(err, tt.wantErr) {
				t.Errorf("validatePodTemplate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
