import { app, BrowserWindow, desktopCapturer, dialog, globalShortcut, Menu, nativeImage, safeStorage, screen, session, shell, Tray } from "electron";
import type { Server } from "node:http";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { agentHookFile, createAgentHookToken, installAgentHook, releaseAgentRequests, removeAgentHookFile, writeAgentHookFile } from "./agent-hook.mts";
import { createAttachmentStore, installAttachments, type AttachmentStore } from "./attachments.mts";
import { createCaptureController, createCaptureService, describeCapture, installCaptureController, type CaptureController, type CaptureService } from "./capture.mts";
import { createClaudeTaskRegistry, installClaudeTasks, type ClaudeTaskRegistry } from "./claude-tasks.mts";
import { createCodexAppServer, installCodexEngine, installSystemBrowser, type CodexAppServer } from "./codex-app-server.mts";
import { configureEnvironment, DOT_PORT, dotOrigin, mascotUrl, TOGGLE_SHORTCUT } from "./config.mts";
import { controlCookieDetails, createControlToken, installControlToken } from "./control-token.mts";
import { createCursorFeed, type CursorFeed } from "./cursor.mts";
import { captureControllerScreenSource, codexConfigOverrides, codexMcpEnv, startDotMcpServer, type DotMcpServer } from "./dot-mcp.mts";
import { detectExternalMcp, installExternalMcp } from "./external-mcp.mts";
import { PortInUseError } from "./listen.mts";
import { createMascotController, installWindowController, mascotWindowOptions, type MascotController, type Point, type Rect } from "./mascot.mts";
import { isAllowedNavigation, isSignInUrl, systemBrowserTarget } from "./navigation.mts";
import { createPreferenceStore, DEFAULT_PREFERENCES, initialPreferences, LOGIN_ITEM_ARGS, loadPreferences, mascotStartup, shouldManageLoginItem } from "./preferences.mts";
import { installStorageProtector } from "./protector.mts";
import { createWindowsOcr } from "./ocr.mts";
import { loadGuardPatterns, type CompiledGuardRule } from "./redaction.mts";
import { createScreenCapture } from "./screen-capture.mts";
import { createNativeRedactor, createScreenPreparer } from "./screen-prep.mts";
import { startDotServer } from "./server.mts";
import { createToolRunner, installToolRunner } from "./tool-runner.mts";
import { installVoiceController } from "./speech.mts";

const assets = join(dirname(fileURLToPath(import.meta.url)), "..", "assets");

let window: BrowserWindow | undefined;
let mascot: MascotController | undefined;
/** Pushes the cursor to the island page for the character's eye; it polls only while the island shows something to look at. */
let cursorFeed: CursorFeed | undefined;
let pendingPosition: Point | undefined;
let positionTimer: NodeJS.Timeout | undefined;
let tray: Tray | undefined;
let server: Server | undefined;
let quitting = false;
let reloadOnFocus = false;
let started = false;
let showWhenStarted = false;
let preferences: ReturnType<typeof createPreferenceStore> | undefined;
let captureService: CaptureService | undefined;
let captureController: CaptureController | undefined;
/** The Claude Code hook discovery file and the token it holds; both exist only while the server listens. */
let hookDiscovery: { file: string; token: string } | undefined;
/** The Codex engine and what it needs: Dot's MCP server, the background Claude Code tasks, the files the user drops on the chat. */
let codexServer: CodexAppServer | undefined;
let dotMcp: DotMcpServer | undefined;
let claudeTasks: ClaudeTaskRegistry | undefined;
let attachments: AttachmentStore | undefined;

function showWindow() {
  if (!window || window.isDestroyed()) createMascot();
  mascot?.show();
}

/** The tray item "Ocultar Dot" / "Mostrar Dot": the user hides the island until they show it again. The choice is saved. */
function toggleIslandHidden() {
  if (!window || window.isDestroyed()) createMascot();
  if (mascot) mascot.setHidden(!mascot.state().hidden);
}

/** Ctrl+Shift+Space: the chat card opens on the island, or closes when it is the open one; a hidden island comes back with it. */
function toggleWindow() {
  if (!window || window.isDestroyed()) createMascot();
  mascot?.toggleChat();
}

function openExternal(url: string) {
  const target = systemBrowserTarget(url);
  if (!target) return;
  // The sign-in finishes in the system browser; refresh this window when the user comes back.
  if (isSignInUrl(url)) reloadOnFocus = true;
  void shell.openExternal(target);
}

/** Work areas of every display, primary first. */
function workAreas(): Rect[] {
  const primary = screen.getPrimaryDisplay();
  return [primary, ...screen.getAllDisplays().filter((display) => display.id !== primary.id)].map((display) => display.workArea);
}

/** Dragging reports a position many times a second; write it once the mascot settles. */
function schedulePositionSave(position: Point) {
  pendingPosition = position;
  clearTimeout(positionTimer);
  positionTimer = setTimeout(flushPosition, 400);
}

function flushPosition() {
  clearTimeout(positionTimer);
  const position = pendingPosition;
  pendingPosition = undefined;
  if (!position) return;
  preferences?.update({ mascotPosition: position }).catch((error: unknown) => console.error("Could not save the mascot position.", error));
}

/** Tells the page the shell changed the mode (shortcut or tray). The script is a constant; nothing is interpolated. */
function announceMode(created: BrowserWindow) {
  if (created.isDestroyed()) return;
  created.webContents.executeJavaScript('window.dispatchEvent(new Event("click-dot:mode"))').catch(() => {});
}

/** Tells the page Claude Code needs the user, so it reads the pending request now. Constant script, nothing interpolated. */
function announceAttention(created: BrowserWindow) {
  if (created.isDestroyed()) return;
  created.webContents.executeJavaScript('window.dispatchEvent(new Event("click-dot:agent"))').catch(() => {});
}

function createMascot() {
  const created = new BrowserWindow({ ...mascotWindowOptions({ devTools: !app.isPackaged }), icon: join(assets, "icon.png") });
  window = created;
  mascot = createMascotController({
    window: {
      // GTK/X11 ignore programmatic resizes of a non-resizable window; Windows (the target) does not, and toggling the
      // flag there would bring back the frame shadow. The user still cannot resize the window by hand.
      setBounds: (bounds) => {
        if (process.platform !== "linux") return created.setBounds(bounds);
        created.setResizable(true);
        created.setBounds(bounds);
        created.setResizable(false);
      },
      show: () => created.show(),
      showInactive: () => created.showInactive(),
      hide: () => created.hide(),
      focus: () => created.focus(),
      isVisible: () => created.isVisible(),
      setAlwaysOnTop: (flag, level) => created.setAlwaysOnTop(flag, level),
      setVisibleOnAllWorkspaces: (visible, options) => created.setVisibleOnAllWorkspaces(visible, options),
      moveTop: () => created.moveTop(),
    },
    workAreas,
    primaryWorkArea: () => screen.getPrimaryDisplay().workArea,
    position: preferences?.get().mascotPosition,
    savePosition: schedulePositionSave,
    onNativeMode: () => announceMode(created),
    onAttention: () => announceAttention(created),
    hidden: preferences?.get().islandHidden === true,
    onHiddenChange: (hidden) => {
      preferences
        ?.update({ islandHidden: hidden })
        .catch((error: unknown) => {
          console.error("Could not save whether Dot is hidden.", error);
          dialog.showErrorBox("Click Dot", hidden ? "Dot is hidden now, but Click Dot could not save that choice, so it may be back next time." : "Dot is shown now, but Click Dot could not save that choice, so it may be hidden next time.");
        })
        .finally(refreshTray);
    },
    onChange: () => cursorFeed?.sync(),
  });
  installWindowController(globalThis, mascot);
  // The eye follows the cursor anywhere on the screen. Only this process can read it; the page gets two bounded integers.
  cursorFeed?.stop();
  cursorFeed = createCursorFeed({
    getCursor: () => screen.getCursorScreenPoint(),
    getBounds: () => created.getBounds(),
    isActive: () => !created.isDestroyed() && created.isVisible() && mascot?.state().mode !== "collapsed",
    send: (script) => {
      if (!created.isDestroyed()) created.webContents.executeJavaScript(script).catch(() => {});
    },
  });
  const contents = created.webContents;
  const guard = (event: { preventDefault(): void }, url: string) => {
    if (isAllowedNavigation(url)) return;
    event.preventDefault();
    openExternal(url);
  };
  contents.on("will-navigate", guard);
  contents.on("will-redirect", guard);
  // Subframes never leave the app's origin and never open anything externally.
  contents.on("will-frame-navigate", (details) => {
    if (!details.isMainFrame && !isAllowedNavigation(details.url)) details.preventDefault();
  });
  contents.setWindowOpenHandler(({ url }) => {
    openExternal(url);
    return { action: "deny" };
  });
  created.on("close", (event) => {
    if (quitting) return;
    event.preventDefault();
    created.hide();
  });
  created.on("focus", () => {
    if (!reloadOnFocus) return;
    reloadOnFocus = false;
    contents.reload();
  });
  void created.loadURL(mascotUrl());
  return created;
}

function applyLoginItem(enabled: boolean) {
  if (!shouldManageLoginItem({ isPackaged: app.isPackaged, argv: process.argv, env: process.env })) return;
  app.setLoginItemSettings({ openAtLogin: enabled, args: LOGIN_ITEM_ARGS });
}

function trayIcon(capturing: boolean) {
  return nativeImage.createFromPath(join(assets, capturing ? "tray-capturing.png" : "tray.png"));
}

function reportTrayError(message: string, error: unknown) {
  console.error(message, error);
  dialog.showErrorBox("Click Dot", `${message} Nothing was changed.`);
}

/** Rebuilt on every capture change so the tray, the tooltip and the page always tell the same story. */
function refreshTray() {
  if (!tray) return;
  const status = captureController?.status() ?? { enabled: false, paused: false, frames: 0, lastCapturedAt: null };
  const capture = describeCapture(status);
  tray.setImage(trayIcon(capture.capturing));
  tray.setToolTip(capture.tooltip);
  tray.setContextMenu(
    Menu.buildFromTemplate([
      { label: (mascot?.state().hidden ?? preferences?.get().islandHidden === true) ? "Mostrar Dot" : "Ocultar Dot", click: toggleIslandHidden },
      {
        label: "Let Dot see my screen",
        type: "checkbox",
        checked: status.enabled,
        click: (item) => {
          const turningOn = item.checked;
          captureController
            ?.apply(turningOn ? "enable" : "disable")
            .catch((error: unknown) => {
              console.error("Could not change the screen capture setting.", error);
              dialog.showErrorBox(
                "Click Dot",
                turningOn
                  ? "Click Dot could not turn on screen capture. Nothing was changed."
                  : "Screen capture is stopped, but Click Dot could not save that choice, so it may start again next time.",
              );
            })
            .finally(refreshTray);
        },
      },
      {
        label: capture.pauseLabel,
        enabled: capture.pauseEnabled,
        click: () => {
          void captureController?.apply(status.paused ? "resume" : "pause").catch(() => {});
        },
      },
      { type: "separator" },
      {
        label: "Start with Windows",
        type: "checkbox",
        checked: preferences?.get().startAtLogin ?? DEFAULT_PREFERENCES.startAtLogin,
        click: (item) => {
          preferences
            ?.update({ startAtLogin: item.checked })
            .then((saved) => applyLoginItem(saved.startAtLogin))
            .catch((error: unknown) => {
              // Keep the menu, the saved file and the OS login item telling the same story.
              reportTrayError("Click Dot could not save the Start with Windows setting.", error);
            })
            .finally(refreshTray);
        },
      },
      { type: "separator" },
      {
        label: "Quit",
        click: () => {
          quitting = true;
          app.quit();
        },
      },
    ]),
  );
}

function createTray() {
  tray = new Tray(trayIcon(false));
  tray.on("click", showWindow);
  refreshTray();
}

/**
 * Codex as Dot's engine (S1, S5, S6). Dot's MCP server, the optional Chrome and computer-use servers and the app-server are
 * wired here with `-c` overrides (nothing in the user's config files is edited), and the engine is published on globalThis
 * for the chat route. Nothing here is fatal: when Codex is missing or unsupported the status says so in Spanish and Dot keeps
 * answering with its built-in assistant. The app-server itself starts in the background; the page polls its status.
 */
async function startCodexEngine() {
  try {
    claudeTasks = createClaudeTaskRegistry();
    installClaudeTasks(globalThis, claudeTasks);
    dotMcp = await startDotMcpServer({ tasks: claudeTasks, screen: captureControllerScreenSource(globalThis) });
    const external = await detectExternalMcp();
    installExternalMcp(globalThis, external.info);
    // Every override is a TOML literal string with no spaces or shell characters, so it passes a Windows `.cmd` shim intact.
    const overrides = [...codexConfigOverrides(dotMcp, { readApprovalMode: "approve" }), ...external.overrides];
    codexServer = createCodexAppServer({
      env: { ...process.env, ...codexMcpEnv(dotMcp), ...external.env, ANONYMIZED_TELEMETRY: "false" },
      configOverrides: overrides,
    });
    installCodexEngine(globalThis, codexServer);
    // The sign-in page of Codex must open exactly as Codex built it (its callback is the Codex process on this computer).
    installSystemBrowser(globalThis, (url) => void shell.openExternal(url));
    void codexServer.start().then((result) => {
      if (!result.ok) console.error(`Codex is not available (${result.reason}); Dot uses its built-in assistant.`);
    });
  } catch (error) {
    console.error("Could not set up the Codex engine; Dot uses its built-in assistant.", error);
  }
}

async function start() {
  Menu.setApplicationMenu(null);
  const userData = app.getPath("userData");
  configureEnvironment(process.env, userData);
  const encryptionAvailable = installStorageProtector(globalThis, safeStorage);
  if (!encryptionAvailable) console.error("OS credential encryption is unavailable; Click Dot storage stays closed.");
  // Deny every web permission request; features that need one will opt in explicitly.
  session.defaultSession.setPermissionRequestHandler((_contents, _permission, callback) => callback(false));
  session.defaultSession.setPermissionCheckHandler(() => false);

  // Per-launch control token: the routes that act on this computer (capture, voice, the mascot window) only answer a
  // client holding this cookie, which only this window's session has. Published and set before the server or the window
  // exist; if the cookie cannot be set, the start fails instead of leaving those routes open.
  const controlToken = createControlToken();
  installControlToken(globalThis, controlToken);
  await session.defaultSession.cookies.set(controlCookieDetails(dotOrigin(), controlToken));

  const preferencesFile = join(userData, "desktop-preferences.json");
  const loaded = await loadPreferences(preferencesFile);
  const osStartAtLogin = app.getLoginItemSettings({ args: LOGIN_ITEM_ARGS }).openAtLogin;
  const store = createPreferenceStore(preferencesFile, initialPreferences(loaded, osStartAtLogin));
  preferences = store;
  // An unreadable preferences file leaves the OS login item exactly as it is.
  if (loaded) applyLoginItem(loaded.startAtLogin);

  // Screen capture runs here, in the main process, so web permissions stay denied. Frames stay in memory only.
  let shownState = "";
  captureService = createCaptureService({
    capture: createScreenCapture({ screen, desktopCapturer }),
    // New frames do not change the tray; only the on/off/paused state does.
    onChange: (status) => {
      const state = `${status.enabled}:${status.paused}`;
      if (state === shownState) return;
      shownState = state;
      refreshTray();
    },
  });
  // Chat gets the newest frame only after local OCR and redaction. The patterns file ships beside the app
  // (lib/guard-patterns.json); if it cannot be read, preparing a screen fails and nothing is sent.
  const service = captureService;
  let patterns: Promise<CompiledGuardRule[]> | undefined;
  const prepareScreen = createScreenPreparer({
    frames: () => service.frames(),
    isCapturing: () => {
      const status = service.status();
      return status.enabled && !status.paused;
    },
    session: () => service.session(),
    ocr: createWindowsOcr(),
    patterns: () => {
      patterns ??= loadGuardPatterns(join(app.getAppPath(), "lib", "guard-patterns.json")).catch((error: unknown) => {
        patterns = undefined;
        throw error;
      });
      return patterns;
    },
    redact: createNativeRedactor(nativeImage),
  });
  captureController = createCaptureController({
    service: captureService,
    prepareScreen,
    saveEnabled: async (enabled) => {
      await store.update({ captureEnabled: enabled });
    },
  });
  installCaptureController(globalThis, captureController);
  // "Voz": Windows speech recognition through a PowerShell helper; installs nothing off Windows.
  installVoiceController(globalThis);

  // Per-launch token for the Claude Code hook relay (`click dot-hook`). The endpoint refuses everything until it is published.
  const agentHookToken = createAgentHookToken();
  installAgentHook(globalThis, agentHookToken);

  // Agent tools (PowerShell, Claude Code): a Windows-only runtime the chat route reaches through globalThis. Elsewhere nothing
  // is published and the chat behaves as a plain chat.
  if (process.platform === "win32") installToolRunner(globalThis, createToolRunner());

  try {
    server = await startDotServer(app.getAppPath(), { workingDirectory: userData });
  } catch (error) {
    const message =
      error instanceof PortInUseError
        ? `Click Dot needs port ${error.port} on this computer, but another program is using it. Close that program and start Click Dot again.`
        : `Click Dot could not start its local server.\n\n${error instanceof Error ? error.message : String(error)}`;
    dialog.showErrorBox("Click Dot", message);
    quitting = true;
    app.quit();
    return;
  }

  // Files dropped on the chat live in a private temp folder until Dot quits.
  attachments = createAttachmentStore();
  installAttachments(globalThis, attachments);
  await startCodexEngine();

  // The relay finds the port and token here. Without the file the companion stays quiet and Claude Code asks in its terminal.
  const discoveryFile = agentHookFile(userData, process.env);
  try {
    await writeAgentHookFile(discoveryFile, { port: DOT_PORT, token: agentHookToken, pid: process.pid });
    hookDiscovery = { file: discoveryFile, token: agentHookToken };
  } catch (error) {
    console.error("Could not write the Claude Code hook discovery file.", error);
  }

  createTray();
  // The saved choice carries over restarts; a pause does not.
  if (loaded?.captureEnabled) captureService.enable();
  if (!globalShortcut.register(TOGGLE_SHORTCUT, toggleWindow)) {
    console.error(`Could not register the ${TOGGLE_SHORTCUT} shortcut; it may be in use by another program.`);
  }
  createMascot();
  // Keep the mascot on a visible display when monitors come and go.
  const reposition = () => mascot?.reposition();
  screen.on("display-added", reposition);
  screen.on("display-removed", reposition);
  screen.on("display-metrics-changed", reposition);
  started = true;
  // A start with Windows waits in the tray; a manual start (or a second launch meanwhile) shows the mascot.
  if (mascotStartup(process.argv) === "shown" || showWhenStarted) window?.once("ready-to-show", () => mascot?.showInactive());
  if (!encryptionAvailable) {
    void dialog.showMessageBox({
      type: "warning",
      title: "Click Dot",
      message: "Windows could not provide encryption for Click Dot.",
      detail: "Sign-in and conversation history stay disabled so nothing is stored unprotected.",
    });
  }
}

function failStart(error: unknown) {
  console.error("Click Dot could not start.", error);
  dialog.showErrorBox("Click Dot", `Click Dot could not start.\n\n${error instanceof Error ? error.message : String(error)}`);
  quitting = true;
  app.quit();
}

if (!app.requestSingleInstanceLock()) {
  app.quit();
} else {
  app.on("second-instance", () => {
    // Before the local server listens, a window would only show a connection error.
    if (started) showWindow();
    else showWhenStarted = true;
  });
  // The tray keeps the app alive when every window is closed.
  app.on("window-all-closed", () => {});
  app.on("before-quit", () => {
    quitting = true;
    flushPosition();
  });
  let cleaned = false;
  let shuttingDown = false;
  app.on("will-quit", (event) => {
    if (!cleaned) {
      cleaned = true;
      // Nobody may be left waiting for a window that is gone, and the token must not outlive this launch.
      releaseAgentRequests(globalThis);
      if (hookDiscovery) removeAgentHookFile(hookDiscovery.file, hookDiscovery.token);
      globalShortcut.unregisterAll();
      cursorFeed?.stop();
      captureService?.dispose();
      claudeTasks?.dispose();
      attachments?.dispose();
      server?.close();
    }
    // Codex and Dot's MCP server get a moment to stop (its process tree is killed after a short grace), then the app quits for real.
    if (!shuttingDown && (codexServer || dotMcp)) {
      shuttingDown = true;
      event.preventDefault();
      const stopped = Promise.allSettled([codexServer?.stop(), dotMcp?.close()]);
      const grace = new Promise<void>((resolve) => setTimeout(resolve, 5_000));
      void Promise.race([stopped, grace]).finally(() => app.quit());
    }
  });
  app.whenReady().then(start).catch(failStart);
}
