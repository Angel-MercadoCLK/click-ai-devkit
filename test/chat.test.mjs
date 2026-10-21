import assert from "node:assert/strict";
import { after, before, test } from "node:test";
import { mkdtemp, rm, writeFile, chmod } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";

const directory = await mkdtemp(join(tmpdir(), "click-dot-chat-test-"));
await chmod(directory, 0o700);
process.env.CLICK_DOT_AUTH_FILE = join(directory, "auth.json");
process.env.CLICK_DOT_CHAT_FILE = join(directory, "conversations.json");

const authStore = await import("../lib/auth-store.ts");
const { listAvailableModels, streamChatResponse } = await import("../lib/chat-provider.ts");
const historyStore = await import("../lib/conversation-store.ts");
const modelsRoute = await import("../app/api/chat/models/route.ts");
const historyRoute = await import("../app/api/chat/history/route.ts");
const streamRoute = await import("../app/api/chat/stream/route.ts");
const disconnectRoute = await import("../app/api/auth/disconnect/route.ts");
const originalFetch = globalThis.fetch;

const accessToken = "offline-fixture-token-never-a-real-credential";
let subject = "account-one";

async function writeAuth({ expiresAt = Date.now() + 60_000, scopes = ["chatgpt.tokens.use.direct"], user = subject } = {}) {
  await writeFile(process.env.CLICK_DOT_AUTH_FILE, JSON.stringify({
    hostId: "urn:uuid:offline-fixture",
    generation: 4,
    attempts: {},
    issuedClientId: "client-offline-fixture",
    connection: {
      clientId: "client-offline-fixture",
      issuer: "https://auth.openai.com",
      subject: user,
      accessToken,
      refreshToken: "fixture-refresh-token",
      idToken: "fixture-id-token",
      scopes,
      expiresAt,
    },
  }), { mode: 0o600 });
  await chmod(process.env.CLICK_DOT_AUTH_FILE, 0o600);
}

function localRequest(url, init = {}) {
  const method = init.method ?? "GET";
  return new Request(`http://127.0.0.1:3000${url}`, {
    ...init,
    method,
    headers: {
      host: "127.0.0.1:3000",
      ...(method !== "GET" ? { origin: "http://127.0.0.1:3000" } : {}),
      ...(init.headers ?? {}),
    },
  });
}

function fakeModels() {
  return new Response(JSON.stringify({ models: [
    { slug: "model-first", display_name: "First model", visibility: "list" },
    { slug: "hidden-model", display_name: "Hidden", visibility: "private" },
    { slug: "model-second", display_name: "Second model", visibility: "list" },
    { slug: "bad/slug", display_name: "Invalid", visibility: "list" },
  ] }), { headers: { "content-type": "application/json" } });
}

function catalogResponse(body, chunkBytes = 16_384, onCancel = () => {}) {
  const bytes = new TextEncoder().encode(body);
  let offset = 0;
  return new Response(new ReadableStream({
    pull(controller) {
      if (offset >= bytes.byteLength) { controller.close(); return; }
      const end = Math.min(offset + chunkBytes, bytes.byteLength);
      controller.enqueue(bytes.subarray(offset, end));
      offset = end;
    },
    cancel: onCancel,
  }, { highWaterMark: 0 }), { headers: { "content-type": "application/json" } });
}

function responseStream(chunks, delay = 0) {
  let index = 0;
  return new Response(new ReadableStream({
    async pull(controller) {
      if (index >= chunks.length) { controller.close(); return; }
      if (delay) await new Promise((resolve) => setTimeout(resolve, delay));
      controller.enqueue(new TextEncoder().encode(chunks[index++]));
    },
  }), { headers: { "content-type": "text/event-stream" } });
}

const deltaFrame = 'data: {"type":"response.output_text.delta",\ndata: "delta":"Hello"}\r\n\r\n';
const completeFrame = 'data: {"type":"response.completed"}\n\n';

before(async () => writeAuth());
after(async () => {
  globalThis.fetch = originalFetch;
  await rm(directory, { recursive: true, force: true });
});

test("model catalog keeps only listed models and sends the OAuth bearer only upstream", async () => {
  const calls = [];
  const models = await listAvailableModels((await authStore.getConnection()), async (url, init) => {
    calls.push({ url: String(url), init });
    return fakeModels();
  });
  assert.deepEqual(models, [
    { slug: "model-first", displayName: "First model" },
    { slug: "model-second", displayName: "Second model" },
  ]);
  assert.equal(calls[0].url, "https://api.openai.com/v1/models");
  assert.equal(calls[0].init.headers.authorization, `Bearer ${accessToken}`);
});

test("catalog accepts large chunked JSON and still returns only listed models in order", async () => {
  const catalog = { models: [
    { slug: "large-first", display_name: "First", visibility: "list" },
    { slug: "hidden-large", display_name: "Hidden", visibility: "private" },
    { slug: "large-second", display_name: "Second", visibility: "list" },
  ], metadata: "" };
  const padding = 626_192 - Buffer.byteLength(JSON.stringify(catalog), "utf8");
  catalog.metadata = "x".repeat(padding);
  const payload = JSON.stringify(catalog);
  assert.equal(Buffer.byteLength(payload, "utf8"), 626_192);
  assert.ok(Buffer.byteLength(payload, "utf8") > 512_000);
  assert.ok(Buffer.byteLength(payload, "utf8") < 2 * 1024 * 1024);
  const models = await listAvailableModels({ accessToken: "offline-synthetic-token" }, async () => catalogResponse(payload));
  assert.deepEqual(models, [
    { slug: "large-first", displayName: "First" },
    { slug: "large-second", displayName: "Second" },
  ]);
});

test("catalog applies the 2 MiB bound to UTF-8 bytes and cancels oversized bodies", async () => {
  const payload = JSON.stringify({ models: [], metadata: "é".repeat(1_048_600) });
  assert.ok(payload.length < 2 * 1024 * 1024);
  assert.ok(Buffer.byteLength(payload, "utf8") > 2 * 1024 * 1024);
  let canceled = false;
  await assert.rejects(
    () => listAvailableModels({ accessToken: "offline-synthetic-token" }, async () => catalogResponse(payload, 65_536, () => { canceled = true; })),
    /invalid or too large/,
  );
  assert.equal(canceled, true);
});

test("models route accepts a synthetic account catalog larger than the previous limit", async () => {
  await writeAuth();
  const payload = JSON.stringify({ models: [
    { slug: "route-first", display_name: "First", visibility: "list" },
    { slug: "route-private", display_name: "Private", visibility: "private" },
  ], metadata: "é".repeat(260_000) });
  globalThis.fetch = async (url, init) => {
    assert.equal(String(url), "https://api.openai.com/v1/models");
    assert.equal(init.headers.authorization, `Bearer ${accessToken}`);
    return catalogResponse(payload, 32_768);
  };
  const response = await modelsRoute.GET(localRequest("/api/chat/models"));
  assert.equal(response.status, 200);
  assert.deepEqual(await response.json(), { models: [{ slug: "route-first", displayName: "First" }] });
});

test("Responses request is server-only, has no tools, and requires completed SSE across split multiline chunks", async () => {
  const connection = await authStore.getConnection();
  let requestBody;
  let requestHeaders;
  const events = [];
  for await (const event of streamChatResponse(connection, "model-first", [{ role: "user", content: "Hi" }], async (url, init) => {
    assert.equal(String(url), "https://api.openai.com/v1/responses");
    requestBody = JSON.parse(init.body);
    requestHeaders = init.headers;
    return responseStream([deltaFrame.slice(0, 24), deltaFrame.slice(24, 66), deltaFrame.slice(66), completeFrame]);
  })) events.push(event);
  assert.equal(requestHeaders.authorization, `Bearer ${accessToken}`);
  assert.deepEqual(requestBody, { model: "model-first", input: [{ role: "user", content: "Hi" }], store: false, stream: true });
  assert.deepEqual(events, [{ type: "delta", text: "Hello" }, { type: "completed", text: "Hello" }]);
});

test("incomplete, failed, malformed, and truncated SSE never yield completion", async (t) => {
  const cases = [
    ["incomplete", 'data: {"type":"response.incomplete"}\n\n'],
    ["failed", 'data: {"type":"response.failed"}\n\n'],
    ["malformed", "data: nope\n\n"],
    ["eof", deltaFrame],
  ];
  for (const [name, frame] of cases) await t.test(name, async () => {
    const events = [];
    await assert.rejects(async () => {
      for await (const event of streamChatResponse(await authStore.getConnection(), "model-first", [{ role: "user", content: "Hi" }], async () => responseStream([frame]))) events.push(event);
    });
    assert.equal(events.some((event) => event.type === "completed"), false);
  });
});

test("input, output, and event limits reject provider requests without claiming completion", async (t) => {
  let fetchCalls = 0;
  const connection = await authStore.getConnection();
  await assert.rejects(async () => {
    for await (const _event of streamChatResponse(connection, "model-first", [{ role: "user", content: "x".repeat(16_001) }], async () => {
      fetchCalls += 1;
      return fakeModels();
    })) { /* validation must fail before a provider request */ }
  }, /too large or invalid/);
  assert.equal(fetchCalls, 0);

  await t.test("output bound", async () => {
    const frame = `data: ${JSON.stringify({ type: "response.output_text.delta", delta: "x".repeat(16_001) })}\n\n`;
    await assert.rejects(async () => {
      for await (const _event of streamChatResponse(connection, "model-first", [{ role: "user", content: "Hi" }], async () => responseStream([frame]))) { /* collect only for rejection */ }
    }, /safe size limit/);
  });

  await t.test("event count bound", async () => {
    const event = `data: ${JSON.stringify({ type: "response.unknown" })}\n\n`;
    const frames = Array.from({ length: 1_201 }, () => event);
    await assert.rejects(async () => {
      for await (const _event of streamChatResponse(connection, "model-first", [{ role: "user", content: "Hi" }], async () => responseStream(frames))) { /* collect only for rejection */ }
    }, /event limit/);
  });
});

test("chat route rejects non-local origins and expired authorization before provider requests", async () => {
  let calls = 0;
  globalThis.fetch = async () => { calls += 1; return fakeModels(); };
  const crossOrigin = await modelsRoute.GET(new Request("http://127.0.0.1:3000/api/chat/models", { headers: { host: "127.0.0.1:3000", origin: "http://evil.example" } }));
  assert.equal(crossOrigin.status, 403);
  await writeAuth({ expiresAt: Date.now() - 1 });
  const expired = await modelsRoute.GET(localRequest("/api/chat/models"));
  assert.equal(expired.status, 401);
  assert.equal(calls, 0);
  await writeAuth();
});

test("stream endpoint validates user/model input, keeps credentials out of the response, and stores only completed turns", async () => {
  const calls = [];
  globalThis.fetch = async (url, init) => {
    calls.push({ url: String(url), init });
    return String(url).endsWith("/models") ? fakeModels() : responseStream([deltaFrame, completeFrame]);
  };
  const rejectedModel = await streamRoute.POST(localRequest("/api/chat/stream", { method: "POST", headers: { "content-type": "application/json" }, body: JSON.stringify({ conversationId: crypto.randomUUID(), revision: 0, model: "not-in-catalog", prompt: "Hello" }) }));
  assert.equal(rejectedModel.status, 400);
  assert.equal(calls.length, 1);

  const id = crypto.randomUUID();
  const response = await streamRoute.POST(localRequest("/api/chat/stream", { method: "POST", headers: { "content-type": "application/json" }, body: JSON.stringify({ conversationId: id, revision: 0, model: "model-first", prompt: "Hello" }) }));
  assert.equal(response.status, 200);
  const text = await response.text();
  assert.match(text, /event: delta/);
  assert.match(text, /event: completed/);
  assert.doesNotMatch(text, /offline-fixture-token|refresh-token|id-token/);
  assert.equal(calls[1].url, "https://api.openai.com/v1/models");
  assert.equal(calls[2].url, "https://api.openai.com/v1/responses");
  const historyResponse = await historyRoute.GET(localRequest("/api/chat/history"));
  const history = await historyResponse.json();
  assert.equal(history.conversations.length, 1);
  assert.deepEqual(history.conversations[0].messages.map(({ role, content }) => [role, content]), [["user", "Hello"], ["assistant", "Hello"]]);
});

test("provider failure after deltas reports error and never persists partial output", async () => {
  await historyStore.clearConversations(await authStore.getConnectionSnapshot());
  globalThis.fetch = async (url) => String(url).endsWith("/models") ? fakeModels() : responseStream([deltaFrame, 'data: {"type":"response.failed"}\n\n']);
  const beforeCount = (await historyStore.listConversations(await authStore.getConnectionSnapshot())).conversations.length;
  const id = crypto.randomUUID();
  const revision = (await historyStore.listConversations(await authStore.getConnectionSnapshot())).revision;
  const response = await streamRoute.POST(localRequest("/api/chat/stream", { method: "POST", headers: { "content-type": "application/json" }, body: JSON.stringify({ conversationId: id, revision, model: "model-first", prompt: "Do not save" }) }));
  const text = await response.text();
  assert.match(text, /event: delta/);
  assert.match(text, /event: error/);
  assert.doesNotMatch(text, /event: completed/);
  const current = await (await historyRoute.GET(localRequest("/api/chat/history"))).json();
  assert.equal(current.conversations.length, beforeCount);
});

test("history is account-scoped and clear invalidates an already-started response", async () => {
  const first = await authStore.getConnectionSnapshot();
  const beforeClear = await historyStore.listConversations(first);
  await historyStore.clearConversations(first);
  await assert.rejects(historyStore.appendCompletedTurn(first, beforeClear.revision, crypto.randomUUID(), "model-first", "old prompt", "old answer"), /connection changed|history changed/);
  assert.equal((await historyStore.listConversations(first)).conversations.length, 0);

  const otherUser = "account-two";
  await writeAuth({ user: otherUser });
  const second = await authStore.getConnectionSnapshot();
  assert.equal((await historyStore.listConversations(second)).conversations.length, 0);
  await historyStore.appendCompletedTurn(second, 0, crypto.randomUUID(), "model-first", "private prompt", "private answer");
  assert.equal((await historyStore.listConversations(first)).conversations.length, 0);
  assert.equal((await historyStore.listConversations(second)).conversations.length, 1);
  subject = "account-one";
  await writeAuth();
});

test("disconnect during a provider stream prevents the completed turn from being saved", async () => {
  await historyStore.clearConversations(await authStore.getConnectionSnapshot());
  const revision = (await historyStore.listConversations(await authStore.getConnectionSnapshot())).revision;
  let releaseFinal;
  const finalChunk = new Promise((resolve) => { releaseFinal = resolve; });
  globalThis.fetch = async (url) => {
    if (String(url).endsWith("/models")) return fakeModels();
    return new Response(new ReadableStream({
      async start(controller) {
        controller.enqueue(new TextEncoder().encode(deltaFrame));
        await finalChunk;
        controller.enqueue(new TextEncoder().encode(completeFrame));
        controller.close();
      },
    }), { headers: { "content-type": "text/event-stream" } });
  };
  const response = await streamRoute.POST(localRequest("/api/chat/stream", { method: "POST", headers: { "content-type": "application/json" }, body: JSON.stringify({ conversationId: crypto.randomUUID(), revision, model: "model-first", prompt: "In-flight" }) }));
  const reader = response.body.getReader();
  const firstChunk = await reader.read();
  assert.match(new TextDecoder().decode(firstChunk.value), /event: delta/);
  await disconnectRoute.POST(localRequest("/api/auth/disconnect", { method: "POST" }));
  releaseFinal();
  let remainder = "";
  while (true) {
    const item = await reader.read();
    if (item.done) break;
    remainder += new TextDecoder().decode(item.value);
  }
  assert.match(remainder, /event: error/);
  await writeAuth();
  assert.equal((await historyStore.listConversations(await authStore.getConnectionSnapshot())).conversations.length, 0);
});

test("canceling the local response aborts upstream and cannot save a completed turn", async () => {
  const snapshot = await authStore.getConnectionSnapshot();
  await historyStore.clearConversations(snapshot);
  const revision = (await historyStore.listConversations(snapshot)).revision;
  let releaseFinal;
  const finalChunk = new Promise((resolve) => { releaseFinal = resolve; });
  let upstreamSignal;
  globalThis.fetch = async (url, init) => {
    if (String(url).endsWith("/models")) return fakeModels();
    upstreamSignal = init.signal;
    return new Response(new ReadableStream({
      async start(controller) {
        controller.enqueue(new TextEncoder().encode(deltaFrame));
        await finalChunk;
        controller.enqueue(new TextEncoder().encode(completeFrame));
        controller.close();
      },
    }), { headers: { "content-type": "text/event-stream" } });
  };
  const response = await streamRoute.POST(localRequest("/api/chat/stream", { method: "POST", headers: { "content-type": "application/json" }, body: JSON.stringify({ conversationId: crypto.randomUUID(), revision, model: "model-first", prompt: "Cancel me" }) }));
  const reader = response.body.getReader();
  const firstChunk = await reader.read();
  assert.match(new TextDecoder().decode(firstChunk.value), /event: delta/);
  const aborted = new Promise((resolve) => upstreamSignal.addEventListener("abort", resolve, { once: true }));
  await reader.cancel("client disconnected");
  await Promise.race([aborted, new Promise((_, reject) => setTimeout(() => reject(new Error("upstream request was not aborted")), 250))]);
  releaseFinal();
  await new Promise((resolve) => setTimeout(resolve, 0));
  assert.equal((await historyStore.listConversations(snapshot)).conversations.length, 0);
});

test("request abort aborts upstream and cannot save a completed turn", async () => {
  const snapshot = await authStore.getConnectionSnapshot();
  await historyStore.clearConversations(snapshot);
  const revision = (await historyStore.listConversations(snapshot)).revision;
  let releaseFinal;
  const finalChunk = new Promise((resolve) => { releaseFinal = resolve; });
  let upstreamSignal;
  globalThis.fetch = async (url, init) => {
    if (String(url).endsWith("/models")) return fakeModels();
    upstreamSignal = init.signal;
    return new Response(new ReadableStream({
      async start(controller) {
        controller.enqueue(new TextEncoder().encode(deltaFrame));
        await finalChunk;
        controller.enqueue(new TextEncoder().encode(completeFrame));
        controller.close();
      },
    }), { headers: { "content-type": "text/event-stream" } });
  };
  const abortRequest = new AbortController();
  const response = await streamRoute.POST(localRequest("/api/chat/stream", { method: "POST", signal: abortRequest.signal, headers: { "content-type": "application/json" }, body: JSON.stringify({ conversationId: crypto.randomUUID(), revision, model: "model-first", prompt: "Abort request" }) }));
  const reader = response.body.getReader();
  const firstChunk = await reader.read();
  assert.match(new TextDecoder().decode(firstChunk.value), /event: delta/);
  const aborted = new Promise((resolve) => upstreamSignal.addEventListener("abort", resolve, { once: true }));
  abortRequest.abort("request disconnected");
  await Promise.race([aborted, new Promise((_, reject) => setTimeout(() => reject(new Error("upstream request was not aborted")), 250))]);
  releaseFinal();
  await new Promise((resolve) => setTimeout(resolve, 0));
  assert.equal((await historyStore.listConversations(snapshot)).conversations.length, 0);
});

test("history reads and writes stay private and reject invalid model request surfaces", async () => {
  await writeAuth({ scopes: [] });
  const noScope = await historyRoute.GET(localRequest("/api/chat/history"));
  assert.equal(noScope.status, 401);
  await writeAuth();
  const wrongOrigin = await historyRoute.GET(new Request("http://127.0.0.1:3000/api/chat/history", { headers: { host: "127.0.0.1:3000", origin: "http://other.example" } }));
  assert.equal(wrongOrigin.status, 403);
  const extraField = await streamRoute.POST(localRequest("/api/chat/stream", { method: "POST", headers: { "content-type": "application/json" }, body: JSON.stringify({ conversationId: crypto.randomUUID(), revision: 0, model: "model-first", prompt: "Hello", tools: [{ type: "shell" }] }) }));
  assert.equal(extraField.status, 400);
  assert.deepEqual((await (await historyRoute.GET(localRequest("/api/chat/history"))).json()).conversations, []);
});

test("the stream route accepts exactly one new field, screenPreviewId, and still rejects every other extra field", async () => {
  globalThis.fetch = async (url) => String(url).endsWith("/models") ? fakeModels() : responseStream([deltaFrame, completeFrame]);
  const snapshot = await authStore.getConnectionSnapshot();
  await historyStore.clearConversations(snapshot);
  const revision = (await historyStore.listConversations(snapshot)).revision;
  for (const extra of [{ tools: [] }, { instructions: "x" }, { role: "system" }, { image: "data:image/png;base64,AAAA" }, { store: true }]) {
    const response = await streamRoute.POST(localRequest("/api/chat/stream", { method: "POST", headers: { "content-type": "application/json" }, body: JSON.stringify({ conversationId: crypto.randomUUID(), revision, model: "model-first", prompt: "Hello", ...extra }) }));
    assert.equal(response.status, 400, JSON.stringify(extra));
  }
  const withUnknownPreview = await streamRoute.POST(localRequest("/api/chat/stream", { method: "POST", headers: { "content-type": "application/json" }, body: JSON.stringify({ conversationId: crypto.randomUUID(), revision, model: "model-first", prompt: "Hello", screenPreviewId: "A".repeat(22) }) }));
  // The field is accepted (not 400); without desktop capture running, the screen is refused before the id is used.
  assert.equal(withUnknownPreview.status, 409);
  const textOnly = await streamRoute.POST(localRequest("/api/chat/stream", { method: "POST", headers: { "content-type": "application/json" }, body: JSON.stringify({ conversationId: crypto.randomUUID(), revision, model: "model-first", prompt: "Hello" }) }));
  assert.equal(textOnly.status, 200);
  assert.match(await textOnly.text(), /event: completed/);
});
