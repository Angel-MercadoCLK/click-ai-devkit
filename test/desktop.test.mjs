import assert from "node:assert/strict";
import { createServer } from "node:net";
import { mkdir, mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import { createServer as createHttpServer } from "node:http";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { after, test } from "node:test";

const config = await import("../desktop/config.mts");
const navigation = await import("../desktop/navigation.mts");
const preferences = await import("../desktop/preferences.mts");
const protector = await import("../desktop/protector.mts");
const listen = await import("../desktop/listen.mts");
const nextConfig = await import("../desktop/next-config.mts");

const scratch = await mkdtemp(join(tmpdir(), "click-dot-desktop-test-"));
after(() => rm(scratch, { recursive: true, force: true }));

test("the app uses one fixed loopback port so the OAuth callback stays stable", async () => {
  assert.equal(config.DOT_HOST, "127.0.0.1");
  assert.equal(config.DOT_PORT, 47321);
  assert.equal(config.dotOrigin(), "http://127.0.0.1:47321");
  assert.equal(config.dotUrl(), "http://127.0.0.1:47321/");
  const { callbackUri } = await import("../lib/siwc.ts");
  assert.equal(callbackUri(String(config.DOT_PORT)), `${config.dotOrigin()}/auth/callback`);
});

test("the environment points storage at the per-user data directory and fixes the port", () => {
  const env = { NEXT_TELEMETRY_DISABLED: "0", PORT: "3000", CLICK_DOT_AUTH_FILE: "/elsewhere/auth.json" };
  config.configureEnvironment(env, join(scratch, "userData"));
  assert.equal(env.PORT, "47321");
  assert.equal(env.NEXT_TELEMETRY_DISABLED, "1");
  assert.equal(env.CLICK_DOT_AUTH_FILE, join(scratch, "userData", "auth.json"));
  assert.equal(env.CLICK_DOT_CHAT_FILE, join(scratch, "userData", "conversations.json"));
});

test("navigation stays on the Click Dot loopback origin", () => {
  for (const allowed of ["http://127.0.0.1:47321/", "http://127.0.0.1:47321/auth/callback?code=x", "http://127.0.0.1:47321"]) {
    assert.equal(navigation.isAllowedNavigation(allowed), true, allowed);
  }
  for (const blocked of [
    "https://auth.openai.com/api/accounts/authorize",
    "http://localhost:47321/",
    "http://127.0.0.1:3000/",
    "http://127.0.0.1:47321@evil.example/",
    "https://127.0.0.1:47321/",
    "http://127.0.0.1.evil.example:47321/",
    "file:///C:/Windows/win.ini",
    "javascript:alert(1)",
    "not a url",
    "",
  ]) {
    assert.equal(navigation.isAllowedNavigation(blocked), false, blocked);
  }
});

test("only https links leave the app, and they open in the system browser", () => {
  assert.equal(navigation.externalUrlToOpen("https://auth.openai.com/api/accounts/authorize?x=1"), "https://auth.openai.com/api/accounts/authorize?x=1");
  for (const rejected of ["http://example.com/", "file:///C:/x", "javascript:alert(1)", "ms-msdt:/id", "data:text/html,hi", "https://user:pw@example.com/", "nonsense", ""]) {
    assert.equal(navigation.externalUrlToOpen(rejected), undefined, rejected);
  }
  assert.equal(navigation.externalUrlToOpen("http://127.0.0.1:47321/"), undefined);
});

test("only the ChatGPT sign-in origin counts as a sign-in hand-off", () => {
  assert.equal(navigation.isSignInUrl("https://auth.openai.com/api/accounts/authorize?x=1"), true);
  for (const other of ["https://auth.openai.com.evil.example/", "http://auth.openai.com/", "https://example.com/", "nonsense"]) {
    assert.equal(navigation.isSignInUrl(other), false, other);
  }
});

test("sign-in hands the whole flow to the system browser so its callback finds the pending cookie", () => {
  // The callback lands in the system browser; starting there keeps the attempt cookie in that browser.
  assert.equal(navigation.systemBrowserTarget("https://auth.openai.com/api/accounts/authorize?x=1"), "http://127.0.0.1:47321/");
  assert.equal(navigation.systemBrowserTarget("https://example.com/docs"), "https://example.com/docs");
  for (const rejected of ["http://example.com/", "javascript:alert(1)", "https://user:pw@auth.openai.com/", "nonsense"]) {
    assert.equal(navigation.systemBrowserTarget(rejected), undefined, rejected);
  }
});

test("start at login defaults to on and persists the user's choice", async () => {
  const file = join(scratch, "prefs", "desktop-preferences.json");
  assert.deepEqual(await preferences.loadPreferences(file), { startAtLogin: true });
  assert.equal(JSON.parse(await readFile(file, "utf8")).startAtLogin, true);

  assert.deepEqual(await preferences.savePreferences(file, { startAtLogin: false }), { startAtLogin: false });
  assert.deepEqual(await preferences.loadPreferences(file), { startAtLogin: false });
  assert.deepEqual(await preferences.savePreferences(file, { startAtLogin: true }), { startAtLogin: true });
  assert.deepEqual(await preferences.loadPreferences(file), { startAtLogin: true });
});

test("corrupt or unexpected preferences fall back to the safe default without crashing", async () => {
  for (const [index, content] of ["{not json", "[]", "null", '{"startAtLogin":"yes"}', ""].entries()) {
    const file = join(scratch, `bad-${index}.json`);
    await writeFile(file, content);
    assert.deepEqual(await preferences.loadPreferences(file), { startAtLogin: true }, content);
  }
});

test("the login item is only managed for packaged builds and can be disabled", () => {
  assert.equal(preferences.shouldManageLoginItem({ isPackaged: true, argv: ["app.exe"], env: {} }), true);
  assert.equal(preferences.shouldManageLoginItem({ isPackaged: false, argv: [], env: {} }), false);
  assert.equal(preferences.shouldManageLoginItem({ isPackaged: true, argv: ["app.exe", "--no-login-item"], env: {} }), false);
  assert.equal(preferences.shouldManageLoginItem({ isPackaged: true, argv: [], env: { CLICK_DOT_DISABLE_LOGIN_ITEM: "1" } }), false);
  assert.equal(preferences.shouldManageLoginItem({ isPackaged: true, argv: [], env: { CLICK_DOT_DISABLE_LOGIN_ITEM: "0" } }), true);
});

test("login launches start hidden in the tray while manual launches show the window", () => {
  assert.equal(preferences.startsHidden(["app.exe", "--hidden"]), true);
  assert.equal(preferences.startsHidden(["app.exe"]), false);
  assert.deepEqual(preferences.LOGIN_ITEM_ARGS, ["--hidden"]);
});

test("the storage protector wraps Electron safeStorage and rejects bad input", () => {
  const calls = [];
  const fake = {
    isEncryptionAvailable: () => true,
    encryptString: (text) => { calls.push(["encrypt", text]); return Buffer.from(`enc:${text}`); },
    decryptString: (data) => { calls.push(["decrypt", data.toString()]); return data.toString().slice(4); },
  };
  const target = {};
  assert.equal(protector.installStorageProtector(target, fake), true);
  const installed = target[Symbol.for("click-dot.storage-protector")];
  assert.equal(protector.STORAGE_PROTECTOR_KEY, "click-dot.storage-protector");
  const encrypted = installed.encrypt("secret");
  assert.ok(Buffer.isBuffer(encrypted));
  assert.equal(installed.decrypt(encrypted), "secret");
  assert.deepEqual(calls, [["encrypt", "secret"], ["decrypt", "enc:secret"]]);
  assert.throws(() => installed.encrypt(42));
  assert.throws(() => installed.decrypt("not a buffer"));
});

test("without OS encryption the protector is not installed so storage fails closed", () => {
  const target = {};
  const unavailable = { isEncryptionAvailable: () => false, encryptString: () => { throw new Error("no"); }, decryptString: () => { throw new Error("no"); } };
  assert.equal(protector.installStorageProtector(target, unavailable), false);
  assert.equal(Symbol.for("click-dot.storage-protector") in target, false);
  const throwing = { isEncryptionAvailable: () => { throw new Error("boom"); }, encryptString: () => Buffer.alloc(0), decryptString: () => "" };
  assert.equal(protector.installStorageProtector(target, throwing), false);
  assert.equal(Symbol.for("click-dot.storage-protector") in target, false);
});

test("a busy port is reported as such instead of picking another port", async () => {
  const blocker = createServer();
  await new Promise((resolve) => blocker.listen(0, "127.0.0.1", resolve));
  const { port } = blocker.address();
  try {
    const server = createHttpServer();
    await assert.rejects(listen.listenOnLoopback(server, port), (error) => error instanceof listen.PortInUseError && error.port === port);
  } finally {
    await new Promise((resolve) => blocker.close(resolve));
  }
});

test("the server binds to 127.0.0.1 only", async () => {
  const server = createHttpServer((_, response) => response.end("ok"));
  await listen.listenOnLoopback(server, 0);
  try {
    assert.equal(server.address().address, "127.0.0.1");
  } finally {
    await new Promise((resolve) => server.close(resolve));
  }
});

test("the production server config comes from the build output, never from next.config.ts at runtime", async () => {
  const dir = join(scratch, "built-app");
  await mkdir(join(dir, ".next"), { recursive: true });
  const config = { distDir: ".next", output: undefined, images: { unoptimized: true } };
  await writeFile(join(dir, ".next", "required-server-files.json"), JSON.stringify({ version: 1, config }));
  assert.deepEqual(await nextConfig.loadBuiltNextConfig(dir), { distDir: ".next", images: { unoptimized: true } });
});

test("a missing or malformed build output gives a clear error", async () => {
  await assert.rejects(nextConfig.loadBuiltNextConfig(join(scratch, "nothing-here")), /npm run desktop:build/);
  const dir = join(scratch, "broken-app");
  await mkdir(join(dir, ".next"), { recursive: true });
  for (const content of ["{nope", "{}", '{"config":[]}', '{"config":null}']) {
    await writeFile(join(dir, ".next", "required-server-files.json"), content);
    await assert.rejects(nextConfig.loadBuiltNextConfig(dir), /npm run desktop:build/, content);
  }
});
