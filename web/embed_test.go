package web

import (
	"io/fs"
	"testing"
)

func TestStaticContainsEntryPoints(t *testing.T) {
	fsys, err := Static()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"index.html", "css/app.css", "js/main.js", "js/viz/wheel.js", "js/viz/radar.js"} {
		if _, err := fs.Stat(fsys, name); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}
