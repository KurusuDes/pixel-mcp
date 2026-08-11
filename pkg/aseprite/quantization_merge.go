package aseprite

import (
	"image/color"
	"math"
	"sort"

	"github.com/lucasb-eyer/go-colorful"
)

// mergeSimilarColors collapses palette entries that sit close together
// perceptually, keeping the most-used member of each cluster.
//
// Quantizers hand back exactly as many colors as they were asked for, whether
// or not the image justifies that many. The surplus shows up as clusters of
// near-identical entries: five dark blues that no one can tell apart, or a
// handful of near-blacks where the art wants one. Those cost palette slots,
// and they make shading band instead of step cleanly. Collapsing them yields
// a smaller, more deliberate palette, and it settles the near-black case on
// its own, since very dark colors sit close together in LAB.
//
// minDistance is a CIE76 deltaE on the conventional 0-100 scale, where black
// to white is 100. Around 2 is the threshold of visible difference; values
// near 10 merge colors that read as the same tone, and larger values flatten
// shading ramps into fewer steps. Zero disables merging.
//
// The result can be smaller than the requested palette size. That is the
// point: the caller asked for an upper bound, not a quota to fill.
func mergeSimilarColors(paletteColors, samples []color.Color, minDistance float64) []color.Color {
	if minDistance <= 0 || len(paletteColors) < 2 {
		return paletteColors
	}

	labs := make([]colorful.Color, len(paletteColors))
	for i, c := range paletteColors {
		labs[i] = toColorful(c)
	}

	// Population decides which member of a cluster survives, so merging pulls
	// rare near-duplicates into the tone the image actually uses.
	counts := make([]int, len(paletteColors))
	for _, p := range samples {
		lab := toColorful(p)
		best, bestDist := 0, math.MaxFloat64
		for i, entry := range labs {
			if d := labDeltaE(lab, entry); d < bestDist {
				bestDist, best = d, i
			}
		}
		counts[best]++
	}

	order := make([]int, len(paletteColors))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool { return counts[order[a]] > counts[order[b]] })

	kept := make([]int, 0, len(paletteColors))
	for _, candidate := range order {
		tooClose := false
		for _, k := range kept {
			if labDeltaE(labs[candidate], labs[k]) < minDistance {
				tooClose = true
				break
			}
		}
		if !tooClose {
			kept = append(kept, candidate)
		}
	}

	// Return in the original order so the palette stays stable across runs.
	sort.Ints(kept)
	merged := make([]color.Color, len(kept))
	for i, idx := range kept {
		merged[i] = paletteColors[idx]
	}
	return merged
}

// labDeltaE is colorDistanceLab restated on the conventional 0-100 deltaE
// scale. go-colorful normalizes L* to 0-1, so its raw distance is a hundredth
// of the numbers every color reference quotes, which makes any threshold
// expressed in familiar units collapse the whole palette into one color.
func labDeltaE(c1, c2 colorful.Color) float64 {
	return colorDistanceLab(c1, c2) * 100
}

// toColorful converts a color to the normalized form the LAB helpers expect.
func toColorful(c color.Color) colorful.Color {
	r, g, b, _ := c.RGBA()
	return colorful.Color{
		R: float64(r) / 65535.0,
		G: float64(g) / 65535.0,
		B: float64(b) / 65535.0,
	}
}
