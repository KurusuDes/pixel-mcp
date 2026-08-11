package aseprite

import (
	"image/color"
	"testing"
)

// go-colorful normalizes L* to 0-1 rather than the 0-100 of textbook CIELAB,
// so a distance taken straight from it is a hundredth of a familiar deltaE.
// mergeSimilarColors takes its threshold in the familiar units, and this
// pins that down: black to white is the maximum, and it must read as 100.
func TestLabDeltaE_UsesStandardScale(t *testing.T) {
	black := toColorful(color.RGBA{R: 0, G: 0, B: 0, A: 255})
	white := toColorful(color.RGBA{R: 255, G: 255, B: 255, A: 255})

	got := labDeltaE(black, white)
	if got < 99 || got > 101 {
		t.Errorf("black to white deltaE = %.2f, want about 100; the threshold scale is off", got)
	}
}

func TestMergeSimilarColors_CollapsesNearBlacks(t *testing.T) {
	palette := []color.Color{
		color.RGBA{R: 8, G: 8, B: 12, A: 255},   // near-blacks that should
		color.RGBA{R: 14, G: 13, B: 20, A: 255}, // read as one tone
		color.RGBA{R: 18, G: 18, B: 24, A: 255},
		color.RGBA{R: 230, G: 90, B: 40, A: 255}, // clearly distinct
		color.RGBA{R: 60, G: 160, B: 70, A: 255},
	}
	samples := make([]color.Color, 0, 200)
	for _, c := range palette {
		for i := 0; i < 40; i++ {
			samples = append(samples, c)
		}
	}

	merged := mergeSimilarColors(palette, samples, 10)

	if len(merged) != 3 {
		t.Errorf("merged palette has %d colors, want 3 (one dark tone plus the two distinct colors)", len(merged))
		for _, c := range merged {
			r, g, b, _ := c.RGBA()
			t.Logf("  kept #%02X%02X%02X", r>>8, g>>8, b>>8)
		}
	}
}

func TestMergeSimilarColors_KeepsDistinctColors(t *testing.T) {
	palette := []color.Color{
		color.RGBA{R: 220, G: 40, B: 40, A: 255},
		color.RGBA{R: 40, G: 200, B: 60, A: 255},
		color.RGBA{R: 50, G: 70, B: 220, A: 255},
		color.RGBA{R: 240, G: 230, B: 90, A: 255},
	}
	samples := append([]color.Color{}, palette...)

	merged := mergeSimilarColors(palette, samples, 10)
	if len(merged) != len(palette) {
		t.Errorf("merged %d distinct colors down to %d; none should have been collapsed", len(palette), len(merged))
	}
}

func TestMergeSimilarColors_ZeroDistanceIsNoOp(t *testing.T) {
	palette := []color.Color{
		color.RGBA{R: 10, G: 10, B: 10, A: 255},
		color.RGBA{R: 11, G: 11, B: 11, A: 255},
	}
	merged := mergeSimilarColors(palette, palette, 0)
	if len(merged) != 2 {
		t.Errorf("zero distance should disable merging, got %d colors", len(merged))
	}
}

// The survivor of a cluster should be the tone the image actually uses, not
// whichever entry happened to come first.
func TestMergeSimilarColors_KeepsMostUsedOfCluster(t *testing.T) {
	rare := color.RGBA{R: 20, G: 20, B: 28, A: 255}
	common := color.RGBA{R: 26, G: 26, B: 34, A: 255}

	samples := []color.Color{rare}
	for i := 0; i < 50; i++ {
		samples = append(samples, common)
	}

	merged := mergeSimilarColors([]color.Color{rare, common}, samples, 10)
	if len(merged) != 1 {
		t.Fatalf("expected the pair to merge, got %d colors", len(merged))
	}

	r, g, b, _ := merged[0].RGBA()
	wr, wg, wb, _ := common.RGBA()
	if r != wr || g != wg || b != wb {
		t.Errorf("kept #%02X%02X%02X, want the more common #%02X%02X%02X", r>>8, g>>8, b>>8, wr>>8, wg>>8, wb>>8)
	}
}
