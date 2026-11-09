// Command appicon packs WoW Live's icon (icon.svg and icon-small.svg, rendered to png/ by render.sh) where the app
// build needs it: cmd/app/rsrc_windows_amd64.syso, the exe's icon resource that the taskbar, the window and the Start
// menu show, and cmd/app/embed/icon.ico for the tray. build.sh runs it from backend/; both outputs are gitignored.
package main

import (
	"embed"
	"image"
	"image/png"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"

	"github.com/tc-hib/winres"
)

// sizes are the rendered PNGs, one per size Windows asks for.
//
//go:embed png/*.png
var sizes embed.FS

func main() {
	icon, err := winres.NewIconFromImages(images())
	if err != nil {
		log.Fatal(err)
	}
	var rs winres.ResourceSet
	if err := rs.SetIcon(winres.RT_ICON, icon); err != nil { // Wails loads the window icon from resource 3
		log.Fatal(err)
	}
	app := filepath.Join("cmd", "app")
	if err := write(filepath.Join(app, "rsrc_windows_amd64.syso"), func(w io.Writer) error {
		return rs.WriteObject(w, winres.ArchAMD64)
	}); err != nil {
		log.Fatal(err)
	}
	if err := write(filepath.Join(app, "embed", "icon.ico"), icon.SaveICO); err != nil {
		log.Fatal(err)
	}
}

// images reads every rendered size.
func images() []image.Image {
	var out []image.Image
	err := fs.WalkDir(sizes, "png", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		f, err := sizes.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		img, err := png.Decode(f)
		out = append(out, img)
		return err
	})
	if err != nil {
		log.Fatal(err)
	}
	return out
}

// write creates path and fills it with fill.
func write(path string, fill func(io.Writer) error) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := fill(f); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
