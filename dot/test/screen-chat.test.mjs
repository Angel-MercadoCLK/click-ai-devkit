import assert from "node:assert/strict";
import { after, before, test } from "node:test";
import { chmod, mkdtemp, rm, writeFile } from "node:fs/promises";
import { inflateSync, crc32 } from "node:zlib";
import { tmpdir } from "node:os";
import { join } from "node:path";

const directory = await mkdtemp(join(tmpdir(), "click-dot-screen-chat-test-"));
await chmod(directory, 0o700);
process.env.CLICK_DOT_AUTH_FILE = join(directory, "auth.json");
process.env.CLICK_DOT_CHAT_FILE = join(directory, "conversations.json");
process.env.PORT = "3000";

const authStore = await import("../lib/auth-store.ts");
const historyStore = await import("../lib/conversation-store.ts");
const provider = await import("../lib/chat-provider.ts");
const previews = await import("../lib/screen-previews.ts");
const reader = await import("../lib/capture-controller.ts");
const probe = await import("../lib/vision-probe.ts");
const desktopPrep = await import("../desktop/screen-prep.mts");
const previewRoute = await import("../app/api/capture/preview/route.ts");
const streamRoute = await import("../app/api/chat/stream/route.ts");
const historyRoute = await import("../app/api/chat/history/route.ts");
const probeRoute = await import("../app/api/chat/vision-probe/route.ts");
const originalFetch = globalThis.fetch;
const CONTROLLER_KEY = Symbol.for("click-dot.capture-controller");

const accessToken = "offline-fixture-token-never-a-real-credential";
const PNG = probe.syntheticProbePng();

async function writeAuth() {
  await writeFile(process.env.CLICK_DOT_AUTH_FILE, JSON.stringify({
    hostId: "urn:uuid:offline-fixture", generation: 4, attempts: {}, issuedClientId: "client-offline-fixture",
    connection: { clientId: "client-offline-fixture", issuer: "https://auth.openai.com", subject: "account-one", accessToken, refreshToken: "fixture-refresh-token", idToken: "fixture-id-token", scopes: ["chatgpt.tokens.use.direct"], expiresAt: Date.now() + 60_000 },
  }), { mode: 0o600 });
  await chmod(process.env.CLICK_DOT_AUTH_FILE, 0o600);
}

function localRequest(url, init = {}) {
  const method = init.method ?? "GET";
  return new Request(`http://127.0.0.1:3000${url}`, {
    ...init, method,
    headers: { host: "127.0.0.1:3000", ...(method !== "GET" ? { origin: "http://127.0.0.1:3000" } : {}), ...(init.headers ?? {}) },
  });
}

const json = (body) => ({ method: "POST", headers: { "content-type": "application/json" }, body: JSON.stringify(body) });
const fakeModels = () => new Response(JSON.stringify({ models: [{ slug: "model-first", display_name: "First model", visibility: "list" }] }), { headers: { "content-type": "application/json" } });
const sse = (chunks) => new Response(chunks.join(""), { headers: { "content-type": "text/event-stream" } });
const deltaFrame = 'data: {"type":"response.output_text.delta","delta":"Seen"}\n\n';
const completeFrame = 'data: {"type":"response.completed"}\n\n';
const goodStream = () => sse([deltaFrame, completeFrame]);

function installFetch(handler) {
  const calls = [];
  globalThis.fetch = async (url, init) => {
    calls.push({ url: String(url), init });
    return String(url).endsWith("/models") ? fakeModels() : handler(url, init);
  };
  return calls;
}
const responsesCalls = (calls) => calls.filter((call) => call.url.endsWith("/responses"));
const dataUrlOf = (screen) => previews.toDataUrl(screen);

function installController(controller) {
  globalThis[CONTROLLER_KEY] = controller;
}

// Sending a screen requires capture to be on and not paused, as it is in the desktop app.
const installCapturing = () => installController({ status: () => ({ enabled: true, paused: false, frames: 1, lastCapturedAt: Date.now() }), apply: async () => ({}) });

async function freshRevision() {
  const snapshot = await authStore.getConnectionSnapshot();
  await historyStore.clearConversations(snapshot);
  return (await historyStore.listConversations(snapshot)).revision;
}

before(async () => writeAuth());
after(async () => {
  globalThis.fetch = originalFetch;
  delete globalThis[CONTROLLER_KEY];
  await rm(directory, { recursive: true, force: true });
});

// Shared limits -----------------------------------------------------------------------------------------------

test("the desktop and web sides agree on the image byte budget", () => {
  assert.equal(previews.SCREEN_IMAGE_MAX_BYTES, desktopPrep.SCREEN_IMAGE_MAX_BYTES);
  assert.equal(previews.SCREEN_IMAGE_MAX_BYTES, 4 * 1024 * 1024);
});

// Preview store -----------------------------------------------------------------------------------------------

test("a preview can be taken once, expires after its time to live, and unknown ids give nothing", () => {
  let now = 1_000;
  const store = previews.createPreviewStore({ ttlMs: 5_000, now: () => now });
  const { id, expiresAt } = store.put({ image: PNG, mime: "image/png" });
  assert.match(id, previews.PREVIEW_ID_PATTERN);
  assert.equal(expiresAt, 6_000);
  assert.equal(store.take("A".repeat(22)), undefined);
  assert.deepEqual(store.take(id).image, PNG);
  assert.equal(store.take(id), undefined, "one use");
  const second = store.put({ image: PNG, mime: "image/png" });
  now += 5_000;
  assert.equal(store.take(second.id), undefined, "expired");
  assert.equal(store.size(), 0);
});

test("the store is bounded and drops the oldest preview first", () => {
  const store = previews.createPreviewStore({ maxEntries: 2 });
  const first = store.put({ image: PNG, mime: "image/png" });
  const second = store.put({ image: PNG, mime: "image/png" });
  const third = store.put({ image: PNG, mime: "image/png" });
  assert.equal(store.size(), 2);
  assert.equal(store.take(first.id), undefined);
  assert.ok(store.take(second.id));
  assert.ok(store.take(third.id));
});

test("preview ids are random and the shared store is one instance per process", () => {
  const store = previews.createPreviewStore();
  const ids = new Set(Array.from({ length: 4 }, () => store.put({ image: PNG, mime: "image/png" }).id));
  assert.equal(ids.size, 4);
  assert.equal(previews.screenPreviews(), previews.screenPreviews());
});

// Bridge reader -----------------------------------------------------------------------------------------------

const prepared = (overrides = {}) => ({ image: Buffer.from(PNG), mime: "image/png", hiddenRegions: 2, capturedAt: 1_000, ...overrides });

test("the reader accepts only a bounded PNG or JPEG whose bytes match its declared type", () => {
  assert.equal(reader.sanitizePreparedScreen(prepared()).hiddenRegions, 2);
  const jpeg = Buffer.concat([Buffer.from([0xff, 0xd8, 0xff, 0xe0]), Buffer.alloc(32)]);
  assert.equal(reader.sanitizePreparedScreen(prepared({ image: jpeg, mime: "image/jpeg" })).mime, "image/jpeg");
  const rejected = [
    prepared({ mime: "image/jpeg" }),
    prepared({ mime: "image/gif" }),
    prepared({ image: Buffer.from("not an image at all, just text bytes") }),
    prepared({ image: Buffer.alloc(previews.SCREEN_IMAGE_MAX_BYTES + 1) }),
    prepared({ image: "string" }),
    prepared({ hiddenRegions: -1 }),
    prepared({ hiddenRegions: 1.5 }),
    prepared({ capturedAt: Number.NaN }),
    null, "x", 5,
  ];
  for (const value of rejected) assert.equal(reader.sanitizePreparedScreen(value), undefined);
});

test("prepareScreen maps every failure to a known code and never forwards the raw error", async () => {
  await assert.rejects(reader.prepareScreen({ status() {}, apply() {} }), (error) => error.code === "unavailable");
  const failing = (error) => ({ status() {}, apply() {}, prepareScreen: async () => { throw error; } });
  // click:allow-secret synthetic test data; not real personal data.
  await assert.rejects(reader.prepareScreen(failing(Object.assign(new Error("secret: maria@example.com"), { code: "ocr-failed" }))), (error) => error.code === "ocr-failed" && !/secret|maria/.test(error.message));
  await assert.rejects(reader.prepareScreen(failing(Object.assign(new Error("x"), { code: "made-up" }))), (error) => error.code === "invalid-result");
  await assert.rejects(reader.prepareScreen(failing(new Error("plain"))), (error) => error.code === "invalid-result");
  await assert.rejects(reader.prepareScreen({ status() {}, apply() {}, prepareScreen: async () => ({ image: "no" }) }), (error) => error.code === "invalid-result");
  assert.equal((await reader.prepareScreen({ status() {}, apply() {}, prepareScreen: async () => prepared() })).capturedAt, 1_000);
});

// Preview route -----------------------------------------------------------------------------------------------

test("the preview route is local-only and needs the exact origin", async () => {
  installController({ status: () => ({}), apply: async () => ({}), prepareScreen: async () => prepared() });
  const noOrigin = await previewRoute.POST(new Request("http://127.0.0.1:3000/api/capture/preview", { method: "POST", headers: { host: "127.0.0.1:3000" } }));
  assert.equal(noOrigin.status, 403);
  const other = await previewRoute.POST(localRequest("/api/capture/preview", { method: "POST", headers: { origin: "http://evil.example" } }));
  assert.equal(other.status, 403);
});

test("the preview route fails with 503 outside the desktop app or when the controller offers no screen", async () => {
  delete globalThis[CONTROLLER_KEY];
  const missing = await previewRoute.POST(localRequest("/api/capture/preview", { method: "POST" }));
  assert.equal(missing.status, 503);
  installController({ status: () => ({}), apply: async () => ({}) });
  const unsupported = await previewRoute.POST(localRequest("/api/capture/preview", { method: "POST" }));
  assert.equal(unsupported.status, 503);
});

test("the preview route returns a data URL, the hidden-region count and an opaque id, and keeps the image server-side", async () => {
  installController({ status: () => ({}), apply: async () => ({}), prepareScreen: async () => prepared({ hiddenRegions: 3 }) });
  const response = await previewRoute.POST(localRequest("/api/capture/preview", { method: "POST", body: "ignored", headers: { "content-type": "text/plain" } }));
  assert.equal(response.status, 200);
  assert.equal(response.headers.get("cache-control"), "no-store, private");
  const body = await response.json();
  assert.deepEqual(Object.keys(body).sort(), ["capturedAt", "expiresAt", "hiddenRegions", "image", "previewId"]);
  assert.equal(body.hiddenRegions, 3);
  assert.equal(body.image, `data:image/png;base64,${PNG.toString("base64")}`);
  const stored = previews.screenPreviews().take(body.previewId);
  assert.deepEqual(Buffer.from(stored.image), PNG);
  assert.equal(previews.screenPreviews().take(body.previewId), undefined);
});

test("the preview route reports a fixed, non-leaking reason when preparation fails and stores nothing", async () => {
  previews.screenPreviews().clear();
  // click:allow-secret synthetic test data; not real personal data.
  installController({ status: () => ({}), apply: async () => ({}), prepareScreen: async () => { throw Object.assign(new Error("maria@example.com leaked in stack"), { code: "ocr-failed" }); } });
  const response = await previewRoute.POST(localRequest("/api/capture/preview", { method: "POST" }));
  assert.equal(response.status, 409);
  const text = await response.text();
  assert.doesNotMatch(text, /maria|leaked|stack/);
  assert.equal(JSON.parse(text).code, "ocr-failed");
  assert.equal(previews.screenPreviews().size(), 0);
  installController({ status: () => ({}), apply: async () => ({}), prepareScreen: async () => ({ image: Buffer.from("x") }) });
  const invalid = await previewRoute.POST(localRequest("/api/capture/preview", { method: "POST" }));
  assert.equal(invalid.status, 409);
  assert.equal(previews.screenPreviews().size(), 0);
});

// Provider ----------------------------------------------------------------------------------------------------

const textOnlyBody = (prompt) => JSON.stringify({ model: "model-first", input: [{ role: "user", content: prompt }], store: false, stream: true });
const imagePart = (url = dataUrlOf({ image: PNG, mime: "image/png" })) => [{ type: "input_text", text: "Look" }, { type: "input_image", image_url: url }];

async function drain(generator) {
  const events = [];
  for await (const event of generator) events.push(event);
  return events;
}

test("a text-only provider request is byte-identical to the previous shape", async () => {
  const connection = await authStore.getConnection();
  let body;
  await drain(provider.streamChatResponse(connection, "model-first", [{ role: "user", content: "Hello" }], async (_url, init) => { body = init.body; return goodStream(); }));
  assert.equal(body, textOnlyBody("Hello"));
});

test("an image message sends input_text plus input_image with the same store:false stream:true request", async () => {
  const connection = await authStore.getConnection();
  let sent;
  const url = dataUrlOf({ image: PNG, mime: "image/png" });
  await drain(provider.streamChatResponse(connection, "model-first", [{ role: "assistant", content: "earlier" }, { role: "user", content: imagePart(url) }].slice(1), async (_u, init) => { sent = JSON.parse(init.body); return goodStream(); }));
  assert.deepEqual(sent, { model: "model-first", input: [{ role: "user", content: [{ type: "input_text", text: "Look" }, { type: "input_image", image_url: url }] }], store: false, stream: true });
});

test("the image has its own byte budget, separate from the 128,000-byte text budget", async () => {
  const connection = await authStore.getConnection();
  let calls = 0;
  const fetcher = async () => { calls += 1; return goodStream(); };
  const bigButAllowed = `data:image/png;base64,${Buffer.alloc(previews.SCREEN_IMAGE_MAX_BYTES - 1_000, 1).toString("base64")}`;
  assert.ok(bigButAllowed.length > 128_000);
  await drain(provider.streamChatResponse(connection, "model-first", [{ role: "user", content: imagePart(bigButAllowed) }], fetcher));
  assert.equal(calls, 1);

  const tooBig = `data:image/png;base64,${Buffer.alloc(previews.SCREEN_IMAGE_MAX_BYTES + 1, 1).toString("base64")}`;
  await assert.rejects(drain(provider.streamChatResponse(connection, "model-first", [{ role: "user", content: imagePart(tooBig) }], fetcher)), /too large or invalid/);
  assert.equal(calls, 1, "an oversized image is rejected before any request");

  const history = Array.from({ length: 10 }, (_, index) => ({ role: index % 2 ? "assistant" : "user", content: "x".repeat(15_000) }));
  await assert.rejects(drain(provider.streamChatResponse(connection, "model-first", [...history.slice(0, 9), { role: "user", content: imagePart() }], fetcher)), /too large or invalid/);
  assert.equal(calls, 1, "the text budget still applies when an image is attached");
});

test("only one image, only on a user message, only as a PNG or JPEG data URL", async () => {
  const connection = await authStore.getConnection();
  let calls = 0;
  const fetcher = async () => { calls += 1; return goodStream(); };
  const reject = async (input) => assert.rejects(drain(provider.streamChatResponse(connection, "model-first", input, fetcher)), /too large or invalid/);
  await reject([{ role: "user", content: imagePart() }, { role: "assistant", content: "ok" }, { role: "user", content: imagePart() }]);
  await reject([{ role: "assistant", content: imagePart() }]);
  await reject([{ role: "user", content: imagePart("https://example.com/screen.png") }]);
  await reject([{ role: "user", content: imagePart("data:image/svg+xml;base64,AAAA") }]);
  await reject([{ role: "user", content: [{ type: "input_image", image_url: dataUrlOf({ image: PNG, mime: "image/png" }) }] }]);
  await reject([{ role: "user", content: [...imagePart(), { type: "input_text", text: "extra" }] }]);
  await reject([{ role: "user", content: [{ type: "input_text", text: "Look", detail: "x" }, imagePart()[1]] }]);
  await reject([{ role: "user", content: [{ type: "input_text", text: "" }, imagePart()[1]] }]);
  assert.equal(calls, 0);
});

test("a model that refuses images gets a clear, non-leaking error; other failures stay generic", async () => {
  const connection = await authStore.getConnection();
  const attempt = (response, input) => drain(provider.streamChatResponse(connection, "model-first", input, async () => response()));
  const withImage = [{ role: "user", content: imagePart() }];
  const refused = new Response(JSON.stringify({ error: { message: "This model does not support image inputs: SECRET-DETAIL", code: "unsupported_modality" } }), { status: 400 });
  await assert.rejects(attempt(() => refused, withImage), (error) => error instanceof provider.ScreenImageRejectedError && !/SECRET/.test(error.message) && /did not accept the screen image/.test(error.message));
  await assert.rejects(attempt(() => new Response(JSON.stringify({ error: { message: "bad request" } }), { status: 400 }), withImage), (error) => !(error instanceof provider.ScreenImageRejectedError) && /could not start/.test(error.message));
  await assert.rejects(attempt(() => new Response("image", { status: 500 }), withImage), (error) => !(error instanceof provider.ScreenImageRejectedError));
  const failedEvent = 'data: {"type":"response.failed","response":{"error":{"code":"invalid_image","message":"cannot read the image SECRET"}}}\n\n';
  await assert.rejects(attempt(() => sse([failedEvent]), withImage), (error) => error instanceof provider.ScreenImageRejectedError && !/SECRET/.test(error.message));
  // Without an image in the request, an image-flavoured error body is not reinterpreted.
  await assert.rejects(attempt(() => new Response(JSON.stringify({ error: { message: "image" } }), { status: 400 }), [{ role: "user", content: "text only" }]), (error) => !(error instanceof provider.ScreenImageRejectedError));
});

// Stream route ------------------------------------------------------------------------------------------------

test("text-only messages through the stream route are unchanged: no image part, no marker, same request body", async () => {
  const revision = await freshRevision();
  const calls = installFetch(() => goodStream());
  const response = await streamRoute.POST(localRequest("/api/chat/stream", json({ conversationId: crypto.randomUUID(), revision, model: "model-first", prompt: "Plain" })));
  assert.equal(response.status, 200);
  assert.match(await response.text(), /event: completed/);
  assert.equal(responsesCalls(calls)[0].init.body, textOnlyBody("Plain"));
  const history = await (await historyRoute.GET(localRequest("/api/chat/history"))).json();
  assert.deepEqual(history.conversations[0].messages.map((m) => m.content), ["Plain", "Seen"]);
});

test("a prepared preview is never sent once capture is paused, off or unavailable, and pending previews are dropped", async () => {
  for (const state of [{ enabled: true, paused: true }, { enabled: false, paused: false }, undefined]) {
    const revision = await freshRevision();
    const { id } = previews.screenPreviews().put({ image: PNG, mime: "image/png" });
    if (state) installController({ status: () => ({ ...state, frames: 0, lastCapturedAt: null }), apply: async () => ({}) });
    else delete globalThis[CONTROLLER_KEY];
    const calls = installFetch(() => goodStream());
    const response = await streamRoute.POST(localRequest("/api/chat/stream", json({ conversationId: crypto.randomUUID(), revision, model: "model-first", prompt: "Look", screenPreviewId: id })));
    assert.equal(response.status, 409, JSON.stringify(state));
    assert.equal(responsesCalls(calls).length, 0, "nothing goes upstream");
    assert.equal(previews.screenPreviews().size(), 0, "pending previews are dropped");
  }
});

test("a screen preview id becomes input_text plus input_image, and the history keeps a marker instead of the image", async () => {
  installCapturing();
  const revision = await freshRevision();
  const { id } = previews.screenPreviews().put({ image: PNG, mime: "image/png" });
  const calls = installFetch(() => goodStream());
  const conversationId = crypto.randomUUID();
  const response = await streamRoute.POST(localRequest("/api/chat/stream", json({ conversationId, revision, model: "model-first", prompt: "What is this?", screenPreviewId: id })));
  assert.equal(response.status, 200);
  assert.match(await response.text(), /event: completed/);
  const sent = JSON.parse(responsesCalls(calls)[0].init.body);
  assert.deepEqual(sent.input, [{ role: "user", content: [{ type: "input_text", text: "What is this?" }, { type: "input_image", image_url: `data:image/png;base64,${PNG.toString("base64")}` }] }]);
  assert.equal(sent.store, false);
  assert.equal(sent.stream, true);
  assert.equal(sent.tools, undefined);
  const historyText = await (await historyRoute.GET(localRequest("/api/chat/history"))).text();
  assert.doesNotMatch(historyText, /base64|data:image/);
  const history = JSON.parse(historyText);
  assert.deepEqual(history.conversations[0].messages.map((m) => m.content), ["What is this?\n\n[screen attached]", "Seen"]);
  // The stored file holds no image bytes either.
  const raw = await historyStore.getHistoryPathForTests();
  const { readFile } = await import("node:fs/promises");
  assert.doesNotMatch(await readFile(raw, "utf8"), new RegExp(PNG.toString("base64").slice(0, 40)));
});

test("a preview can be used once; unknown, used, expired and malformed ids never reach the provider", async () => {
  installCapturing();
  const revision = await freshRevision();
  const { id } = previews.screenPreviews().put({ image: PNG, mime: "image/png" });
  const calls = installFetch(() => goodStream());
  const send = async (screenPreviewId) => streamRoute.POST(localRequest("/api/chat/stream", json({
    conversationId: crypto.randomUUID(),
    revision: (await historyStore.listConversations(await authStore.getConnectionSnapshot())).revision,
    model: "model-first", prompt: "Again", screenPreviewId,
  })));
  assert.equal((await (await send(id)).text()).includes("event: completed"), true);
  assert.ok(revision >= 0);
  const used = await send(id);
  assert.equal(used.status, 410);
  assert.match((await used.json()).error, /expired/);
  assert.equal((await send("A".repeat(22))).status, 410);
  assert.equal((await send("short")).status, 400);
  assert.equal((await send(42)).status, 400);
  assert.equal((await send(null)).status, 400);
  assert.equal(responsesCalls(calls).length, 1);

  let now = 0;
  const store = previews.createPreviewStore({ ttlMs: 10, now: () => now });
  const expired = store.put({ image: PNG, mime: "image/png" });
  now = 11;
  assert.equal(store.take(expired.id), undefined);
});

test("a preview is only spent after the other checks pass", async () => {
  const revision = await freshRevision();
  const { id } = previews.screenPreviews().put({ image: PNG, mime: "image/png" });
  installFetch(() => goodStream());
  const badModel = await streamRoute.POST(localRequest("/api/chat/stream", json({ conversationId: crypto.randomUUID(), revision, model: "not-in-catalog", prompt: "x", screenPreviewId: id })));
  assert.equal(badModel.status, 400);
  const staleRevision = await streamRoute.POST(localRequest("/api/chat/stream", json({ conversationId: crypto.randomUUID(), revision: revision + 7, model: "model-first", prompt: "x", screenPreviewId: id })));
  assert.equal(staleRevision.status, 409);
  assert.ok(previews.screenPreviews().take(id), "still available after failed checks");
});

test("the browser can never supply image bytes: image-shaped fields are rejected before anything else happens", async () => {
  const revision = await freshRevision();
  const calls = installFetch(() => goodStream());
  const base = { conversationId: crypto.randomUUID(), revision, model: "model-first", prompt: "Hello" };
  const dataUrl = `data:image/png;base64,${PNG.toString("base64")}`;
  for (const extra of [
    { image: dataUrl }, { images: [dataUrl] }, { screen: dataUrl }, { screenshot: dataUrl }, { image_url: dataUrl },
    { input: [{ role: "user", content: [{ type: "input_image", image_url: dataUrl }] }] },
    { attachments: [{ type: "input_image", image_url: dataUrl }] }, { screenPreview: dataUrl },
  ]) {
    const response = await streamRoute.POST(localRequest("/api/chat/stream", json({ ...base, ...extra })));
    assert.equal(response.status, 400, JSON.stringify(Object.keys(extra)));
  }
  assert.equal((await streamRoute.POST(localRequest("/api/chat/stream", json({ ...base, prompt: dataUrl.slice(0, 20) })))).status, 200, "a prompt that merely looks like a data URL is just text");
  const sent = JSON.parse(responsesCalls(calls)[0].init.body);
  assert.equal(typeof sent.input[0].content, "string", "such a prompt stays a plain string, never an image part");
  // The request body limit still protects the route from a large inline image.
  const huge = await streamRoute.POST(localRequest("/api/chat/stream", json({ ...base, image: "x".repeat(30_000) })));
  assert.equal(huge.status, 400);
});

test("when the model rejects the image the page gets the clear message, nothing leaks, and nothing is saved", async () => {
  installCapturing();
  const revision = await freshRevision();
  const { id } = previews.screenPreviews().put({ image: PNG, mime: "image/png" });
  installFetch(() => new Response(JSON.stringify({ error: { message: "model cannot take image: SECRET-UPSTREAM" } }), { status: 400 }));
  const response = await streamRoute.POST(localRequest("/api/chat/stream", json({ conversationId: crypto.randomUUID(), revision, model: "model-first", prompt: "Look", screenPreviewId: id })));
  const text = await response.text();
  assert.match(text, /event: error/);
  assert.match(text, /did not accept the screen image/);
  assert.doesNotMatch(text, /SECRET|offline-fixture-token|data:image|base64/);
  const history = await (await historyRoute.GET(localRequest("/api/chat/history"))).json();
  assert.deepEqual(history.conversations, []);
});

test("an assistant error about something else keeps the previous generic wording", async () => {
  const revision = await freshRevision();
  installFetch(() => new Response("nope", { status: 500 }));
  const response = await streamRoute.POST(localRequest("/api/chat/stream", json({ conversationId: crypto.randomUUID(), revision, model: "model-first", prompt: "Plain" })));
  assert.match(await response.text(), /ChatGPT could not complete and save this response\. Try again or reconnect/);
});

// Vision probe ------------------------------------------------------------------------------------------------

test("the probe image is a valid 64x64 PNG with correct checksums and no payload beyond flat colour", () => {
  assert.deepEqual([...PNG.subarray(0, 8)], [0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]);
  let offset = 8;
  const types = [];
  let idat;
  while (offset < PNG.length) {
    const length = PNG.readUInt32BE(offset);
    const type = PNG.subarray(offset + 4, offset + 8).toString("ascii");
    const data = PNG.subarray(offset + 8, offset + 8 + length);
    assert.equal(PNG.readUInt32BE(offset + 8 + length), crc32(PNG.subarray(offset + 4, offset + 8 + length)), `${type} checksum`);
    if (type === "IHDR") { assert.equal(data.readUInt32BE(0), 64); assert.equal(data.readUInt32BE(4), 64); }
    if (type === "IDAT") idat = data;
    types.push(type);
    offset += 12 + length;
  }
  assert.deepEqual(types, ["IHDR", "IDAT", "IEND"]);
  assert.equal(inflateSync(idat).length, (64 * 3 + 1) * 64);
  assert.ok(PNG.length < 1_000);
});

test("the probe reports acceptance with only status, event types and no text", async () => {
  const connection = await authStore.getConnection();
  let sent;
  const result = await probe.runVisionProbe(connection, "model-first", async (_url, init) => {
    sent = JSON.parse(init.body);
    return sse(['data: {"type":"response.created"}\n\n', 'data: {"type":"response.output_text.delta","delta":"OK-SECRET-TEXT"}\n\n', completeFrame]);
  });
  assert.deepEqual(result, { model: "model-first", httpStatus: 200, accepted: true, eventTypes: ["response.created", "response.output_text.delta", "response.completed"], errorCode: null });
  assert.doesNotMatch(JSON.stringify(result), /SECRET/);
  assert.equal(sent.store, false);
  assert.equal(sent.stream, true);
  assert.equal(sent.input[0].content[1].type, "input_image");
  assert.match(sent.input[0].content[1].image_url, /^data:image\/png;base64,/);
});

test("the probe reports a refusal as status plus a sanitized error code", async () => {
  const connection = await authStore.getConnection();
  const run = (response) => probe.runVisionProbe(connection, "model-first", async () => response());
  const http = await run(() => new Response(JSON.stringify({ error: { code: "unsupported_modality", message: "SECRET detail" } }), { status: 400 }));
  assert.deepEqual(http, { model: "model-first", httpStatus: 400, accepted: false, eventTypes: [], errorCode: "unsupported_modality" });
  const unsafe = await run(() => new Response(JSON.stringify({ error: { code: "has spaces and SECRET", type: "also bad!" } }), { status: 400 }));
  assert.equal(unsafe.errorCode, "unknown");
  const garbage = await run(() => new Response("<html>SECRET</html>", { status: 502 }));
  assert.equal(garbage.errorCode, "unknown");
  const event = await run(() => sse(['data: {"type":"response.failed","response":{"error":{"code":"invalid_image","message":"SECRET"}}}\n\n']));
  assert.deepEqual(event, { model: "model-first", httpStatus: 200, accepted: false, eventTypes: ["response.failed"], errorCode: "invalid_image" });
  const truncated = await run(() => sse(['data: {"type":"response.created"}\n\n']));
  assert.equal(truncated.accepted, false);
  for (const result of [http, unsafe, garbage, event, truncated]) assert.doesNotMatch(JSON.stringify(result), /SECRET/);
});

test("the probe route needs the local origin and a catalog model, and returns only the summary", async () => {
  const wrong = await probeRoute.POST(localRequest("/api/chat/vision-probe", { ...json({ model: "model-first" }), headers: { "content-type": "application/json", origin: "http://evil.example" } }));
  assert.equal(wrong.status, 403);
  const calls = installFetch(() => goodStream());
  assert.equal((await probeRoute.POST(localRequest("/api/chat/vision-probe", json({ model: "not-in-catalog" })))).status, 400);
  assert.equal((await probeRoute.POST(localRequest("/api/chat/vision-probe", json({ model: "model-first", extra: 1 })))).status, 400);
  assert.equal(responsesCalls(calls).length, 0);
  const ok = await probeRoute.POST(localRequest("/api/chat/vision-probe", json({ model: "model-first" })));
  assert.equal(ok.status, 200);
  const text = await ok.text();
  assert.doesNotMatch(text, /offline-fixture-token|base64/);
  assert.equal(JSON.parse(text).accepted, true);
  assert.equal(responsesCalls(calls).length, 1);
});
