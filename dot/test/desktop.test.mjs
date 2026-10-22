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
    // click:allow-secret synthetic test data; not real personal data.
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
  // click:allow-secret synthetic test data; not real personal data.
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
  // click:allow-secret synthetic test data; not real personal data.
  for (const rejected of ["http://example.com/", "javascript:alert(1)", "https://user:pw@auth.openai.com/", "nonsense"]) {
    assert.equal(navigation.systemBrowserTarget(rejected), undefined, rejected);
  }
});

test("start at login defaults to on and persists the user's choice", async () => {
  const file = join(scratch, "prefs", "desktop-preferences.json");
  assert.deepEqual(await preferences.loadPreferences(file), { startAtLogin: true, captureEnabled: false });
  assert.equal(JSON.parse(await readFile(file, "utf8")).startAtLogin, true);

  assert.deepEqual(await preferences.savePreferences(file, { startAtLogin: false, captureEnabled: false }), { startAtLogin: false, captureEnabled: false });
  assert.deepEqual(await preferences.loadPreferences(file), { startAtLogin: false, captureEnabled: false });
  assert.deepEqual(await preferences.savePreferences(file, { startAtLogin: true, captureEnabled: false }), { startAtLogin: true, captureEnabled: false });
  assert.deepEqual(await preferences.loadPreferences(file), { startAtLogin: true, captureEnabled: false });
});

test("corrupt or unexpected preferences are reported as unknown without crashing", async () => {
  // Unknown means "leave the OS login item as it is": never re-enable autostart the user turned off.
  for (const [index, content] of ["{not json", "[]", "null", '{"startAtLogin":"yes"}', ""].entries()) {
    const file = join(scratch, `bad-${index}.json`);
    await writeFile(file, content);
    assert.equal(await preferences.loadPreferences(file), undefined, content);
    assert.equal(await readFile(file, "utf8"), content, "the file is left untouched");
  }
});

test("an unreadable preferences file is reported as unknown, not as the default", async () => {
  const file = join(scratch, "prefs-is-a-directory");
  await mkdir(file);
  assert.equal(await preferences.loadPreferences(file), undefined);
});

test("unknown preferences are seeded from the real OS login item, never from the default", () => {
  // A later save writes the seed back to disk, so it must match what the OS already does.
  assert.deepEqual(preferences.initialPreferences(undefined, false), { startAtLogin: false, captureEnabled: false });
  assert.deepEqual(preferences.initialPreferences(undefined, true), { startAtLogin: true, captureEnabled: false });
  const saved = { startAtLogin: false, captureEnabled: true };
  assert.deepEqual(preferences.initialPreferences(saved, true), saved);
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

// Release hardening (electron-builder.yml). The file is read as text on purpose: Dot has no YAML dependency.
const builderConfig = await readFile(new URL("../electron-builder.yml", import.meta.url), "utf8");
const packageJson = JSON.parse(await readFile(new URL("../package.json", import.meta.url), "utf8"));
const lockPackages = JSON.parse(await readFile(new URL("../package-lock.json", import.meta.url), "utf8")).packages;

function builderValue(key) {
  return builderConfig.match(new RegExp(`^\\s*${key}:\\s*(.+?)\\s*$`, "m"))?.[1];
}

function excludedPackages() {
  return [...builderConfig.matchAll(/^\s*-\s*"!\*\*\/node_modules\/((?:@[^/"]+\/)?[^/"*]+)\/\*\*"/gm)].map((match) => match[1]);
}

// Same resolution the package manager used: a package's dependency lives in its own node_modules or an ancestor's.
function dependencyClosure(roots) {
  const seen = new Set();
  const pending = roots.map((name) => `node_modules/${name}`);
  while (pending.length > 0) {
    const path = pending.pop();
    if (seen.has(path)) continue;
    seen.add(path);
    const entry = lockPackages[path];
    const names = Object.keys({ ...entry.dependencies, ...entry.optionalDependencies, ...entry.peerDependencies });
    for (const name of names) {
      let base = path;
      for (;;) {
        const candidate = `${base}/node_modules/${name}`.replace(/^\//, "");
        if (lockPackages[candidate]) {
          pending.push(candidate);
          break;
        }
        const cut = base.lastIndexOf("/node_modules/");
        if (cut === -1) {
          if (lockPackages[`node_modules/${name}`]) pending.push(`node_modules/${name}`);
          break;
        }
        base = base.slice(0, cut);
      }
    }
  }
  return new Set([...seen].map((path) => path.slice(path.lastIndexOf("node_modules/") + "node_modules/".length)));
}

test("the packaged exe disables the Node escape hatches and only loads app code from a validated asar", () => {
  assert.equal(builderValue("asar"), "true");
  const fuses = {
    runAsNode: "false",
    enableNodeOptionsEnvironmentVariable: "false",
    enableNodeCliInspectArguments: "false",
    enableCookieEncryption: "true",
    grantFileProtocolExtraPrivileges: "false",
    onlyLoadAppFromAsar: "true",
    enableEmbeddedAsarIntegrityValidation: "true",
  };
  for (const [fuse, value] of Object.entries(fuses)) assert.equal(builderValue(fuse), value, fuse);
});

test("the exe gets its icon and version resources without needing wine", async () => {
  assert.equal(builderValue("icon"), "desktop/assets/icon.png");
  // Resource editing stays on (the default); only code signing, which is not set up yet, is skipped.
  assert.equal(builderValue("signExecutable"), "false");
  assert.equal(builderValue("signAndEditExecutable"), undefined);
  assert.equal(builderValue("productName"), "Click Dot");
  await readFile(new URL("../desktop/assets/icon.png", import.meta.url));
});

test("the package never excludes a dependency the desktop shell or the Next.js server needs", async () => {
  const mainImports = new Set();
  for (const file of ["main", "server", "ocr", "screen-capture", "capture", "redaction", "protector", "preferences"]) {
    const source = await readFile(new URL(`../desktop/${file}.mts`, import.meta.url), "utf8");
    for (const match of source.matchAll(/\bfrom\s+"((?:@[^/"]+\/)?[^"./][^"/]*)(?:\/[^"]*)?"/g)) mainImports.add(match[1]);
    for (const match of source.matchAll(/\bimport\("((?:@[^/"]+\/)?[^"./][^"/]*)(?:\/[^"]*)?"\)/g)) mainImports.add(match[1]);
  }
  mainImports.delete("electron"); // supplied by the Electron runtime, never packaged as a dependency
  const needed = dependencyClosure(["next", "react", "react-dom", ...[...mainImports].filter((name) => !name.startsWith("node:"))]);
  const excluded = excludedPackages();
  assert.ok(excluded.length > 0);
  for (const name of excluded) assert.equal(needed.has(name), false, `${name} is needed at runtime but excluded from the package`);
});

test("every production dependency is either needed at runtime or explicitly excluded from the package", () => {
  const needed = dependencyClosure(["next", "react", "react-dom"]);
  const excluded = new Set(excludedPackages());
  for (const name of Object.keys(packageJson.dependencies)) {
    assert.ok(needed.has(name) || excluded.has(name), `${name}: add it to the runtime check or exclude it in electron-builder.yml`);
  }
});

test("Next.js never writes its response cache next to the read-only app", async () => {
  const { default: nextConfigObject } = await import("../next.config.ts");
  assert.equal(nextConfigObject.experimental?.isrFlushToDisk, false);
});

test("the home page is rendered per request so its canonical origin uses the running port", async () => {
  // A page prerendered at build time froze PORT's default (3000); the desktop app on 47321 then redirected to a
  // blocked origin and never left "Checking this device…".
  const source = await readFile(new URL("../app/page.tsx", import.meta.url), "utf8");
  assert.match(source, /^export const dynamic = "force-dynamic";$/m);
});

// Mascot position (S7/A7). Optional like captureEnabled was: files from earlier versions stay valid and unchanged.
test("preferences from before the mascot have no position and still load exactly as before", async () => {
  const file = join(scratch, "legacy-mascot.json");
  await writeFile(file, JSON.stringify({ startAtLogin: false, captureEnabled: true }));
  assert.deepEqual(await preferences.loadPreferences(file), { startAtLogin: false, captureEnabled: true });
  await writeFile(file, JSON.stringify({ startAtLogin: true }));
  assert.deepEqual(await preferences.loadPreferences(file), { startAtLogin: true, captureEnabled: false });
});

test("the mascot position is saved with the other choices and read back", async () => {
  const file = join(scratch, "mascot-position.json");
  const saved = await preferences.savePreferences(file, { startAtLogin: true, captureEnabled: false, mascotPosition: { x: -1200, y: 340 } });
  assert.deepEqual(saved, { startAtLogin: true, captureEnabled: false, mascotPosition: { x: -1200, y: 340 } });
  assert.deepEqual(await preferences.loadPreferences(file), saved);
  // A later save of another setting keeps the position.
  const store = preferences.createPreferenceStore(file, saved);
  assert.deepEqual(await store.update({ captureEnabled: true }), { startAtLogin: true, captureEnabled: true, mascotPosition: { x: -1200, y: 340 } });
  assert.deepEqual(await store.update({ mascotPosition: { x: 10, y: 20 } }), { startAtLogin: true, captureEnabled: true, mascotPosition: { x: 10, y: 20 } });
});

test("saving a preference without a position never writes one", async () => {
  const file = join(scratch, "no-mascot-position.json");
  const saved = await preferences.savePreferences(file, { startAtLogin: true, captureEnabled: false });
  assert.deepEqual(saved, { startAtLogin: true, captureEnabled: false });
  assert.equal("mascotPosition" in JSON.parse(await readFile(file, "utf8")), false);
});

test("an invalid position is not saved", async () => {
  const file = join(scratch, "bad-mascot-save.json");
  for (const bad of [{ x: Number.NaN, y: 0 }, { x: 1.5, y: 0 }, { x: 0, y: Infinity }, { x: "1", y: 2 }, { x: 1 }, null, [], 5, { x: 1e9, y: 0 }]) {
    const saved = await preferences.savePreferences(file, { startAtLogin: true, captureEnabled: false, mascotPosition: bad });
    assert.equal("mascotPosition" in saved, false, JSON.stringify(bad));
  }
});

test("a saved position of the wrong shape makes the file unknown, like any other corrupt choice", async () => {
  for (const [index, position] of [{ x: "1", y: 2 }, { x: 1 }, [1, 2], "corner", { x: 1.5, y: 2 }, { x: 1e9, y: 0 }, null].entries()) {
    const file = join(scratch, `bad-mascot-${index}.json`);
    const content = JSON.stringify({ startAtLogin: true, captureEnabled: false, mascotPosition: position });
    await writeFile(file, content);
    assert.equal(await preferences.loadPreferences(file), undefined, content);
    assert.equal(await readFile(file, "utf8"), content, "the file is left untouched");
  }
});
