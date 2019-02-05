// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package initcontainer

// LinkedFile describes a symbolic link with source and target.
type LinkedFile struct {
	Source string
	Target string
}

// LinkedFilesArray contains all files to be linked in the init container.
type LinkedFilesArray struct {
	Array []LinkedFile
}
