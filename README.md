# rss-cli

Lector RSS para terminal escrito en Go, con interfaz TUI basada en Bubble Tea y persistencia local en SQLite.

## Stack

- Go
- `cobra` para la CLI
- `bubbletea`, `bubbles`, `lipgloss` y `glamour` para la TUI
- `gofeed` para RSS/Atom
- `modernc.org/sqlite` para SQLite sin CGO

## Compilacion

macOS:

```bash
CGO_ENABLED=1 GO_EXTLINK_ENABLED=1 go build -ldflags='-linkmode external -buildid=' -o rss-cli .
```

Alternativa si ya compilaste el binario:

```bash
codesign --force --sign - rss-cli
```

Generico:

```bash
go build -o rss-cli .
```

Binario generado:

```bash
./rss-cli
```

## Uso

Abrir la interfaz TUI:

```bash
./rss-cli
```

Agregar un feed:

```bash
./rss-cli add https://example.com/feed.xml
```

Eliminar un feed por URL:

```bash
./rss-cli remove https://example.com/feed.xml
```

Sincronizar todos los feeds registrados:

```bash
./rss-cli sync
```

## Datos locales

La base de datos se guarda en:

```text
~/.config/rss-cli/data.db
```

Si `XDG_CONFIG_HOME` esta definido, se usa esa ruta base.

## Atajos de la TUI

- `j` / `k` o flechas: navegar listas y scroll de lectura
- `Tab`: cambiar el foco entre feeds y articulos
- `Enter`: abrir el articulo seleccionado
- `Esc`: volver atras o cerrar modales/errores
- `a`: agregar un feed
- `d`: eliminar el feed o articulo seleccionado segun el foco actual
- `r`: sincronizar el feed seleccionado
- `q` o `Ctrl+C`: salir

## Flujo basico

1. Agrega un feed con `./rss-cli add <url>` o desde la TUI con `a`.
2. Ejecuta `./rss-cli` para abrir la interfaz.
3. Selecciona un feed en el panel izquierdo.
4. Abre articulos con `Enter`.
5. Sincroniza el feed actual con `r`.

## Desarrollo

Verificar compilacion:

```bash
CGO_ENABLED=1 GO_EXTLINK_ENABLED=1 go build -ldflags='-linkmode external -buildid=' ./...
```

Ejecutar tests:

```bash
go test ./...
```

## Estado actual

Implementado:

- alta y baja de feeds
- sincronizacion de feeds
- listado de feeds y articulos
- lectura de articulos en pantalla completa
- borrado individual de articulos desde la TUI
- manejo basico de errores en overlays dentro de la TUI

Pendiente:

- tests unitarios reales
- refresco visual inmediato del estado "read" sin recargar listas
- empaquetado e instalacion formal
