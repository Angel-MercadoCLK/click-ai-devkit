import assert from "node:assert/strict";
import { test } from "node:test";
import { readFile } from "node:fs/promises";
import { fileURLToPath } from "node:url";

const redaction = await import("../desktop/redaction.mts");
const ocrModule = await import("../desktop/ocr.mts");
const prep = await import("../desktop/screen-prep.mts");

const patterns = redaction.compileGuardPatterns(redaction.parseGuardPatterns(JSON.parse(await readFile(fileURLToPath(new URL("../lib/guard-patterns.json", import.meta.url)), "utf8"))));
const IMAGE = { width: 1000, height: 300 };

// Words are laid out left to right on one line each, 10 px per character, 20 px tall, like a real OCR result.
function layout(lines, { top = 20, gap = 40, left = 20 } = {}) {
  const words = [];
  lines.forEach((text, line) => {
    let x = left;
    for (const word of text.split(" ")) {
      words.push({ text: word, x, y: top + line * gap, w: word.length * 10, h: 20, line });
      x += word.length * 10 + 10;
    }
  });
  return words;
}

const ids = (regions) => regions.flatMap((region) => region.rules);

test("a single-word match hides exactly that word with padding", () => {
  // click:allow-secret synthetic test data; not real personal data.
  const words = layout(["Contact maria@example.com now"]);
  const regions = redaction.findSensitiveRegions(words, IMAGE, patterns);
  assert.equal(regions.length, 1);
  assert.deepEqual(regions[0].rules, ["pii/email"]);
  const email = words[1];
  const [region] = regions;
  assert.ok(region.x <= email.x && region.x + region.w >= email.x + email.w, "covers the word horizontally");
  assert.ok(region.y <= email.y && region.y + region.h >= email.y + email.h, "covers the word vertically");
  assert.ok(region.x > words[0].x + words[0].w - 20, "does not swallow the previous word");
  assert.ok(region.x + region.w < words[2].x + words[2].w, "does not reach the end of the next word");
});

test("a match spanning several words covers every word it touches and only those", () => {
  // click:allow-secret synthetic test data; not real personal data.
  const words = layout(["Teléfono +54 11 5555-1234 del asegurado", "unrelated words here"]);
  const regions = redaction.findSensitiveRegions(words, IMAGE, patterns);
  assert.deepEqual(ids(regions).sort(), ["pii/phone"]);
  const [region] = regions;
  // The phone rule starts at the country code, so "Teléfono" itself is not part of the match.
  const first = words[1];
  const last = words[3];
  assert.deepEqual([first.text, last.text], ["+54", "5555-1234"]);
  assert.ok(region.x <= first.x && region.x + region.w >= last.x + last.w);
  assert.ok(region.x > words[0].x, "the leading label is left alone");
  assert.ok(region.y + region.h < words.find((item) => item.line === 1).y, "the next line is untouched");
});

test("a value wrapped across two lines is found by scanning neighbouring lines together", () => {
  // "Siniestro" ends line 0 and its identifier starts line 1: neither line matches alone.
  // click:allow-secret synthetic test data; not real personal data.
  const words = layout(["something about Siniestro", "SIN-2026-000123 escalado ahora"]);
  const alone = redaction.findSensitiveRegions(words.filter((word) => word.line === 0), IMAGE, patterns);
  assert.deepEqual(alone, []);
  const regions = redaction.findSensitiveRegions(words, IMAGE, patterns);
  assert.ok(ids(regions).includes("claim-ids/placeholder"));
  const keyword = words.find((word) => word.text === "Siniestro");
  // click:allow-secret synthetic test data; not real personal data.
  const identifier = words.find((word) => word.text === "SIN-2026-000123");
  const covers = (word) => regions.some((region) => region.x <= word.x && region.x + region.w >= word.x + word.w && region.y <= word.y && region.y + region.h >= word.y + word.h);
  assert.ok(covers(keyword) && covers(identifier), "both wrapped halves are hidden");
  const trailing = words.find((word) => word.text === "ahora");
  assert.equal(regions.some((region) => region.x < trailing.x + trailing.w && region.x + region.w > trailing.x && region.y < trailing.y + trailing.h && region.y + region.h > trailing.y), false);
});

test("a value split mid-word across two lines is found by also joining the lines without a space", () => {
  const words = layout(["contact maria.gomez@", "example.com today"]);
  const regions = redaction.findSensitiveRegions(words, IMAGE, patterns);
  const covers = (word) => regions.some((region) => region.x <= word.x && region.x + region.w >= word.x + word.w && region.y <= word.y && region.y + region.h >= word.y + word.h);
  assert.ok(covers(words.find((word) => word.text === "maria.gomez@")), "first half hidden");
  assert.ok(covers(words.find((word) => word.text === "example.com")), "second half hidden");
  assert.equal(covers(words.find((word) => word.text === "today")), false, "unrelated words stay visible");
});

test("several matches give several regions, and overlapping ones are merged", () => {
  // click:allow-secret synthetic test data; not real personal data.
  const separate = redaction.findSensitiveRegions(layout(["maria@example.com", "plain line", "DNI 12345678"]), IMAGE, patterns);
  assert.equal(separate.length, 2);
  // click:allow-secret synthetic test data; not real personal data.
  const merged = redaction.findSensitiveRegions(layout(["email a@example.com b@example.org"]), IMAGE, patterns);
  assert.equal(merged.length, 1, "touching boxes on one line become one region");
});

test("text without personal data yields no regions", () => {
  assert.deepEqual(redaction.findSensitiveRegions(layout(["The quick brown fox", "version 2.10.3 on port 47321"]), IMAGE, patterns), []);
  assert.deepEqual(redaction.findSensitiveRegions([], IMAGE, patterns), []);
});

test("regions are clamped to the image", () => {
  // click:allow-secret synthetic test data; not real personal data.
  const words = [{ text: "a@example.com", x: 0, y: 0, w: 130, h: 20, line: 0 }, { text: "b@example.com", x: 880, y: 285, w: 130, h: 20, line: 1 }];
  const regions = redaction.findSensitiveRegions(words, IMAGE, patterns);
  assert.equal(regions.length, 2);
  for (const region of regions) {
    assert.ok(region.x >= 0 && region.y >= 0 && region.x + region.w <= IMAGE.width && region.y + region.h <= IMAGE.height);
  }
});

test("words with unusable boxes are ignored instead of producing bad regions", () => {
  // click:allow-secret synthetic test data; not real personal data.
  const words = [{ text: "a@example.com", x: Number.NaN, y: 0, w: 100, h: 20, line: 0 }, { text: "b@example.com", x: 0, y: 0, w: 0, h: 20, line: 1 }];
  assert.deepEqual(redaction.findSensitiveRegions(words, IMAGE, patterns), []);
});

test("a very long line is scanned in windows, so a match late in it is still found", () => {
  // Sized from the window so the line always spans several windows, whatever the window size is.
  const filler = Array.from({ length: Math.ceil((redaction.MAX_LINE_CHARS * 1.5) / 6) }, () => "lorem").join(" ");
  assert.ok(filler.length > redaction.MAX_LINE_CHARS * 1.4);
  // click:allow-secret synthetic test data; not real personal data.
  const words = layout([`${filler} maria@example.com ${filler}`], { left: 0 });
  const regions = redaction.findSensitiveRegions(words, { width: 200_000, height: 300 }, patterns);
  assert.ok(regions.some((region) => region.rules.includes("pii/email")));
});

test("a pathological line does not stall the matcher", () => {
  const started = Date.now();
  redaction.findSensitiveRegions(layout(["a".repeat(5_000)]), { width: 60_000, height: 300 }, patterns);
  assert.ok(Date.now() - started < 2_000);
});

function bitmap(width, height) {
  const data = Buffer.alloc(width * height * 4);
  for (let i = 0; i < data.length; i += 1) data[i] = (i * 31 + (i >> 3)) & 0xff;
  return data;
}

test("redactBitmap changes pixels inside the rectangle and leaves every other byte identical", () => {
  const source = bitmap(60, 40);
  const original = Buffer.from(source);
  const out = redaction.redactBitmap(source, 60, 40, [{ x: 10, y: 8, w: 30, h: 12 }]);
  assert.deepEqual(source, original, "the input is never modified");
  let changed = 0;
  for (let y = 0; y < 40; y += 1) {
    for (let x = 0; x < 60; x += 1) {
      const at = (y * 60 + x) * 4;
      const inside = x >= 10 && x < 40 && y >= 8 && y < 20;
      const same = out.subarray(at, at + 4).equals(original.subarray(at, at + 4));
      if (!inside) assert.ok(same, `pixel ${x},${y} outside the box changed`);
      else if (!same) changed += 1;
    }
  }
  assert.ok(changed > 300, "most of the box changed");
});

test("redactBitmap fills each block with one flat colour at least as large as the box height", () => {
  const out = redaction.redactBitmap(bitmap(80, 40), 80, 40, [{ x: 4, y: 4, w: 64, h: 20 }]);
  const pixel = (x, y) => out.subarray((y * 80 + x) * 4, (y * 80 + x) * 4 + 4).toString("hex");
  // 64 wide / 20 block = 3 equal columns; one row because the block is as tall as the box.
  const columns = [[4, 25], [25, 46], [46, 68]];
  for (const [from, to] of columns) {
    const colour = pixel(from, 4);
    for (let y = 4; y < 24; y += 1) for (let x = from; x < to; x += 1) assert.equal(pixel(x, y), colour);
  }
  assert.notEqual(pixel(4, 4), pixel(30, 4));
});

test("redactBitmap clamps rectangles that run off the edges and ignores empty or outside ones", () => {
  const source = bitmap(30, 20);
  const out = redaction.redactBitmap(source, 30, 20, [{ x: -10, y: -10, w: 25, h: 25 }, { x: 25, y: 15, w: 100, h: 100 }, { x: 100, y: 100, w: 5, h: 5 }, { x: 5, y: 5, w: 0, h: 0 }]);
  assert.equal(out.length, source.length);
  const at = (x, y) => (y * 30 + x) * 4;
  assert.ok(!out.subarray(at(0, 0), at(0, 0) + 4).equals(source.subarray(at(0, 0), at(0, 0) + 4)), "the clamped top-left box changed");
  assert.ok(!out.subarray(at(29, 19), at(29, 19) + 4).equals(source.subarray(at(29, 19), at(29, 19) + 4)), "the clamped bottom-right box changed");
  assert.ok(out.subarray(at(20, 8), at(20, 8) + 4).equals(source.subarray(at(20, 8), at(20, 8) + 4)), "untouched middle pixel");
  assert.ok(!out.equals(source));
});

test("redactBitmap rejects a bitmap that does not match its size and invalid rectangles", () => {
  assert.throws(() => redaction.redactBitmap(Buffer.alloc(10), 4, 4, []), /does not match/);
  assert.throws(() => redaction.redactBitmap(Buffer.alloc(64), 4, 4, [{ x: Number.NaN, y: 0, w: 1, h: 1 }]), /invalid/);
  assert.deepEqual(redaction.redactBitmap(bitmap(4, 4), 4, 4, []), bitmap(4, 4));
});

// OCR helper ---------------------------------------------------------------------------------------------------

function fakeChild({ stdout = "", code = 0, delay = 0, never = false, chunks, spawnError = false } = {}) {
  const listeners = {};
  const stdinWrites = [];
  const state = { killed: false, stdinEnded: false, stdinWrites };
  const child = {
    stdin: { write: (chunk) => { stdinWrites.push(chunk); return true; }, end: () => { state.stdinEnded = true; }, on: () => {} },
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

const word = (overrides = {}) => ({ text: "hello", x: 1, y: 2, w: 30, h: 12, line: 0, ...overrides });
const PNG = Buffer.from([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0, 0, 0, 0, 0, 0, 0, 0]);

test("the OCR helper streams the PNG over stdin, returns validated words, and never puts the image on the command line", async () => {
  const calls = [];
  const { child, state } = fakeChild({ stdout: JSON.stringify([word(), word({ text: "wörld", x: 40, line: 1 })]) });
  const ocr = ocrModule.createWindowsOcr({ executable: "powershell.exe", spawn: (executable, args) => { calls.push({ executable, args }); return child; } });
  const words = await ocr(PNG);
  assert.deepEqual(words.map((item) => item.text), ["hello", "wörld"]);
  assert.equal(calls[0].executable, "powershell.exe");
  assert.deepEqual(calls[0].args.slice(0, 7), ["-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-InputFormat", "None", "-EncodedCommand"]);
  assert.equal(calls[0].args.length, 8);
  assert.ok(!calls[0].args.join(" ").includes(PNG.toString("base64")));
  assert.deepEqual(state.stdinWrites, [PNG]);
  assert.equal(state.stdinEnded, true);
  assert.equal(Buffer.from(calls[0].args[7], "base64").toString("utf16le"), ocrModule.OCR_SCRIPT);
});

test("the OCR script reads stdin in memory, uses the Windows engine and writes no files", () => {
  const script = ocrModule.OCR_SCRIPT;
  assert.match(script, /OpenStandardInput/);
  assert.match(script, /InMemoryRandomAccessStream/);
  assert.match(script, /TryCreateFromUserProfileLanguages/);
  assert.match(script, /MaxImageDimension/);
  assert.doesNotMatch(script, /Out-File|Set-Content|\[System\.IO\.File\]|GetTempFileName|Add-Content|\.Save\(/);
});

test("OCR output validation rejects malformed, unexpected and oversized results", async () => {
  const cases = {
    "not json": "garbage",
    "not an array": JSON.stringify({ words: [] }),
    "word without text": JSON.stringify([word({ text: "" })]),
    "non integer box": JSON.stringify([word({ x: 1.5 })]),
    "zero width": JSON.stringify([word({ w: 0 })]),
    "negative line": JSON.stringify([word({ line: -1 })]),
    "extra key": JSON.stringify([{ ...word(), confidence: 1 }]),
    "missing key": JSON.stringify([{ text: "a", x: 1, y: 1, w: 1, h: 1 }]),
    "text not a string": JSON.stringify([word({ text: 5 })]),
    "text too long": JSON.stringify([word({ text: "x".repeat(201) })]),
    "too many words": JSON.stringify(Array.from({ length: ocrModule.OCR_MAX_WORDS + 1 }, () => word())),
  };
  for (const [name, stdout] of Object.entries(cases)) {
    const ocr = ocrModule.createWindowsOcr({ executable: "x", spawn: () => fakeChild({ stdout }).child });
    await assert.rejects(ocr(PNG), (error) => error instanceof ocrModule.OcrError && error.reason === "invalid-output", name);
  }
  assert.deepEqual(ocrModule.parseOcrOutput("[]"), []);
});

test("OCR kills the helper and fails when stdout is larger than the cap", async () => {
  const { child, state } = fakeChild({ chunks: ["[", "x".repeat(2_000), "x".repeat(2_000)] });
  const ocr = ocrModule.createWindowsOcr({ executable: "x", maxOutputBytes: 3_000, spawn: () => child });
  await assert.rejects(ocr(PNG), (error) => error.reason === "invalid-output");
  assert.equal(state.killed, true);
});

test("OCR times out, kills the helper, and fails", async () => {
  const { child, state } = fakeChild({ never: true });
  const ocr = ocrModule.createWindowsOcr({ executable: "x", timeoutMs: 20, spawn: () => child });
  await assert.rejects(ocr(PNG), (error) => error.reason === "timeout");
  assert.equal(state.killed, true);
});

test("OCR failures map to fixed reasons: missing language, crash, missing PowerShell, wrong platform", async () => {
  const run = (options) => ocrModule.createWindowsOcr({ executable: "x", ...options })(PNG);
  await assert.rejects(run({ spawn: () => fakeChild({ code: 2 }).child }), (error) => error.reason === "unavailable");
  await assert.rejects(run({ spawn: () => fakeChild({ code: 1 }).child }), (error) => error.reason === "failed");
  await assert.rejects(run({ spawn: () => fakeChild({ spawnError: true }).child }), (error) => error.reason === "unavailable");
  await assert.rejects(run({ spawn: () => { throw new Error("boom"); } }), (error) => error.reason === "unavailable");
  await assert.rejects(run({ spawn: () => fakeChild().child }), (error) => error.reason === "invalid-output");
  await assert.rejects(ocrModule.createWindowsOcr({ executable: "x", spawn: () => fakeChild().child })(Buffer.alloc(0)), (error) => error.reason === "failed");
  if (process.platform !== "win32") await assert.rejects(ocrModule.createWindowsOcr({ spawn: () => assert.fail("must not start") })(PNG), (error) => error.reason === "unavailable");
  assert.throws(() => ocrModule.powershellPath({}, "linux"), /only available on Windows/);
  assert.match(ocrModule.powershellPath({ SystemRoot: "D:\\Win" }, "win32"), /D:\\Win.*powershell\.exe$/);
});

// Pipeline -----------------------------------------------------------------------------------------------------

function frame(overrides = {}) {
  return { image: Buffer.from("PNG-BYTES"), width: 1000, height: 300, capturedAt: 1_000, ...overrides };
}

function makePreparer(overrides = {}) {
  const calls = { ocr: 0, redact: 0, rects: undefined };
  const preparer = prep.createScreenPreparer({
    frames: () => [frame({ capturedAt: 500 }), frame()],
    isCapturing: () => true,
    session: () => 1,
    // click:allow-secret synthetic test data; not real personal data.
    ocr: async () => { calls.ocr += 1; return layout(["write to maria@example.com now"]); },
    patterns: async () => patterns,
    redact: async (png, size, rects) => { calls.redact += 1; calls.rects = rects; return { image: Buffer.from("REDACTED"), mime: "image/png" }; },
    now: () => 1_500,
    ...overrides,
  });
  return { preparer, calls };
}

test("the pipeline OCRs the newest frame, redacts the found regions and returns only the redacted image", async () => {
  const { preparer, calls } = makePreparer();
  const result = await preparer();
  assert.equal(result.image.toString(), "REDACTED");
  assert.equal(result.mime, "image/png");
  assert.equal(result.hiddenRegions, 1);
  assert.equal(result.capturedAt, 1_000, "the newest frame was used");
  assert.deepEqual(Object.keys(result).sort(), ["capturedAt", "hiddenRegions", "image", "mime"]);
  assert.equal(calls.rects.length, 1);
});

test("the pipeline fails closed with a fixed code when any stage fails, returning nothing", async () => {
  const expectCode = async (overrides, code, calls) => {
    const { preparer, calls: seen } = makePreparer(overrides);
    await assert.rejects(preparer(), (error) => error instanceof prep.ScreenPrepareError && error.code === code, code);
    return seen;
  };
  // click:allow-secret synthetic test data; not real personal data.
  await expectCode({ ocr: async () => { throw new Error("secret detail maria@example.com"); } }, "ocr-failed");
  await expectCode({ ocr: async () => { throw new ocrModule.OcrError("unavailable", "x"); } }, "ocr-unavailable");
  await expectCode({ ocr: async () => { throw new ocrModule.OcrError("timeout", "x"); } }, "ocr-failed");
  await expectCode({ patterns: async () => { throw new Error("bad file"); } }, "patterns-unavailable");
  await expectCode({ redact: async () => { throw new Error("decode failed"); } }, "redaction-failed");
  await expectCode({ frames: () => [] }, "no-frame");
  await expectCode({ frames: () => [frame({ capturedAt: 1_000 })], now: () => 1_000 + prep.MAX_FRAME_AGE_MS + 1 }, "stale-frame");
  await expectCode({ isCapturing: () => false }, "capture-stopped");
  const seen = await expectCode({ ocr: async () => { throw new Error("x"); } }, "ocr-failed");
  assert.equal(seen.redact, 0, "nothing is redacted or returned after an OCR failure");
});

test("a result that finishes after capture was paused or turned off is discarded", async () => {
  let capturing = true;
  const { preparer } = makePreparer({ redact: async () => { capturing = false; return { image: Buffer.from("R"), mime: "image/png" }; }, isCapturing: () => capturing });
  await assert.rejects(preparer(), (error) => error.code === "capture-stopped");
});

test("a frame from an earlier capture session is discarded when capture went off and on, or paused and resumed, during OCR", async () => {
  const { createCaptureService } = await import("../desktop/capture.mts");
  const scenarios = [
    ["off then on", (service) => { service.disable(); service.enable(); }],
    ["paused then resumed", (service) => { service.pause(); service.resume(); }],
  ];
  for (const [label, interrupt] of scenarios) {
    const service = createCaptureService({
      capture: async () => ({ image: Buffer.from("PNG"), width: 10, height: 10 }),
      timers: { setInterval: () => 1, clearInterval() {}, now: () => 1_000 },
      log() {},
    });
    service.enable();
    await new Promise((resolve) => setImmediate(resolve));
    let ocrStarted;
    const started = new Promise((resolve) => { ocrStarted = resolve; });
    let release;
    const gate = new Promise((resolve) => { release = resolve; });
    const preparer = prep.createScreenPreparer({
      frames: () => service.frames(),
      isCapturing: () => service.status().enabled && !service.status().paused,
      session: () => service.session(),
      // click:allow-secret synthetic test data; not real personal data.
      ocr: async () => { ocrStarted(); await gate; return layout(["write to maria@example.com now"]); },
      patterns: async () => patterns,
      redact: async () => ({ image: Buffer.from("REDACTED"), mime: "image/png" }),
      now: () => 1_000,
    });
    const pending = preparer();
    const outcome = pending.then(() => "returned", (error) => error.code);
    await started;
    interrupt(service);
    release();
    assert.equal(await outcome, "capture-stopped", label);
  }
});

test("an uninterrupted run returns the redacted image and the session identity is read when preparation starts", async () => {
  let id = 7;
  const reads = [];
  const { preparer } = makePreparer({ session: () => { reads.push(id); return id; } });
  const result = await preparer();
  assert.equal(result.image.toString(), "REDACTED");
  assert.equal(reads[0], 7);
  const { preparer: changing } = makePreparer({ session: () => id++ });
  await assert.rejects(changing(), (error) => error.code === "capture-stopped");
});

test("concurrent requests share one OCR run, and the next request starts a new one", async () => {
  const { preparer, calls } = makePreparer();
  const [a, b] = await Promise.all([preparer(), preparer()]);
  assert.equal(a, b);
  assert.equal(calls.ocr, 1);
  await preparer();
  assert.equal(calls.ocr, 2);
});

test("the pipeline never keeps the original frame: only the redacted image is in the result", async () => {
  const original = Buffer.from("ORIGINAL-FRAME-PIXELS");
  const { preparer } = makePreparer({ frames: () => [frame({ image: original })] });
  const result = await preparer();
  assert.equal(result.image.includes(original), false);
});

function fakeNativeImage({ size = { width: 4, height: 2 }, png = Buffer.alloc(100), jpeg = Buffer.alloc(50), empty = false } = {}) {
  const created = [];
  const image = (bytes) => ({ isEmpty: () => empty, getSize: () => size, toBitmap: () => bytes ?? Buffer.alloc(size.width * size.height * 4, 7), toPNG: () => png, toJPEG: (quality) => { created.push(quality); return jpeg; } });
  return { api: { createFromBuffer: () => image(), createFromBitmap: (bitmap, options) => { created.push(["bitmap", bitmap.length, options]); return image(bitmap); } }, created };
}

test("the nativeImage wrapper redacts the bitmap and re-encodes it as PNG at the original size", async () => {
  const { api, created } = fakeNativeImage();
  const redact = prep.createNativeRedactor(api);
  const result = await redact(Buffer.from("png"), { width: 4, height: 2 }, [{ x: 0, y: 0, w: 4, h: 2 }]);
  assert.equal(result.mime, "image/png");
  assert.deepEqual(created[0], ["bitmap", 32, { width: 4, height: 2 }]);
});

test("the nativeImage wrapper falls back to JPEG only above the image budget, and fails when even that is too big", async () => {
  const big = Buffer.alloc(prep.SCREEN_IMAGE_MAX_BYTES + 1);
  const fallback = await prep.createNativeRedactor(fakeNativeImage({ png: big }).api)(Buffer.from("png"), { width: 4, height: 2 }, []);
  assert.equal(fallback.mime, "image/jpeg");
  await assert.rejects(prep.createNativeRedactor(fakeNativeImage({ png: big, jpeg: big }).api)(Buffer.from("png"), { width: 4, height: 2 }, []), (error) => error.code === "too-large");
  await assert.rejects(prep.createNativeRedactor(fakeNativeImage({ size: { width: 9, height: 9 } }).api)(Buffer.from("png"), { width: 4, height: 2 }, []), (error) => error.code === "redaction-failed");
  await assert.rejects(prep.createNativeRedactor(fakeNativeImage({ empty: true }).api)(Buffer.from("png"), { width: 4, height: 2 }, []), (error) => error.code === "redaction-failed");
});
