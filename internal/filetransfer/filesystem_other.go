//go:build !windows && !darwin

package filetransfer

func systemFilesystem(home string) (*filesystemScope, error) { return unixFilesystem(home) }
