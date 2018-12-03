package diskutil

import (
	"os"
	"path"
	"testing"

	"github.com/stretchr/testify/assert"
)

func createPathAndDelete(t *testing.T, p string) (string, func()) {
	if p == "" {
		return p, func() {}
	}
	if err := os.MkdirAll(p, 0775); err != nil {
		t.Fatal(err)
	}
	return p, func() { os.RemoveAll(p) }
}

func TestEnsureDirExists(t *testing.T) {
	type args struct {
		path string
	}
	tests := []struct {
		name string
		args args
		err  string
	}{
		{
			name: "fails with empy path",
			args: args{},
			err:  "mkdir : no such file or directory",
		},
		{
			name: "fails with empy path",
			args: args{
				path: path.Join(os.TempDir(), "somepath"),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, cleanup := createPathAndDelete(t, tt.args.path)
			defer cleanup()
			if err := EnsureDirExists(tt.args.path); err != nil {
				assert.Equal(t, tt.err, err.Error())
				return
			}
			if tt.err != "" {
				assert.Fail(t, "Expected error not present", tt.err)
			}
		})
	}
}
