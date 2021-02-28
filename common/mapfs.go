package common

import (
	"bytes"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"
)

// MapFST just serves a purpose of holding buildDir and implementing serveHTTP so we are a HandlerFunc
type MapFST struct {
	buildDir string
}

// NewH ok
func NewH(buildDir string) *MapFST {
	// Verify all provided paths are relative before proceeding.
	var pathsWithLeadingSlash []string
	for p := range mapFS {
		if strings.HasPrefix(p, "/") {
			pathsWithLeadingSlash = append(pathsWithLeadingSlash, p)
		}
	}

	return &MapFST{buildDir: buildDir}
}

// Basically the inverse of StripPrefix logic.
// There is an overlap with names that breaks things as one can overwrite the other, "public"+... avoids this.
// eventually this and basically all other build related logic could become a "Builder" type tha store all config etc..
func (h *MapFST) ServeHTTP(w http.ResponseWriter, r *http.Request) {

	upath := strings.Trim(r.URL.Path, "/")
	upath = filepath.Clean(h.buildDir + "/" + upath)
	// home
	if upath == h.buildDir+"/" {
		upath += "/index.html"
	}
	var f io.ReadSeeker
	// .js| file with extension etc..
	if b, ok := mapFS[upath]; ok {
		f = bytes.NewReader(b.B)
		// /blog etc.. so try blog/index.html
	} else if !strings.Contains(upath, ".") {
		if b, ok := mapFS[(upath + "/index.html")]; ok {
			f = bytes.NewReader(b.B)
		}

	}
	// not in map
	if f == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	// could just write bytes but this
	http.ServeContent(w, r, upath, time.Time{}, f)
}
