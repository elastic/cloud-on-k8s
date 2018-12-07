package lvm

import (
	"reflect"
	"testing"

	"github.com/elastic/stack-operators/local-volume/pkg/driver/flex"
)

func TestDriver_Init(t *testing.T) {
	type fields struct {
		options Options
	}
	tests := []struct {
		name   string
		fields fields
		want   flex.Response
	}{
		{
			want: flex.Response{
				Status:  flex.StatusSuccess,
				Message: "driver is available",
				Capabilities: flex.Capabilities{
					Attach: false, // only implement mount and unmount
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := &Driver{
				options: tt.fields.options,
			}
			if got := d.Init(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Driver.Init() = %v, want %v", got, tt.want)
			}
		})
	}
}
