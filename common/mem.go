package common

import (
	"hash/crc32"
	"path/filepath"
)

// UseMemFS determines if local dev files are stored on disk or in memory
var UseMemFS = false

// MapFS ok
var mapFS = map[string]*FData{}

// Set ok
func Set(k string, v *FData) {
	mapFS[filepath.Clean(k)] = v
}

// Get ok
func Get(k string) *FData {
	return mapFS[filepath.Clean(k)]
}

// GetOrSet ok
func GetOrSet(k string) *FData {
	clean := filepath.Clean(k)
	if v, ok := mapFS[clean]; ok {
		return v
	}
	d := &FData{}
	mapFS[clean] = d
	return d
}

// Del ok
func Del(k string) {
	delete(mapFS, k)
}

// Iter ok
func Iter() map[string]*FData {
	return mapFS
}

// FData OK
type FData struct {
	// Hash of last check, only applies original files on disk.
	// TODO: integrate content/css
	Hash uint32
	// store content/css/ssr per component/layout.
	// B can be the concatenated css for bundle.css, the layout file bytes or the compiled component.
	// If memory is an issue maybe let a flag decide what we keep in mem.
	B []byte
	// A component can have a script and maybe style so this applies in the latter case else it is nil.
	// Might be a litle confusing to see stylePath storing/using B and CSS getting appended but CSS is each component's css
	// of which we combine all.
	CSS []byte
	// Used to store prev compiled ssr which is reused if no changes to layout file.
	SSR []byte
	// flag that is used for now to signify that once file has been processed once it doesn't need to be done again.
	// This is specific to gopack for now anyway
	Processed bool
}

var crc32q = crc32.MakeTable(0xD5828281)

// CRC32Hasher is a simple means to check for changes
func CRC32Hasher(b []byte) uint32 {
	crc32q := crc32.MakeTable(0xD5828281)
	return crc32.Checksum(b, crc32q)

}
