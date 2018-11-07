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
