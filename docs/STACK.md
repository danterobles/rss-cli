Desarrollar un lector de RSS para la terminal en Go es un proyecto excelente. Go genera binarios estáticos que no dependen de librerías externas, lo que garantiza que tu aplicación correrá de forma nativa y ultrarrápida en macOS, Linux (como tus servidores AlmaLinux) o Windows, y se verá increíble en emuladores modernos como Ghostty corriendo bajo zsh.

Para lograr una experiencia de usuario (UX) fluida, con atajos de teclado y un renderizado de texto impecable, utilizaremos el ecosistema de **Charmbracelet**, que es el estándar de oro actual para TUIs en Go.

Aquí tienes el diseño de la arquitectura y la guía paso a paso para desarrollarlo.

---

### 1. Stack Tecnológico Recomendado

* **TUI Framework:** `github.com/charmbracelet/bubbletea` (Implementa la arquitectura Elm: Modelo, Vista, Actualización).
* **Componentes UI:** `github.com/charmbracelet/bubbles` (Provee listas, inputs de texto, y un "viewport" ideal para leer artículos largos).
* **Estilos:** `github.com/charmbracelet/lipgloss` (Para bordes, colores y layouts).
* **CLI y Parámetros:** `github.com/spf13/cobra` (Para manejar los comandos de inicio rápido).
* **Parseo de RSS:** `github.com/mmcdole/gofeed` (Robusto, maneja RSS y Atom sin problemas).
* **Renderizado de Artículos:** `github.com/charmbracelet/glamour` (Para convertir el HTML/Markdown de los artículos en texto formateado y legible en la terminal).
* **Base de Datos:** `modernc.org/sqlite` (Un driver de SQLite escrito en Go puro. Evita usar `mattn/go-sqlite3` para no depender de CGO y facilitar la compilación cruzada a otros sistemas operativos).

---

### 2. Diseño de la Base de Datos (SQLite)

La persistencia de datos debe ser mínima pero eficiente. Solo necesitas dos tablas principales:

```sql
CREATE TABLE feeds (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    title TEXT NOT NULL,
    url TEXT UNIQUE NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE articles (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    feed_id INTEGER,
    title TEXT,
    link TEXT,
    content TEXT,
    published_at DATETIME,
    is_read BOOLEAN DEFAULT 0,
    FOREIGN KEY(feed_id) REFERENCES feeds(id) ON DELETE CASCADE
);
```

---

### 3. Diseño de Comandos de Consola (Cobra)

Para agilizar el inicio y el manejo desde tu `.zshrc` o scripts, tu programa debería tener la siguiente estructura de comandos:

* `rss-cli` : Inicia la TUI directamente.
* `rss-cli add <url>` : Agrega un feed a la base de datos sin abrir la UI.
* `rss-cli remove <url>` : Elimina un feed.
* `rss-cli sync` : Fuerza la descarga de nuevos artículos en segundo plano (ideal para un cronjob).

---

### 4. Diseño de la Interfaz (TUI) y Flujo de Navegación

La interfaz de Bubble Tea funcionará con una **Máquina de Estados**. Necesitas tres vistas principales:

1.  **Vista de Feeds (Panel Izquierdo):** Lista de los RSS suscritos.
2.  **Vista de Artículos (Panel Derecho):** Lista de artículos del feed seleccionado.
3.  **Vista de Lectura (Pantalla Completa):** El componente `viewport` de Bubbles que muestra el artículo renderizado con Glamour.

**Manejo de Shortcuts (Atajos de Teclado):**
* `j` / `k` o `Flechas` : Navegar arriba y abajo en las listas o scrollear el artículo.
* `Tab` : Cambiar el foco entre el panel de Feeds y el panel de Artículos.
* `Enter` : Leer el artículo seleccionado.
* `Esc` : Volver atrás (de la vista de lectura a las listas).
* `a` : Abrir un input modal en la parte inferior para escribir/pegar la URL de un nuevo RSS.
* `d` : Eliminar el feed o artículo seleccionado (con confirmación `y/n`).
* `r` : Refrescar/Sincronizar el feed actual.
* `q` o `Ctrl+C` : Salir de la aplicación.

---

### 5. Guía de Desarrollo Paso a Paso

**Paso 1: Inicialización del Proyecto**
```bash
go mod init github.com/tu-usuario/rss-cli
go get github.com/charmbracelet/bubbletea github.com/charmbracelet/bubbles github.com/charmbracelet/lipgloss
go get github.com/spf13/cobra
go get github.com/mmcdole/gofeed
go get modernc.org/sqlite
```

**Paso 2: Capa de Base de Datos y Lógica (Backend)**
Crea un paquete `db` o `storage` que inicialice el archivo `data.db` (en `~/.config/rss-cli/` por ejemplo) y maneje las operaciones CRUD para los feeds usando SQL puro o algo ligero como `sqlx`.

**Paso 3: Integración de GoFeed**
Crea un servicio que reciba una URL, use `gofeed.NewParser().ParseURL(url)`, y guarde los items resultantes en la tabla `articles` de SQLite.

**Paso 4: Implementar la CLI con Cobra**
Configura `main.go` para que evalúe si se pasaron parámetros (ej. `add`, `sync`). Si no se pasan comandos, invoca la inicialización de Bubble Tea.

**Paso 5: Construir el Modelo de Bubble Tea**
En Bubble Tea, todo reside en un `struct` que implementa tres métodos: `Init()`, `Update()`, y `View()`.

```go
type model struct {
    feeds       []Feed         // Datos de SQLite
    articles    []Article      // Datos de SQLite
    state       sessionState   // Enum: viewFeeds, viewArticles, viewReading
    list        list.Model     // Componente Bubbles para listas
    viewport    viewport.Model // Componente Bubbles para leer
    // ...
}
```

* **`Update(msg tea.Msg)`:** Aquí capturarás `tea.KeyMsg` para tus atajos de teclado (`j`, `k`, `Enter`, `q`).
* **`View()`:** Aquí usarás `lipgloss` para dibujar dos cajas (paneles) lado a lado o una caja a pantalla completa dependiendo del `state` actual.

---
