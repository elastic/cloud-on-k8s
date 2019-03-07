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

	"github.com/stretchr/testify/require"
)

func Test_filesCache(t *testing.T) {
	// test fixtures
	files := map[string][]byte{
		"file1": []byte("file1content"),
		"file2": []byte("file2content"),
	}
	filter := []string{"file2"}
	withFilter := map[string][]byte{
		"file2": []byte("file2content"),
	}
	withNewFile := map[string][]byte{
		"file1": []byte("file1content"),
		"file2": []byte("file2content"),
		"file3": []byte("file3content"),
	}
	withFilesChanged := map[string][]byte{
		"file1": []byte("file1contentchanged"),
		"file2": []byte("file2contentchanged"),
	}
	withFileRemoved := map[string][]byte{
		"file1": []byte("file1content"),
	}
	withHiddenFiles := map[string][]byte{
		"file1":  []byte("file1content"),
		".file2": []byte("file2content"),
		".file3": []byte("file3content"),
	}
	withoutHiddenFiles := map[string][]byte{
		"file1": []byte("file1content"),
	}
	withSubDir := map[string][]byte{
		"file1": []byte("file1content"),
		"file2": []byte("file2content"),
		"dir_1": []byte{},
	}
	tests := []struct {
		name              string
		ignoreHiddenFiles bool
		filter            []string
		currentCache      FilesContent
		setupFiles        FilesContent
		wantCache         FilesContent
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
			currentCache:   files,
			setupFiles:     nil,
			wantCache:      nil,
			wantHasChanged: true,
		},
		{
			name:           "empty directory, didn't exist before",
			currentCache:   nil,
			setupFiles:     map[string][]byte{},
			wantCache:      map[string][]byte{},
			wantHasChanged: true,
		},
		{
			name:           "empty directory, did exist before",
			currentCache:   map[string][]byte{},
			setupFiles:     map[string][]byte{},
			wantCache:      map[string][]byte{},
			wantHasChanged: false,
		},
		{
			name:           "some files written in an empty directory",
			currentCache:   map[string][]byte{},
			setupFiles:     files,
			wantCache:      files,
			wantHasChanged: true,
		},
		{
			name:           "some files written in a directory that did not exist before",
			currentCache:   map[string][]byte{},
			setupFiles:     files,
			wantCache:      files,
			wantHasChanged: true,
		},
		{
			name:           "some files written when a filter is set",
			currentCache:   map[string][]byte{},
			filter:         filter,
			setupFiles:     files,
			wantCache:      withFilter,
			wantHasChanged: true,
		},
		{
			name:           "no change on existing files",
			currentCache:   files,
			setupFiles:     files,
			wantCache:      files,
			wantHasChanged: false,
		},
		{
			name:           "new file added",
			currentCache:   files,
			setupFiles:     withNewFile,
			wantCache:      withNewFile,
			wantHasChanged: true,
		},
		{
			name:           "files have changed",
			currentCache:   files,
			setupFiles:     withFilesChanged,
			wantCache:      withFilesChanged,
			wantHasChanged: true,
		},
		{
			name:           "file removed",
			currentCache:   files,
			setupFiles:     withFileRemoved,
			wantCache:      withFileRemoved,
			wantHasChanged: true,
		},
		{
			name:              "hidden files should not appear in the cache",
			ignoreHiddenFiles: true,
			currentCache:      files,
			setupFiles:        withHiddenFiles,
			wantCache:         withoutHiddenFiles,
			wantHasChanged:    true,
		},
		{
			name:              "hidden files should appear in the cache if not ignored",
			ignoreHiddenFiles: false,
			currentCache:      files,
			setupFiles:        withHiddenFiles,
			wantCache:         withHiddenFiles,
			wantHasChanged:    true,
		},
		{
			name:           "sub-directories should be ignored",
			currentCache:   files,
			setupFiles:     withSubDir,
			wantCache:      files, // same without the subdir
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
			// actual cache should be updated and returned
			require.True(t, c.cache.Equals(tt.wantCache))
			require.True(t, files.Equals(c.cache))
		})
	}
}

func TestFilesContent_Equals(t *testing.T) {
	type args struct {
	}
	tests := []struct {
		name string
		f    FilesContent
		f2   FilesContent
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
			f2:   FilesContent{},
			want: false,
		},
		{
			name: "f2 nil",
			f:    FilesContent{},
			f2:   nil,
			want: false,
		},
		{
			name: "both empty",
			f:    FilesContent{},
			f2:   FilesContent{},
			want: true,
		},
		{
			name: "same files",
			f:    FilesContent{"a": []byte("aaa"), "b": []byte("bbb")},
			f2:   FilesContent{"a": []byte("aaa"), "b": []byte("bbb")},
			want: true,
		},
		{
			name: "different file content",
			f:    FilesContent{"a": []byte("aaa"), "b": []byte("bbb")},
			f2:   FilesContent{"a": []byte("aaa"), "b": []byte("different")},
			want: false,
		},
		{
			name: "more files in f",
			f:    FilesContent{"a": []byte("aaa"), "b": []byte("bbb")},
			f2:   FilesContent{"a": []byte("aaa")},
			want: false,
		},
		{
			name: "more files in f2",
			f:    FilesContent{"a": []byte("aaa"), "b": []byte("different")},
			f2:   FilesContent{"a": []byte("aaa"), "b": []byte("bbb")},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.f.Equals(tt.f2); got != tt.want {
				t.Errorf("FilesContent.Equals() = %v, want %v", got, tt.want)
			}
		})
	}
}
