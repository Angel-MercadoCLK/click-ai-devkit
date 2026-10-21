import assert from "node:assert/strict";
import { test } from "node:test";

process.env.PORT = "3003";

const capture = await import("../desktop/capture.mts");
const screenCapture = await import("../desktop/screen-capture.mts");
const preferences = await import("../desktop/preferences.mts");
const reader = await import("../lib/capture-controller.ts");
const statusRoute = await import("../app/api/capture/status/route.ts");
const actionRoute = await import("../app/api/capture/route.ts");

const flush = () => new Promise((resolve) => setImmediate(resolve));
const KEY = Symbol.for("click-dot.capture-controller");

function fakeTimers() {
  let now = 1_000;
  let next = 1;
  const handles = new Map();
  return {
    timers: {
      setInterval(fn, ms) {
        const id = next++;
        handles.set(id, { fn, ms });
        return id;
      },
      clearInterval(id) {
        handles.delete(id);
      },
      now: () => now,
    },
    active: () => handles.size,
    intervals: () => [...handles.values()].map((handle) => handle.ms),
    async tick() {
      now += 10_000;
      for (const { fn } of [...handles.values()]) fn();
      await flush();
    },
  };
}

function frameSource() {
  let count = 0;
  const capture = async () => {
    count += 1;
    return { image: Buffer.from(`frame-${count}`), width: 8, height: 4 };
  };
  return { capture, calls: () => count };
}

function makeService(overrides = {}) {
  const clock = fakeTimers();
  const source = frameSource();
  const logs = [];
  const changes = [];
  const service = capture.createCaptureService({
    capture: source.capture,
    timers: clock.timers,
    log: (...args) => logs.push(args),
    onChange: (status) => changes.push(status),
    ...overrides,
  });
  return { service, clock, source, logs, changes };
}

const text = (frame) => frame.image.toString();

test("the capture design constants follow the ten second, six frame plan", () => {
  assert.equal(capture.CAPTURE_INTERVAL_MS, 10_000);
  assert.equal(capture.CAPTURE_FRAME_LIMIT, 6);
  assert.equal(capture.CAPTURE_MAX_WIDTH, 1920);
});

test("the frame ring keeps only the newest frames, oldest first, and clears completely", () => {
  const ring = new capture.FrameRing(6);
  for (let index = 1; index <= 8; index += 1) ring.push({ image: Buffer.from(String(index)), width: 1, height: 1, capturedAt: index });
  assert.equal(ring.size, 6);
  assert.deepEqual(ring.frames().map(text), ["3", "4", "5", "6", "7", "8"]);
  ring.clear();
  assert.equal(ring.size, 0);
  assert.deepEqual(ring.frames(), []);
  assert.throws(() => new capture.FrameRing(0));
});

test("turning capture on starts a ten second timer and takes a first frame", async () => {
  const { service, clock, source } = makeService();
  assert.deepEqual(service.status(), { enabled: false, paused: false, frames: 0, lastCapturedAt: null });
  assert.equal(clock.active(), 0);
  service.enable();
  await flush();
  assert.equal(clock.active(), 1);
  assert.deepEqual(clock.intervals(), [10_000]);
  assert.equal(source.calls(), 1);
  assert.equal(service.status().enabled, true);
  assert.equal(service.status().frames, 1);
  for (let index = 0; index < 8; index += 1) await clock.tick();
  assert.equal(service.status().frames, 6, "never more than the ring capacity");
  assert.deepEqual(service.frames().map(text), ["frame-4", "frame-5", "frame-6", "frame-7", "frame-8", "frame-9"]);
  assert.equal(service.status().lastCapturedAt, service.frames().at(-1).capturedAt);
});

test("pausing drops every frame and stops the timer; resuming restarts it", async () => {
  const { service, clock } = makeService();
  service.enable();
  await flush();
  await clock.tick();
  assert.equal(service.status().frames, 2);
  service.pause();
  assert.deepEqual(service.status(), { enabled: true, paused: true, frames: 0, lastCapturedAt: null });
  assert.equal(clock.active(), 0);
  assert.deepEqual(service.frames(), []);
  await clock.tick();
  assert.equal(service.status().frames, 0, "nothing is captured while paused");
  service.resume();
  await flush();
  assert.equal(clock.active(), 1);
  assert.equal(service.status().paused, false);
  assert.equal(service.status().frames, 1);
});

test("turning capture off clears frames and stops the timer; dispose does the same", async () => {
  const { service, clock } = makeService();
  service.enable();
  await flush();
  service.disable();
  assert.deepEqual(service.status(), { enabled: false, paused: false, frames: 0, lastCapturedAt: null });
  assert.equal(clock.active(), 0);
  service.enable();
  await flush();
  assert.equal(service.status().frames, 1);
  service.dispose();
  assert.equal(service.status().enabled, false);
  assert.equal(service.status().frames, 0);
  assert.equal(clock.active(), 0);
});

test("pause, resume and disable are harmless while capture is off", () => {
  const { service, clock } = makeService();
  service.pause();
  service.resume();
  service.disable();
  assert.deepEqual(service.status(), { enabled: false, paused: false, frames: 0, lastCapturedAt: null });
  assert.equal(clock.active(), 0);
});

test("a failing capture is logged and skipped without crashing or storing a frame", async () => {
  let fail = true;
  const { service, clock, logs } = makeService({
    capture: async () => {
      if (fail) throw new Error("no screen permission");
      return { image: Buffer.from("ok"), width: 2, height: 2 };
    },
  });
  service.enable();
  await flush();
  assert.equal(service.status().frames, 0);
  assert.equal(service.status().enabled, true);
  assert.equal(logs.length, 1);
  fail = false;
  await clock.tick();
  assert.equal(service.status().frames, 1, "the next tick recovers");
});

test("empty or malformed captures are skipped", async () => {
  const results = [undefined, { image: Buffer.alloc(0), width: 2, height: 2 }, { image: "not a buffer", width: 2, height: 2 }, { image: Buffer.from("x"), width: 0, height: 2 }];
  const { service, clock } = makeService({ capture: async () => results.shift() });
  service.enable();
  await flush();
  for (let index = 0; index < 3; index += 1) await clock.tick();
  assert.equal(service.status().frames, 0);
});

test("a slow capture never overlaps with the next tick", async () => {
  let release;
  let calls = 0;
  const { service, clock } = makeService({
    capture: () => {
      calls += 1;
      return new Promise((resolve) => {
        release = () => resolve({ image: Buffer.from("slow"), width: 1, height: 1 });
      });
    },
  });
  service.enable();
  await flush();
  await clock.tick();
  await clock.tick();
  assert.equal(calls, 1);
  release();
  await flush();
  assert.equal(service.status().frames, 1);
});

test("a frame that finishes after pause or off is discarded, never stored", async () => {
  let release;
  const { service } = makeService({
    capture: () => new Promise((resolve) => {
      release = () => resolve({ image: Buffer.from("late"), width: 1, height: 1 });
    }),
  });
  service.enable();
  await flush();
  service.pause();
  release();
  await flush();
  assert.equal(service.status().frames, 0);
  service.resume();
  await flush();
  service.disable();
  release();
  await flush();
  assert.equal(service.status().frames, 0);
});

test("status reports counts and times only, never image bytes", async () => {
  const { service } = makeService();
  service.enable();
  await flush();
  const status = service.status();
  assert.deepEqual(Object.keys(status).sort(), ["enabled", "frames", "lastCapturedAt", "paused"]);
  assert.equal(JSON.stringify(status).includes("frame-1"), false);
});

test("changes are announced so the tray and the page stay in step", async () => {
  const { service, changes } = makeService();
  service.enable();
  await flush();
  service.pause();
  service.resume();
  await flush();
  service.disable();
  assert.ok(changes.length >= 4);
  assert.equal(changes.at(-1).enabled, false);
  assert.ok(changes.some((status) => status.paused));
});

test("the controller saves the choice before enabling and stays off if saving fails", async () => {
  const { service } = makeService();
  const saved = [];
  const controller = capture.createCaptureController({ service, saveEnabled: async (enabled) => { saved.push(enabled); } });
  assert.equal((await controller.apply("enable")).enabled, true);
  assert.deepEqual(saved, [true]);
  await controller.apply("disable");
  assert.deepEqual(saved, [true, false]);

  const failing = capture.createCaptureController({ service, saveEnabled: async () => { throw new Error("disk full"); } });
  await assert.rejects(failing.apply("enable"));
  assert.equal(service.status().enabled, false);
});

test("turning capture off stops it even when the choice cannot be saved", async () => {
  const { service } = makeService();
  const controller = capture.createCaptureController({ service, saveEnabled: async (enabled) => { if (!enabled) throw new Error("disk full"); } });
  await controller.apply("enable");
  await assert.rejects(controller.apply("disable"));
  assert.equal(service.status().enabled, false, "privacy first: capture is already stopped");
  assert.equal(service.status().frames, 0);
});

test("the controller pauses, resumes and rejects unknown actions", async () => {
  const { service } = makeService();
  const controller = capture.createCaptureController({ service, saveEnabled: async () => {} });
  await controller.apply("enable");
  assert.equal((await controller.apply("pause")).paused, true);
  assert.equal((await controller.apply("resume")).paused, false);
  assert.equal(controller.status().enabled, true);
  await assert.rejects(controller.apply("delete-everything"));
  await assert.rejects(controller.apply(undefined));
});

test("the controller is published under the shared key and the page-side reader agrees on the name", () => {
  assert.equal(capture.CAPTURE_CONTROLLER_KEY, "click-dot.capture-controller");
  assert.equal(reader.CAPTURE_CONTROLLER_KEY, Symbol.for(capture.CAPTURE_CONTROLLER_KEY));
  const target = {};
  const controller = { status: () => ({}), apply: async () => ({}) };
  capture.installCaptureController(target, controller);
  assert.equal(Reflect.get(target, Symbol.for("click-dot.capture-controller")), controller);
});

test("the tray text follows the capture state", () => {
  assert.deepEqual(capture.describeCapture({ enabled: false, paused: false, frames: 0, lastCapturedAt: null }), {
    tooltip: "Click Dot",
    pauseLabel: "Pause screen capture",
    pauseEnabled: false,
    capturing: false,
  });
  assert.deepEqual(capture.describeCapture({ enabled: true, paused: false, frames: 2, lastCapturedAt: 1 }), {
    tooltip: "Click Dot — seeing your screen",
    pauseLabel: "Pause screen capture",
    pauseEnabled: true,
    capturing: true,
  });
  assert.deepEqual(capture.describeCapture({ enabled: true, paused: true, frames: 0, lastCapturedAt: null }), {
    tooltip: "Click Dot — screen capture paused",
    pauseLabel: "Resume screen capture",
    pauseEnabled: true,
    capturing: false,
  });
});

test("the capture size is the primary display in device pixels, bounded to 1920 wide", () => {
  assert.deepEqual(screenCapture.thumbnailSizeFor({ size: { width: 1280, height: 720 }, scaleFactor: 1 }), { width: 1280, height: 720 });
  assert.deepEqual(screenCapture.thumbnailSizeFor({ size: { width: 1920, height: 1080 }, scaleFactor: 1.25 }), { width: 1920, height: 1080 });
  assert.deepEqual(screenCapture.thumbnailSizeFor({ size: { width: 2560, height: 1440 }, scaleFactor: 2 }), { width: 1920, height: 1080 });
  assert.deepEqual(screenCapture.thumbnailSizeFor({ size: { width: 800, height: 600 }, scaleFactor: 1.5 }), { width: 1200, height: 900 });
  assert.deepEqual(screenCapture.thumbnailSizeFor({ size: { width: 0, height: 0 }, scaleFactor: 0 }), { width: 1, height: 1 });
});

function fakeElectron({ sources, display = { id: 7, size: { width: 3840, height: 2160 }, scaleFactor: 1 } }) {
  const requests = [];
  return {
    requests,
    deps: {
      screen: { getPrimaryDisplay: () => display },
      desktopCapturer: {
        getSources: async (options) => {
          requests.push(options);
          return sources;
        },
      },
    },
  };
}

const thumbnail = (bytes, empty = false, size = { width: 1920, height: 1080 }) => ({ isEmpty: () => empty, getSize: () => size, toPNG: () => Buffer.from(bytes) });

test("the screen capture asks only for screens and picks the primary display's source", async () => {
  const electron = fakeElectron({ sources: [{ display_id: "3", thumbnail: thumbnail("other") }, { display_id: "7", thumbnail: thumbnail("primary") }] });
  const frame = await screenCapture.createScreenCapture(electron.deps)();
  assert.deepEqual(electron.requests, [{ types: ["screen"], thumbnailSize: { width: 1920, height: 1080 } }]);
  assert.equal(frame.image.toString(), "primary");
  assert.deepEqual([frame.width, frame.height], [1920, 1080]);
});

test("the screen capture falls back to the first screen and skips empty thumbnails", async () => {
  const first = fakeElectron({ sources: [{ display_id: "", thumbnail: thumbnail("only") }] });
  assert.equal((await screenCapture.createScreenCapture(first.deps)()).image.toString(), "only");
  const empty = fakeElectron({ sources: [{ display_id: "7", thumbnail: thumbnail("x", true) }] });
  assert.equal(await screenCapture.createScreenCapture(empty.deps)(), undefined);
  const none = fakeElectron({ sources: [] });
  assert.equal(await screenCapture.createScreenCapture(none.deps)(), undefined);
});

test("capture preferences default to off and old files without the field stay valid", async () => {
  const { mkdtemp, writeFile, readFile, rm } = await import("node:fs/promises");
  const { tmpdir } = await import("node:os");
  const { join } = await import("node:path");
  const dir = await mkdtemp(join(tmpdir(), "click-dot-capture-prefs-"));
  try {
    assert.deepEqual(preferences.DEFAULT_PREFERENCES, { startAtLogin: true, captureEnabled: false });
    const legacy = join(dir, "legacy.json");
    await writeFile(legacy, '{\n  "startAtLogin": false\n}\n');
    assert.deepEqual(await preferences.loadPreferences(legacy), { startAtLogin: false, captureEnabled: false });
    assert.equal(await readFile(legacy, "utf8"), '{\n  "startAtLogin": false\n}\n', "the old file is not rewritten on load");

    const file = join(dir, "prefs.json");
    assert.deepEqual(await preferences.savePreferences(file, { startAtLogin: true, captureEnabled: true }), { startAtLogin: true, captureEnabled: true });
    assert.deepEqual(await preferences.loadPreferences(file), { startAtLogin: true, captureEnabled: true });

    for (const [index, content] of ['{"startAtLogin":true,"captureEnabled":"yes"}', "{not json", "[]", ""].entries()) {
      const bad = join(dir, `bad-${index}.json`);
      await writeFile(bad, content);
      assert.equal(await preferences.loadPreferences(bad), undefined, content);
    }
  } finally {
    await rm(dir, { recursive: true, force: true });
  }
});

test("the preference store merges changes and keeps concurrent saves from losing each other", async () => {
  const { mkdtemp, rm } = await import("node:fs/promises");
  const { tmpdir } = await import("node:os");
  const { join } = await import("node:path");
  const dir = await mkdtemp(join(tmpdir(), "click-dot-capture-store-"));
  try {
    const file = join(dir, "prefs.json");
    const store = preferences.createPreferenceStore(file, { ...preferences.DEFAULT_PREFERENCES });
    await Promise.all([store.update({ captureEnabled: true }), store.update({ startAtLogin: false })]);
    assert.deepEqual(store.get(), { startAtLogin: false, captureEnabled: true });
    assert.deepEqual(await preferences.loadPreferences(file), { startAtLogin: false, captureEnabled: true });
  } finally {
    await rm(dir, { recursive: true, force: true });
  }
});

test("the page-side reader accepts only a well-formed controller", () => {
  const ok = { status: () => ({}), apply: async () => ({}) };
  for (const bad of [undefined, null, "controller", 5, {}, { status: () => ({}) }, { apply: async () => ({}) }, { status: "no", apply: async () => ({}) }]) {
    Reflect.set(globalThis, KEY, bad);
    assert.equal(reader.getCaptureController(), undefined, String(bad));
  }
  Reflect.set(globalThis, KEY, ok);
  assert.equal(reader.getCaptureController(), ok);
  Reflect.deleteProperty(globalThis, KEY);
  assert.equal(reader.getCaptureController(), undefined);
});

test("the reader sanitizes status and drops anything that is not a plain status", () => {
  const good = { enabled: true, paused: false, frames: 3, lastCapturedAt: 12345 };
  assert.deepEqual(reader.readCaptureStatus({ status: () => ({ ...good, image: Buffer.from("secret") }), apply: async () => good }), good);
  assert.deepEqual(reader.readCaptureStatus({ status: () => ({ ...good, lastCapturedAt: null }), apply: async () => good }), { ...good, lastCapturedAt: null });
  for (const bad of [null, "x", { ...good, enabled: "yes" }, { ...good, frames: -1 }, { ...good, frames: 1.5 }, { ...good, frames: 1e9 }, { ...good, lastCapturedAt: "now" }, { ...good, paused: undefined }]) {
    assert.equal(reader.readCaptureStatus({ status: () => bad, apply: async () => good }), undefined, JSON.stringify(bad));
  }
  assert.equal(reader.readCaptureStatus({ status: () => { throw new Error("boom"); }, apply: async () => good }), undefined);
});

test("only the four exact actions are understood", () => {
  for (const action of ["enable", "disable", "pause", "resume"]) assert.equal(reader.parseCaptureAction({ action }), action);
  for (const bad of [null, [], "enable", {}, { action: "ENABLE" }, { action: "stop" }, { action: 1 }, { action: "enable", extra: true }, { action: "enable", frames: [] }]) {
    assert.equal(reader.parseCaptureAction(bad), undefined, JSON.stringify(bad));
  }
});

const get = (headers = { host: "127.0.0.1:3003" }) => statusRoute.GET(new Request("http://127.0.0.1:3003/api/capture/status", { headers }));
const post = (body, { headers = {}, raw } = {}) =>
  actionRoute.POST(
    new Request("http://127.0.0.1:3003/api/capture", {
      method: "POST",
      headers: { host: "127.0.0.1:3003", origin: "http://127.0.0.1:3003", "content-type": "application/json", ...headers },
      body: raw ?? JSON.stringify(body),
    }),
  );

function installFakeController() {
  const calls = [];
  let state = { enabled: false, paused: false, frames: 0, lastCapturedAt: null };
  Reflect.set(globalThis, KEY, {
    status: () => ({ ...state, image: Buffer.from("secret-pixels") }),
    apply: async (action) => {
      calls.push(action);
      if (action === "enable") state = { enabled: true, paused: false, frames: 1, lastCapturedAt: 5 };
      if (action === "pause") state = { enabled: true, paused: true, frames: 0, lastCapturedAt: null };
      if (action === "disable") state = { enabled: false, paused: false, frames: 0, lastCapturedAt: null };
      return { ...state, image: Buffer.from("secret-pixels") };
    },
  });
  return calls;
}

test("status without the desktop controller says capture is unavailable", async () => {
  Reflect.deleteProperty(globalThis, KEY);
  const response = await get();
  assert.equal(response.status, 200);
  assert.equal(response.headers.get("cache-control"), "no-store, private");
  assert.deepEqual(await response.json(), { available: false, enabled: false, paused: false, frames: 0, lastCapturedAt: null });
});

test("status is local-only and never returns image bytes", async () => {
  installFakeController();
  assert.equal((await get({ host: "evil.example" })).status, 403);
  assert.equal((await get({ host: "127.0.0.1:3003", origin: "http://evil.example" })).status, 403);
  const response = await get();
  assert.equal(response.status, 200);
  const raw = await response.text();
  assert.equal(raw.includes("secret-pixels"), false);
  assert.deepEqual(JSON.parse(raw), { available: true, enabled: false, paused: false, frames: 0, lastCapturedAt: null });
  Reflect.deleteProperty(globalThis, KEY);
});

test("a malformed controller reads as unavailable", async () => {
  Reflect.set(globalThis, KEY, { status: () => ({ enabled: "maybe" }), apply: async () => ({}) });
  assert.equal((await (await get()).json()).available, false);
  Reflect.deleteProperty(globalThis, KEY);
});

test("changing capture without the desktop controller fails with 503", async () => {
  Reflect.deleteProperty(globalThis, KEY);
  const response = await post({ action: "enable" });
  assert.equal(response.status, 503);
  assert.equal(response.headers.get("cache-control"), "no-store, private");
  const body = await response.json();
  assert.equal(body.available, false);
  assert.equal(typeof body.error, "string");
});

test("changing capture needs the exact local origin", async () => {
  const calls = installFakeController();
  assert.equal((await post({ action: "enable" }, { headers: { origin: "http://localhost:3003" } })).status, 403);
  assert.equal((await post({ action: "enable" }, { headers: { origin: "https://evil.example" } })).status, 403);
  assert.equal((await post({ action: "enable" }, { headers: { host: "evil.example" } })).status, 403);
  const noOrigin = await actionRoute.POST(new Request("http://127.0.0.1:3003/api/capture", {
    method: "POST", headers: { host: "127.0.0.1:3003", "content-type": "application/json" }, body: JSON.stringify({ action: "enable" }),
  }));
  assert.equal(noOrigin.status, 403);
  assert.deepEqual(calls, []);
  Reflect.deleteProperty(globalThis, KEY);
});

test("invalid bodies are rejected before the controller is touched", async () => {
  const calls = installFakeController();
  const cases = [
    post({ action: "stop" }),
    post({ action: "enable", extra: 1 }),
    post({}),
    post([]),
    post(undefined, { raw: "{not json" }),
    post(undefined, { raw: "" }),
    post({ action: "enable" }, { headers: { "content-type": "text/plain" } }),
  ];
  for (const pending of cases) assert.equal((await pending).status, 400);
  assert.equal((await post(undefined, { raw: JSON.stringify({ action: "enable", padding: "x".repeat(5000) }) })).status, 413);
  assert.deepEqual(calls, []);
  Reflect.deleteProperty(globalThis, KEY);
});

test("valid actions reach the controller and return the new status without image bytes", async () => {
  const calls = installFakeController();
  const enabled = await post({ action: "enable" });
  assert.equal(enabled.status, 200);
  assert.equal(enabled.headers.get("cache-control"), "no-store, private");
  const raw = await enabled.text();
  assert.equal(raw.includes("secret-pixels"), false);
  assert.deepEqual(JSON.parse(raw), { available: true, enabled: true, paused: false, frames: 1, lastCapturedAt: 5 });
  assert.equal((await (await post({ action: "pause" })).json()).paused, true);
  assert.equal((await (await post({ action: "disable" })).json()).enabled, false);
  assert.deepEqual(calls, ["enable", "pause", "disable"]);
  Reflect.deleteProperty(globalThis, KEY);
});

test("a controller failure is reported without leaking its message", async () => {
  Reflect.set(globalThis, KEY, { status: () => ({ enabled: false, paused: false, frames: 0, lastCapturedAt: null }), apply: async () => { throw new Error("C:\\Users\\secret\\path"); } });
  const response = await post({ action: "enable" });
  assert.equal(response.status, 503);
  assert.equal((await response.text()).includes("secret"), false);
  Reflect.deleteProperty(globalThis, KEY);
});

test("T3 keeps the capture bridge as it was: no prepareScreen unless one is supplied, and status and actions never return pixels", async () => {
  const { service } = makeService();
  const plain = capture.createCaptureController({ service, saveEnabled: async () => {} });
  assert.deepEqual(Object.keys(plain).sort(), ["apply", "status"]);
  const prepared = async () => ({ image: Buffer.from("redacted"), mime: "image/png", hiddenRegions: 0, capturedAt: 1 });
  const withScreen = capture.createCaptureController({ service, saveEnabled: async () => {}, prepareScreen: prepared });
  assert.deepEqual(Object.keys(withScreen).sort(), ["apply", "prepareScreen", "status"]);
  assert.equal(withScreen.prepareScreen, prepared);

  Reflect.set(globalThis, KEY, { status: () => ({ enabled: true, paused: false, frames: 1, lastCapturedAt: 5 }), apply: async () => ({ enabled: true, paused: false, frames: 1, lastCapturedAt: 5 }), prepareScreen: async () => ({ image: Buffer.from("secret-pixels"), mime: "image/png", hiddenRegions: 0, capturedAt: 1 }) });
  const status = await (await get()).text();
  const action = await (await post({ action: "enable" })).text();
  for (const raw of [status, action]) {
    assert.equal(raw.includes("secret-pixels"), false);
    assert.equal(raw.includes("prepareScreen"), false);
  }
  assert.deepEqual(JSON.parse(status), { available: true, enabled: true, paused: false, frames: 1, lastCapturedAt: 5 });
  Reflect.deleteProperty(globalThis, KEY);
});

test("the session identity changes on every enable, pause, resume and disable, and never on a no-op", async () => {
  const { service } = makeService();
  const seen = [service.session()];
  const changed = (label) => {
    const value = service.session();
    assert.notEqual(value, seen.at(-1), label);
    assert.ok(!seen.includes(value), `${label}: an earlier session id came back`);
    seen.push(value);
  };
  const unchanged = (label) => assert.equal(service.session(), seen.at(-1), label);
  service.pause();
  unchanged("pause while off is a no-op");
  service.enable();
  changed("enable");
  service.enable();
  unchanged("enable while running is a no-op");
  service.pause();
  changed("pause");
  service.pause();
  unchanged("pause while paused is a no-op");
  service.resume();
  changed("resume");
  service.disable();
  changed("disable");
  service.disable();
  unchanged("disable while off is a no-op");
  service.enable();
  changed("enable after disable");
  await flush();
});

test("the session identity does not change when a frame is stored", async () => {
  const { service, clock } = makeService();
  service.enable();
  const id = service.session();
  await flush();
  await clock.tick();
  assert.equal(service.status().frames > 1, true);
  assert.equal(service.session(), id);
});
