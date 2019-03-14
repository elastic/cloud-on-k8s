// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

// +build integration

package fs

import (
	"hash/crc32"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_filesCache(t *testing.T) {
	data1 := []byte("data1")
	data2 := []byte("data2")
	crc1 := crc32.ChecksumIEEE(data1)
	crc2 := crc32.ChecksumIEEE(data2)
	tests := []struct {
		name              string
		ignoreHiddenFiles bool
		filter            []string
		currentCache      FilesCRC
		setupFiles        map[string][]byte
		wantCache         FilesCRC
		wantHasChanged    bool
	}{
		{
			name:           "non-existing directory",
			currentCache:   nil,
			setupFiles:     nil,
			wantCache:      nil,
			wantHasChanged: false,
		},
		{
			name:           "non-existing directory, did exist before",
			currentCache:   FilesCRC{"file1": crc1},
			setupFiles:     nil,
			wantCache:      nil,
			wantHasChanged: true,
		},
		{
			name:           "empty directory, didn't exist before",
			currentCache:   nil,
			setupFiles:     map[string][]byte{},
			wantCache:      FilesCRC{},
			wantHasChanged: true,
		},
		{
			name:           "empty directory, did exist before",
			currentCache:   FilesCRC{},
			setupFiles:     map[string][]byte{},
			wantCache:      FilesCRC{},
			wantHasChanged: false,
		},
		{
			name:         "some files written in an empty directory",
			currentCache: FilesCRC{},
			setupFiles: map[string][]byte{
				"file1": data1,
				"file2": data1,
			},
			wantCache: FilesCRC{
				"file1": crc1,
				"file2": crc1,
			},
			wantHasChanged: true,
		},
		{
			name:         "some files written in a directory that did not exist before",
			currentCache: nil,
			setupFiles: map[string][]byte{
				"file1": data1,
				"file2": data1,
			},
			wantCache: FilesCRC{
				"file1": crc1,
				"file2": crc1,
			},
			wantHasChanged: true,
		},
		{
			name:         "some files written when a filter is set",
			currentCache: FilesCRC{},
			filter:       []string{"file2"},
			setupFiles: map[string][]byte{
				"file1": data1,
				"file2": data1,
			},
			wantCache: FilesCRC{
				"file2": crc1,
			},
			wantHasChanged: true,
		},
		{
			name: "no change on existing files",
			currentCache: FilesCRC{
				"file1": crc1,
				"file2": crc1,
			},
			setupFiles: map[string][]byte{
				"file1": data1,
				"file2": data1,
			},
			wantCache: FilesCRC{
				"file1": crc1,
				"file2": crc1,
			},
			wantHasChanged: false,
		},
		{
			name: "new file added",
			currentCache: FilesCRC{
				"file1": crc1,
				"file2": crc1,
			},
			setupFiles: map[string][]byte{
				"file1": data1,
				"file2": data1,
				"file3": data2,
			},
			wantCache: FilesCRC{
				"file1": crc1,
				"file2": crc1,
				"file3": crc2,
			},
			wantHasChanged: true,
		},
		{
			name: "files have changed",
			currentCache: FilesCRC{
				"file1": crc1,
				"file2": crc1,
			},
			setupFiles: map[string][]byte{
				"file1": data1,
				"file2": data2,
			},
			wantCache: FilesCRC{
				"file1": crc1,
				"file2": crc2,
			},
			wantHasChanged: true,
		},
		{
			name: "file removed",
			currentCache: FilesCRC{
				"file1": crc1,
				"file2": crc1,
			},
			setupFiles: map[string][]byte{
				"file1": data1,
			},
			wantCache: FilesCRC{
				"file1": crc1,
			},
			wantHasChanged: true,
		},
		{
			name:              "hidden files should not appear in the cache",
			ignoreHiddenFiles: true,
			currentCache: FilesCRC{
				"file1": crc1,
				"file2": crc1,
			},
			setupFiles: map[string][]byte{
				"file1":  data1,
				".file2": data1,
				".file3": data1,
			},
			wantCache: FilesCRC{
				"file1": crc1,
			},
			wantHasChanged: true,
		},
		{
			name:              "hidden files should appear in the cache if not ignored",
			ignoreHiddenFiles: false,
			currentCache: FilesCRC{
				"file1": crc1,
				"file2": crc1,
			},
			setupFiles: map[string][]byte{
				"file1":  data1,
				".file2": data2,
				".file3": data2,
			},
			wantCache: FilesCRC{
				"file1":  crc1,
				".file2": crc2,
				".file3": crc2,
			},
			wantHasChanged: true,
		},
		{
			name: "sub-directories should be ignored",
			currentCache: FilesCRC{
				"file1": crc1,
				"file2": crc1,
			},
			setupFiles: map[string][]byte{
				"file1": data1,
				"file2": data1,
				"dir_1": data1,
			},
			wantCache: FilesCRC{
				"file1": crc1,
				"file2": crc1,
			}, // same without the subdir
			wantHasChanged: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// work in a tmp directory
			directory, err := ioutil.TempDir("", "tmpdir")
			require.NoError(t, err)
			defer os.RemoveAll(directory)

			c, err := newFilesCache(directory, tt.ignoreHiddenFiles, tt.filter)
			require.NoError(t, err)

			// simulate existing cache
			c.cache = tt.currentCache

			if tt.setupFiles == nil {
				// simulate non-existing dir by removing the one we created
				err := os.Remove(directory)
				require.NoError(t, err)
			} else {
				// create files in the directory
				for filename, content := range tt.setupFiles {
					if strings.HasPrefix(filename, "dir") {
						// create a directory
						err := os.Mkdir(filepath.Join(directory, filename), 06444)
						require.NoError(t, err)
					} else {
						// create a file
						err := ioutil.WriteFile(filepath.Join(directory, filename), content, 0644)
						require.NoError(t, err)
					}
				}
			}

			files, hasChanged, err := c.update()
			require.NoError(t, err)
			require.Equal(t, tt.wantHasChanged, hasChanged)
			// returned files should match the current (updated) cache
			require.True(t, files.Equals(c.cache))
			// cache should be the expected one
			require.True(t, c.cache.Equals(tt.wantCache))
		})
	}
}

func TestFilesCRC_Equals(t *testing.T) {
	crc1 := crc32.ChecksumIEEE([]byte("data1"))
	crc2 := crc32.ChecksumIEEE([]byte("data2"))
	tests := []struct {
		name string
		f    FilesCRC
		f2   FilesCRC
		want bool
	}{
		{
			name: "both nil",
			f:    nil,
			f2:   nil,
			want: true,
		},
		{
			name: "f nil",
			f:    nil,
			f2:   FilesCRC{},
			want: false,
		},
		{
			name: "f2 nil",
			f:    FilesCRC{},
			f2:   nil,
			want: false,
		},
		{
			name: "both empty",
			f:    FilesCRC{},
			f2:   FilesCRC{},
			want: true,
		},
		{
			name: "equal",
			f:    FilesCRC{"a": crc1, "b": crc2},
			f2:   FilesCRC{"a": crc1, "b": crc2},
			want: true,
		},
		{
			name: "different crc",
			f:    FilesCRC{"a": crc1, "b": crc1},
			f2:   FilesCRC{"a": crc1, "b": crc2},
			want: false,
		},
		{
			name: "more files in f",
			f:    FilesCRC{"a": crc1, "b": crc1},
			f2:   FilesCRC{"a": crc1},
			want: false,
		},
		{
			name: "more files in f2",
			f:    FilesCRC{"a": crc1},
			f2:   FilesCRC{"a": crc1, "b": crc1},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.f.Equals(tt.f2); got != tt.want {
				t.Errorf("FilesCRC.Equals() = %v, want %v", got, tt.want)
			}
		})
	}
}
