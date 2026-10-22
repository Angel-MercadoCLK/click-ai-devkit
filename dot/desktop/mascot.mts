// The mascot window: where it sits, how big it is in each mode, and how the page and the shortcut drive it.
// Everything here is plain data and functions so it can be tested without Electron; main.mts wires the real window.

export type Rect = { x: number; y: number; width: number; height: number };
export type Point = { x: number; y: number };
export type MascotMode = "collapsed" | "menu" | "chat";
/** Which corner of the window holds the mascot: the menu or chat opens toward the side with more room. */
export type Anchor = { horizontal: "left" | "right"; vertical: "top" | "bottom" };

export const MASCOT_MODES: readonly MascotMode[] = ["collapsed", "menu", "chat"];
export const MASCOT_SIZE = 96;
export const MASCOT_MARGIN = 16;

/**
 * Window size per mode. The menu is the mascot plus three buttons; the chat is the ~360 x 520 bubble with the mascot
 * column beside it (360 + 12 gap + 96).
 */
export const MODE_SIZES: Record<MascotMode, { width: number; height: number }> = {
  collapsed: { width: MASCOT_SIZE, height: MASCOT_SIZE },
  menu: { width: 204, height: 256 },
  chat: { width: 468, height: 520 },
};

const isMode = (value: unknown): value is MascotMode => MASCOT_MODES.some((mode) => mode === value);

/** Bottom-right corner of the primary display's work area (the part not covered by the taskbar). */
export function defaultMascotPosition(workArea: Rect): Point {
  return { x: workArea.x + workArea.width - MASCOT_SIZE - MASCOT_MARGIN, y: workArea.y + workArea.height - MASCOT_SIZE - MASCOT_MARGIN };
}

const distanceToRect = (point: Point, area: Rect) => {
  const dx = Math.max(area.x - point.x, 0, point.x - (area.x + area.width));
  const dy = Math.max(area.y - point.y, 0, point.y - (area.y + area.height));
  return dx * dx + dy * dy;
};

/** The work area holding the point, or the closest one when it is in a gap or on a display that is gone. */
export function nearestWorkArea(point: Point, areas: readonly Rect[]): Rect {
  let best = areas[0];
  let bestDistance = Number.POSITIVE_INFINITY;
  for (const area of areas) {
    const distance = distanceToRect(point, area);
    if (distance < bestDistance) {
      best = area;
      bestDistance = distance;
    }
  }
  return best;
}

const centerOf = (position: Point): Point => ({ x: position.x + MASCOT_SIZE / 2, y: position.y + MASCOT_SIZE / 2 });
const clamp = (value: number, low: number, high: number) => Math.max(low, Math.min(value, high));

/** Keeps the whole mascot inside the work area it is closest to. */
export function clampMascotPosition(position: Point, areas: readonly Rect[]): Point {
  const area = nearestWorkArea(centerOf(position), areas);
  return {
    x: clamp(position.x, area.x, area.x + area.width - MASCOT_SIZE),
    y: clamp(position.y, area.y, area.y + area.height - MASCOT_SIZE),
  };
}

export function anchorFor(position: Point, area: Rect): Anchor {
  const center = centerOf(position);
  return {
    horizontal: center.x > area.x + area.width / 2 ? "right" : "left",
    vertical: center.y > area.y + area.height / 2 ? "bottom" : "top",
  };
}

/** The window size for a mode, reduced when the work area is smaller than the design size. */
export function windowSize(mode: MascotMode, area: Rect) {
  const size = MODE_SIZES[mode];
  return { width: Math.min(size.width, area.width), height: Math.min(size.height, area.height) };
}

/** Where the mascot sits inside a window of this size: in the anchored corner. */
export function mascotOffset(anchor: Anchor, size: { width: number; height: number }): Point {
  return {
    x: anchor.horizontal === "right" ? size.width - MASCOT_SIZE : 0,
    y: anchor.vertical === "bottom" ? size.height - MASCOT_SIZE : 0,
  };
}

/** The window rectangle for a mode with the mascot on its saved spot, shifted only as far as needed to stay on screen. */
export function placeWindow({ mascot, mode, anchor, areas }: { mascot: Point; mode: MascotMode; anchor: Anchor; areas: readonly Rect[] }): Rect {
  const area = nearestWorkArea(centerOf(mascot), areas);
  const size = windowSize(mode, area);
  const offset = mascotOffset(anchor, size);
  return {
    x: clamp(mascot.x - offset.x, area.x, area.x + area.width - size.width),
    y: clamp(mascot.y - offset.y, area.y, area.y + area.height - size.height),
    ...size,
  };
}

/** Clicking the mascot opens the menu, or closes whatever is open. */
export function nextModeOnMascotClick(mode: MascotMode): MascotMode {
  return mode === "collapsed" ? "menu" : "collapsed";
}

/** The shortcut opens the chat bubble, or closes it when it is the open one. A hidden mascot comes back with the chat. */
export function nextModeOnShortcut(mode: MascotMode, visible: boolean): MascotMode {
  return visible && mode === "chat" ? "collapsed" : "chat";
}

/**
 * Frameless and transparent so only the mascot shows; always on top and off the taskbar so it behaves like a companion,
 * not an app. Resizing is done by the app from the main process, never by the user. There is no preload: the page asks
 * for mode changes through a validated loopback route.
 */
export function mascotWindowOptions({ devTools = false }: { devTools?: boolean } = {}) {
  return {
    width: MASCOT_SIZE,
    height: MASCOT_SIZE,
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

/** The slice of an Electron BrowserWindow the controller uses. */
export type MascotWindow = {
  setBounds(bounds: Rect): void;
  show(): void;
  showInactive(): void;
  hide(): void;
  focus(): void;
  isVisible(): boolean;
};

export type MascotControllerOptions = {
  window: MascotWindow;
  /** Work areas of every connected display, primary first. */
  workAreas: () => readonly Rect[];
  primaryWorkArea: () => Rect;
  /** The saved position, if any. It is clamped to a visible display; the clamp itself is never saved. */
  position?: Point;
  /** Called with the position the mascot ends up at after a drag. */
  savePosition: (position: Point) => void;
  /** Called when the app (not the page) changed the mode, so the page can follow. */
  onNativeMode?: (mode: MascotMode) => void;
};

export type MascotController = ReturnType<typeof createMascotController>;

export function createMascotController(options: MascotControllerOptions) {
  const { window: win } = options;
  const areas = () => options.workAreas();
  const anchorAt = (spot: Point) => anchorFor(spot, nearestWorkArea(centerOf(spot), areas()));

  let position = clampMascotPosition(options.position ?? defaultMascotPosition(options.primaryWorkArea()), areas());
  let mode: MascotMode = "collapsed";
  let anchor = anchorAt(position);

  const layout = () => win.setBounds(placeWindow({ mascot: position, mode, anchor, areas: areas() }));
  layout();

  function setMode(next: MascotMode, source: "page" | "native") {
    if (!isMode(next)) throw new TypeError("Unknown mascot mode.");
    mode = next;
    // The side is chosen when the layout changes, not while dragging, so the controls never flip under the pointer.
    anchor = anchorAt(position);
    layout();
    if (source === "native") options.onNativeMode?.(mode);
  }

  /** `x` and `y` are the window's top-left target while dragging; the mascot is what must stay on screen. */
  function moveTo(x: number, y: number) {
    if (!Number.isFinite(x) || !Number.isFinite(y)) return;
    const area = nearestWorkArea(centerOf(position), areas());
    const offset = mascotOffset(anchor, windowSize(mode, area));
    position = clampMascotPosition({ x: Math.round(x) + offset.x, y: Math.round(y) + offset.y }, areas());
    layout();
    options.savePosition({ ...position });
  }

  /** After a display change: bring the mascot back onto a visible display. The saved choice stays as it was. */
  function reposition() {
    position = clampMascotPosition(position, areas());
    anchor = anchorAt(position);
    layout();
  }

  return {
    position: (): Point => ({ ...position }),
    state: () => ({ mode, anchor: { ...anchor } }),
    setMode,
    moveTo,
    reposition,
    show() {
      reposition();
      win.show();
      win.focus();
    },
    showInactive() {
      reposition();
      win.showInactive();
    },
    toggleChat() {
      const visible = win.isVisible();
      const next = nextModeOnShortcut(mode, visible);
      setMode(next, "native");
      if (!visible) {
        win.show();
        win.focus();
      } else if (next === "chat") win.focus();
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
  });
}
