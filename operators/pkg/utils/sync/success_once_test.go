package sync

import (
	"testing"

	"github.com/pkg/errors"
)

type one int

func (o *one) Increment() {
	*o++
}

func TestSuccessOnce_Do(t *testing.T) {
	type args func(*one) []func() error
	tests := []struct {
		name  string
		args  args
		panic bool
	}{
		{
			name: "success just once",
			args: func(o *one) []func() error {
				return []func() error{
					func() error {
						o.Increment()
						return nil
					},
					func() error {
						o.Increment()
						return nil
					},
					func() error {
						o.Increment()
						return nil
					},
				}
			},
		},
		{
			name: "failure then success",
			args: func(o *one) []func() error {
				return []func() error{
					func() error {
						return errors.New("transient")
					},
					func() error {
						o.Increment()
						return nil
					},
					func() error {
						t.Fatalf("SuccessOnce.Do called again")
						return nil
					},
				}
			},
		},
		{
			name: "panic counts as error (assuming recover)",
			args: func(o *one) []func() error {
				return []func() error{
					func() (e error) {
						panic("failed")
					},
					func() error {
						o.Increment()
						return nil
					},
					func() error {
						t.Fatalf("SuccessOnce.Do called again")
						return nil
					},
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var once SuccessOnce
			var o one
			for _, fn := range tt.args(&o) {
				func() {
					defer func() {
						recover()
					}()
					_ = once.Do(fn)
				}()
			}
			if o != 1 {
				t.Errorf("once invariant violated: %d is not 1", o)
			}

		})
	}
}
