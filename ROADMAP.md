# ROADMAP — Fork pixel-mcp (KurusuDes)

## Objetivo

Que un cliente MCP (Claude u otro) pueda recibir **una imagen de
referencia** y producir **pixel art basado en esa imagen**, con la menor
fricción posible, y mejorar este repo en lo que haga falta para lograrlo.

## Estado de viabilidad (experimento 2026-08-11)

**Viable.** El repo ya trae las piezas y encadenadas funcionan:

```
analyze_reference → downsample_image → quantize_palette → export_sprite
```

Harness reproducible en `examples/refart/`:

```bash
go run ./examples/refart -input foto.jpg -width 96 -height 60 -colors 32 -algorithm kmeans -dither
```

Hallazgos del experimento (referencia: wallpaper Windows 11 ThemeB,
3840x2400 → 96x60):

- `median_cut` sin dithering a 16 colores: formas bien, pero lava los
  tonos saturados minoritarios (el amarillo se fue a crema).
- `kmeans` a 32 colores con Floyd-Steinberg: claramente superior —
  conserva amarillo/naranja y los degradados del fondo.
- La paleta que extrae `analyze_reference` **no se usa** en el resto del
  pipeline: `quantize_palette` deriva la suya propia. Son dos caminos
  desconectados.
- Generar un preview inspeccionable requirió 3 llamadas extra
  (`save_as` + `scale_sprite` + `export_sprite`).

## Fase 1 — Pipeline de conversión de referencia (prioridad)

- [ ] **Tool compuesta `pixelize_reference`**: una sola llamada MCP que
      haga downsample + cuantización (+ opcionalmente dithering y
      snapping a paleta provista) y devuelva sprite + PNG. Hoy el cliente
      tiene que orquestar 4-6 tools y conocer el orden correcto.
- [ ] **Conectar `analyze_reference` con la cuantización**: permitir
      pasar una paleta explícita (p. ej. la extraída del análisis, o una
      paleta fija tipo PICO-8/DB16) a `quantize_palette` en lugar de que
      siempre derive la suya.
- [ ] **Defaults más inteligentes**: kmeans + dither como recomendación
      documentada para conversión de fotos/renders (el default actual,
      median_cut sin dither, es el caso peor del experimento).
- [ ] **Param `scale` en `export_sprite`**: exportar a Nx con nearest
      neighbor sin mutar el sprite (evita el trío save_as/scale/export
      para previews).

## Fase 2 — Calidad del resultado

- [ ] Evaluar pre-procesado antes del downsample (saturación/contraste)
      para que colores minoritarios saturados sobrevivan la cuantización.
- [ ] Probar `suggest_antialiasing` y `apply_auto_shading` como pasos
      opcionales de post-procesado del pipeline.
- [ ] Batería de imágenes de prueba variadas (foto, render, dibujo plano)
      y comparación sistemática de algoritmos/parámetros.
- [ ] Explorar preservación de bordes usando el `edge_map` de
      `analyze_reference` (hoy se calcula y se descarta).

## Fase 3 — Higiene del fork

- [x] Fix Windows: JSON con backslashes en `save_as`/`export_spritesheet`
      (commit `9a29828`).
- [ ] Arreglar los 4 tests de `pkg/aseprite` que asumen Unix
      (`echo`/`true`/`sh` como ejecutables falsos) para que corran en
      Windows, y valorar PR upstream de ambos fixes.
- [ ] CI: agregar Windows al matrix (upstream solo cubre Linux/macOS,
      verificar) o al menos documentar el gap.

## Reglas de trabajo

- Rama de trabajo: `custom/juan` (nunca push directo a `main`).
- `upstream` = repo original (willibrandon/pixel-mcp) para sincronizar.
- Leer `CLAUDE.md` (convenciones del autor) y `CLAUDE.local.md` (entorno
  Windows de esta máquina) antes de tocar código.
- `go vet ./...` + `go test -cover ./...` (sin `-race`, no hay cgo) antes
  de dar por buena cualquier modificación.
