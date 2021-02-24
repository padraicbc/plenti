package common

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	pathpkg "path"
	"sort"
	"strings"
	"time"

	"golang.org/x/tools/godoc/vfs"
)

// mapFST is the map based implementation of FileSystem
type mapFST map[string]*FData

func (fs mapFST) Open(p string) (vfs.ReadSeekCloser, error) {

	b, ok := fs[filename(p)]

	if !ok {
		return nil, os.ErrNotExist
	}
	return nopCloser{bytes.NewReader(b.B)}, nil
}

func (fs mapFST) String() string {
	s := []string{}
	for k := range fs {
		s = append(s, k)
	}
	bb, err := json.MarshalIndent(s, "", "    ")
	if err != nil {
		return ""
	}

	return string(bb)
}

func (fs mapFST) RootType(p string) vfs.RootType {
	return ""
}

func (fs mapFST) Close() error { return nil }

func filename(p string) string {

	return strings.TrimPrefix(p, "/")
}

func fileInfo(name string, contents []byte) os.FileInfo {
	return mapFI{name: pathpkg.Base(name), size: len(contents)}
}

func dirInfo(name string) os.FileInfo {
	return mapFI{name: pathpkg.Base(name), dir: true}
}

type nopCloser struct {
	io.ReadSeeker
}

func (nc nopCloser) Close() error { return nil }
func (fs mapFST) Lstat(p string) (os.FileInfo, error) {

	b, ok := fs[filename(p)]

	if ok {

		return fileInfo(p, b.B), nil
	}
	ents, _ := fs.ReadDir(p)
	if len(ents) > 0 {
		return dirInfo(p), nil
	}
	return nil, os.ErrNotExist
}

func (fs mapFST) Stat(p string) (os.FileInfo, error) {
	return fs.Lstat(p)
}

// slashdir returns path.Dir(p), but special-cases paths not beginning
// with a slash to be in the root.
func slashdir(p string) string {
	d := pathpkg.Dir(p)
	if d == "." {
		return "/"
	}
	if strings.HasPrefix(p, "/") {
		return d
	}
	return "/" + d
}
func (fs mapFST) Readdir(count int) ([]os.FileInfo, error) {

	return nil, nil
}
func (fs mapFST) ReadDir(p string) ([]os.FileInfo, error) {
	p = pathpkg.Clean(p)
	var ents []string
	fim := make(map[string]os.FileInfo) // base -> fi
	for fn, b := range fs {
		dir := slashdir(fn)
		isFile := true
		var lastBase string
		for {
			if dir == p {
				base := lastBase
				if isFile {
					base = pathpkg.Base(fn)
				}
				if fim[base] == nil {
					var fi os.FileInfo
					if isFile {
						fi = fileInfo(fn, b.B)
					} else {
						fi = dirInfo(base)
					}
					ents = append(ents, base)
					fim[base] = fi
				}
			}
			if dir == "/" {
				break
			} else {
				isFile = false
				lastBase = pathpkg.Base(dir)
				dir = pathpkg.Dir(dir)
			}
		}
	}
	if len(ents) == 0 {
		return nil, os.ErrNotExist
	}

	sort.Strings(ents)
	var list []os.FileInfo
	for _, dir := range ents {
		list = append(list, fim[dir])
	}
	return list, nil
}

// mapFI is the map-based implementation of FileInfo.
type mapFI struct {
	name string
	size int
	dir  bool
}

func (fi mapFI) Close() error       { return nil }
func (fi mapFI) IsDir() bool        { return fi.dir }
func (fi mapFI) ModTime() time.Time { return time.Time{} }
func (fi mapFI) Mode() os.FileMode {
	if fi.IsDir() {
		return 0755 | os.ModeDir
	}
	return 0444
}
func (fi mapFI) Name() string     { return pathpkg.Base(fi.name) }
func (fi mapFI) Size() int64      { return int64(fi.size) }
func (fi mapFI) Sys() interface{} { return nil }
