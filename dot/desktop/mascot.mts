// The island window: where it sits, how big it is in each mode, and how the page and the shortcut drive it.
// Everything here is plain data and functions so it can be tested without Electron; main.mts wires the real window.
//
// Dot hangs from the top edge of the primary display like an island: centered, flush with the top of the work area, and
// sized to exactly what the page shows in each mode, so there is no transparent area around it that could block clicks.

export type Rect = { x: number; y: number; width: number; height: number };
export type Point = { x: number; y: number };
export type MascotMode = "collapsed" | "compact" | "menu" | "chat" | "approval";
/** Kept for the state the page reads back; the island is always at the top, so this never changes. */
export type Anchor = { horizontal: "left" | "right"; vertical: "top" | "bottom" };

export const MASCOT_MODES: readonly MascotMode[] = ["collapsed", "compact", "menu", "chat", "approval"];
/** The character's size in the expanded cards (px). */
export const MASCOT_SIZE = 96;

/** The window never moves; the island hangs from the top edge, so the anchor the page reads back is constant. */
const ISLAND_ANCHOR: Anchor = { horizontal: "left", vertical: "top" };

/**
 * Window size per mode (px). collapsed is an invisible hot strip on the top edge that only hovering turns into the pill (nothing is
 * drawn in it, so with nothing going on there is no visible bar); compact is the one-line pill
 * (character, latest action, session count); the others are the expanded island: sessions and quick actions, the chat, and the
 * permission or question card.
 */
export const MODE_SIZES: Record<MascotMode, { width: number; height: number }> = {
  collapsed: { width: 180, height: 4 },
  compact: { width: 500, height: 44 },
  menu: { width: 640, height: 300 },
  chat: { width: 600, height: 560 },
  approval: { width: 640, height: 340 },
};

const isMode = (value: unknown): value is MascotMode => MASCOT_MODES.some((mode) => mode === value);

/** The window rectangle for a mode: top center of the work area, reduced only when the work area is smaller than the design size. */
export function islandBounds(mode: MascotMode, workArea: Rect): Rect {
  const size = MODE_SIZES[mode];
  const width = Math.min(size.width, workArea.width);
  const height = Math.min(size.height, workArea.height);
  return { x: workArea.x + Math.round((workArea.width - width) / 2), y: workArea.y, width, height };
}

/** Clicking the island opens the menu from the bar or the pill, or closes whatever is open. */
export function nextModeOnMascotClick(mode: MascotMode): MascotMode {
  return mode === "collapsed" || mode === "compact" ? "menu" : "collapsed";
}

/** The shortcut opens the chat card, or closes it when it is the open one. A hidden island comes back with the chat. */
export function nextModeOnShortcut(mode: MascotMode, visible: boolean): MascotMode {
  return visible && mode === "chat" ? "collapsed" : "chat";
}

/**
 * Frameless and transparent so only the island shows; always on top and off the taskbar so it behaves like a companion,
 * not an app. It stays focusable: the composer and the buttons need it once the user clicks. Resizing is done by the app from
 * the main process, never by the user. There is no preload: the page asks for mode changes through a validated loopback route.
 */
export function mascotWindowOptions({ devTools = false }: { devTools?: boolean } = {}) {
  return {
    width: MODE_SIZES.collapsed.width,
    height: MODE_SIZES.collapsed.height,
    show: false,
    frame: false,
    transparent: true,
    backgroundColor: "#00000000",
    hasShadow: false,
    thickFrame: false,
    resizable: false,
    maximizable: false,
    minimizable: false,
    fullscreenable: false,
    skipTaskbar: true,
    alwaysOnTop: true,
    title: "Click Dot",
    webPreferences: { contextIsolation: true, nodeIntegration: false, sandbox: true, devTools },
  };
}

/**
 * The slice of an Electron BrowserWindow the controller uses. The three stacking methods are optional so a shell (or a test
 * window) without them still works; when present they keep the island above every other window.
 */
export type MascotWindow = {
  setBounds(bounds: Rect): void;
  show(): void;
  showInactive(): void;
  hide(): void;
  focus(): void;
  isVisible(): boolean;
  setAlwaysOnTop?(flag: boolean, level?: "screen-saver"): void;
  setVisibleOnAllWorkspaces?(visible: boolean, options?: { visibleOnFullScreen?: boolean }): void;
  moveTop?(): void;
};

export type MascotControllerOptions = {
  window: MascotWindow;
  /** Work areas of every connected display, primary first. Not used any more: the island always lives on the primary one. */
  workAreas?: () => readonly Rect[];
  primaryWorkArea: () => Rect;
  /** The position saved by the old draggable mascot. Ignored: the island is fixed at the top center. */
  position?: Point;
  /** Never called any more; kept so the shell that still passes it keeps compiling. */
  savePosition?: (position: Point) => void;
  /** Called when the app (not the page) changed the mode, so the page can follow. */
  onNativeMode?: (mode: MascotMode) => void;
  /** Called when the app brought the island up because Claude Code needs the user, so the page looks at once instead of on its next poll. */
  onAttention?: () => void;
  /** Whether the user has hidden Dot (the tray item "Ocultar Dot"), as saved from the last run. */
  hidden?: boolean;
  /** Called when the user hides or shows Dot, so the choice can be saved and the tray can say what it does next. */
  onHiddenChange?: (hidden: boolean) => void;
  /** Called after anything that can change what the island shows (mode, visibility), so a poller can start or stop. */
  onChange?: () => void;
};

export type MascotController = ReturnType<typeof createMascotController>;

export function createMascotController(options: MascotControllerOptions) {
  const { window: win } = options;

  let mode: MascotMode = "collapsed";
  let hidden = options.hidden === true;
  let bounds: Rect = islandBounds(mode, options.primaryWorkArea());

  /** Windows can drop the topmost state of a window that was shown inactive or resized, so every show and resize asserts it again. */
  function stayOnTop() {
    win.setAlwaysOnTop?.(true, "screen-saver");
    win.setVisibleOnAllWorkspaces?.(true, { visibleOnFullScreen: true });
    win.moveTop?.();
  }

  const layout = () => {
    bounds = islandBounds(mode, options.primaryWorkArea());
    win.setBounds(bounds);
    stayOnTop();
  };
  layout();

  const changed = () => options.onChange?.();

  /**
   * Hidden by the user, the window is on screen only while something needs it: any card, or the pill the page asks for while the
   * screen is shared. At rest (collapsed) there is no strip either, so nothing is on screen and nothing can be hovered.
   */
  function syncHidden() {
    if (!hidden) return;
    if (mode === "collapsed") {
      if (win.isVisible()) win.hide();
    } else if (!win.isVisible()) {
      win.showInactive();
      stayOnTop();
    }
  }

  function setMode(next: MascotMode, source: "page" | "native") {
    if (!isMode(next)) throw new TypeError("Unknown mascot mode.");
    mode = next;
    layout();
    syncHidden();
    if (source === "native") options.onNativeMode?.(mode);
    changed();
  }

  /** The tray's "Ocultar Dot" / "Mostrar Dot": hiding closes whatever is open and takes the island off screen until it is shown again. */
  function setHidden(next: boolean) {
    if (hidden === next) return;
    hidden = next;
    mode = "collapsed";
    layout();
    if (hidden) {
      if (win.isVisible()) win.hide();
    } else {
      win.showInactive();
      stayOnTop();
    }
    options.onNativeMode?.(mode);
    options.onHiddenChange?.(hidden);
    changed();
  }

  /** The island is fixed at the top center. A move request from an older page is accepted and changes nothing. */
  function moveTo(_x: number, _y: number) {}

  /** After a display change: the island follows the primary work area. */
  function reposition() {
    layout();
    changed();
  }

  return {
    position: (): Point => ({ x: bounds.x, y: bounds.y }),
    state: () => ({ mode, anchor: { ...ISLAND_ANCHOR }, hidden }),
    setMode,
    setHidden,
    moveTo,
    reposition,
    /** Summoning Dot (the tray icon, a second launch): the resting island shows nothing, so it opens the menu. */
    show() {
      reposition();
      if (mode === "collapsed") setMode("menu", "native");
      win.show();
      stayOnTop();
      win.focus();
      changed();
    },
    /** The quiet show at startup. Hidden by the user, the island stays off screen until something needs it. */
    showInactive() {
      reposition();
      if (hidden && mode === "collapsed") return;
      win.showInactive();
      stayOnTop();
      changed();
    },
    /** Claude Code needs the user: the island comes up (also from the tray, also while hidden) without taking focus from what they are doing. */
    attention() {
      reposition();
      win.showInactive();
      stayOnTop();
      options.onAttention?.();
      changed();
    },
    toggleChat() {
      const visible = win.isVisible();
      const next = nextModeOnShortcut(mode, visible);
      setMode(next, "native");
      if (!visible) {
        win.show();
        stayOnTop();
        win.focus();
      } else if (next === "chat") win.focus();
      changed();
    },
  };
}

/** Same pattern as the capture controller: the page-facing slice of the controller goes on `globalThis` before routes use it. */
export const WINDOW_CONTROLLER_KEY = "click-dot.window-controller";

export function installWindowController(target: object, controller: MascotController) {
  Reflect.set(target, Symbol.for(WINDOW_CONTROLLER_KEY), {
    state: () => controller.state(),
    setMode: (mode: MascotMode) => controller.setMode(mode, "page"),
    moveTo: (x: number, y: number) => controller.moveTo(x, y),
    showInactive: () => controller.attention(),
  });
}
