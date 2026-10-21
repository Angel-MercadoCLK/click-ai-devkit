/** Name shared with lib/capture-controller.ts, which reads globalThis[Symbol.for(CAPTURE_CONTROLLER_KEY)]. */
export const CAPTURE_CONTROLLER_KEY = "click-dot.capture-controller";

/** A frame every ten seconds, the newest six kept in memory (about one minute). Never written to disk. */
export const CAPTURE_INTERVAL_MS = 10_000;
export const CAPTURE_FRAME_LIMIT = 6;
export const CAPTURE_MAX_WIDTH = 1920;

export type RawFrame = { image: Buffer; width: number; height: number };
export type CaptureFrame = RawFrame & { capturedAt: number };
export type CaptureFn = () => Promise<RawFrame | undefined>;

/** What the page and the tray may know: counts and times, never pixels. */
export type CaptureStatus = { enabled: boolean; paused: boolean; frames: number; lastCapturedAt: number | null };
export type CaptureAction = "enable" | "disable" | "pause" | "resume";

export type CaptureTimers = {
  setInterval(callback: () => void, ms: number): unknown;
  clearInterval(handle: unknown): void;
  now(): number;
};

export const systemTimers: CaptureTimers = {
  setInterval: (callback, ms) => {
    const handle = setInterval(callback, ms);
    handle.unref?.();
    return handle;
  },
  clearInterval: (handle) => clearInterval(handle as NodeJS.Timeout),
  now: () => Date.now(),
};

/** Keeps only the newest frames, oldest first. */
export class FrameRing {
  readonly #capacity: number;
  #items: CaptureFrame[] = [];

  constructor(capacity: number) {
    if (!Number.isInteger(capacity) || capacity < 1) throw new RangeError("The frame ring needs a positive capacity.");
    this.#capacity = capacity;
  }

  get size() {
    return this.#items.length;
  }

  push(frame: CaptureFrame) {
    this.#items.push(frame);
    if (this.#items.length > this.#capacity) this.#items.splice(0, this.#items.length - this.#capacity);
  }

  frames(): CaptureFrame[] {
    return [...this.#items];
  }

  clear() {
    this.#items = [];
  }
}

export type CaptureService = {
  enable(): void;
  disable(): void;
  pause(): void;
  resume(): void;
  status(): CaptureStatus;
  /** Main process only. T2 publishes no way to read frames out of this process. */
  frames(): CaptureFrame[];
  dispose(): void;
};

const validFrame = (value: unknown): value is RawFrame => {
  if (!value || typeof value !== "object") return false;
  const { image, width, height } = value as Partial<RawFrame>;
  return Buffer.isBuffer(image) && image.length > 0 && Number.isInteger(width) && (width as number) > 0 && Number.isInteger(height) && (height as number) > 0;
};

/**
 * Capture state machine: off, on, or on-but-paused. Frames exist only while capture is running; leaving the
 * running state (pause or off) drops them at once and discards any capture still in flight.
 */
export function createCaptureService({
  capture,
  timers = systemTimers,
  intervalMs = CAPTURE_INTERVAL_MS,
  capacity = CAPTURE_FRAME_LIMIT,
  onChange = () => {},
  log = console.error,
}: {
  capture: CaptureFn;
  timers?: CaptureTimers;
  intervalMs?: number;
  capacity?: number;
  onChange?: (status: CaptureStatus) => void;
  log?: (message: string, error?: unknown) => void;
}): CaptureService {
  const ring = new FrameRing(capacity);
  let enabled = false;
  let paused = false;
  let timer: unknown;
  let generation = 0;
  let inFlight = -1;
  let lastCapturedAt: number | null = null;

  const status = (): CaptureStatus => ({ enabled, paused, frames: ring.size, lastCapturedAt });
  const notify = () => {
    try {
      onChange(status());
    } catch (error) {
      log("Screen capture listener failed.", error);
    }
  };

  async function tick() {
    if (inFlight === generation) return;
    const current = generation;
    inFlight = current;
    let frame: RawFrame | undefined;
    try {
      frame = await capture();
    } catch (error) {
      log("Screen capture failed; skipping this frame.", error);
    } finally {
      if (inFlight === current) inFlight = -1;
    }
    // Paused or turned off while the frame was being taken: it must not be kept.
    if (current !== generation || frame === undefined) return;
    if (!validFrame(frame)) {
      log("Screen capture returned an unusable frame; skipping it.");
      return;
    }
    const capturedAt = timers.now();
    ring.push({ image: frame.image, width: frame.width, height: frame.height, capturedAt });
    lastCapturedAt = capturedAt;
    notify();
  }

  function start() {
    if (timer !== undefined) return;
    timer = timers.setInterval(() => void tick(), intervalMs);
    void tick();
  }

  function stop() {
    if (timer !== undefined) timers.clearInterval(timer);
    timer = undefined;
    generation += 1;
    ring.clear();
    lastCapturedAt = null;
  }

  return {
    enable() {
      if (enabled && !paused) return;
      enabled = true;
      paused = false;
      start();
      notify();
    },
    disable() {
      if (!enabled && !paused) return;
      enabled = false;
      paused = false;
      stop();
      notify();
    },
    pause() {
      if (!enabled || paused) return;
      paused = true;
      stop();
      notify();
    },
    resume() {
      if (!enabled || !paused) return;
      paused = false;
      start();
      notify();
    },
    status,
    frames: () => ring.frames(),
    dispose() {
      enabled = false;
      paused = false;
      stop();
    },
  };
}

/** What the web side calls, through globalThis. No frame is reachable from here. */
export type CaptureController = {
  status(): CaptureStatus;
  apply(action: CaptureAction): Promise<CaptureStatus>;
};

const ACTIONS: readonly CaptureAction[] = ["enable", "disable", "pause", "resume"];

/**
 * One controller drives both the tray and the web page, so they cannot disagree. Changes run one at a time.
 * Turning capture on is saved first and does not happen if saving fails. Turning it off stops capture first
 * and then saves, so a failed save never leaves the screen being watched.
 */
export function createCaptureController({ service, saveEnabled }: { service: CaptureService; saveEnabled(enabled: boolean): Promise<void> }): CaptureController {
  let queue: Promise<unknown> = Promise.resolve();

  async function run(action: CaptureAction): Promise<CaptureStatus> {
    switch (action) {
      case "enable":
        if (!service.status().enabled) await saveEnabled(true);
        service.enable();
        break;
      case "disable": {
        const wasEnabled = service.status().enabled;
        service.disable();
        if (wasEnabled) await saveEnabled(false);
        break;
      }
      case "pause":
        service.pause();
        break;
      case "resume":
        service.resume();
        break;
    }
    return service.status();
  }

  return {
    status: () => service.status(),
    apply(action) {
      if (!ACTIONS.includes(action)) return Promise.reject(new TypeError("Unknown screen capture action."));
      const result = queue.then(() => run(action));
      queue = result.catch(() => {});
      return result;
    },
  };
}

/** Publishes the controller for the Next.js server code running in this same process. */
export function installCaptureController(target: object, controller: CaptureController) {
  Reflect.set(target, Symbol.for(CAPTURE_CONTROLLER_KEY), controller);
}

/** Tray wording and icon for a capture state. */
export function describeCapture(status: CaptureStatus) {
  const capturing = status.enabled && !status.paused;
  return {
    tooltip: capturing ? "Click Dot — seeing your screen" : status.enabled ? "Click Dot — screen capture paused" : "Click Dot",
    pauseLabel: status.paused ? "Resume screen capture" : "Pause screen capture",
    pauseEnabled: status.enabled,
    capturing,
  };
}
