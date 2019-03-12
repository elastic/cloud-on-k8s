// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

// +build integration

package fs

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func Test_filesCache(t *testing.T) {
	// round time to the minute since the time we set up here
	// would otherwise be more precise than the one we retrieve from the OS
	// and could not be easily compared
	modificationTime := time.Now().Add(-1 * time.Hour).Round(time.Minute)
	modificationTimeLater := time.Now().Round(time.Minute)
	tests := []struct {
		name              string
		ignoreHiddenFiles bool
		filter            []string
		currentCache      FilesModTime
		setupFiles        FilesModTime
		wantCache         FilesModTime
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
			currentCache:   map[string]time.Time{"file1": time.Now()},
			setupFiles:     nil,
			wantCache:      nil,
			wantHasChanged: true,
		},
		{
			name:           "empty directory, didn't exist before",
			currentCache:   nil,
			setupFiles:     map[string]time.Time{},
			wantCache:      map[string]time.Time{},
			wantHasChanged: true,
		},
		{
			name:           "empty directory, did exist before",
			currentCache:   map[string]time.Time{},
			setupFiles:     map[string]time.Time{},
			wantCache:      map[string]time.Time{},
			wantHasChanged: false,
		},
		{
			name:         "some files written in an empty directory",
			currentCache: map[string]time.Time{},
			setupFiles: map[string]time.Time{
				"file1": modificationTime,
				"file2": modificationTime,
			},
			wantCache: map[string]time.Time{
				"file1": modificationTime,
				"file2": modificationTime,
			},
			wantHasChanged: true,
		},
		{
			name:         "some files written in a directory that did not exist before",
			currentCache: nil,
			setupFiles: map[string]time.Time{
				"file1": modificationTime,
				"file2": modificationTime,
			},
			wantCache: map[string]time.Time{
				"file1": modificationTime,
				"file2": modificationTime,
			},
			wantHasChanged: true,
		},
		{
			name:         "some files written when a filter is set",
			currentCache: map[string]time.Time{},
			filter:       []string{"file2"},
			setupFiles: map[string]time.Time{
				"file1": modificationTime,
				"file2": modificationTime,
			},
			wantCache: map[string]time.Time{
				"file2": modificationTime,
			},
			wantHasChanged: true,
		},
		{
			name: "no change on existing files",
			currentCache: map[string]time.Time{
				"file1": modificationTime,
				"file2": modificationTime,
			},
			setupFiles: map[string]time.Time{
				"file1": modificationTime,
				"file2": modificationTime,
			},
			wantCache: map[string]time.Time{
				"file1": modificationTime,
				"file2": modificationTime,
			},
			wantHasChanged: false,
		},
		{
			name: "new file added",
			currentCache: map[string]time.Time{
				"file1": modificationTime,
				"file2": modificationTime,
			},
			setupFiles: map[string]time.Time{
				"file1": modificationTime,
				"file2": modificationTime,
				"file3": modificationTimeLater,
			},
			wantCache: map[string]time.Time{
				"file1": modificationTime,
				"file2": modificationTime,
				"file3": modificationTimeLater,
			},
			wantHasChanged: true,
		},
		{
			name: "files have changed",
			currentCache: map[string]time.Time{
				"file1": modificationTime,
				"file2": modificationTime,
			},
			setupFiles: map[string]time.Time{
				"file1": modificationTime,
				"file2": modificationTimeLater,
			},
			wantCache: map[string]time.Time{
				"file1": modificationTime,
				"file2": modificationTimeLater,
			},
			wantHasChanged: true,
		},
		{
			name: "file removed",
			currentCache: map[string]time.Time{
				"file1": modificationTime,
				"file2": modificationTime,
			},
			setupFiles: map[string]time.Time{
				"file1": modificationTime,
			},
			wantCache: map[string]time.Time{
				"file1": modificationTime,
			},
			wantHasChanged: true,
		},
		{
			name:              "hidden files should not appear in the cache",
			ignoreHiddenFiles: true,
			currentCache: map[string]time.Time{
				"file1": modificationTime,
				"file2": modificationTime,
			},
			setupFiles: map[string]time.Time{
				"file1":  modificationTime,
				".file2": modificationTime,
				".file3": modificationTime,
			},
			wantCache: map[string]time.Time{
				"file1": modificationTime,
			},
			wantHasChanged: true,
		},
		{
			name:              "hidden files should appear in the cache if not ignored",
			ignoreHiddenFiles: false,
			currentCache: map[string]time.Time{
				"file1": modificationTime,
				"file2": modificationTime,
			},
			setupFiles: map[string]time.Time{
				"file1":  modificationTime,
				".file2": modificationTimeLater,
				".file3": modificationTimeLater,
			},
			wantCache: map[string]time.Time{
				"file1":  modificationTime,
				".file2": modificationTimeLater,
				".file3": modificationTimeLater,
			},
			wantHasChanged: true,
		},
		{
			name: "sub-directories should be ignored",
			currentCache: map[string]time.Time{
				"file1": modificationTime,
				"file2": modificationTime,
			},
			setupFiles: map[string]time.Time{
				"file1": modificationTime,
				"file2": modificationTime,
				"dir_1": modificationTime,
			},
			wantCache: map[string]time.Time{
				"file1": modificationTime,
				"file2": modificationTime,
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
				for filename, modTime := range tt.setupFiles {
					if strings.HasPrefix(filename, "dir") {
						// create a directory
						err := os.Mkdir(filepath.Join(directory, filename), 06444)
						require.NoError(t, err)
					} else {
						// create a file
						err := ioutil.WriteFile(filepath.Join(directory, filename), []byte("content"), 0644)
						require.NoError(t, err)
						// override last modification time
						err = os.Chtimes(filepath.Join(directory, filename), modTime, modTime)
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

func TestFilesModTime_Equals(t *testing.T) {
	modificationTime := time.Now().Add(-1 * time.Minute)
	modificationTimeLater := time.Now().Add(time.Minute)
	tests := []struct {
		name string
		f    FilesModTime
		f2   FilesModTime
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
			f2:   FilesModTime{},
			want: false,
		},
		{
			name: "f2 nil",
			f:    FilesModTime{},
			f2:   nil,
			want: false,
		},
		{
			name: "both empty",
			f:    FilesModTime{},
			f2:   FilesModTime{},
			want: true,
		},
		{
			name: "equal",
			f:    FilesModTime{"a": modificationTime, "b": modificationTimeLater},
			f2:   FilesModTime{"a": modificationTime, "b": modificationTimeLater},
			want: true,
		},
		{
			name: "different modification times",
			f:    FilesModTime{"a": modificationTime, "b": modificationTime},
			f2:   FilesModTime{"a": modificationTime, "b": modificationTimeLater},
			want: false,
		},
		{
			name: "more files in f",
			f:    FilesModTime{"a": modificationTime, "b": modificationTime},
			f2:   FilesModTime{"a": modificationTime},
			want: false,
		},
		{
			name: "more files in f2",
			f:    FilesModTime{"a": modificationTime},
			f2:   FilesModTime{"a": modificationTime, "b": modificationTime},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.f.Equals(tt.f2); got != tt.want {
				t.Errorf("FilesModTime.Equals() = %v, want %v", got, tt.want)
			}
		})
	}
}
