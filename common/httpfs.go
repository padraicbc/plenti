package common

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"golang.org/x/tools/godoc/vfs"
)

// NewFS returns a new FileSystem from the provided map.
// Map keys must be forward slash-separated paths with
// no leading slash, such as "file1.txt" or "dir/file2.txt".
// New panics if any of the paths contain a leading slash.
func NewFS() *httpFS {
	// Verify all provided paths are relative before proceeding.
	var pathsWithLeadingSlash []string
	for p := range MapFS {
		if strings.HasPrefix(p, "/") {
			pathsWithLeadingSlash = append(pathsWithLeadingSlash, p)
		}
	}

	return &httpFS{FS: mapFST(MapFS)}
}

type httpFS struct {
	FS mapFST
}

func (h *httpFS) Open(name string) (http.File, error) {

	fi, err := h.FS.Stat(name)

	if err != nil {
		return nil, err
	}
	if fi.IsDir() {
		return &httpDir{h.FS, name, nil}, nil
	}
	f, err := h.FS.Open(name)
	if err != nil {
		return nil, err
	}
	return &httpFile{h.FS, f, name}, nil
}

// httpDir implements http.File for a directory in a FileSystem.
type httpDir struct {
	FS      vfs.FileSystem
	name    string
	pending []os.FileInfo
}

func (h *httpDir) Close() error               { return nil }
func (h *httpDir) Stat() (os.FileInfo, error) { return h.FS.Stat(h.name) }
func (h *httpDir) Read([]byte) (int, error) {
	return 0, fmt.Errorf("cannot Read from directory %s", h.name)
}

func (h *httpDir) Seek(offset int64, whence int) (int64, error) {
	if offset == 0 && whence == 0 {
		h.pending = nil
		return 0, nil
	}
	return 0, fmt.Errorf("unsupported Seek in directory %s", h.name)
}

func (h *httpDir) Readdir(count int) ([]os.FileInfo, error) {
	if h.pending == nil {
		d, err := h.FS.ReadDir(h.name)
		if err != nil {
			return nil, err
		}
		if d == nil {
			d = []os.FileInfo{} // not nil
		}
		h.pending = d
	}

	if len(h.pending) == 0 && count > 0 {
		return nil, io.EOF
	}
	if count <= 0 || count > len(h.pending) {
		count = len(h.pending)
	}
	d := h.pending[:count]
	h.pending = h.pending[count:]
	return d, nil
}

// httpFile implements http.File for a file (not directory) in a FileSystem.
type httpFile struct {
	FS vfs.FileSystem
	vfs.ReadSeekCloser
	name string
}

func (h *httpFile) Stat() (os.FileInfo, error) { return h.FS.Stat(h.name) }
func (h *httpFile) Readdir(int) ([]os.FileInfo, error) {
	return nil, fmt.Errorf("cannot Readdir from file %s", h.name)
}
