import assert from "node:assert/strict";
import { createCipheriv, createDecipheriv, randomBytes } from "node:crypto";
import { chmod, link, lstat, mkdir, mkdtemp, readdir, readFile, rm, stat, symlink, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test, { after, afterEach } from "node:test";

const root = await mkdtemp(join(tmpdir(), "click-dot-storage-windows-test-"));
process.env.CLICK_DOT_AUTH_LOCK_TIMEOUT_MS = "200";
const auth = await import("../lib/auth-store.ts");
const history = await import("../lib/conversation-store.ts");
const protectorModule = await import("../lib/storage-protector.ts");
const { STORAGE_PROTECTOR_KEY, getStorageProtector, setStoragePlatformForTests } = protectorModule;

const token = "windows-fixture-access-token-never-a-real-credential";
const refreshToken = "windows-fixture-refresh-token";
const message = "A distinctive prompt that must never be readable on disk";
const reply = "A distinctive answer that must never be readable on disk";

// AES-256-GCM stand-in for Electron safeStorage; the key lives only in this process, like DPAPI's.
function fakeProtector() {
  const key = randomBytes(32);
  const calls = { encrypt: 0, decrypt: 0 };
  return {
    calls,
    encrypt(text) {
      calls.encrypt += 1;
      const iv = randomBytes(12);
      const cipher = createCipheriv("aes-256-gcm", key, iv);
      const body = Buffer.concat([cipher.update(text, "utf8"), cipher.final()]);
      return Buffer.concat([iv, cipher.getAuthTag(), body]);
    },
    decrypt(data) {
      calls.decrypt += 1;
      const decipher = createDecipheriv("aes-256-gcm", key, data.subarray(0, 12));
      decipher.setAuthTag(data.subarray(12, 28));
      return Buffer.concat([decipher.update(data.subarray(28)), decipher.final()]).toString("utf8");
    },
  };
}

async function freshStorage({ directoryMode = 0o700 } = {}) {
  const directory = await mkdtemp(join(root, "case-"));
  await chmod(directory, directoryMode);
  process.env.CLICK_DOT_AUTH_FILE = join(directory, "auth.json");
  process.env.CLICK_DOT_CHAT_FILE = join(directory, "conversations.json");
  return directory;
}

function useWindows(protector) {
  setStoragePlatformForTests("win32");
  if (protector) globalThis[STORAGE_PROTECTOR_KEY] = protector;
  else delete globalThis[STORAGE_PROTECTOR_KEY];
}

async function signIn(subject = "windows-account") {
  const { attempt } = await auth.createAttempt("attempt-1", {
    state: "state", nonce: "nonce", verifier: "verifier", expiresAt: Date.now() + 60_000,
    redirectUri: "http://127.0.0.1:3003/auth/callback",
  }, "dynamic_agent_client");
  await auth.saveIssuedClientId("client-windows", 0);
  const connection = {
    clientId: "client-windows", issuer: "https://auth.openai.com", subject, accessToken: token, refreshToken,
    idToken: "windows-fixture-id-token", scopes: ["chatgpt.tokens.use.direct"], expiresAt: Date.now() + 60_000,
  };
  assert.equal(await auth.saveConnection(connection, attempt), true);
  return (await auth.getConnectionSnapshot());
}

afterEach(() => {
  setStoragePlatformForTests(undefined);
  delete globalThis[STORAGE_PROTECTOR_KEY];
});
after(async () => {
  await rm(root, { recursive: true, force: true });
});

test("Windows with a protector round-trips credentials without plaintext on disk", async () => {
  const directory = await freshStorage({ directoryMode: 0o755 });
  const protector = fakeProtector();
  useWindows(protector);
  const snapshot = await signIn();
  assert.equal(snapshot.connection.accessToken, token);
  assert.equal((await auth.getConnection()).refreshToken, refreshToken);
  assert.equal(await auth.getIssuedClientId(), "client-windows");
  const bytes = await readFile(process.env.CLICK_DOT_AUTH_FILE);
  for (const secret of [token, refreshToken, "windows-account", "windows-fixture-id-token", "client-windows"]) {
    assert.equal(bytes.includes(Buffer.from(secret)), false, `plaintext ${secret} found on disk`);
  }
  assert.ok(protector.calls.encrypt > 0 && protector.calls.decrypt > 0);
  assert.deepEqual((await readdir(directory)).sort(), ["auth.json"]);
  await auth.clearConnection();
  assert.equal(await auth.getConnection(), undefined);
});

test("Windows with a protector round-trips conversation history without plaintext on disk", async () => {
  await freshStorage({ directoryMode: 0o755 });
  useWindows(fakeProtector());
  const snapshot = await signIn();
  const id = "11111111-1111-4111-8111-111111111111";
  await history.appendCompletedTurn(snapshot, 0, id, "model-a", message, reply);
  const listed = await history.listConversations(snapshot);
  assert.equal(listed.revision, 1);
  assert.equal(listed.conversations[0].messages[0].content, message);
  assert.equal((await history.getConversation(snapshot, id)).messages[1].content, reply);
  const bytes = await readFile(process.env.CLICK_DOT_CHAT_FILE);
  assert.equal(bytes.includes(Buffer.from(message)), false);
  assert.equal(bytes.includes(Buffer.from(reply)), false);
  assert.equal(bytes.includes(Buffer.from("conversations")), false);
  assert.equal(await history.deleteConversation(snapshot, id), true);
  assert.equal((await history.listConversations(snapshot)).conversations.length, 0);
  assert.equal(await history.clearConversations(snapshot), 3);
});

test("a tampered or plaintext auth file fails closed without leaking its contents", async () => {
  await freshStorage();
  useWindows(fakeProtector());
  await signIn();
  const path = process.env.CLICK_DOT_AUTH_FILE;
  const original = await readFile(path);
  const tampered = Buffer.from(original);
  tampered[tampered.length - 1] ^= 0xff;
  await writeFile(path, tampered);
  await assert.rejects(auth.getConnection(), (error) => {
    assert.match(error.message, /Protected local storage could not be read/);
    assert.equal(error.message.includes(token), false);
    return true;
  });
  await assert.rejects(auth.saveIssuedClientId("other-client", 1), /Protected local storage could not be read/);
  assert.deepEqual(await readFile(path), tampered, "a rejected read must not rewrite the file");
  await writeFile(path, JSON.stringify({ hostId: "urn:uuid:plain", attempts: {}, connection: { accessToken: token } }));
  await assert.rejects(auth.getConnection(), /Protected local storage could not be read/);
  await writeFile(path, original.subarray(0, 8));
  await assert.rejects(auth.getConnection(), /Protected local storage could not be read/);
});

test("a tampered history file or a different protector key fails closed", async () => {
  await freshStorage();
  useWindows(fakeProtector());
  const snapshot = await signIn();
  const id = "22222222-2222-4222-8222-222222222222";
  await history.appendCompletedTurn(snapshot, 0, id, "model-a", message, reply);
  const path = process.env.CLICK_DOT_CHAT_FILE;
  const original = await readFile(path);
  const tampered = Buffer.from(original);
  tampered[tampered.length - 1] ^= 0xff;
  await writeFile(path, tampered);
  await assert.rejects(history.listConversations(snapshot), (error) => {
    assert.match(error.message, /Protected local storage could not be read/);
    assert.equal(error.message.includes(message), false);
    return true;
  });
  await assert.rejects(history.appendCompletedTurn(snapshot, 1, id, "model-a", "next", "answer"), /Protected local storage could not be read/);
  await writeFile(path, original);
  useWindows(fakeProtector());
  await assert.rejects(history.listConversations(snapshot), /Protected local storage could not be read/);
});

test("a protector that throws or returns the wrong type fails closed and writes nothing", async () => {
  const directory = await freshStorage();
  useWindows({ encrypt: () => { throw new Error(`boom ${token}`); }, decrypt: () => "" });
  await assert.rejects(auth.saveIssuedClientId("client-windows", 0), (error) => {
    assert.match(error.message, /Protected local storage could not be written/);
    assert.equal(error.message.includes(token), false);
    return true;
  });
  useWindows({ encrypt: () => "not a buffer", decrypt: () => "" });
  await assert.rejects(auth.saveIssuedClientId("client-windows", 0), /Protected local storage could not be written/);
  assert.deepEqual(await readdir(directory), [], "no auth file, temp file or lock may remain");
});

test("Windows without a protector keeps failing closed and creates nothing", async () => {
  const directory = await freshStorage();
  const protector = fakeProtector();
  useWindows(protector);
  const snapshot = await signIn();
  const authBefore = await readFile(process.env.CLICK_DOT_AUTH_FILE);
  useWindows(undefined);
  await assert.rejects(auth.getConnection(), /not supported on native Windows/);
  await assert.rejects(auth.saveIssuedClientId("client-windows", 1), /not supported on native Windows/);
  await assert.rejects(history.listConversations(snapshot), /not supported on native Windows/);
  await assert.rejects(history.appendCompletedTurn(snapshot, 0, "33333333-3333-4333-8333-333333333333", "m", "p", "a"), /not supported on native Windows/);
  assert.deepEqual(await readFile(process.env.CLICK_DOT_AUTH_FILE), authBefore);
  assert.deepEqual((await readdir(directory)).sort(), ["auth.json"]);
});

test("protector lookup rejects malformed globals", async () => {
  const valid = fakeProtector();
  for (const malformed of [undefined, null, "protector", 42, {}, { encrypt() {} }, { decrypt() {} }, { encrypt: "x", decrypt: "y" }]) {
    if (malformed === undefined) delete globalThis[STORAGE_PROTECTOR_KEY];
    else globalThis[STORAGE_PROTECTOR_KEY] = malformed;
    assert.equal(getStorageProtector(), undefined);
  }
  globalThis[STORAGE_PROTECTOR_KEY] = valid;
  assert.equal(getStorageProtector(), valid);
  // A malformed global behaves like no protector: native Windows keeps failing closed.
  await freshStorage();
  setStoragePlatformForTests("win32");
  globalThis[STORAGE_PROTECTOR_KEY] = { encrypt() {} };
  await assert.rejects(auth.getConnection(), /not supported on native Windows/);
  // Off Windows the injected protector is ignored.
  setStoragePlatformForTests(undefined);
  globalThis[STORAGE_PROTECTOR_KEY] = valid;
  await freshStorage();
  await auth.saveIssuedClientId("client-posix", 0);
  assert.match(await readFile(process.env.CLICK_DOT_AUTH_FILE, "utf8"), /client-posix/);
  assert.equal(valid.calls.encrypt, 0);
});

test("Windows storage still rejects symlinks, hard links, oversized files and unsafe locks", async () => {
  const directory = await freshStorage();
  useWindows(fakeProtector());
  const snapshot = await signIn();
  const path = process.env.CLICK_DOT_AUTH_FILE;
  const real = join(directory, "real-auth.json");
  await writeFile(real, await readFile(path));
  await rm(path);
  await symlink(real, path);
  await assert.rejects(auth.getConnection());
  await assert.rejects(auth.saveIssuedClientId("client-windows", 1), /not private|ELOOP/);
  await rm(path);
  await writeFile(path, await readFile(real));
  assert.equal((await auth.getConnection()).accessToken, token);

  await link(path, join(directory, "second-name.json"));
  await assert.rejects(auth.getConnection(), /not private/);
  await rm(join(directory, "second-name.json"));
  assert.equal((await auth.getConnection()).accessToken, token);

  const linked = join(directory, "linked-dir");
  await symlink(directory, linked);
  process.env.CLICK_DOT_AUTH_FILE = join(linked, "auth.json");
  await assert.rejects(auth.getConnection(), /unsafe directory/);
  process.env.CLICK_DOT_AUTH_FILE = path;

  await writeFile(process.env.CLICK_DOT_CHAT_FILE, Buffer.alloc(2_100_000, 1));
  await assert.rejects(history.listConversations(snapshot), /not private or exceeds its storage limit/);
  await rm(process.env.CLICK_DOT_CHAT_FILE);

  await mkdir(`${path}.lock`);
  await assert.rejects(auth.saveIssuedClientId("client-windows", 1), /lock acquisition timed out/);
  assert.equal((await lstat(`${path}.lock`)).isDirectory(), true, "an existing lock must be left untouched");
  await writeFile(`${path}.lock-target`, "");
  await rm(`${path}.lock`, { recursive: true });
  await symlink(`${path}.lock-target`, `${path}.lock`);
  await assert.rejects(auth.saveIssuedClientId("client-windows", 1), /unsafe local account lock/);
});

test("POSIX behavior is unchanged: files are 0600 plaintext JSON and wrong modes are rejected", async () => {
  await freshStorage();
  const snapshot = await signIn();
  const authInfo = await stat(process.env.CLICK_DOT_AUTH_FILE);
  assert.equal(authInfo.mode & 0o777, 0o600);
  assert.equal(JSON.parse(await readFile(process.env.CLICK_DOT_AUTH_FILE, "utf8")).connection.accessToken, token);
  const id = "44444444-4444-4444-8444-444444444444";
  await history.appendCompletedTurn(snapshot, 0, id, "model-a", message, reply);
  assert.equal((await stat(process.env.CLICK_DOT_CHAT_FILE)).mode & 0o777, 0o600);
  assert.equal(JSON.parse(await readFile(process.env.CLICK_DOT_CHAT_FILE, "utf8")).version, 1);
  await chmod(process.env.CLICK_DOT_CHAT_FILE, 0o644);
  await assert.rejects(history.listConversations(snapshot), /not private or exceeds its storage limit/);
  await chmod(process.env.CLICK_DOT_CHAT_FILE, 0o600);
  await chmod(process.env.CLICK_DOT_AUTH_FILE, 0o644);
  await assert.rejects(auth.getConnection(), /Local account storage file is not private/);
  await chmod(process.env.CLICK_DOT_AUTH_FILE, 0o600);
  assert.equal((await auth.getConnection()).accessToken, token);
});
