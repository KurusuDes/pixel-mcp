package aseprite

import (
	"strings"
	"testing"
)

func TestDownsampleImage_DefaultsToPlainBoxFilter(t *testing.T) {
	g := &LuaGenerator{}
	script := g.DownsampleImage("/src.png", "/out.aseprite", 64, 48)

	if !strings.Contains(script, "local edgeStrength = 0.000") {
		t.Error("DownsampleImage should generate a plain box filter, with edge preservation disabled")
	}
}

func TestDownsampleImageEdgeAware_EmitsRequestedStrength(t *testing.T) {
	g := &LuaGenerator{}
	script := g.DownsampleImageEdgeAware("/src.png", "/out.aseprite", 64, 48, 0.85)

	if !strings.Contains(script, "local edgeStrength = 0.850") {
		t.Error("edge strength was not carried into the generated script")
	}
	for _, expected := range []string{
		"local edgeContrast",
		"local edgeMinFraction",
		"targetSprite:saveAs(\"/out.aseprite\")",
	} {
		if !strings.Contains(script, expected) {
			t.Errorf("generated script missing %q", expected)
		}
	}
}

// Out-of-range values must be clamped rather than reaching Lua, where a
// negative or oversized strength would push pixels past the dark cluster
// instead of toward it.
func TestDownsampleImageEdgeAware_ClampsStrength(t *testing.T) {
	g := &LuaGenerator{}

	if s := g.DownsampleImageEdgeAware("/src.png", "/out.aseprite", 8, 8, -3); !strings.Contains(s, "local edgeStrength = 0.000") {
		t.Error("negative strength should clamp to 0")
	}
	if s := g.DownsampleImageEdgeAware("/src.png", "/out.aseprite", 8, 8, 7.5); !strings.Contains(s, "local edgeStrength = 1.000") {
		t.Error("strength above 1 should clamp to 1")
	}
}
