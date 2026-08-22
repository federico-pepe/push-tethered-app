// Package identify helps a user tell physically identical Push units apart
// when pairing more than one to pushapp-ui.
//
// A display flash alone cannot finish a pairing: it says which USB unit a
// physical box is, but nothing about which MIDI port belongs to that same
// box. Flash and FlashLEDs answer the two different halves of that question,
// and a pairing UI needs both — FlashLEDs is also the only one that works
// while Live holds the display, under -no-display, and for a PortRef the
// midi package could not pair automatically (see PortRef.Ambiguous).
//
// There is no scaled text anywhere in this stack — core/gfx/text is a fixed
// 7px-wide font — so the marker does not try to draw a giant digit. It
// alternates the whole screen between a caller-assigned colour and black,
// with the ASCII label centred over it; the same colour appears as a swatch
// on that unit's row in the UI, so matching a screen to a row is immediate
// and needs no font work.
package identify

import (
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"strings"
	"time"

	"github.com/federico-pepe/ableton-push-hack/core/gfx"
	"github.com/federico-pepe/ableton-push-hack/core/gfx/text"
	"github.com/federico-pepe/ableton-push-hack/core/push3"
	"github.com/federico-pepe/push-tethered-app/internal/display"
	pmidi "github.com/federico-pepe/push-tethered-app/internal/midi"
)

// blinkHz is the marker's flash rate. Push redraws its own idle screen a
// short time after a host stops writing (see display.Blank's doc and
// docs/protocol/display.md), so Flash keeps writing continuously regardless
// of this rate — it only controls how fast the colour alternates.
const blinkHz = 2

// SanitizeLabel strips anything that would render as a missing-glyph box in
// core/gfx/text, which is ASCII-only, and caps the length so a long serial
// does not run off the 960px-wide screen. gousb already substitutes "?" for
// non-ASCII in USB string descriptors, so that gets dropped too rather than
// reaching the screen.
func SanitizeLabel(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= 0x20 && r < 0x7F && r != '?' {
			b.WriteRune(r)
		}
	}
	const maxRunes = 40 // 40*7px = 280px, comfortably inside VisW at any x
	return text.Truncate(strings.TrimSpace(b.String()), maxRunes)
}

// PaintMarker draws one identify frame into img. phase alternates the
// background between swatch and black; the label is always drawn in
// whichever colour contrasts with the current background, so it stays
// legible through the blink rather than disappearing against swatch on
// alternating frames.
//
// Exported so host.Runtime.Identify can paint the same marker into a frame
// already in flight, without claiming the display a second time — Flash
// claims fresh, this overlays onto a connection the caller already holds.
func PaintMarker(img *image.NRGBA, label string, swatch color.NRGBA, phase int) {
	black := push3.ColorForIndex(0).RGB // "off" — the only palette entry black maps to
	bg, fg := swatch, black
	if phase%2 == 1 {
		bg, fg = black, swatch
	}
	gfx.FillRect(img, 0, 0, push3.VisW, push3.VisH, bg)

	label = SanitizeLabel(label)
	x := (push3.VisW - text.Width(label)) / 2
	if x < 0 {
		x = 0
	}
	baseline := push3.VisH/2 + 5 // basicfont.Face7x13's baseline sits a few px below center
	text.Draw(img, x, baseline, label, fg)
}

// Flash claims sel's display (see display.OpenID) and blinks an identifying
// marker on it for d, then blanks the screen and releases the claim.
//
// fps controls the refresh rate of the underlying writes, not the blink rate
// — it must stay high enough that Push never reclaims the screen between
// frames (see docs/protocol/display.md's "must be refreshed continuously").
// 10-15 is comfortably enough for a static marker; the exact minimum has not
// been measured (see plans/2026-08-19-multi-device.md, open question 6).
func Flash(ctx context.Context, sel, label string, swatch color.NRGBA, d time.Duration, fps int) error {
	if fps <= 0 {
		fps = 12
	}
	dev, err := display.OpenID(sel)
	if err != nil {
		return fmt.Errorf("identify: claiming display for %q: %w", sel, err)
	}
	defer dev.Close()

	fctx, cancel := context.WithTimeout(ctx, d)
	defer cancel()

	img := image.NewNRGBA(image.Rect(0, 0, push3.VisW, push3.VisH))
	ticker := time.NewTicker(time.Second / time.Duration(fps))
	defer ticker.Stop()

	phase := 0
	blinkEvery := fps / (blinkHz * 2)
	if blinkEvery < 1 {
		blinkEvery = 1
	}
	frame := 0
	for {
		select {
		case <-fctx.Done():
			_ = dev.Blank(context.Background())
			if errors.Is(fctx.Err(), context.DeadlineExceeded) {
				return nil
			}
			return ctx.Err()
		case <-ticker.C:
			if frame%blinkEvery == 0 {
				phase++
			}
			frame++
			PaintMarker(img, label, swatch, phase)
			if err := dev.WriteFrame(fctx, img); err != nil {
				if errors.Is(err, display.ErrDisconnected) {
					return fmt.Errorf("identify: %w", err)
				}
				// A single write timeout is not fatal to the identify
				// session; the next tick tries again.
			}
		}
	}
}

// FlashLEDs opens outNum (a driver output port number, e.g. PortRef.OutNum)
// and lights every pad in colour for d, then clears and closes.
//
// Deliberately takes a bare port number rather than a PortRef: this is the
// only identify path that works when Live holds the display, under
// -no-display, and — the case that matters most — for a cable the midi
// package could not pair automatically (PortRef.Ambiguous). Opening only the
// output, with no paired input and no re-validation against a remembered
// name, means the caller can flash every candidate out cable in turn to find
// the right one even when automatic pairing already gave up; going through
// midi.OpenRef instead would refuse exactly the ambiguous case this exists
// to resolve.
func FlashLEDs(ctx context.Context, outNum int, colour byte, d time.Duration) error {
	cable, err := pmidi.OpenOutCable(outNum)
	if err != nil {
		return fmt.Errorf("identify: opening output %d for LED flash: %w", outNum, err)
	}
	defer cable.Close()

	for note := byte(push3.PadNoteMin); note <= push3.PadNoteMax; note++ {
		_ = cable.SetPad(note, colour)
	}

	select {
	case <-time.After(d):
	case <-ctx.Done():
	}
	// cable.Close() clears every pad it lit — no separate clear needed here.
	return ctx.Err()
}
