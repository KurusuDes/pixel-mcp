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

## Experimento 2 (2026-08-11): foto de pájaro, límites de reducción

Referencia: foto de un azulejo (410x354) sobre fondo verde desenfocado —
sujeto orgánico, textura fina, fondo liso. Bastante más exigente que el
render del experimento 1.

### El fondo se come la paleta

Medido con `examples/refart` + análisis de asignación de paleta:

| Config | Slots de fondo | Slots de sujeto |
|---|---|---|
| 64x55, 16 colores, uniforme | 8 de 15 (53%) | 7 |
| 64x55, 16 colores, `detail_strength=8` | 4 de 15 (27%) | 11 |

El fondo cubre el 74% del área pero no aporta detalle. Con muestreo
uniforme se lleva más de la mitad de la paleta. De ahí salió el muestreo
ponderado por detalle (ya implementado, ver `pkg/aseprite/quantization_detail.go`).

**Limitación importante**: el peso por detalle actúa duplicando muestras,
así que solo llega a algoritmos sensibles a población. Funciona con
`kmeans`; `median_cut` elige el bucket por extensión de color y `octree`
apenas responde. Medido con kmeans.

### Hasta dónde se puede encoger

Con detalle 6, 16 colores, kmeans:

| Tamaño | Resultado |
|---|---|
| 96x83 | Excelente, claramente un azulejo |
| 64x55 | Muy bueno |
| 48x41 | Bueno, sigue siendo un pájaro reconocible |
| 32x28 | Límite: silueta correcta, cara y ojo perdidos |
| 24x21 | Degradado, la rama compite con el pájaro |
| 16x14 | Irreconocible |

### El pipeline no se adapta solo — ahí entra el agente

El hallazgo más importante: **el pipeline es puramente mecánico**. Aplica
el mismo filtro de caja sin importar el tamaño destino, sin noción de
dónde está el sujeto. Por debajo de ~48px el sujeto ocupa tan pocos
píxeles que el filtro lo mezcla con el fondo.

Prueba que lo demuestra: recortando la foto al bounding box del pájaro
(235x285, descarta ~54% de fondo) **antes** de reducir, a 32px de ancho
el resultado es dramáticamente mejor que reducir el encuadre completo al
mismo tamaño — cara, pecho y ala quedan definidos.

Es decir: la "inteligencia" no está ni puede estar en el CLI. Tiene que
venir del agente que decide encuadre, tamaño, número de colores y fuerza
de detalle mirando la imagen. El servidor debe darle las herramientas y
el feedback para hacerlo, no adivinar.

## Experimento 3 (2026-08-11): escena ilustrada, dirección de arte

Referencia: escena de isla flotante con cabaña, personajes y cielo nocturno,
1254x1254. **No es pixel art real**: medida la alineación de bordes contra
rejillas de 1 a 16 px, el "lift" sobre azar es 1.00x en todas — no hay
rejilla nativa. El 79,7% de las secuencias de color son de 1 píxel y tiene
171.653 colores únicos. Es una ilustración en estilo pixel art.

Dos problemas de dirección de arte y sus arreglos:

### El filtro de caja disuelve el contorno

Un contorno oscuro de 1px ocupa poca fracción del bloque de origen, así que
el promedio apenas se oscurece y la línea se vuelve un tono medio embarrado
— justo lo que difumina la silueta. `edge_strength` en `downsample_image`
detecta bloques con estructura oscura coherente y los sesga hacia ella.

A 128x128: los tonos medios embarrados bajan del 32,4% al 27,6% de la
imagen y la estructura oscura sube del 49,7% al 52,1%.

### La paleta se llena de casi-duplicados

Los cuantizadores devuelven exactamente los colores pedidos, los justifique
la imagen o no. El sobrante son grupos de entradas casi idénticas que
gastan slots, producen banding y —en el caso de los casi-negros— compiten
con el contorno. `min_color_distance` en `quantize_palette` colapsa cada
grupo perceptual conservando el más usado.

Pidiendo 32 colores: quedan 25 con umbral 9, y 20 con umbral 14.

**Cuidado con la escala del umbral**: `go-colorful` normaliza L* a 0-1, no a
0-100. Pasarle un deltaE convencional colapsaba la paleta entera a 1 color.
`labDeltaE` lo reexpresa y hay un test que fija negro-a-blanco en 100.

### Receta que funciona para este tipo de referencia

```bash
go run ./examples/refart -input escena.png -width 128 -colors 32 \
  -algorithm kmeans -detail 5 -edge 0.85 -merge 9
```

## Fase 1 — Pipeline de conversión de referencia (prioridad)

- [ ] **Tool compuesta `pixelize_reference`**: una sola llamada MCP que
      haga crop opcional + downsample + cuantización (+ opcionalmente
      dithering y snapping a paleta provista) y devuelva sprite + PNG.
      Hoy el cliente tiene que orquestar 4-6 tools y conocer el orden
      correcto.
- [ ] **Crop de sujeto en el pipeline**: aceptar un rectángulo de recorte
      antes del downsample. Es lo que más mejora los tamaños chicos
      (ver experimento 2) y hoy no hay forma de hacerlo sobre la imagen
      de referencia, solo sobre un sprite ya creado (`crop_sprite`).
- [ ] **Devolver el preview al agente**: para que el modelo pueda iterar
      sobre parámetros necesita *ver* el resultado. Evaluar devolver el
      PNG escalado como contenido de imagen en la respuesta MCP, no solo
      una ruta en disco.
- [ ] **`suggest_pixelization_params`**: dado un archivo de referencia,
      devolver tamaño, número de colores y fuerza de detalle sugeridos a
      partir del análisis (área de fondo plano, densidad de bordes,
      dispersión de la paleta). No para decidir por el agente, sino para
      darle un punto de partida informado en vez de que adivine.
- [ ] **Conectar `analyze_reference` con la cuantización**: permitir
      pasar una paleta explícita (p. ej. la extraída del análisis, o una
      paleta fija tipo PICO-8/DB16) a `quantize_palette` en lugar de que
      siempre derive la suya.
- [ ] **Defaults más inteligentes**: kmeans como recomendación documentada
      para conversión de referencias. Sobre el dithering no hay una
      recomendación única — ver Fase 2.
- [ ] **Param `scale` en `export_sprite`**: exportar a Nx con nearest
      neighbor sin mutar el sprite (evita el trío save_as/scale/export
      para previews).

## Fase 2 — Calidad del resultado

- [ ] **Cuándo ayuda el dithering** (los dos experimentos se contradicen):
      mejoró mucho el render sintético del experimento 1 y empeoró la foto
      del experimento 2, donde ensució el fondo liso y emborronó la cara
      del pájaro. Hipótesis a validar: ayuda en degradados sintéticos
      amplios y estorba en sujetos orgánicos a baja resolución. Hasta
      tenerlo claro, no ponerlo por defecto.
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
- [x] Muestreo ponderado por detalle en la cuantización, expuesto como
      `detail_strength` en `quantize_palette` (commit `31b2460`).
- [x] `refart` deriva el alto de la proporción original (commit `d649b45`).
- [x] Preservación de contorno en el downsample, expuesta como `edge_strength`
      en `downsample_image` (commit `ecce210`).
- [x] Fusión de colores perceptualmente cercanos, expuesta como
      `min_color_distance` en `quantize_palette` (commit `fa5e674`).
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
