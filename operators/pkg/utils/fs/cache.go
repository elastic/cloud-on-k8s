// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package fs

import (
	"hash/crc32"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
)

// filesCache caches a directory's files last modification time in memory.
type filesCache struct {
	directory         string              // directory to read files in
	ignoreHiddenFiles bool                // if set to true, don't cache hidden files
	cache             FilesCRC            // files CRC in memory
	filter            map[string]struct{} // filter on these files only (if empty, cache all files)
}

// newFilesCache creates a new populated filesCache.
// If filter is not empty, cache the given files only.
func newFilesCache(directory string, ignoreHiddenFiles bool, filter []string) (*filesCache, error) {
	filterAsMap := make(map[string]struct{}, len(filter))
	for _, f := range filter {
		filterAsMap[f] = struct{}{}
	}
	filesCache := filesCache{
		directory:         directory,
		ignoreHiddenFiles: ignoreHiddenFiles,
		filter:            filterAsMap,
		cache:             nil, // default to a non-existing directory
	}
	// update it at least once
	_, _, err := filesCache.update()
	return &filesCache, err
}

// update reads the directory's files to update the cache.
// Sub-directories are ignored.
func (c *filesCache) update() (files FilesCRC, hasChanged bool, err error) {
	filesInDir, err := ioutil.ReadDir(c.directory)
	if err != nil && os.IsNotExist(err) {
		// handle non-existing directory
		if c.cache != nil {
			// it existed before and was deleted, update the cache
			c.cache = nil
			return nil, true, nil
		}
		// it didn't exist before, no change
		return nil, false, nil
	} else if err != nil {
		return nil, false, err
	}

	// read all files last modification time
	newCache := make(map[string]uint32)
	for _, f := range filesInDir {
		if c.shouldIgnore(f) {
			continue
		}
		data, err := ioutil.ReadFile(filepath.Join(c.directory, f.Name()))
		if err != nil {
			return nil, false, err
		}
		newCache[f.Name()] = crc32.ChecksumIEEE(data)
	}

	hasChanged = !c.cache.Equals(newCache)
	c.cache = newCache
	return newCache, hasChanged, nil
}

// shouldIgnore returns true if
// - the file is a directory
// - the file is hidden and we ignore hidden files
// - a filter is set up and the file does not belong to it
func (c *filesCache) shouldIgnore(f os.FileInfo) bool {
	if f.IsDir() {
		return true
	}
	if c.ignoreHiddenFiles && IsHiddenFile(f.Name()) {
		return true
	}
	if len(c.filter) > 0 {
		if _, inFilter := c.filter[f.Name()]; !inFilter {
			return true
		}
	}
	return false
}

// FilesCRC defines a map of file name -> file content CRC.
type FilesCRC map[string]uint32

// Equals returns true if both f and f2 are considered equal.
func (f FilesCRC) Equals(f2 FilesCRC) bool {
	// differentiate nil (no directory) from zero value (empty directory)
	if (f == nil && f2 != nil) || (f != nil && f2 == nil) {
		return false
	}
	if len(f) != len(f2) {
		return false
	}
	for filename, crc1 := range f {
		crc2, exists := f2[filename]
		if !exists {
			return false
		}
		if crc1 != crc2 {
			return false
		}
	}
	return true
}

// IsHiddenFile returns true if the given filename corresponds to a hidden file.
func IsHiddenFile(filename string) bool {
	return strings.HasPrefix(filename, ".")
}
