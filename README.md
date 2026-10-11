# click-ai-devkit

CLI en Go (`click`) que instala y gestiona las herramientas de desarrollo de Click Seguros sobre
gentle-ai v4.0.0. Gentle AI es dueño del flujo ODD/RDD, permisos, modelos, adaptadores de runtime y
la configuración compartida de Engram/Context7; Click conserva sus políticas de negocio/memoria,
la guardia de memoria, tres plugins de Claude Code y la inscripción de Engram Cloud.

## Qué es / qué hace

`click-ai-devkit` es un instalador y gestor, no un motor de orquestación propio. Provisiona la
versión fijada de Gentle AI para los runtimes seleccionados y sincroniza contenido específico de
Click. Claude Code recibe `click-memory`, `click-review` y `click-skills`; Codex recibe un bloque
gestionado de `AGENTS.md`; OpenClaw recibe el contenido Click que mantiene este proyecto.

Click mantiene políticas de negocio/memoria, la guardia de memoria y la inscripción de Engram Cloud.
Gentle AI es dueño del flujo ODD/RDD, permisos, selección de modelos, adaptadores de runtime y
configuración compartida de Engram/Context7. `click doctor` ofrece diagnósticos de solo lectura.

## Requisitos

- **Sistema operativo:** Click se distribuye para Windows mediante Scoop y publica artefactos de
  release para macOS/Linux. La provisión de gentle-ai v4.0.0 en Windows requiere Go **1.25.10 o
  posterior**: no hay binario Windows upstream para este pin y Click usa el módulo/tag fijados con
  `go install`. El smoke del paquete se ejecutó correctamente en un runner desechable de Windows;
  los CLIs externos se probaron con fixtures offline, sin aprovisionamiento upstream en vivo.
- **[Scoop](https://scoop.sh/):** gestor de paquetes para Windows usado para instalar y actualizar
  `click`.
- **Git:** imprescindible por dos motivos independientes:
  - Scoop actualiza sus buckets con `git pull`; si Scoop no tiene git en el PATH,
    `scoop update click` sigue leyendo el manifiesto local desactualizado y reporta la versión
    ya instalada como "la última", aunque exista un release más reciente.
  - Cuando Claude Code está seleccionado, `click install` y `click update` usan Git para agregar el
    marketplace mediante `claude plugin marketplace add`.

  > **Importante:** si una versión nueva de `click` no aparece al actualizar, instale git primero
  > (`scoop install git`) y luego ejecute `scoop update; scoop update click`. Esto es un
  > requisito propio de Scoop, independiente de Git, que Click necesita para registrar el
  > marketplace cuando Claude Code está seleccionado.

- **Go 1.25.10+ en Windows:** necesario para que Click provisione gentle-ai v4.0.0.
- **Go 1.25+ (otras plataformas):** solo necesario si se quiere compilar `click` desde el código
  fuente en lugar de instalar el binario publicado.

## Instalación

`click` se instala directamente desde este repositorio vía Scoop — GoReleaser publica el
manifiesto en la carpeta `bucket/` en cada release etiquetado, así que no hace falta un
repositorio de bucket aparte:

```powershell
scoop bucket add click https://github.com/Angel-MercadoCLK/click-ai-devkit
scoop install click
```

**Rutas de runtimes:** gentle-ai v4.0.0 resuelve sus destinos desde el home del proceso y no ofrece
flags para rutas por target. Si un target seleccionado está configurado fuera de `~/.claude`,
`~/.codex` o `~/.openclaw`, Click lo rechaza antes de crear respaldos o ejecutar provisionamiento;
no redirige la operación ni inspecciona el home real (D51).

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
| `click` (sin argumentos, en una TTY) | Lanza el menú interactivo para instalar, actualizar, diagnosticar, detectar runtimes o desinstalar. No ofrece selectores de modelos. Fuera de una TTY imprime la ayuda. |
| `click install` | Provisiona/configura gentle-ai v4.0.0 para los runtimes seleccionados; si Claude Code está seleccionado, sincroniza sus tres plugins, el bloque gestionado de `CLAUDE.md` y el hook `memory-guard`. Configura Engram Cloud cuando corresponde. `--yes`/`--non-interactive` omiten pantallas interactivas. Los flags de modelos/perfil retirados se rechazan antes de modificar el estado. |
| `click update` | Actualiza los componentes gentle-ai seleccionados y sincroniza el contenido Click administrado, incluida la inscripción Cloud cuando corresponde. No administra elecciones de modelos. |
| `click doctor` | Diagnóstico de solo lectura del pin gentle-ai y del contenido/integraciones Click seleccionados. No instala ni repara estado. |
| `click plugins` | Lista los tres plugins Claude gestionados y el staging local. Es solo lectura. |
| `click targets` | Detecta Claude Code, OpenClaw y Codex y resume las capacidades soportadas de cada target. |
| `click configure-targets` | Selector interactivo (TUI) de Claude Code, Codex y OpenClaw. Persiste la selección en `targets.json`; gentle-ai solo recibe los runtimes seleccionados. |
| `click configure-openclaw-model` | Comando de compatibilidad retirado; devuelve una deprecación sin modificar configuración. Use la configuración nativa de OpenClaw. |
| `click uninstall` | Quita el contenido Click gestionado y desinstala solo plugins Click presentes en el ámbito de usuario, conservando sus datos. Preserva la configuración compartida de Gentle AI (incluidos Engram/Context7) y contenido ajeno. No elimina el marketplace ni desregistra dependencias compartidas. |
| `click rollback --yes` | Restaura del último snapshot solo el contenido cuya propiedad Click puede verificarse; preserva el token Cloud actual, texto/configuración ajenos, estado compartido y rutas sin prueba suficiente. |
| `click agent-builder` | Asistente interactivo (TUI) para crear un agente propio de Claude Code, personal o compartible, guiando al desarrollador paso a paso hasta generar y colocar el archivo `.md` del agente. |
| `click manage-backups` | Comando de compatibilidad para inspeccionar/restaurar/eliminar respaldos históricos de modelos; Click ya no los genera ni administra durante install/update. |
| `click configure-models` | Comando de compatibilidad retirado; devuelve una deprecación sin abrir el TUI ni cambiar configuración. Para roles compatibles, use Configure Models de gentle-ai. Click no define una interfaz de selección de modelos para OpenClaw. |
| `click --version` | Imprime la versión del CLI, inyectada en tiempo de compilación vía `ldflags` (`internal/version`). |

## Qué se instala

- Tres plugins de Claude Code (`click-memory`, `click-review` y `click-skills`) registrados por el
  flujo nativo de marketplace. Click ya no incluye el plugin `click-sdd` ni un orquestador propio.
- Un bloque gestionado en `~/.claude/CLAUDE.md`, delimitado por marcadores, para poder insertarlo,
  reemplazarlo o eliminarlo por completo sin tocar el resto del archivo.
- La entrada del hook `memory-guard` (PreToolUse) en `~/.claude/settings.json`.
- Gentle AI v4.0.0 con el preset `full-gentleman`, los runtimes seleccionados, los componentes
  `engram,skills,context7,permissions,persona` y la persona `neutral` (D49). Gentle AI es dueño del
  binario/registro de Engram y de Context7; Click no duplica esas entradas.
- OpenClaw recibe la guía del workspace administrada por Click y la skill `clickhola`; Codex recibe
  el bloque Click de `AGENTS.md`. Click no escribe modelos nativos en esos runtimes. Consulte el
  [runbook de portabilidad](documentacion/portability-runbook.md) y el [target Codex](documentacion/codex-target.md).
- Un hook `memory-guard` de Click y la inscripción opcional de Engram Cloud. El rollback preserva
  el token Cloud actual; uninstall quita solo la inscripción local de Click y conserva el registro
  y la configuración compartidos upstream.

## El menú interactivo

Ejecutar `click` sin argumentos en una terminal interactiva abre un menú visual con logo de marca
(el "spark" de Click AI Devkit) y las siguientes opciones:

- Iniciar instalación
- Actualizar herramientas
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

### Avisos de actualización

Al ejecutar `click` de forma interactiva (sin argumentos, en una TTY), el CLI consulta GitHub como
máximo una vez cada 24 horas para detectar una versión nueva. Si existe, muestra un aviso en español
antes del menú y las instrucciones de actualización según la instalación:

```
Hay una nueva versión de click disponible: v{latest} (tienes v{current}).
[i] La instalación está gestionada por Scoop desde el bucket <bucket>.
[i] Para actualizar, ejecuta: scoop update; scoop update click

o

[i] La nueva versión debe descargarse manualmente.
[i] Visita: https://github.com/Angel-MercadoCLK/click-ai-devkit/releases/latest
```

Nota: `click update` sincroniza el devkit (plugins y componentes de Gentle AI), no el binario `click`.
Actualice el binario por la vía indicada en el aviso.

- El caché vive en `<CLICK_STATE_HOME>/update-check.json` (normalmente `~/.config/click-ai-devkit/update-check.json` en Linux/macOS y `%AppData%\click-ai-devkit\update-check.json` en Windows).
- Las compilaciones de desarrollo (`version == "dev"`) no consultan actualizaciones.
- `CLICK_NO_SELF_UPDATE=1` desactiva esta consulta.
- La consulta usa la API de Releases de GitHub con un límite de 2 segundos; `GITHUB_TOKEN` o `GH_TOKEN` pueden aumentar el límite, pero no son obligatorios.
- Los errores de red, caché o análisis son silenciosos y no bloquean el menú.

## memory-guard

`memory-guard` es un hook PreToolUse de Claude Code que intercepta cada llamada a una herramienta
de Engram que persiste contenido (`mem_save`, `mem_update`, `mem_save_prompt`,
`mem_session_summary`, `mem_session_end`, `mem_capture_passive`, `mem_judge`, `mem_compare`), tanto
con el nombre del plugin (`mcp__plugin_engram_engram__*`) como con el del servidor MCP independiente
(`mcp__engram__*`), antes de que pueda llegar a Engram. Las herramientas de solo lectura
(`mem_search`, `mem_context`, `mem_get_observation`, ...) no se escanean. La lista vive en
`internal/guard/tools.go`; `click update` migra un hook antiguo que solo cubría `mem_save`, y
`click doctor` lo marca como desactualizado hasta entonces. El plugin de OpenClaw aplica la misma
lista, pero esa aplicación no se ha verificado en un runtime real de OpenClaw. Es:

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
- `internal/cli/` — comandos Cobra de instalación, actualización, diagnóstico, desinstalación y
  utilidades de Click; los selectores de modelos se conservan solo como mensajes de deprecación.
- `internal/installer/` — provisionamiento de Gentle AI y sincronización/rollback del contenido
  Click propio.
- `internal/doctor/` — chequeos de salud de solo lectura.
- `internal/guard/` — el motor de coincidencia de patrones de memory-guard.
- `internal/audit/` — logging de auditoría solo-hash para las decisiones de la guardia.
- `internal/agentbuilder/` — lógica del asistente `agent-builder`: especificación del agente,
  motores disponibles, validación y escritura del archivo final.
- `internal/manifest/` — el manifiesto de release embebido (pines de versión de plugins y Engram).
- `internal/menu/` — el menú interactivo permanente (`click` sin argumentos en una TTY).
- `internal/modelconfig/` — compatibilidad con artefactos históricos de modelos; no es una interfaz
  activa de selección ni propiedad del runtime.
- `internal/ui/` — pantallas TUI compartidas y utilidades de render.
- `internal/version/` — metadatos de versión inyectados en tiempo de compilación vía `ldflags`.
- `internal/crossplatformlint/` — comprobaciones de lint específicas para mantener el código
  compatible entre plataformas (Windows/macOS/Linux).
- `plugins/` — los árboles de código fuente de los tres plugins servidos por el marketplace de
  Click (`click-memory`, `click-review`, `click-skills`).
- `bucket/` — el manifiesto de Scoop (`click.json`), publicado automáticamente por GoReleaser en
  cada release.

## Documentación adicional

La documentación vigente e histórica se organiza en [`documentacion/README.md`](documentacion/README.md):

- [`documentacion/implementation-plan.md`](documentacion/implementation-plan.md) — plan histórico
  de construcción.
- [`documentacion/tech-spec.md`](documentacion/tech-spec.md) — especificación técnica histórica.
- [`documentacion/00-decisions-and-open-questions.md`](documentacion/00-decisions-and-open-questions.md)
  — registro de decisiones (incluida D13, el mandato de TDD estricto) y preguntas abiertas.
- [`documentacion/vision.md`](documentacion/vision.md), [`documentacion/architecture.md`](documentacion/architecture.md),
  [`documentacion/prd.md`](documentacion/prd.md) — visión, arquitectura y PRD históricos.
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

## Release metadata and current contracts

Release data has independent sources: GoReleaser injects the CLI version from the release tag via `ldflags`; `internal/manifest/manifest.yaml` embeds runtime dependency pins; and `bucket/click.json` describes the currently published Scoop artifact. Validate Scoop version, archive URL, and SHA-256 against one another, and validate the packaged CLI version independently.

Para actualizar el binario de `click` mediante Scoop, la actualización del bucket requiere Git. Si
Scoop indica que la versión instalada ya es la última mientras existe una etiqueta más nueva,
ejecute `scoop update`, confirme que Git está instalado y vuelva a intentar `scoop update click`.
