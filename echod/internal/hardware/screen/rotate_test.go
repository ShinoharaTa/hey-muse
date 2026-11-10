//go:build !dot && !spot

package screen

import (
	"bytes"
	"encoding/binary"
	"image"
	"math/rand"
	"testing"
)

// fake is a Show's panel in RAM: 480 wide, 960 tall, rows padded as a real framebuffer's can be.
func fake(shift [4]uint) (*Device, []byte) {
	d := &Device{panelW: 480, panelH: 960, line: 480*4 + 64, pages: 3, shift: shift}
	d.pageBytes = d.line * d.panelH
	d.canvas = image.NewRGBA(image.Rect(0, 0, 960, 480))
	return d, make([]byte, d.pageBytes)
}

// oldRotate is how Present used to rotate, a pixel at a time straight into the page: what rotate
// has to match.
func oldRotate(d *Device, dst []byte) {
	img := d.canvas
	w, h := img.Rect.Dx(), img.Rect.Dy()
	sr, sg, sb, sa := d.shift[0], d.shift[1], d.shift[2], d.shift[3]
	for x := 0; x < w && x < d.panelH; x++ {
		row := dst[x*d.line : x*d.line+d.panelW*4]
		for y := 0; y < h && y < d.panelW; y++ {
			i := y*img.Stride + x*4
			px := (d.panelW - 1 - y) * 4
			pixel := uint32(img.Pix[i])<<sr | uint32(img.Pix[i+1])<<sg | uint32(img.Pix[i+2])<<sb | uint32(img.Pix[i+3])<<sa
			binary.LittleEndian.PutUint32(row[px:px+4], pixel)
		}
	}
}

func TestRotateMatchesThePixelAtATimeWay(t *testing.T) {
	for _, shift := range [][4]uint{{16, 8, 0, 24}, {0, 8, 16, 24}, {24, 16, 8, 0}} {
		d, dst := fake(shift)
		rng := rand.New(rand.NewSource(1))
		rng.Read(d.canvas.Pix)
		want := make([]byte, len(dst))
		oldRotate(d, want)
		d.rotate(dst, 0)
		if !bytes.Equal(dst, want) {
			t.Fatalf("shift %v: the first frame differs", shift)
		}
		// A second frame with a little changed, which goes through the rows that are skipped.
		for i := 0; i < 2000; i++ {
			d.canvas.Pix[rng.Intn(len(d.canvas.Pix))] ^= 0xff
		}
		oldRotate(d, want)
		d.rotate(dst, 0)
		if !bytes.Equal(dst, want) {
			t.Fatalf("shift %v: a changed frame differs", shift)
		}
	}
}

// Every pixel changed every frame, as a page scrolling is: the worst case.
func BenchmarkRotateEverythingChanged(b *testing.B) {
	d, dst := fake([4]uint{16, 8, 0, 24})
	rand.New(rand.NewSource(1)).Read(d.canvas.Pix)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		d.canvas.Pix[0]++
		for j := 4; j < len(d.canvas.Pix); j += 4 * 480 {
			d.canvas.Pix[j]++
		}
		for x := 0; x < len(d.canvas.Pix); x += 4 {
			d.canvas.Pix[x] ^= 1
		}
		d.rotate(dst, 0)
	}
}

func BenchmarkOldRotate(b *testing.B) {
	d, dst := fake([4]uint{16, 8, 0, 24})
	rand.New(rand.NewSource(1)).Read(d.canvas.Pix)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for x := 0; x < len(d.canvas.Pix); x += 4 {
			d.canvas.Pix[x] ^= 1
		}
		oldRotate(d, dst)
	}
}

// Frames that change one corner, painted onto the pages in turn as Present flips them: every page has
// to end up as the pixel-at-a-time way would draw that frame, the bands skipped as unchanged included.
func TestRotateSkipsOnlyBandsThePageAlreadyHolds(t *testing.T) {
	d, _ := fake([4]uint{16, 8, 0, 24})
	pages := make([][]byte, d.pages)
	for p := range pages {
		pages[p] = make([]byte, d.pageBytes)
	}
	rng := rand.New(rand.NewSource(2))
	rng.Read(d.canvas.Pix)
	want := make([]byte, d.pageBytes)
	for frame := range 12 {
		// A picture at the right moving a few pixels, as the character breathing does.
		for y := 100 + frame; y < 300+frame; y++ {
			for x := 700; x < 900; x++ {
				d.canvas.Pix[y*d.canvas.Stride+x*4] = byte(frame * 20)
			}
		}
		next := frame % d.pages
		oldRotate(d, want)
		d.rotate(pages[next], next)
		if !bytes.Equal(pages[next], want) {
			t.Fatalf("frame %d on page %d differs", frame, next)
		}
	}
}

// One picture moving at the right, the rest still: the idle page with the character breathing.
func BenchmarkRotateOnePictureMoves(b *testing.B) {
	d, dst := fake([4]uint{16, 8, 0, 24})
	rand.New(rand.NewSource(1)).Read(d.canvas.Pix)
	d.rotate(dst, 0)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for y := 60; y < 470; y++ {
			for x := 640; x < 940; x += 7 {
				d.canvas.Pix[y*d.canvas.Stride+x*4]++
			}
		}
		d.rotate(dst, 0)
	}
}
