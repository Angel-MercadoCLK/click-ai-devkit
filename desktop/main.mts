import { app, BrowserWindow, dialog, globalShortcut, Menu, nativeImage, safeStorage, session, shell, Tray } from "electron";
import type { Server } from "node:http";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { configureEnvironment, dotUrl, TOGGLE_SHORTCUT } from "./config.mts";
import { PortInUseError } from "./listen.mts";
import { isAllowedNavigation, isSignInUrl, systemBrowserTarget } from "./navigation.mts";
import { LOGIN_ITEM_ARGS, loadPreferences, savePreferences, shouldManageLoginItem, startsHidden, type Preferences } from "./preferences.mts";
import { installStorageProtector } from "./protector.mts";
import { startDotServer } from "./server.mts";

const assets = join(dirname(fileURLToPath(import.meta.url)), "..", "assets");

let window: BrowserWindow | undefined;
let tray: Tray | undefined;
let server: Server | undefined;
let quitting = false;
let reloadOnFocus = false;
let preferences: Preferences = { startAtLogin: true };

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
          void savePreferences(preferencesFile, { startAtLogin: item.checked }).then((saved) => {
            preferences = saved;
            applyLoginItem(saved.startAtLogin);
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
  if (!installStorageProtector(globalThis, safeStorage)) {
    console.error("OS credential encryption is unavailable; Click Dot storage stays closed.");
  }
  // Deny every web permission request; features that need one will opt in explicitly.
  session.defaultSession.setPermissionRequestHandler((_contents, _permission, callback) => callback(false));
  session.defaultSession.setPermissionCheckHandler(() => false);

  const preferencesFile = join(userData, "desktop-preferences.json");
  preferences = await loadPreferences(preferencesFile);
  applyLoginItem(preferences.startAtLogin);

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
  if (!startsHidden(process.argv)) window.once("ready-to-show", showWindow);
}

if (!app.requestSingleInstanceLock()) {
  app.quit();
} else {
  app.on("second-instance", () => {
    if (app.isReady()) showWindow();
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
  void app.whenReady().then(start);
}
