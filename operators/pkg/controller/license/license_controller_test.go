package license

import (
	"reflect"
	"testing"
	"time"

	"github.com/elastic/k8s-operators/operators/pkg/utils/test"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func Test_nextReconcileRelativeTo(t *testing.T) {
	now := test.MustParseTime("2019-02-01")
	type args struct {
		expiry time.Time
		safety time.Duration
	}
	tests := []struct {
		name string
		args args
		want reconcile.Result
	}{
		{
			name: "remaining time too short: requeue immediately ",
			args: args{
				expiry: test.MustParseTime("2019-02-02"),
				safety: 30 * 24 * time.Hour,
			},
			want: reconcile.Result{Requeue: true},
		},
		{
			name: "default: requeue after expiry - safety/2 ",
			args: args{
				expiry: test.MustParseTime("2019-02-03"),
				safety: 48 * time.Hour,
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
