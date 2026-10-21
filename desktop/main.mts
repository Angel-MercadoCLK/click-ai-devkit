import { app, BrowserWindow, dialog, globalShortcut, Menu, nativeImage, safeStorage, session, shell, Tray } from "electron";
import type { Server } from "node:http";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { configureEnvironment, dotUrl, TOGGLE_SHORTCUT } from "./config.mts";
import { PortInUseError } from "./listen.mts";
import { isAllowedNavigation, isSignInUrl, systemBrowserTarget } from "./navigation.mts";
import { DEFAULT_PREFERENCES, LOGIN_ITEM_ARGS, loadPreferences, savePreferences, shouldManageLoginItem, startsHidden, type Preferences } from "./preferences.mts";
import { installStorageProtector } from "./protector.mts";
import { startDotServer } from "./server.mts";

const assets = join(dirname(fileURLToPath(import.meta.url)), "..", "assets");

let window: BrowserWindow | undefined;
let tray: Tray | undefined;
let server: Server | undefined;
let quitting = false;
let reloadOnFocus = false;
let started = false;
let showWhenStarted = false;
let preferences: Preferences = { ...DEFAULT_PREFERENCES };

function showWindow() {
  if (!window || window.isDestroyed()) window = createWindow();
  if (window.isMinimized()) window.restore();
  window.show();
  window.focus();
}

function toggleWindow() {
  if (window && !window.isDestroyed() && window.isVisible() && window.isFocused()) window.hide();
  else showWindow();
}

function openExternal(url: string) {
  const target = systemBrowserTarget(url);
  if (!target) return;
  // The sign-in finishes in the system browser; refresh this window when the user comes back.
  if (isSignInUrl(url)) reloadOnFocus = true;
  void shell.openExternal(target);
}

function createWindow() {
  const created = new BrowserWindow({
    width: 460,
    height: 740,
    show: false,
    title: "Click Dot",
    icon: join(assets, "icon.png"),
    backgroundColor: "#ffffff",
    autoHideMenuBar: true,
    webPreferences: {
      contextIsolation: true,
      nodeIntegration: false,
      sandbox: true,
      devTools: !app.isPackaged,
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
  void created.loadURL(dotUrl());
  return created;
}

function applyLoginItem(enabled: boolean) {
  if (!shouldManageLoginItem({ isPackaged: app.isPackaged, argv: process.argv, env: process.env })) return;
  app.setLoginItemSettings({ openAtLogin: enabled, args: LOGIN_ITEM_ARGS });
}

function createTray(preferencesFile: string) {
  tray = new Tray(nativeImage.createFromPath(join(assets, "tray.png")));
  tray.setToolTip("Click Dot");
  tray.on("click", showWindow);
  tray.setContextMenu(
    Menu.buildFromTemplate([
      { label: "Open Dot", click: showWindow },
      {
        label: "Start with Windows",
        type: "checkbox",
        checked: preferences.startAtLogin,
        click: (item) => {
          savePreferences(preferencesFile, { startAtLogin: item.checked })
            .then((saved) => {
              preferences = saved;
              applyLoginItem(saved.startAtLogin);
            })
            .catch((error: unknown) => {
              // Keep the menu, the saved file and the OS login item telling the same story.
              item.checked = preferences.startAtLogin;
              console.error("Could not save the Start with Windows preference.", error);
              dialog.showErrorBox("Click Dot", "Click Dot could not save the Start with Windows setting. Nothing was changed.");
            });
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

async function start() {
  Menu.setApplicationMenu(null);
  const userData = app.getPath("userData");
  configureEnvironment(process.env, userData);
  const encryptionAvailable = installStorageProtector(globalThis, safeStorage);
  if (!encryptionAvailable) console.error("OS credential encryption is unavailable; Click Dot storage stays closed.");
  // Deny every web permission request; features that need one will opt in explicitly.
  session.defaultSession.setPermissionRequestHandler((_contents, _permission, callback) => callback(false));
  session.defaultSession.setPermissionCheckHandler(() => false);

  const preferencesFile = join(userData, "desktop-preferences.json");
  const loaded = await loadPreferences(preferencesFile);
  preferences = loaded ?? { ...DEFAULT_PREFERENCES };
  // An unreadable preferences file leaves the OS login item exactly as it is.
  if (loaded) applyLoginItem(loaded.startAtLogin);

  try {
    server = await startDotServer(app.getAppPath());
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

  createTray(preferencesFile);
  if (!globalShortcut.register(TOGGLE_SHORTCUT, toggleWindow)) {
    console.error(`Could not register the ${TOGGLE_SHORTCUT} shortcut; it may be in use by another program.`);
  }
  window = createWindow();
  started = true;
  if (!startsHidden(process.argv) || showWhenStarted) window.once("ready-to-show", showWindow);
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
  });
  app.on("will-quit", () => {
    globalShortcut.unregisterAll();
    server?.close();
  });
  app.whenReady().then(start).catch(failStart);
}
