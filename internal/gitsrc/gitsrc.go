// Package gitsrc reads the files tours point at, confined to the repo they
// belong to.
package gitsrc

import (
	"context"
	"errors"
)

var (
	// ErrNotFound means the path doesn't exist at the requested ref.
	ErrNotFound = errors.New("file not found")
	// ErrEscapesRoot means the path resolves outside its root, lexically or
	// through a symlink. Such a path is never read.
	ErrEscapesRoot = errors.New("path escapes its root")
	// ErrIsDirectory means a file was asked for and the path is a directory.
	ErrIsDirectory = errors.New("path is a directory")
	// ErrNotDirectory means a directory was asked for and the path is a file.
	ErrNotDirectory = errors.New("path is not a directory")
)

type (
	// Entry is one child of a directory.
	Entry struct {
		_     struct{} `json:"-"`
		Name  string   `json:"name"`
		IsDir bool     `json:"isDir"`
	}

	// FileSource reads files and directories under a root. An empty ref means
	// the working tree; anything else is a git revision.
	FileSource interface {
		ReadFile(ctx context.Context, root, ref, path string) ([]byte, error)
		ReadDir(ctx context.Context, root, ref, path string) ([]Entry, error)
	}
)
