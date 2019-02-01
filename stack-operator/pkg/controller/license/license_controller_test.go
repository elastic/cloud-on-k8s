package license

import (
	"reflect"
	"testing"
	"time"

	"github.com/elastic/stack-operators/stack-operator/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/stack-operators/stack-operator/pkg/utils/test"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func Test_nextReconcileRelativeTo(t *testing.T) {
	now := test.Time("2019-02-01")
	type args struct {
		expiry time.Time
		safety v1alpha1.SafetyMargin
	}
	tests := []struct {
		name string
		args args
		want reconcile.Result
	}{
		{
			name: "remaining time too short: requeue immediately ",
			args: args{
				expiry: test.Time("2019-02-02"),
				safety: v1alpha1.SafetyMargin{
					ValidFor: 30 * 24 * time.Hour,
				},
			},
			want: reconcile.Result{Requeue: true},
		},
		{
			name: "default: requeue after expiry - safety/2 ",
			args: args{
				expiry: test.Time("2019-02-03"),
				safety: v1alpha1.SafetyMargin{
					ValidFor: 48 * time.Hour,
				},
			},
			want: reconcile.Result{RequeueAfter: 24 * time.Hour},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := nextReconcileRelativeTo(now, tt.args.expiry, tt.args.safety); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("nextReconcileRelativeTo() = %v, want %v", got, tt.want)
			}
		})
	}
}
