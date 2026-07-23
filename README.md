# click-ai-devkit

CLI en Go (`click`) que instala y gestiona el kit de desarrollo de Click Seguros para Claude Code,
con soporte adicional para OpenClaw y Codex en sus límites nativos documentados.

## Qué es / qué hace

`click-ai-devkit` es un instalador y gestor de línea de comandos, escrito en Go, que deja
configurado el flujo SDD (Spec-Driven Development) interno de la compañía: un orquestador propio
(`ClickOrchestrator`), agentes y skills especializados, una guardia de memoria determinística, y
una instancia de Engram (memoria persistente) integrada y fijada a una versión concreta.

El CLI en sí es deliberadamente delgado: no es el "cerebro" de la orquestación, sino la
herramienta que registra, actualiza y desinstala el sistema mediante el flujo nativo de Claude Code,
y escribe la guía/archivos soportados para OpenClaw y Codex sin asumir APIs nativas no verificadas.
`click doctor` comprueba en cualquier momento que el estado gestionado sigue siendo consistente.

## Requisitos

- **Sistema operativo:** el flujo de instalación soportado y probado es **Windows**, vía Scoop.
  GoReleaser también publica binarios para macOS y Linux en cada release (ver sección
  [Instalación](#instalación)), pero hoy no existe un gestor de paquetes (brew, apt, etc.)
  integrado para esas plataformas; el tap de Homebrew está contemplado pero diferido (ver
  `.goreleaser.yaml`).
- **[Scoop](https://scoop.sh/):** gestor de paquetes para Windows usado para instalar y actualizar
  `click`.
- **Git:** imprescindible por dos motivos independientes:
  - Scoop actualiza sus buckets con `git pull`; si Scoop no tiene git en el PATH,
    `scoop update click` sigue leyendo el manifiesto local desactualizado y reporta la versión
    ya instalada como "la última", aunque exista un release más reciente.
  - `click install` y `click update` usan git internamente (a través de
    `claude plugin marketplace add`) para registrar el marketplace de plugins de Click.

  > **Importante:** si una versión nueva de `click` no aparece al actualizar, instale git primero
  > (`scoop install git`) y luego ejecute `scoop update; scoop update click`. Esto es un
  > requisito propio de Scoop, independiente del git que `click install`/`click update` necesitan
  > para registrar el marketplace de plugins.

- **Go 1.25+ (opcional):** solo necesario si se quiere compilar `click` desde el código fuente en
  lugar de instalar el binario publicado.

## Instalación

`click` se instala directamente desde este repositorio vía Scoop — GoReleaser publica el
manifiesto en la carpeta `bucket/` en cada release etiquetado, así que no hace falta un
repositorio de bucket aparte:

```powershell
scoop bucket add click https://github.com/Angel-MercadoCLK/click-ai-devkit
scoop install click
```

### Compilar desde el código fuente

Si prefiere compilar el binario usted mismo (por ejemplo, para probar un cambio local):

```powershell
go build ./cmd/click
```

Esto genera un ejecutable `click` (o `click.exe` en Windows) en el directorio actual, usando el
mismo punto de entrada (`cmd/click/`) que usa el proceso de release oficial.

## Comandos

| Comando | Qué hace |
|---|---|
| `click` (sin argumentos, en una TTY) | Lanza el menú interactivo (`internal/menu`) para llegar a instalar/actualizar/diagnosticar/desinstalar/configurar modelos sin memorizar flags. Fuera de una TTY, o con `--no-interactive`, imprime la ayuda en su lugar. |
| `click install` | Registra el marketplace de Click con `claude plugin marketplace add`, instala los plugins `click-sdd`, `click-memory`, `click-review` y `click-skills` vía el CLI nativo `claude plugin`, escribe/actualiza el bloque gestionado de `CLAUDE.md`, registra el hook `memory-guard` (PreToolUse) y permite elegir los modelos por fase (o un perfil de orquestación) de forma interactiva. Acepta `--yes`/`--non-interactive` para saltar el TUI y `--profile` para fijar el perfil (`balanced`/`cost-saver`/`quality`) sin interacción. |
| `click update` | Vuelve a sincronizar los cuatro plugins, reescribe el bloque gestionado de `CLAUDE.md`, re-registra el hook `memory-guard` y sincroniza el pin de Engram a la versión fijada en el manifiesto de release. |
| `click doctor` | Chequeo de salud de solo lectura: verifica que los cuatro plugins estén realmente registrados en Claude Code, que exista el bloque gestionado de `CLAUDE.md`, y que el hook `memory-guard` esté registrado. Nunca modifica el estado. |
| `click plugins` | Lista los cuatro plugins gestionados, su estado en los registros de Claude Code y el repositorio/staging local. Es solo lectura: dejar archivos allí no los instala ni los activa. |
| `click targets` | Detecta Claude Code, OpenClaw y Codex y resume las capacidades soportadas de cada target. |
| `click configure-targets` | Selector interactivo (TUI) de los runtimes que Click debe instalar/actualizar (Claude Code siempre primario; OpenClaw y Codex opcionales). Persiste la elección en `targets.json`. Fuera de una TTY imprime una guía y no cambia nada. |
| `click configure-openclaw-model` | Configura el modelo nativo de OpenClaw (`<provider/model>` + fallbacks opcionales) delegando en el CLI oficial de OpenClaw, sin editar su config a mano. Sin argumentos imprime la guía de uso. |
| `click uninstall` | Revierte todo lo que `install`/`update` escribieron: desinstala los cuatro plugins vía el CLI nativo `claude plugin`, quita el registro del marketplace de Click, elimina el bloque gestionado de `CLAUDE.md`, da de baja el hook `memory-guard`, y quita la configuración/estado de Engram y Context7 si `click` los llegó a instalar. Idempotente. |
| `click agent-builder` | Asistente interactivo (TUI) para crear un agente propio de Claude Code, personal o compartible, guiando al desarrollador paso a paso hasta generar y colocar el archivo `.md` del agente. |
| `click manage-backups` | Comando de solo flags (oculto de `--help`, alcanzable desde el menú) para inspeccionar, restaurar (`--restore`) o eliminar (`--delete`) la copia de seguridad de `models.json` que genera una migración de configuración obsoleta. |
| `click configure-models` | Reabre el selector interactivo de modelos por fase (18 fases, o un perfil de orquestación completo) sin pasar por una instalación/actualización completa, preservando el perfil actualmente guardado. Oculto de `--help`, se alcanza principalmente desde el menú interactivo. |
| `click --version` | Imprime la versión del CLI, inyectada en tiempo de compilación vía `ldflags` (`internal/version`). |

## Qué se instala

- Cuatro plugins de Claude Code registrados a través del flujo nativo de marketplace/registro:
  `click-sdd` (el flujo SDD, sus agentes y sus 18 fases configurables por modelo), `click-memory`
  (política y curación de memoria), `click-review` (revisión de PR y checklist de pre-merge) y
  `click-skills` (skills de ingeniería de Click Seguros para backend .NET, frontend Next.js/React
  y móvil Ionic/Capacitor).
- Un bloque gestionado en `~/.claude/CLAUDE.md`, delimitado por marcadores, para poder insertarlo,
  reemplazarlo o eliminarlo por completo sin tocar el resto del archivo.
- La entrada del hook `memory-guard` (PreToolUse) en `~/.claude/settings.json`.
- Engram instalado como plugin de Claude Code (`engram@engram`), de forma idempotente y
  respetuosa con una instalación previa; su binario se provee vía `go install` cuando falta. Un
  pequeño archivo de estado de `click` registra qué instaló `click` exactamente, para que
  `uninstall` nunca elimine un Engram que el desarrollador ya tenía instalado por su cuenta.
- Context7 registrado como MCP HTTP de ámbito de usuario vía `claude mcp add` — también
  idempotente y respetuoso con una configuración previa.
- OpenClaw recibe los archivos SDD nativos soportados, memory guard y la configuración de modelo
  solo mediante sus comandos documentados; consulte [`documentacion/portability-runbook.md`](documentacion/portability-runbook.md).
- Codex recibe la guía gestionada en `AGENTS.md`; Click no modifica `config.toml`, credenciales ni
  modelos. Consulte [`documentacion/codex-target.md`](documentacion/codex-target.md).

## El menú interactivo

Ejecutar `click` sin argumentos en una terminal interactiva abre un menú visual con logo de marca
(el "spark" de Click AI Devkit) y las siguientes opciones:

- Iniciar instalación
- Actualizar herramientas
- Configurar modelos
- Ejecutar diagnóstico
- Plugins
- Detectar runtimes compatibles
- Desinstalar
- Crear agente propio
- Gestionar backups
- Salir

La navegación se hace con las flechas ↑/↓ o con `j`/`k` (estilo vim), `Enter` selecciona la
opción resaltada, y `q`/`Esc` sale del menú. Cada opción dispara internamente el mismo comando
que existe como subcomando de `click` (por ejemplo, "Iniciar instalación" ejecuta `click
install`), así que el menú es una capa de descubrimiento, no un camino paralelo. Fuera de una TTY
(scripts, CI) `click` sin argumentos imprime la ayuda en vez de intentar abrir el menú.

## memory-guard

`memory-guard` es un hook PreToolUse de Claude Code que intercepta cada llamada a `mem_save`
antes de que pueda llegar a Engram. Es:

- **Solo bloqueo:** si el contenido coincide con un patrón prohibido, la llamada se deniega por
  completo; todavía no existe una ruta de redacción/edición parcial (está contemplada, pero sin
  fecha).
- **Fail-closed:** cualquier error interno (fallo al decodificar el payload, fallo al cargar los
  patrones, panic) también resulta en una denegación, nunca en una autorización silenciosa.
- **Auditoría solo con hash:** cada decisión se añade a un log JSONL local
  (`~/.claude/logs/click-memory-guard.jsonl`) que contiene únicamente un hash SHA-256 del
  payload, la decisión, la categoría y el id de sesión — nunca el contenido en crudo
  (`internal/audit`).

## Estructura del repositorio

- `cmd/click/` — punto de entrada del CLI.
- `internal/cli/` — árbol de comandos de cobra (install/update/doctor/uninstall/agent-builder/
  manage-backups/configure-models/memory-guard).
- `internal/installer/` — lógica de instalación/desinstalación: plugins, bloque de `CLAUDE.md`,
  registro del hook, configuración de MCP de Engram y Context7.
- `internal/doctor/` — chequeos de salud de solo lectura.
- `internal/guard/` — el motor de coincidencia de patrones de memory-guard.
- `internal/audit/` — logging de auditoría solo-hash para las decisiones de la guardia.
- `internal/agentbuilder/` — lógica del asistente `agent-builder`: especificación del agente,
  motores disponibles, validación y escritura del archivo final.
- `internal/manifest/` — el manifiesto de release embebido (pines de versión de plugins y Engram).
- `internal/menu/` — el menú interactivo permanente (`click` sin argumentos en una TTY).
- `internal/modelconfig/` — la taxonomía de 18 fases por modelo, sus valores por defecto y los
  perfiles de orquestación.
- `internal/ui/` — pantallas compartidas de TUI en bubbletea (selección de modelo, selección de
  perfil, utilidades de render).
- `internal/version/` — metadatos de versión inyectados en tiempo de compilación vía `ldflags`.
- `internal/crossplatformlint/` — comprobaciones de lint específicas para mantener el código
  compatible entre plataformas (Windows/macOS/Linux).
- `plugins/` — los árboles de código fuente de los cuatro plugins servidos por el marketplace de
  Click (`click-sdd`, `click-memory`, `click-review`, `click-skills`).
- `bucket/` — el manifiesto de Scoop (`click.json`), publicado automáticamente por GoReleaser en
  cada release.

## Documentación adicional

La documentación de planeación y diseño vive en [`documentacion/`](documentacion/), entre otros:

- [`documentacion/implementation-plan.md`](documentacion/implementation-plan.md) — plan de
  construcción e historial de slices.
- [`documentacion/tech-spec.md`](documentacion/tech-spec.md) — especificación técnica.
- [`documentacion/00-decisions-and-open-questions.md`](documentacion/00-decisions-and-open-questions.md)
  — registro de decisiones (incluida D13, el mandato de TDD estricto) y preguntas abiertas.
- [`documentacion/vision.md`](documentacion/vision.md), [`documentacion/architecture.md`](documentacion/architecture.md),
  [`documentacion/prd.md`](documentacion/prd.md) — visión, arquitectura y PRD del proyecto.
- [`documentacion/codex-target.md`](documentacion/codex-target.md) — límites y flujo del target Codex.
- [`documentacion/portability-runbook.md`](documentacion/portability-runbook.md) — validación del flujo portable y OpenClaw.

## Desarrollo

```powershell
go build ./...
go test ./...
```

TDD estricto es obligatorio para cualquier cambio en Go en este repositorio: primero se escribe
una prueba que falla, luego la implementación mínima para que pase. Ver la decisión D13 en
`documentacion/00-decisions-and-open-questions.md` y `CLAUDE.md`.

## Versión actual

**v0.4.7** es la última versión publicada vía `scoop install click` (ver [Instalación](#instalación)).
El árbol de trabajo puede estar adelantado respecto al último tag; la versión de release vive en
`click_version` dentro del manifiesto (`internal/manifest/manifest.yaml`).
