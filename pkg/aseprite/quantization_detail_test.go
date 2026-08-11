package aseprite

import (
	"image"
	"image/color"
	"testing"
)

// referenceLikeImage builds the situation detail weighting exists to fix: a
// large, smoothly graded background that carries no detail, plus a smaller,
// finely textured subject. Both span many distinct colors, so both compete for
// palette slots; only their area and their local gradient differ.
func referenceLikeImage() image.Image {
	const size = 120
	img := image.NewRGBA(image.Rect(0, 0, size, size))

	// Background: a smooth gradient spanning a wide range of greens, so it
	// genuinely competes for palette slots. Adjacent pixels barely differ, so
	// its local gradient stays near zero.
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			img.Set(x, y, color.RGBA{
				R: uint8(20 + x/8),
				G: uint8(50 + y),
				B: uint8(30 + x/10),
				A: 255,
			})
		}
	}

	// Subject: a small patch whose colors change sharply from pixel to pixel,
	// over a modest color range. Same idea as a textured subject in a
	// photograph: high local contrast, limited overall gamut.
	for y := 45; y < 75; y++ {
		for x := 45; x < 75; x++ {
			img.Set(x, y, color.RGBA{
				R: uint8(120 + (x*37)%70),
				G: uint8(30 + (y*17)%40),
				B: uint8(60 + (y*53)%70),
				A: 255,
			})
		}
	}

	return img
}

// isBackgroundGreen reports whether a color belongs to the green backdrop
// rather than the subject.
func isBackgroundGreen(r, g, b int) bool {
	return g > r+12 && g > b+12
}

// countSubjectSamples counts sampled pixels that came from the subject.
func countSubjectSamples(pixels []color.Color) int {
	subject := 0
	for _, p := range pixels {
		r16, g16, b16, _ := p.RGBA()
		if !isBackgroundGreen(int(r16>>8), int(g16>>8), int(b16>>8)) {
			subject++
		}
	}
	return subject
}

func TestSampleWeightedByDetail_OverRepresentsDetailedRegions(t *testing.T) {
	img := referenceLikeImage()

	uniform := samplePixels(img, 10000)
	weighted := sampleWeightedByDetail(img, 10000, 8)

	uniformShare := float64(countSubjectSamples(uniform)) / float64(len(uniform))
	weightedShare := float64(countSubjectSamples(weighted)) / float64(len(weighted))

	if weightedShare <= uniformShare {
		t.Errorf("detail weighting should over-represent the subject: uniform share %.3f, weighted share %.3f",
			uniformShare, weightedShare)
	}

	// The subject covers 6.25% of the canvas; weighting should lift its
	// influence well above simple area share.
	if weightedShare < 2*uniformShare {
		t.Errorf("weighted share %.3f is less than twice the uniform share %.3f, weighting is too weak to matter",
			weightedShare, uniformShare)
	}
}

// These tests cover the sampling mechanism, which is what this file adds and
// what behaves deterministically. How much palette a given algorithm then
// spends on the subject is measured against real reference images instead:
// synthetic images do not reproduce the mix of area, color spread and texture
// that makes the weighting matter, and the three algorithms respond very
// differently. See ROADMAP.md for the measured results.

func TestQuantizePaletteWithDetail_ZeroStrengthMatchesUniform(t *testing.T) {
	img := referenceLikeImage()

	viaDefault, _, err := QuantizePalette(img, 8, "median_cut", false)
	if err != nil {
		t.Fatalf("QuantizePalette failed: %v", err)
	}

	viaZero, _, err := QuantizePaletteWithDetail(img, 8, "median_cut", false, 0)
	if err != nil {
		t.Fatalf("QuantizePaletteWithDetail failed: %v", err)
	}

	if len(viaDefault) != len(viaZero) {
		t.Fatalf("palette sizes differ: %d vs %d", len(viaDefault), len(viaZero))
	}
	for i := range viaDefault {
		if viaDefault[i] != viaZero[i] {
			t.Errorf("strength 0 should match uniform sampling; entry %d: %s vs %s", i, viaDefault[i], viaZero[i])
		}
	}
}

func TestSampleWeightedByDetail_FlatImageStaysUniform(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 40, 40))
	for y := 0; y < 40; y++ {
		for x := 0; x < 40; x++ {
			img.Set(x, y, color.RGBA{R: 10, G: 120, B: 30, A: 255})
		}
	}

	// A flat image has no detail to favor, so weighting must not duplicate
	// samples or change the sample count.
	weighted := sampleWeightedByDetail(img, 10000, 6)
	uniform := samplePixels(img, 10000)

	if len(weighted) != len(uniform) {
		t.Errorf("flat image sample count = %d, want %d", len(weighted), len(uniform))
	}
}

// A backdrop covering most of the frame has a small gradient of its own. If
// that baseline is not discounted, every backdrop pixel scores as detailed as
// the subject and the weighting silently does nothing.
func TestSampleWeightedByDetail_IgnoresBackdropOwnGradient(t *testing.T) {
	img := referenceLikeImage()

	weighted := sampleWeightedByDetail(img, 10000, 8)
	uniform := samplePixels(img, 10000)

	if len(weighted) == len(uniform) {
		t.Error("no samples were duplicated, so the backdrop gradient swallowed the whole detail range")
	}
}

func TestSampleWeightedByDetail_RespectsCap(t *testing.T) {
	img := referenceLikeImage()

	weighted := sampleWeightedByDetail(img, 10000, 10)
	if len(weighted) > detailSampleCap {
		t.Errorf("sample count %d exceeds cap %d", len(weighted), detailSampleCap)
	}
}
