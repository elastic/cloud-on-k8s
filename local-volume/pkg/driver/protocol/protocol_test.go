package protocol

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMountOptions_AsStrMap(t *testing.T) {
	type fields struct {
		SizeBytes int64
	}
	tests := []struct {
		name   string
		fields fields
		want   map[string]string
	}{
		{
			name:   "Correct formatting",
			fields: fields{SizeBytes: 1024},
			want:   map[string]string{"sizeBytes": "1024"},
		},
		{
			name:   "Correct formatting big",
			fields: fields{SizeBytes: 1024000},
			want:   map[string]string{"sizeBytes": "1024000"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := MountOptions{
				SizeBytes: tt.fields.SizeBytes,
			}
			got := m.AsStrMap()
			assert.Equal(t, tt.want, got)
		})
	}
}
