import assert from "node:assert/strict";
import { test } from "node:test";

process.env.PORT = "3003";

const speech = await import("../desktop/speech.mts");
const reader = await import("../lib/voice-controller.ts");
const listenRoute = await import("../app/api/voice/listen/route.ts");
const cancelRoute = await import("../app/api/voice/cancel/route.ts");

const KEY = Symbol.for("click-dot.voice-controller");

// Helper process ------------------------------------------------------------------------------------------------

function fakeChild({ stdout = "", code = 0, delay = 0, never = false, chunks, spawnError = false } = {}) {
  const listeners = {};
  const state = { killed: false, stdinEnded: false, stdinWrites: [] };
  const child = {
    stdin: { write: (chunk) => { state.stdinWrites.push(String(chunk)); return true; }, end: () => { state.stdinEnded = true; }, on: () => {} },
    stdout: { on: (event, listener) => { listeners.stdout = listener; } },
    stderr: { on: () => {} },
    on: (event, listener) => { listeners[event] = listener; },
    kill: () => { state.killed = true; },
  };
  queueMicrotask(() => setTimeout(() => {
    if (never || state.killed) return;
    if (spawnError) { listeners.error?.(new Error("ENOENT")); return; }
    for (const piece of chunks ?? [stdout]) listeners.stdout?.(Buffer.from(piece));
    listeners.close?.(code);
  }, delay));
  return { child, state };
}

const make = (options = {}, child = fakeChild(options)) => {
  const calls = [];
  const controller = speech.createWindowsSpeech({
    executable: "powershell.exe",
    locale: "es-MX",
    ...options,
    spawn: (executable, args) => { calls.push({ executable, args }); return child.child; },
  });
  return { controller, calls, state: child.state };
};
const reasonOf = (code) => (error) => error instanceof speech.SpeechError && error.code === code;

test("the speech helper runs PowerShell 5.1 without a profile, gets language preferences over stdin and returns the transcript", async () => {
  const { controller, calls, state } = make({ stdout: `${JSON.stringify({ text: "hola Dot, ¿qué hora es?" })}\r\n` });
  assert.deepEqual(await controller.listen(), { text: "hola Dot, ¿qué hora es?" });
  assert.equal(calls[0].executable, "powershell.exe");
  assert.deepEqual(calls[0].args.slice(0, 7), ["-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-InputFormat", "None", "-EncodedCommand"]);
  assert.equal(calls[0].args.length, 8);
  assert.equal(Buffer.from(calls[0].args[7], "base64").toString("utf16le"), speech.SPEECH_SCRIPT);
  assert.equal(state.stdinWrites.join(""), "es-MX,es,es-ES,en-US,en\n");
  assert.equal(state.stdinEnded, true);
});

test("the script never writes audio to disk or stdout and uses only local, in-memory recognition APIs", () => {
  const script = speech.SPEECH_SCRIPT;
  assert.doesNotMatch(script, /Set-Content|Out-File|\.wav|SaveAudio|SetInputToWaveFile|SetInputToAudioStream|\[System\.IO\.File\]|Invoke-WebRequest|Invoke-RestMethod/i);
  assert.match(script, /SetInputToDefaultAudioDevice/);
  assert.match(script, /DictationGrammar/);
  assert.match(script, /InstalledRecognizers/);
  assert.match(script, /SupportedTopicLanguages/);
  // The ASCII-only guard keeps the console code page from touching the text.
  assert.match(script, /\\x00-\\x7F/);
});

test("culture preference is the user's UI culture, then Spanish, then English", () => {
  const D = ["es-MX", "es", "es-ES", "en-US", "en"];
  assert.deepEqual(speech.preferredCultureTags("es-MX"), D);
  assert.deepEqual(speech.preferredCultureTags("en-GB"), ["en-GB", "en", "es-MX", "es", "es-ES", "en-US"]);
  assert.deepEqual(speech.preferredCultureTags("es-AR"), ["es-AR", "es", "es-MX", "es-ES", "en-US", "en"]);
  assert.deepEqual(speech.preferredCultureTags("fr-FR"), ["fr-FR", "fr", "es-MX", "es", "es-ES", "en-US", "en"]);
  assert.deepEqual(speech.preferredCultureTags(undefined), D);
  // Anything that is not a plain language tag is dropped, so no caller text can reach the script.
  for (const bad of ["", "x", "es_MX", "es-MX; Remove-Item *", "../../etc", "a".repeat(40), 5, null]) {
    assert.deepEqual(speech.preferredCultureTags(bad), D, String(bad));
  }
  assert.ok(speech.preferredCultureTags("zh-Hans-CN").every((tag) => /^[A-Za-z]{2,3}(-[A-Za-z0-9]{2,8})*$/.test(tag)));
});

test("every helper error code is surfaced as a SpeechError with that code and never the helper's own text", async () => {
  for (const code of ["unavailable", "no-recognizer", "no-microphone", "mic-denied", "privacy", "no-speech", "timeout", "failed"]) {
    const { controller } = make({ stdout: JSON.stringify({ code, error: "C:\\Users\\maria\\secret detail" }) });
    await assert.rejects(controller.listen(), (error) => {
      assert.ok(error instanceof speech.SpeechError);
      assert.equal(error.code, code);
      assert.doesNotMatch(error.message, /maria|secret|Users/);
      return true;
    }, code);
  }
});

test("malformed, oversized or unexpected helper output fails closed as 'failed'", async () => {
  const text = (value) => JSON.stringify({ text: value });
  const cases = {
    "empty stdout": "",
    "not json": "listening...",
    "array": "[]",
    "null": "null",
    "string": JSON.stringify("hola"),
    "text not a string": JSON.stringify({ text: 5 }),
    "extra key": JSON.stringify({ text: "hola", audio: "AAAA" }),
    "both shapes": JSON.stringify({ text: "hola", code: "failed", error: "x" }),
    "unknown code": JSON.stringify({ code: "rm -rf", error: "x" }),
    "controller-only code busy": JSON.stringify({ code: "busy", error: "x" }),
    "controller-only code cancelled": JSON.stringify({ code: "cancelled", error: "x" }),
    "error not a string": JSON.stringify({ code: "failed", error: 7 }),
    "code without error": JSON.stringify({ code: "failed" }),
    "text over the cap": text("a".repeat(speech.SPEECH_MAX_TEXT_CHARS + 1)),
    "two lines": `${text("a")}\n${text("b")}`,
  };
  for (const [name, stdout] of Object.entries(cases)) {
    const { controller } = make({ stdout });
    await assert.rejects(controller.listen(), reasonOf("failed"), name);
  }
  assert.deepEqual(speech.parseSpeechOutput(`${text("hola")}\n`), { text: "hola" });
  assert.throws(() => speech.parseSpeechOutput(text("   ")), reasonOf("no-speech"));
});

test("stdout over the bound kills the helper and fails closed", async () => {
  const child = fakeChild({ chunks: ["x".repeat(600), "y".repeat(600)] });
  const { controller } = make({ maxOutputBytes: 1000 }, child);
  await assert.rejects(controller.listen(), reasonOf("failed"));
  assert.equal(child.state.killed, true);
});

test("a non-zero exit is a failure even when stdout looks valid", async () => {
  const { controller } = make({ stdout: JSON.stringify({ text: "hola" }), code: 1 });
  await assert.rejects(controller.listen(), reasonOf("failed"));
});

test("a helper that cannot start reports unavailable; one that outlives the timeout is killed", async () => {
  await assert.rejects(make({ spawnError: true }).controller.listen(), reasonOf("unavailable"));
  const throwing = speech.createWindowsSpeech({ executable: "x", spawn: () => { throw new Error("boom"); } });
  await assert.rejects(throwing.listen(), reasonOf("unavailable"));
  const child = fakeChild({ never: true });
  const { controller } = make({ timeoutMs: 20 }, child);
  await assert.rejects(controller.listen(), reasonOf("timeout"));
  assert.equal(child.state.killed, true);
  if (process.platform !== "win32") {
    await assert.rejects(speech.createWindowsSpeech({ spawn: () => assert.fail("must not start") }).listen(), reasonOf("unavailable"));
  }
});

test("cancel kills the helper and rejects the pending listen as 'cancelled'; with nothing running it is a no-op", async () => {
  const child = fakeChild({ never: true });
  const { controller } = make({}, child);
  assert.doesNotThrow(() => controller.cancel());
  const pending = controller.listen();
  controller.cancel();
  await assert.rejects(pending, reasonOf("cancelled"));
  assert.equal(child.state.killed, true);
  // A late close from the killed process must not resolve or throw anything.
  const second = make({ stdout: JSON.stringify({ text: "otra vez" }) });
  assert.deepEqual(await second.controller.listen(), { text: "otra vez" });
});

test("a second listen while one is running is rejected as busy and does not start another helper", async () => {
  const child = fakeChild({ delay: 30, stdout: JSON.stringify({ text: "primera" }) });
  const { controller, calls } = make({}, child);
  const first = controller.listen();
  await assert.rejects(controller.listen(), reasonOf("busy"));
  assert.equal(calls.length, 1);
  assert.deepEqual(await first, { text: "primera" });
  // Once the first finished, a new listen starts a new helper.
  const next = fakeChild({ stdout: JSON.stringify({ text: "segunda" }) });
  const again = speech.createWindowsSpeech({ executable: "x", spawn: () => next.child });
  assert.deepEqual(await again.listen(), { text: "segunda" });
});

test("the controller is installed on the global symbol only on Windows", () => {
  const target = {};
  const controller = { listen: async () => ({ text: "x" }), cancel() {} };
  assert.equal(speech.installVoiceController(target, controller, "linux"), false);
  assert.equal(speech.installVoiceController(target, controller, "darwin"), false);
  assert.equal(Reflect.get(target, KEY), undefined);
  assert.equal(speech.installVoiceController(target, controller, "win32"), true);
  assert.equal(Reflect.get(target, KEY), controller);
  assert.equal(speech.VOICE_CONTROLLER_KEY, "click-dot.voice-controller");
});

// Web-side reader -----------------------------------------------------------------------------------------------

test("the reader accepts only a controller with listen and cancel functions", () => {
  const holder = globalThis;
  Reflect.deleteProperty(holder, KEY);
  assert.equal(reader.getVoiceController(), undefined);
  for (const bad of [null, "x", 5, [], {}, { listen() {} }, { cancel() {} }, { listen: 1, cancel() {} }, { listen() {}, cancel: "no" }]) {
    Reflect.set(holder, KEY, bad);
    assert.equal(reader.getVoiceController(), undefined, JSON.stringify(bad));
  }
  const good = { listen: async () => ({ text: "x" }), cancel() {} };
  Reflect.set(holder, KEY, good);
  assert.equal(reader.getVoiceController(), good);
  Reflect.deleteProperty(holder, KEY);
});

test("the sanitizer trims, removes control characters, caps the length and rejects anything but { text }", () => {
  const clean = (text) => reader.sanitizeTranscript({ text })?.text;
  assert.equal(clean("  hola   Dot  "), "hola Dot");
  assert.equal(clean("hola\u0000\u0007 mundo\r\nlinea\tdos\u001b[31m"), "hola mundo linea dos [31m");
  assert.equal(clean("a\u2028b\u2029c\u0085d"), "a b c d");
  assert.equal(clean("a".repeat(5000)).length, reader.VOICE_MAX_TEXT_CHARS);
  assert.equal(reader.VOICE_MAX_TEXT_CHARS, 2000);
  // The cap never splits a surrogate pair.
  const emoji = clean("😀".repeat(1500));
  assert.equal(Array.from(emoji).length, 1500);
  assert.equal(Array.from(clean("😀".repeat(2500))).length, 2000);
  for (const bad of [undefined, null, "hola", 5, [], {}, { text: 5 }, { text: "" }, { text: " \u0000 \n " }, { text: null }]) {
    assert.equal(reader.sanitizeTranscript(bad), undefined, JSON.stringify(bad));
  }
  assert.deepEqual(reader.sanitizeTranscript({ text: "hola", audio: "AAAA", extra: 1 }), { text: "hola" });
});

test("helper failures map to fixed Spanish messages; raw errors never cross", async () => {
  const fail = (error) => reader.listenOnce({ listen: async () => { throw error; }, cancel() {} });
  for (const code of reader.VOICE_CODES) {
    const message = reader.VOICE_MESSAGES[code];
    assert.equal(typeof message, "string");
    assert.ok(message.length > 10);
    await assert.rejects(fail({ code, message: "C:\\Users\\maria\\secret" }), (error) => error instanceof reader.VoiceError && error.code === code && error.message === message, code);
  }
  for (const odd of [new Error("C:\\Users\\maria\\secret"), { code: "rm -rf" }, { code: 5 }, null, undefined, "boom"]) {
    await assert.rejects(fail(odd), (error) => error instanceof reader.VoiceError && error.code === "failed" && !/maria|secret|boom|rm/.test(error.message));
  }
  // Malformed successful results fail closed too.
  for (const bad of [undefined, null, "hola", { text: 5 }, { text: "" }]) {
    await assert.rejects(reader.listenOnce({ listen: async () => bad, cancel() {} }), (error) => error instanceof reader.VoiceError && error.code === "failed");
  }
  // A controller whose listen throws synchronously is handled the same way.
  await assert.rejects(reader.listenOnce({ listen() { throw new Error("sync secret"); }, cancel() {} }), (error) => error.code === "failed" && !/secret/.test(error.message));
  assert.deepEqual(await reader.listenOnce({ listen: async () => ({ text: "  hola \u0000 " }), cancel() {} }), { text: "hola" });
});

test("the Spanish messages are fixed text that names the next step", () => {
  assert.match(reader.VOICE_MESSAGES["no-recognizer"], /reconocimiento de voz/i);
  assert.match(reader.VOICE_MESSAGES["no-microphone"], /micr[oó]fono/i);
  assert.match(reader.VOICE_MESSAGES["mic-denied"], /micr[oó]fono/i);
  assert.match(reader.VOICE_MESSAGES.privacy, /en l[ií]nea/i);
  assert.match(reader.VOICE_MESSAGES["no-speech"], /no (te )?escuch/i);
});

// Routes --------------------------------------------------------------------------------------------------------

const request = (path, { method = "POST", headers = {}, body, rawBody } = {}) =>
  new Request(`http://127.0.0.1:3003${path}`, {
    method,
    headers: { host: "127.0.0.1:3003", origin: "http://127.0.0.1:3003", ...headers },
    body: rawBody ?? body,
  });
const listen = (options) => listenRoute.POST(request("/api/voice/listen", options));
const cancel = (options) => cancelRoute.POST(request("/api/voice/cancel", options));
const json = { "content-type": "application/json" };

function install(listenImpl) {
  const calls = { listen: 0, cancel: 0 };
  Reflect.set(globalThis, KEY, {
    listen: async () => { calls.listen += 1; return listenImpl(); },
    cancel: () => { calls.cancel += 1; },
  });
  return calls;
}
const uninstall = () => Reflect.deleteProperty(globalThis, KEY);

test("without a controller both routes answer unavailable", async () => {
  uninstall();
  for (const response of [await listen(), await cancel()]) {
    assert.equal(response.status, 503);
    assert.equal(response.headers.get("cache-control"), "no-store, private");
    const body = await response.json();
    assert.equal(body.code, "unavailable");
    assert.equal(body.error, reader.VOICE_MESSAGES.unavailable);
  }
});

test("a successful listen returns 200 with only { text }", async () => {
  const calls = install(async () => ({ text: "  hola Dot ", audio: "AAAA" }));
  const response = await listen();
  assert.equal(response.status, 200);
  assert.equal(response.headers.get("cache-control"), "no-store, private");
  assert.deepEqual(await response.json(), { text: "hola Dot" });
  assert.equal(calls.listen, 1);
  uninstall();
});

test("both routes need the exact local Origin and Host", async () => {
  const calls = install(async () => ({ text: "x" }));
  for (const route of [listen, cancel]) {
    for (const headers of [{ origin: "http://localhost:3003" }, { origin: "https://evil.example" }, { host: "evil.example" }, { origin: "null" }]) {
      assert.equal((await route({ headers })).status, 403, JSON.stringify(headers));
    }
  }
  const noOrigin = (route, path) => route.POST(new Request(`http://127.0.0.1:3003${path}`, { method: "POST", headers: { host: "127.0.0.1:3003" } }));
  assert.equal((await noOrigin(listenRoute, "/api/voice/listen")).status, 403);
  assert.equal((await noOrigin(cancelRoute, "/api/voice/cancel")).status, 403);
  assert.deepEqual(calls, { listen: 0, cancel: 0 });
  uninstall();
});

test("only an empty body or {} is accepted, and anything else is rejected before the controller runs", async () => {
  const calls = install(async () => ({ text: "x" }));
  const bad = [
    { headers: json, rawBody: '{"text":"hola"}' },
    { headers: json, rawBody: "[]" },
    { headers: json, rawBody: "null" },
    { headers: json, rawBody: "{not json" },
    { headers: json, rawBody: '{"a":1}' },
    { headers: { "content-type": "text/plain" }, rawBody: "{}" },
    { headers: json, rawBody: "x".repeat(5000) },
    { headers: { ...json, "content-length": "99999" }, rawBody: "{}" },
  ];
  for (const route of [listen, cancel]) {
    for (const options of bad) {
      const response = await route(options);
      assert.ok([400, 413].includes(response.status), `${response.status} ${JSON.stringify(options).slice(0, 60)}`);
      assert.equal(response.headers.get("cache-control"), "no-store, private");
    }
  }
  assert.deepEqual(calls, { listen: 0, cancel: 0 });
  for (const options of [{}, { headers: json }, { headers: json, rawBody: "{}" }, { headers: json, rawBody: " {} \n" }]) {
    assert.equal((await listen(options)).status, 200, JSON.stringify(options));
  }
  assert.equal(calls.listen, 4);
  uninstall();
});

test("helper failures map to a status and a fixed Spanish message, never raw helper text", async () => {
  const expected = { busy: 409, cancelled: 409, timeout: 504, failed: 500, unavailable: 503, "no-recognizer": 422, "no-microphone": 422, "mic-denied": 422, privacy: 422, "no-speech": 422 };
  for (const code of reader.VOICE_CODES) {
    install(async () => { throw Object.assign(new Error("C:\\Users\\maria\\secret"), { code }); });
    const response = await listen();
    assert.equal(response.status, expected[code], code);
    assert.equal(response.headers.get("cache-control"), "no-store, private");
    const body = await response.json();
    assert.deepEqual(Object.keys(body).sort(), ["code", "error"]);
    assert.equal(body.code, code);
    assert.equal(body.error, reader.VOICE_MESSAGES[code]);
    assert.doesNotMatch(JSON.stringify(body), /maria|secret|Users/);
  }
  install(async () => { throw new Error("C:\\Users\\maria\\secret"); });
  const unknown = await listen();
  assert.equal(unknown.status, 500);
  assert.deepEqual(await unknown.json(), { code: "failed", error: reader.VOICE_MESSAGES.failed });
  install(async () => ({ text: 5 }));
  assert.equal((await listen()).status, 500);
  uninstall();
});

test("listen through the real helper wrapper maps busy to 409 and a second caller leaves the first running", async () => {
  const child = fakeChild({ delay: 40, stdout: JSON.stringify({ text: "primera" }) });
  const controller = speech.createWindowsSpeech({ executable: "x", spawn: () => child.child });
  Reflect.set(globalThis, KEY, controller);
  const first = listen();
  const second = await listen();
  assert.equal(second.status, 409);
  assert.equal((await second.json()).code, "busy");
  assert.deepEqual(await (await first).json(), { text: "primera" });
  uninstall();
});

test("cancel calls the controller once and answers 200; a throwing controller answers a fixed 500", async () => {
  const calls = install(async () => ({ text: "x" }));
  const response = await cancel({ headers: json, rawBody: "{}" });
  assert.equal(response.status, 200);
  assert.deepEqual(await response.json(), { cancelled: true });
  assert.equal(calls.cancel, 1);
  Reflect.set(globalThis, KEY, { listen: async () => ({ text: "x" }), cancel() { throw new Error("C:\\Users\\maria\\secret"); } });
  const failed = await cancel();
  assert.equal(failed.status, 500);
  assert.deepEqual(await failed.json(), { code: "failed", error: reader.VOICE_MESSAGES.failed });
  uninstall();
});

test("a listen request that is abandoned by the browser cancels the helper", async () => {
  const abort = new AbortController();
  let release;
  const calls = install(() => new Promise((resolve) => { release = resolve; }));
  const pending = listenRoute.POST(new Request("http://127.0.0.1:3003/api/voice/listen", {
    method: "POST", headers: { host: "127.0.0.1:3003", origin: "http://127.0.0.1:3003" }, signal: abort.signal,
  }));
  await new Promise((resolve) => setImmediate(resolve));
  abort.abort();
  assert.equal(calls.cancel, 1);
  release({ text: "tarde" });
  await pending;
  uninstall();
});

test("only POST is exported by the voice routes", () => {
  for (const route of [listenRoute, cancelRoute]) {
    assert.equal(typeof route.POST, "function");
    for (const method of ["GET", "PUT", "PATCH", "DELETE"]) assert.equal(route[method], undefined);
  }
});
