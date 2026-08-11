// Package main is a viability harness for the reference-image -> pixel-art
// pipeline. It exercises the existing tools end-to-end:
//
//	analyze_reference -> downsample_image -> quantize_palette -> export_sprite
//
// and additionally exports an upscaled preview (nearest neighbor) so the
// result can be inspected at a comfortable size.
//
// Usage:
//
//	go run ./examples/refart -input photo.jpg -width 96 -height 60 -colors 16 -outdir out
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	input := flag.String("input", "", "path to reference image (.png/.jpg/.bmp/.gif)")
	width := flag.Int("width", 96, "target pixel art width")
	height := flag.Int("height", 60, "target pixel art height")
	colors := flag.Int("colors", 16, "target palette size (2-256)")
	algorithm := flag.String("algorithm", "median_cut", "quantization algorithm: median_cut, kmeans, octree")
	dither := flag.Bool("dither", false, "apply Floyd-Steinberg dithering during quantization")
	outdir := flag.String("outdir", "refart-out", "output directory")
	preview := flag.Int("preview", 8, "nearest-neighbor upscale factor for the preview PNG (0 = skip)")
	flag.Parse()

	if *input == "" {
		fmt.Fprintln(os.Stderr, "error: -input is required")
		flag.Usage()
		os.Exit(1)
	}

	if err := run(*input, *width, *height, *colors, *algorithm, *dither, *outdir, *preview); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(input string, width, height, colors int, algorithm string, dither bool, outdir string, preview int) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	absInput, err := filepath.Abs(input)
	if err != nil {
		return err
	}
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

	client := mcp.NewClient(&mcp.Implementation{Name: "refart", Version: "0.1.0"}, nil)
	session, err := client.Connect(ctx, &mcp.CommandTransport{Command: exec.Command(serverPath)}, nil)
	if err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}
	defer session.Close()

	// Step 1: analyze the reference image.
	fmt.Printf("[1/5] analyze_reference (%s, palette_size=%d)\n", filepath.Base(absInput), colors)
	analysisRaw, err := callTool(ctx, session, "analyze_reference", map[string]any{
		"reference_path": absInput,
		"target_width":   width,
		"target_height":  height,
		"palette_size":   colors,
	})
	if err != nil {
		return fmt.Errorf("analyze_reference: %w", err)
	}
	var analysis struct {
		Palette []struct {
			Color string  `json:"color"`
			Usage float64 `json:"usage_percentage"`
		} `json:"palette"`
	}
	if err := json.Unmarshal([]byte(analysisRaw), &analysis); err == nil {
		fmt.Printf("      extracted %d palette colors:", len(analysis.Palette))
		for _, p := range analysis.Palette {
			fmt.Printf(" %s", p.Color)
		}
		fmt.Println()
	}
	if err := os.WriteFile(filepath.Join(absOut, "analysis.json"), []byte(analysisRaw), 0644); err != nil {
		return err
	}

	// Step 2: downsample to target resolution (box filter).
	fmt.Printf("[2/5] downsample_image -> %dx%d\n", width, height)
	spritePath := filepath.Join(absOut, "pixelart.aseprite")
	if _, err := callTool(ctx, session, "downsample_image", map[string]any{
		"source_path":   absInput,
		"target_width":  width,
		"target_height": height,
		"output_path":   spritePath,
	}); err != nil {
		return fmt.Errorf("downsample_image: %w", err)
	}

	// Step 3: quantize to a limited palette.
	fmt.Printf("[3/5] quantize_palette (%s, %d colors, dither=%v)\n", algorithm, colors, dither)
	quantRaw, err := callTool(ctx, session, "quantize_palette", map[string]any{
		"sprite_path":   spritePath,
		"target_colors": colors,
		"algorithm":     algorithm,
		"dither":        dither,
	})
	if err != nil {
		return fmt.Errorf("quantize_palette: %w", err)
	}
	var quant struct {
		OriginalColors  int      `json:"original_colors"`
		QuantizedColors int      `json:"quantized_colors"`
		Palette         []string `json:"palette"`
	}
	if err := json.Unmarshal([]byte(quantRaw), &quant); err == nil {
		fmt.Printf("      %d unique colors -> %d palette colors\n", quant.OriginalColors, quant.QuantizedColors)
	}

	// Step 4: export the pixel art at native resolution.
	fmt.Println("[4/5] export_sprite -> pixelart.png")
	resultPath := filepath.Join(absOut, "pixelart.png")
	if _, err := callTool(ctx, session, "export_sprite", map[string]any{
		"sprite_path":  spritePath,
		"output_path":  resultPath,
		"format":       "png",
		"frame_number": 1,
	}); err != nil {
		return fmt.Errorf("export_sprite: %w", err)
	}

	// Step 5: export an upscaled preview for human inspection.
	if preview > 1 {
		fmt.Printf("[5/5] preview x%d -> preview.png\n", preview)
		previewSprite := filepath.Join(absOut, "preview.aseprite")
		if _, err := callTool(ctx, session, "save_as", map[string]any{
			"sprite_path": spritePath,
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

	fmt.Println("done:", absOut)
	return nil
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
