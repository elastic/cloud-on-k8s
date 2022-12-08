package helm

import (
	"reflect"
	"testing"
)

func Test_separateChartsWithDependencies(t *testing.T) {
	tests := []struct {
		name         string
		charts       []chart
		wantNoDeps   []chart
		wantWithDeps []chart
	}{
		{
			name: "happy path",
			charts: []chart{
				{Name: "eck-elasticsearch"},
				{Name: "eck-kibana"},
				{Name: "eck-agent"},
				{Name: "eck-beat"},
				{Name: "eck-stack", Dependencies: []dependency{
					{Name: "eck-elasticsearch"},
					{Name: "eck-kibana"},
					{Name: "eck-agent"},
					{Name: "eck-beat"},
				}},
			},
			wantNoDeps: []chart{
				{Name: "eck-elasticsearch"},
				{Name: "eck-kibana"},
				{Name: "eck-agent"},
				{Name: "eck-beat"},
			},
			wantWithDeps: []chart{
				{Name: "eck-stack", Dependencies: []dependency{
					{Name: "eck-elasticsearch"},
					{Name: "eck-kibana"},
					{Name: "eck-agent"},
					{Name: "eck-beat"},
				}},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotNoDeps, gotWithDeps := separateChartsWithDependencies(tt.charts)
			if !reflect.DeepEqual(gotNoDeps, tt.wantNoDeps) {
				t.Errorf("separateChartsWithDependencies() gotNoDeps = %v, want %v", gotNoDeps, tt.wantNoDeps)
			}
			if !reflect.DeepEqual(gotWithDeps, tt.wantWithDeps) {
				t.Errorf("separateChartsWithDependencies() gotWithDeps = %v, want %v", gotWithDeps, tt.wantWithDeps)
			}
		})
	}
}
