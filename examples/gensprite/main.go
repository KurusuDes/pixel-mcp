// Package main generates a game sprite from scratch through the MCP server,
// with no reference image involved.
//
// This is the other half of the reference-conversion pipeline in
// examples/refart: there, an image is converted down into pixel art; here, the
// art is authored directly. The sprite is described as a character grid, one
// character per pixel, which is then mapped to a palette and drawn through
// draw_pixels. The point is that the server supplies the drawing primitives
// and the palette handling while the caller decides the shape, the shading and
// the color ramp.
//
// Usage:
//
//	go run ./examples/gensprite -outdir out
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// palette maps each grid character to a hex color. Ordering the ramps
// deliberately (dark outline, two glass tones, three liquid tones) is what
// makes the result read as pixel art rather than as a flat diagram.
var palette = map[rune]string{
	'o': "#241A2E", // outline
	'k': "#6B4423", // cork, shadow side
	'K': "#96612F", // cork, lit side
	'g': "#BFE4EE", // glass, lit side
	'G': "#8FB4C4", // glass, shadow side
	'w': "#FFFFFF", // specular highlight
	'h': "#FF8FA8", // liquid, surface and bubbles
	'l': "#E8305C", // liquid, mid tone
	'L': "#A31840", // liquid, shadow side
}

// potion is a 32x32 health potion: corked bottle, glass shoulders, liquid
// filled to just under the neck, with a single light source at the upper left.
//
// The shoulders widen by exactly one column per row. Widening faster leaves
// gaps between the outline pixels of consecutive rows, which reads as a dotted
// edge instead of a solid one at this scale.
var potion = []string{
	"................................",
	"................................",
	"................................",
	".............oooooo.............",
	".............oKKkko.............",
	".............oKKkko.............",
	".............oKKkko.............",
	".............oooooo.............",
	".............oggGGo.............",
	".............oggGGo.............",
	".............oggGGo.............",
	"............ogggGGGo............",
	"...........oggggGGGGo...........",
	"..........ogggggGGGGGo..........",
	".........oggggggGGGGGGo.........",
	"........ogggggggGGGGGGGo........",
	".......oggggggggGGGGGGGGo.......",
	"......ogggggggggGGGGGGGGGo......",
	"......ogwwggggggGGGGGGGGGo......",
	"......ohhhhhhhhhhhhhhhhhho......",
	"......olwwlllllllLLLLLLLLo......",
	"......olwllllllllLLLLLLLLo......",
	"......ollllllllllLLLLLLLLo......",
	"......ollllllllllLLhLLLLLo......",
	"......ollllllllllLLLLLLLLo......",
	"......ollllhlllllLLLLLLLLo......",
	"......ollllllllllLLLLLLLLo......",
	"......olllllllllLLLLLLLLLo......",
	"......ollllllLLLLLLLLLLLLo......",
	"......oooooooooooooooooooo......",
	"................................",
	"................................",
}

func main() {
	outdir := flag.String("outdir", "gensprite-out", "output directory")
	preview := flag.Int("preview", 10, "nearest-neighbor upscale factor for the preview PNG (0 = skip)")
	flag.Parse()

	if err := run(*outdir, *preview); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(outdir string, preview int) error {
	size, pixels, err := gridPixels(potion)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	if err := os.MkdirAll(outdir, 0755); err != nil {
		return err
	}
	absOut, err := filepath.Abs(outdir)
	if err != nil {
		return err
	}

	serverPath := os.Getenv("ASEPRITE_MCP_PATH")
	if serverPath == "" {
		for _, c := range []string{"./bin/pixel-mcp.exe", "./bin/pixel-mcp", "../../bin/pixel-mcp.exe", "../../bin/pixel-mcp"} {
			if _, err := os.Stat(c); err == nil {
				serverPath, _ = filepath.Abs(c)
				break
			}
		}
	}
	if serverPath == "" {
		return fmt.Errorf("ASEPRITE_MCP_PATH not set and bin/pixel-mcp not found")
	}

	client := mcp.NewClient(&mcp.Implementation{Name: "gensprite", Version: "0.1.0"}, nil)
	session, err := client.Connect(ctx, &mcp.CommandTransport{Command: exec.Command(serverPath)}, nil)
	if err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}
	defer session.Close()

	fmt.Printf("[1/4] create_canvas %dx%d\n", size, size)
	createRaw, err := callTool(ctx, session, "create_canvas", map[string]any{
		"width":      size,
		"height":     size,
		"color_mode": "rgb",
	})
	if err != nil {
		return fmt.Errorf("create_canvas: %w", err)
	}
	var created struct {
		FilePath string `json:"file_path"`
	}
	if err := json.Unmarshal([]byte(createRaw), &created); err != nil {
		return fmt.Errorf("could not read created sprite path: %w", err)
	}

	// Registering the palette up front makes the sprite ship with the exact
	// ramp it was authored against, which is what a game project wants.
	fmt.Printf("[2/4] set_palette (%d colors)\n", len(palette))
	if _, err := callTool(ctx, session, "set_palette", map[string]any{
		"sprite_path": created.FilePath,
		"colors":      paletteColors(),
	}); err != nil {
		return fmt.Errorf("set_palette: %w", err)
	}

	fmt.Printf("[3/4] draw_pixels (%d pixels)\n", len(pixels))
	if _, err := callTool(ctx, session, "draw_pixels", map[string]any{
		"sprite_path":  created.FilePath,
		"layer_name":   "Layer 1",
		"frame_number": 1,
		"pixels":       pixels,
		"use_palette":  false,
	}); err != nil {
		return fmt.Errorf("draw_pixels: %w", err)
	}

	fmt.Println("[4/4] export_sprite -> sprite.png")
	spritePNG := filepath.Join(absOut, "sprite.png")
	if _, err := callTool(ctx, session, "export_sprite", map[string]any{
		"sprite_path":  created.FilePath,
		"output_path":  spritePNG,
		"format":       "png",
		"frame_number": 1,
	}); err != nil {
		return fmt.Errorf("export_sprite: %w", err)
	}

	if preview > 1 {
		previewSprite := filepath.Join(absOut, "preview.aseprite")
		if _, err := callTool(ctx, session, "save_as", map[string]any{
			"sprite_path": created.FilePath,
			"output_path": previewSprite,
		}); err != nil {
			return fmt.Errorf("save_as: %w", err)
		}
		if _, err := callTool(ctx, session, "scale_sprite", map[string]any{
			"sprite_path": previewSprite,
			"scale_x":     float64(preview),
			"scale_y":     float64(preview),
			"algorithm":   "nearest",
		}); err != nil {
			return fmt.Errorf("scale_sprite: %w", err)
		}
		if _, err := callTool(ctx, session, "export_sprite", map[string]any{
			"sprite_path":  previewSprite,
			"output_path":  filepath.Join(absOut, "preview.png"),
			"format":       "png",
			"frame_number": 1,
		}); err != nil {
			return fmt.Errorf("export_sprite (preview): %w", err)
		}
	}

	// Keep the authored .aseprite alongside the exports; it is the file a
	// human would open to keep editing.
	if _, err := callTool(ctx, session, "save_as", map[string]any{
		"sprite_path": created.FilePath,
		"output_path": filepath.Join(absOut, "sprite.aseprite"),
	}); err != nil {
		return fmt.Errorf("save_as (source): %w", err)
	}

	fmt.Println("done:", absOut)
	return nil
}

// gridPixels converts a character grid into draw_pixels arguments, and fails
// loudly on a malformed grid: a row of the wrong length or an unmapped
// character would otherwise shift the whole sprite silently.
func gridPixels(grid []string) (int, []map[string]any, error) {
	size := len(grid)
	pixels := make([]map[string]any, 0, size*size)

	for y, row := range grid {
		if len(row) != size {
			return 0, nil, fmt.Errorf("row %d has %d characters, want %d (grid must be square)", y, len(row), size)
		}
		for x, ch := range row {
			if ch == '.' {
				continue
			}
			hex, ok := palette[ch]
			if !ok {
				return 0, nil, fmt.Errorf("row %d column %d uses %q, which is not in the palette", y, x, string(ch))
			}
			pixels = append(pixels, map[string]any{"x": x, "y": y, "color": hex})
		}
	}

	return size, pixels, nil
}

// paletteColors returns the palette as a stable, sorted list of hex colors.
func paletteColors() []string {
	colors := make([]string, 0, len(palette))
	for _, hex := range palette {
		colors = append(colors, hex)
	}
	sort.Strings(colors)
	return colors
}

func callTool(ctx context.Context, session *mcp.ClientSession, name string, args map[string]any) (string, error) {
	resp, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		return "", fmt.Errorf("tool call failed: %w", err)
	}
	var text string
	if len(resp.Content) > 0 {
		if tc, ok := resp.Content[0].(*mcp.TextContent); ok {
			text = tc.Text
		}
	}
	if resp.IsError {
		return "", fmt.Errorf("tool returned error: %s", text)
	}
	if text == "" {
		return "", fmt.Errorf("no text content in response")
	}
	return text, nil
}
