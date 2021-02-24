package common

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"golang.org/x/tools/godoc/vfs"
)

// NewFS returns a new FileSystem from the provided map.
// Map keys must be forward slash-separated paths with
// no leading slash, such as "file1.txt" or "dir/file2.txt".
// New panics if any of the paths contain a leading slash.
func NewFS() *httpFS {
	// Verify all provided paths are relative before proceeding.
	var pathsWithLeadingSlash []string
	for p := range mapFS {
		if strings.HasPrefix(p, "/") {
			pathsWithLeadingSlash = append(pathsWithLeadingSlash, p)
		}
	}

	return &httpFS{FS: mapFST(mapFS)}
}

type httpFS struct {
	FS mapFST
}

// Basically the inverse of StripPrefix logic.
// This is an ugly hack until we separate concerns and the filesystem only holds files the buildDir files.
// There is an overlap with names that breaks things as one can overwrite the other, "public"+... avoids this.
// This will break for now if you change from public naming convention.
func (h *httpFS) ServeHTTP(w http.ResponseWriter, r *http.Request) {

	upath := strings.Trim(r.URL.Path, "/")
	upath = "public/" + upath
	if upath == "public/" {
		upath += "index.html"
	}
	var f io.ReadSeeker
	var err error

	// a "dir" so see if index.html exists
	if !strings.Contains(upath, ".") {
		f, err = h.Open(upath + "/index.html")
		if err != nil {

			msg, code := toHTTPError(err)
			http.Error(w, msg, code)
			return
		}
	} else {
		// .js etc..
		f, err = h.Open(upath)
		if err != nil {

			msg, code := toHTTPError(err)
			http.Error(w, msg, code)
			return

		}
	}

	http.ServeContent(w, r, upath, time.Time{}, f)
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

func toHTTPError(err error) (msg string, httpStatus int) {
	if os.IsNotExist(err) {
		return "404 page not found", http.StatusNotFound
	}
	if os.IsPermission(err) {
		return "403 Forbidden", http.StatusForbidden
	}
	// Default:
	return "500 Internal Server Error", http.StatusInternalServerError
}
