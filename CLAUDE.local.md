# CLAUDE.local.md — Notas específicas de este fork (Windows / Juan)

Este archivo complementa (no reemplaza) el `CLAUDE.md` original del repo.
Léelos ambos. El `CLAUDE.md` del autor original describe arquitectura,
convenciones y pitfalls; este documenta el entorno real de esta máquina
y los hallazgos de la sesión de setup (2026-08-11).

## Entorno de esta máquina (Windows 11)

- **Aseprite**: 1.3.18.1-x64, instalado vía Steam en
  `C:\Program Files (x86)\Steam\steamapps\common\Aseprite\Aseprite.exe`
- **Go**: 1.26.5 (instalado con `winget install GoLang.Go`). Si `go` no
  se reconoce en una terminal nueva, refrescar el PATH o reabrir la terminal.
- **No hay `make`**: usar los comandos `go` directos (abajo).
- **No hay compilador C**: `go test -race` no funciona (requiere cgo).
  Correr los tests sin `-race`.
- **Config del servidor**: `os.UserHomeDir()` + `.config/...`, o sea en
  Windows queda en `C:\Users\Docente\.config\pixel-mcp\config.json`
  (ya creada, apunta al Aseprite de Steam y a
  `C:\Users\Docente\AppData\Local\Temp\pixel-mcp` como temp_dir).

## Comandos equivalentes sin make

```bash
# Build (en Windows el binario necesita .exe)
go build -o bin/pixel-mcp.exe ./cmd/pixel-mcp

# Tests unitarios (sin -race, ver arriba)
go test -cover ./...

# Tests de integración (usan Aseprite real)
go test -tags=integration ./...

# Lint: golangci-lint no está instalado aún en esta máquina
go vet ./...

# Health check
./bin/pixel-mcp.exe --health

# Cliente de ejemplo end-to-end
ASEPRITE_MCP_PATH=$PWD/bin/pixel-mcp.exe go run ./examples/client
```

## Estado verificado (sesión 2026-08-11)

- Build: OK a la primera (Go 1.26.5).
- `--health`: OK (encuentra Aseprite, versión, temp dir).
- Tests: `pkg/config` (95.9% cov) y `pkg/server` (79.3% cov) pasan.
- Cliente de ejemplo (`examples/client`): completa 21 de 22 pasos —
  todas las categorías de tools funcionan salvo el bug documentado abajo.

## Fallos de test conocidos en Windows (upstream, no son regresiones nuestras)

### 1. Tests de `pkg/aseprite` que asumen Unix (4 fallos, inocuos)

`TestClient_ExecuteCommand_Success`, `TestClient_GetVersion_Success`,
`TestClient_GetVersion_EmptyOutput`, `TestClient_ExecuteCommand_WithStderr`
usan `echo`, `true` y `sh` como ejecutables falsos de Aseprite. Esos
binarios no existen en Windows. Es un problema del test, no del código
productivo.

### 2. Bug real: rutas Windows rompen el JSON de salida de algunas tools — **CORREGIDO**

> **Estado**: corregido en este fork (commit `9a29828`, rama `custom/juan`).
> Se escapan los backslashes en el Lua antes del `string.format` del JSON.
> `TestSaveAs_ViaMCP` y `TestExportSpritesheet_ViaMCP` pasan en Windows.
> Se deja la descripción original como referencia:

**Síntoma**: `TestSaveAs_ViaMCP` y `TestExportSpritesheet_ViaMCP` fallan;
el cliente de ejemplo muere en el paso de `export_spritesheet`.

**Causa raíz**: los generadores Lua `SaveAs` y `ExportSpritesheet`
(`pkg/aseprite/lua_export.go`) interpolan la ruta de salida cruda dentro
del JSON que imprime el script:

```lua
print(string.format('{"success":true,"file_path":"%s"}', newPath))
```

En Windows `newPath` contiene backslashes (`C:\Users\...`), que son
secuencias de escape inválidas en JSON. El `parseJSON` del lado Go falla
y la tool devuelve `IsError=true` — **aunque la operación en sí se
ejecuta bien** (el archivo se guarda/exporta; solo se rompe el reporte).
En Linux/macOS nunca se manifiesta porque las rutas no llevan backslash.

**Candidato a primer fix del fork** (pendiente de confirmar con Juan):
escapar los backslashes de la ruta al construir el JSON de salida en Lua
(p. ej. `newPath:gsub("\\", "\\\\")` antes del `string.format`), o
normalizar a forward slashes, en `SaveAs` y `ExportSpritesheet`. Revisar
si otras tools que imprimen rutas en JSON tienen el mismo patrón
(`create_canvas` funciona porque el temp_dir se imprime desde Go, no
desde Lua — verificar caso por caso).

## Estructura confirmada (explorada, ya no es inferencia)

La sección "Package Organization" del `CLAUDE.md` original es exacta:

- `pkg/aseprite/` — capa de integración: `client.go` (ejecución batch),
  `lua_*.go` (generadores de scripts Lua por categoría), `types.go`,
  `palette.go`, `image_analysis.go`. El escapado anti-inyección vive en
  `EscapeString` (`lua_core.go`).
- `pkg/tools/` — una tool MCP por handler, agrupadas por archivo
  (canvas, drawing, selection, animation, inspection, analysis,
  dithering, quantization, auto_shading, palette_tools, transform,
  export). 50 tools registradas en total.
- `pkg/config/` — carga de config solo por archivo, sin autodetección.
- `pkg/server/` — servidor MCP (SDK oficial `modelcontextprotocol/go-sdk`).

## Estado de git

- Remoto `upstream` → https://github.com/willibrandon/pixel-mcp.git
- Rama de trabajo: `custom/juan` (no trabajar sobre `main`).
- **`origin` pendiente**: falta crear el fork en la cuenta de GitHub de
  Juan y agregarlo (`git remote add origin <url-del-fork>`). No hay `gh`
  autenticado en esta máquina.

## Objetivo del fork (definido por Juan, 2026-08-11)

Ver si es viable pasar una imagen de referencia y generar pixel art a
partir de ella, y mejorar el repo para lograrlo. Ver `ROADMAP.md` para
el plan de trabajo y `examples/refart/` para el harness de viabilidad
(pipeline: analyze_reference → downsample_image → quantize_palette →
export_sprite + preview escalado).

Resultado: **viable**. El pipeline existente ya produce pixel art
reconocible desde una foto o un render. Dos cosas a tener presentes al
tocar este código:

- `detail_strength` (añadido en este fork) solo surte efecto con `kmeans`.
  Actúa duplicando muestras, y `median_cut` elige bucket por extensión de
  color, no por población.
- El dithering no es una mejora universal: ayudó en un render sintético y
  empeoró una foto con sujeto orgánico. No ponerlo por defecto.

Los tests de `quantization_detail_test.go` cubren el **mecanismo de
muestreo**, no el resultado final de la cuantización: las imágenes
sintéticas no reproducen la mezcla de área, dispersión de color y textura
que hace que el peso importe, y los tres algoritmos responden distinto.
Las cifras de resultado están medidas sobre imágenes reales y anotadas en
`ROADMAP.md`.
