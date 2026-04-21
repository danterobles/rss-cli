# CHANGELOG

## Unreleased

### Added

- Base del proyecto `rss-cli` en Go con `cobra` como entrada principal.
- Persistencia local en SQLite con tablas `feeds` y `articles`.
- Comandos CLI:
  - `rss-cli`
  - `rss-cli add <url>`
  - `rss-cli remove <url>`
  - `rss-cli sync`
- TUI inicial con:
  - panel de feeds
  - panel de articulos
  - vista de lectura
  - modal para agregar feed
  - confirmacion para borrado
- Splash screen con ASCII art `RSS-CLI` antes de cargar la TUI principal.
- Borrado individual de articulos desde la TUI.
- Changelog del proyecto.

### Changed

- El conteo de `sync` ahora solo considera articulos realmente nuevos y no vuelve a contar los ya existentes.
- La lectura del articulo usa desplazamiento propio por lineas para evitar fallos con `k` y flecha arriba.
- El contenido completo del articulo ya no se descarga durante `add` o `sync`.
- La extraccion del HTML del enlace original ahora se hace bajo demanda al abrir el articulo, evitando bloqueos al agregar feeds.
- El proceso de compilacion en macOS fue documentado para usar linker externo y evitar binarios invalidos para `dyld` y `codesign`.

### Fixed

- Error de `dyld` por binarios sin `LC_UUID`.
- Error de macOS `Code Signature Invalid` al ejecutar el binario compilado.
- Bloqueo al agregar un feed causado por descarga y parseo de todos los enlaces del feed durante el alta.
- Problemas de navegacion hacia arriba en la vista de lectura con `k` y flecha arriba.
- Manejo de errores mas claro en la TUI mediante overlays descartables.

### Notes

- Cuando el HTML del articulo original no puede extraerse, `rss-cli` conserva el preview del feed como fallback.
- En macOS el binario final debe compilarse con:

```bash
CGO_ENABLED=1 GO_EXTLINK_ENABLED=1 go build -ldflags='-linkmode external -buildid=' -o rss-cli .
```

- Si ya existe un binario y necesitas re-firmarlo:

```bash
codesign --force --sign - rss-cli
```
