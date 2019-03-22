package settings

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFlatConfig_Sorted(t *testing.T) {
	tests := []struct {
		name string
		c    FlatConfig
		want []KeyValue
	}{
		{
			name: "no settings",
			c:    FlatConfig{},
			want: []KeyValue{},
		},
		{
			name: "settings should be sorted alphabetically",
			c: FlatConfig{
				"c": "aaa",
				"b": "aaa",
				"z": "aaa",
			},
			want: []KeyValue{
				{"b", "aaa"},
				{"c", "aaa"},
				{"z", "aaa"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.c.Sorted(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("FlatConfig.Sorted() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFlatConfig_Render(t *testing.T) {
	config := FlatConfig{
		"aaa":        "aa  a",
		"bbb":        "b  bb",
		"aab":        "a a a",
		"withquotes": "aa\"bb\"aa",
		"zz":         "zzz  z z z",
	}
	output := config.Render()
	expected := []byte(`# --- auto-generated ---
aaa: aa  a
aab: a a a
bbb: b  bb
withquotes: aa"bb"aa
zz: zzz  z z z
# --- end auto-generated ---
`)
	require.Equal(t, expected, output)
}

func TestFlatConfig_MergeWith(t *testing.T) {
	tests := []struct {
		name string
		c    FlatConfig
		c2   FlatConfig
		want FlatConfig
	}{
		{
			name: "both empty",
			c:    FlatConfig{},
			c2:   FlatConfig{},
			want: FlatConfig{},
		},
		{
			name: "both nil",
			c:    nil,
			c2:   nil,
			want: FlatConfig{},
		},
		{
			name: "c2 nil",
			c:    FlatConfig{"a": "b"},
			c2:   nil,
			want: FlatConfig{"a": "b"},
		},
		{
			name: "different values",
			c:    FlatConfig{"a": "b"},
			c2:   FlatConfig{"c": "d"},
			want: FlatConfig{"a": "b", "c": "d"},
		},
		{
			name: "conflict: c2 has precedence",
			c:    FlatConfig{"a": "b"},
			c2:   FlatConfig{"c": "d", "a": "e"},
			want: FlatConfig{"a": "e", "c": "d"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// c and c2 should remain unmodified
			lenC := len(tt.c)
			lenC2 := len(tt.c2)
			if got := tt.c.MergeWith(tt.c2); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("FlatConfig.MergeWith() = %v, want %v", got, tt.want)
			}
			require.Equal(t, lenC, len(tt.c))
			require.Equal(t, lenC2, len(tt.c2))
		})
	}
}

func TestParseConfig(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    FlatConfig
		wantErr bool
	}{
		{
			name:    "no input",
			input:   "",
			want:    FlatConfig{},
			wantErr: false,
		},
		{
			name:    "simple input",
			input:   "a: b\nc:d",
			want:    FlatConfig{"a": "b", "c": "d"},
			wantErr: false,
		},
		{
			name:    "trim whitespaces",
			input:   "      a: b   \n    c:d     ",
			want:    FlatConfig{"a": "b", "c": "d"},
			wantErr: false,
		},
		{
			name:    "trim tabs",
			input:   "\ta: b   \n    c:d     ",
			want:    FlatConfig{"a": "b", "c": "d"},
			wantErr: false,
		},
		{
			name:    "trim whitespaces between key and value",
			input:   "a  :     b",
			want:    FlatConfig{"a": "b"},
			wantErr: false,
		},
		{
			name:    "trim newlines",
			input:   "  \n    a: b   \n\n    c:d    \n\n ",
			want:    FlatConfig{"a": "b", "c": "d"},
			wantErr: false,
		},
		{
			name:    "ignore comments",
			input:   "a: b\n #this is a comment\n c: d",
			want:    FlatConfig{"a": "b", "c": "d"},
			wantErr: false,
		},
		{
			name:    "support quotes",
			input:   `a: "string in quotes"`,
			want:    FlatConfig{"a": `"string in quotes"`},
			wantErr: false,
		},
		{
			name:    "support special characters",
			input:   `a: %.:=+è! /\$`,
			want:    FlatConfig{"a": `%.:=+è! /\$`},
			wantErr: false,
		},
		{
			name:    "stop at first :",
			input:   "a: b: c: d: e",
			want:    FlatConfig{"a": "b: c: d: e"},
			wantErr: false,
		},
		{
			name:    "invalid entry",
			input:   "not key value",
			want:    FlatConfig{},
			wantErr: true,
		},
		{
			name:    "invalid entry among valid entries",
			input:   "a: b\n  not key value \n c:d",
			want:    FlatConfig{},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseConfig(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseConfig() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ParseConfig() = %v, want %v", got, tt.want)
			}
		})
	}
}
