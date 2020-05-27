// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package association

import (
	"reflect"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/record"

	apmv1 "github.com/elastic/cloud-on-k8s/pkg/apis/apm/v1"
	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/utils/rbac"
)

type fakeAccessReviewer struct {
	allowed bool
	err     error
}

func (f *fakeAccessReviewer) AccessAllowed(_ string, _ string, _ runtime.Object) (bool, error) {
	return f.allowed, f.err
}

type fakeUnbinder struct {
	called bool
}

func (f *fakeUnbinder) Unbind(_ commonv1.Association) error {
	f.called = true
	return nil
}

var (
	fetchEvent = func(recorder *record.FakeRecorder) string {
		select {
		case event := <-recorder.Events:
			return event
		default:
			return ""
		}
	}
)

func TestCheckAndUnbind(t *testing.T) {
	apmServer := &apmv1.ApmEsAssociation{
		ApmServer: &apmv1.ApmServer{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "apm-server-sample",
				Namespace: "apmserver-ns",
			},
			Spec: apmv1.ApmServerSpec{},
		},
	}

	es := &esv1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "es-sample",
			Namespace: "es-ns",
		},
	}

	type args struct {
		accessReviewer rbac.AccessReviewer
		associated     commonv1.Association
		object         runtime.Object
		unbinder       fakeUnbinder
		recorder       *record.FakeRecorder
	}
	tests := []struct {
		name                                             string
		args                                             args
		want, wantErr, wantEvent, wantFakeUnbinderCalled bool
	}{
		{
			name: "Association not allowed, ensure unbinder is called",
			args: args{
				associated: apmServer,
				object:     es,
				accessReviewer: &fakeAccessReviewer{
					allowed: false,
				},
				unbinder: fakeUnbinder{},
				recorder: record.NewFakeRecorder(10),
			},
			wantFakeUnbinderCalled: true,
			wantEvent:              true,
			want:                   false,
		},
		{
			name: "Association allowed, ensure unbinder is not called",
			args: args{
				associated: apmServer,
				object:     es,
				accessReviewer: &fakeAccessReviewer{
					allowed: true,
				},
				unbinder: fakeUnbinder{},
				recorder: record.NewFakeRecorder(10),
			},
			wantFakeUnbinderCalled: false,
			wantEvent:              false,
			want:                   true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := CheckAndUnbind(tt.args.accessReviewer, tt.args.associated, tt.args.object, &tt.args.unbinder, tt.args.recorder)
			if (err != nil) != tt.wantErr {
				t.Errorf("CheckAndUnbind() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("CheckAndUnbind() = %v, want %v", got, tt.want)
			}
			if tt.args.unbinder.called != tt.wantFakeUnbinderCalled {
				t.Errorf("fakeUnbinder.called = %v, want %v", tt.args.unbinder.called, tt.wantFakeUnbinderCalled)
			}
			event := fetchEvent(tt.args.recorder)
			if len(event) > 0 != tt.wantEvent {
				t.Errorf("emitted event = %v, want %v", len(event) > 0, tt.wantEvent)
			}

		})
	}
}

func TestNextReconciliation(t *testing.T) {
	type args struct {
		accessReviewer rbac.AccessReviewer
	}
	tests := []struct {
		name                string
		args                args
		wantNonZeroDuration bool
	}{
		{
			name:                "Schedule a requeue if there's some access control",
			args:                args{accessReviewer: rbac.NewSubjectAccessReviewer(fake.NewSimpleClientset())},
			wantNonZeroDuration: true,
		},
		{
			name:                "No requeue if there is no access control",
			args:                args{accessReviewer: rbac.NewPermissiveAccessReviewer()},
			wantNonZeroDuration: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := RequeueRbacCheck(tt.args.accessReviewer); !reflect.DeepEqual(got.RequeueAfter > 0, tt.wantNonZeroDuration) {
				t.Errorf("NextReconciliation() = %v, wantNonZeroDuration: %v", got, tt.wantNonZeroDuration)
			}
		})
	}
}
