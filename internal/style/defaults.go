package style

import (
	"embed"
	"io/fs"
)

//go:embed defaults/*.json
var defaultFS embed.FS

// Defaults returns the example styles built into the binary, keyed by
// file name. They are copied into an empty styles folder on first run.
func Defaults() map[string][]byte {
	out := map[string][]byte{}
	files, _ := fs.Glob(defaultFS, "defaults/*.json")
	for _, f := range files {
		data, err := defaultFS.ReadFile(f)
		if err == nil {
			out[f] = data
		}
	}
	return out
}
