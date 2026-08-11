package aseprite

import (
	"image"
	"image/color"
	"math"
	"sort"
)

// detailSampleCap bounds the weighted sample set so that boosting detailed
// regions does not make clustering arbitrarily slower than uniform sampling.
const detailSampleCap = 40000

// detailSample pairs a sampled pixel with the local gradient magnitude around it.
type detailSample struct {
	c      color.Color
	detail float64
}

// sampleWeightedByDetail samples pixels for quantization, giving pixels in
// visually detailed regions more influence over the resulting palette than
// pixels in flat ones.
//
// Uniform sampling lets a large flat area dominate the palette purely by
// covering more of the canvas. A photographic reference with a blurred
// backdrop is the common case: the backdrop can occupy most of the frame while
// carrying almost no detail, yet it claims most of the palette and starves the
// subject. Weighting by local gradient magnitude spends the palette where the
// image actually varies.
//
// detailStrength controls how strongly detail is favored. 0 reproduces uniform
// sampling; higher values push more of the palette toward detailed regions. A
// pixel at maximum local gradient is counted (1 + detailStrength) times as
// often as a pixel in a perfectly flat region, which always keeps a floor of
// one sample so flat areas stay represented.
func sampleWeightedByDetail(img image.Image, maxSamples int, detailStrength float64) []color.Color {
	if detailStrength <= 0 {
		return samplePixels(img, maxSamples)
	}

	bounds := img.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	if width < 3 || height < 3 {
		return samplePixels(img, maxSamples)
	}

	step := 1
	if total := width * height; total > maxSamples {
		step = int(math.Sqrt(float64(total) / float64(maxSamples)))
		if step < 1 {
			step = 1
		}
	}

	// Gradient magnitude is measured against neighbors one sampling step away,
	// so the detail estimate matches the resolution actually being sampled.
	samples := make([]detailSample, 0, maxSamples)

	for y := bounds.Min.Y; y < bounds.Max.Y; y += step {
		for x := bounds.Min.X; x < bounds.Max.X; x += step {
			left := clampRange(x-step, bounds.Min.X, bounds.Max.X-1)
			right := clampRange(x+step, bounds.Min.X, bounds.Max.X-1)
			up := clampRange(y-step, bounds.Min.Y, bounds.Max.Y-1)
			down := clampRange(y+step, bounds.Min.Y, bounds.Max.Y-1)

			dx := luma(img.At(right, y)) - luma(img.At(left, y))
			dy := luma(img.At(x, down)) - luma(img.At(x, up))

			samples = append(samples, detailSample{c: img.At(x, y), detail: math.Hypot(dx, dy)})
		}
	}

	if len(samples) == 0 {
		return samplePixels(img, maxSamples)
	}

	// Detail is scored relative to the median, not in absolute terms, and the
	// scale comes from a high percentile rather than the maximum.
	//
	// The median anchors "ordinary for this image": a smooth backdrop has a
	// small but non-zero gradient of its own, and without subtracting that
	// baseline a backdrop covering most of the frame scores as highly as the
	// subject. Using the maximum as the scale would instead let one hard edge
	// dwarf the moderate texture that makes up most of a real subject.
	baseline := percentileDetail(samples, 0.5)
	top := percentileDetail(samples, 0.95)

	if top-baseline <= 0 {
		// No meaningful variation in detail, so there is nothing to favor.
		pixels := make([]color.Color, len(samples))
		for i, s := range samples {
			pixels[i] = s.c
		}
		return pixels
	}
	scale := top - baseline

	// Cap the total so a high strength cannot blow up the sample set.
	budget := detailSampleCap
	if limit := int(float64(len(samples)) * (1 + detailStrength)); limit < budget {
		budget = limit
	}

	pixels := make([]color.Color, 0, budget)
	for _, s := range samples {
		weight := (s.detail - baseline) / scale
		if weight < 0 {
			weight = 0
		} else if weight > 1 {
			weight = 1
		}
		repeats := 1 + int(math.Round(detailStrength*weight))
		for i := 0; i < repeats && len(pixels) < budget; i++ {
			pixels = append(pixels, s.c)
		}
		if len(pixels) >= budget {
			break
		}
	}

	return pixels
}

// percentileDetail returns the detail value at the given quantile, used as the
// reference point for "this region is detailed" rather than the outlier
// maximum.
func percentileDetail(samples []detailSample, q float64) float64 {
	values := make([]float64, len(samples))
	for i, s := range samples {
		values[i] = s.detail
	}
	sort.Float64s(values)

	idx := int(float64(len(values)-1) * q)
	return values[idx]
}

// luma returns the perceived brightness of a color on a 0-255 scale.
func luma(c color.Color) float64 {
	r, g, b, _ := c.RGBA()
	return (0.299*float64(r>>8) + 0.587*float64(g>>8) + 0.114*float64(b>>8))
}

// clampRange clamps a value to an arbitrary inclusive range. It complements
// clamp, which is fixed to the 0-255 channel range.
func clampRange(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
