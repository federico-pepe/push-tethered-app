// Command screensim renders named test scenes to PNG without touching
// hardware or building the full app.
//
// Two ways to define a scene:
//
//   - Frame mode (frameScenes): builds a *module.Frame the way a module's
//     Draw would, then renders it exactly as internal/host does, via
//     internal/renderframe.Render. Proves an op renders the same way it
//     would on a real run.
//   - Direct-draw mode (drawScenes): draws straight onto the *image.NRGBA
//     with core/gfx and core/gfx/widgets calls, for trying out a widget
//     before it has a Frame op at all.
//
// Deliberately outside internal/host: that package imports internal/display,
// which imports gousb (cgo/libusb), so a tool that must run on any machine
// with just `go run` cannot link it. This tool only imports the renderer
// (internal/renderframe) and the widget/gfx packages, none of which touch
// gousb.
package main

import (
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"log"
	"os"
	"sort"

	coredisplay "github.com/federico-pepe/ableton-push-hack/core/display"
	"github.com/federico-pepe/ableton-push-hack/core/gfx/layout"
	"github.com/federico-pepe/ableton-push-hack/core/gfx/widgets"
	"github.com/federico-pepe/ableton-push-hack/core/push3"
	"github.com/federico-pepe/push-tethered-app/internal/module"
	"github.com/federico-pepe/push-tethered-app/internal/renderframe"
)

func main() {
	var (
		scene         = flag.String("scene", "", "scene to render (see -list-scenes)")
		out           = flag.String("out", "", "PNG output path")
		raw           = flag.Bool("raw", false, "skip the BGR565 round-trip; renders sharper than the real panel")
		grid          = flag.Bool("grid", false, "overlay the 8-column grid and top/bottom bar guides")
		listScenesArg = flag.Bool("list-scenes", false, "print every available scene name and exit")
	)
	flag.Parse()

	if *listScenesArg {
		listScenes()
		return
	}
	if *scene == "" || *out == "" {
		fmt.Fprintln(os.Stderr, "usage: screensim -scene <name> -out <path.png> [-raw] [-grid]")
		fmt.Fprintln(os.Stderr, "       screensim -list-scenes")
		os.Exit(2)
	}

	img, err := render(*scene)
	if err != nil {
		log.Fatal(err)
	}
	if *grid {
		drawGrid(img)
	}
	if !*raw {
		img = panelAccurate(img)
	}
	if err := writePNG(*out, img); err != nil {
		log.Fatal(err)
	}
}

// render dispatches to whichever registry has the named scene.
func render(name string) (*image.NRGBA, error) {
	img := image.NewNRGBA(image.Rect(0, 0, push3.VisW, push3.VisH))

	if fn, ok := frameScenes[name]; ok {
		f := module.NewFrame(push3.VisW, push3.VisH)
		fn(f)
		renderframe.Render(f, img, widgets.Default)
		return img, nil
	}
	if fn, ok := drawScenes[name]; ok {
		fn(img)
		return img, nil
	}
	return nil, fmt.Errorf("no such scene %q — run -list-scenes", name)
}

func listScenes() {
	var names []string
	for k := range frameScenes {
		names = append(names, k+" (frame)")
	}
	for k := range drawScenes {
		names = append(names, k+" (direct-draw)")
	}
	sort.Strings(names)
	for _, n := range names {
		fmt.Println(n)
	}
}

// panelAccurate round-trips img through the panel's BGR565 encoding, the
// same way internal/capture does for recordings — so what you look at
// matches what the hardware would actually show, including its colour
// banding, rather than the crisper source image.
func panelAccurate(img *image.NRGBA) *image.NRGBA {
	return coredisplay.FromBGR565(coredisplay.ToBGR565(img)[:push3.FrameBytes])
}

// drawGrid overlays the 8-column division lines from core/gfx/layout as a
// visual reference — this is the check that package's own doc points to
// before any module is rewritten to use it.
func drawGrid(img *image.NRGBA) {
	guide := color.NRGBA{R: 255, G: 0, B: 255, A: 80}
	colW := layout.ColWidth(push3.VisW)
	for i := 1; i < layout.Cols; i++ {
		widgets.DrawVLine(img, i*colW, 0, push3.VisH, guide)
	}
}

func writePNG(path string, img *image.NRGBA) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("creating %s: %w", path, err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		return fmt.Errorf("encoding %s: %w", path, err)
	}
	return nil
}
