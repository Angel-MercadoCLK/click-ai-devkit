import assert from "node:assert/strict";
import { existsSync, readdirSync, readFileSync, statSync } from "node:fs";
import { mkdtemp, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { test } from "node:test";

const desktop = await import("../desktop/attachments.mts");
const lib = await import("../lib/attachments.ts");

const PNG = Buffer.concat([Buffer.from([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]), Buffer.alloc(32, 1)]);
const JPEG = Buffer.concat([Buffer.from([0xff, 0xd8, 0xff, 0xe0]), Buffer.alloc(32, 2)]);
const GIF = Buffer.concat([Buffer.from("GIF89a"), Buffer.alloc(32, 3)]);
const WEBP = Buffer.concat([Buffer.from("RIFF"), Buffer.from([0x24, 0, 0, 0]), Buffer.from("WEBP"), Buffer.alloc(32, 4)]);

async function scratch() {
  const base = await mkdtemp(join(tmpdir(), "click-dot-attach-test-"));
  return { base, done: () => rm(base, { recursive: true, force: true }) };
}

// Desktop store ---------------------------------------------------------------------------------------------------

test("the store writes each file under its own folder in the temp directory, returns an absolute path and a sanitized name", async () => {
  const { base, done } = await scratch();
  try {
    const store = desktop.createAttachmentStore({ baseDir: base });
    const saved = store.save({ bytes: PNG, name: "..\\..\\evil/ná me.png", kind: "image", extension: "png" });
    assert.equal(saved.ok, true);
    const { attachment } = saved;
    assert.match(attachment.id, /^att_[0-9a-f]{16}$/);
    assert.ok(attachment.path.startsWith(base), "inside the store's folder");
    assert.ok(!attachment.name.includes("/") && !attachment.name.includes("\\"), "no path separators survive in the name");
    assert.deepEqual(readFileSync(attachment.path), PNG);
    if (process.platform !== "win32") assert.equal(statSync(attachment.path).mode & 0o077, 0, "only the user can read it");
    assert.equal(store.resolve(attachment.id)?.path, attachment.path);
    assert.equal(store.resolve("att_0000000000000000"), undefined);
    assert.equal(store.resolve("../../etc/passwd"), undefined);
  } finally { await done(); }
});

test("size, count and total limits are refused with a Spanish reason, and nothing is written for a refused file", async () => {
  const { base, done } = await scratch();
  try {
    const store = desktop.createAttachmentStore({ baseDir: base, maxFileBytes: 100, maxFiles: 2, maxTotalBytes: 150 });
    assert.equal(store.save({ bytes: Buffer.alloc(101), name: "big.txt", kind: "file" }).ok, false);
    assert.equal(store.save({ bytes: Buffer.alloc(0), name: "empty.txt", kind: "file" }).ok, false);
    assert.equal(store.save({ bytes: Buffer.alloc(90), name: "a.txt", kind: "file" }).ok, true);
    const over = store.save({ bytes: Buffer.alloc(90), name: "b.txt", kind: "file" });
    assert.equal(over.ok, false);
    assert.match(over.error, /espacio|límite/i);
    assert.equal(store.save({ bytes: Buffer.alloc(10), name: "c.txt", kind: "file" }).ok, true);
    const third = store.save({ bytes: Buffer.alloc(1), name: "d.txt", kind: "file" });
    assert.equal(third.ok, false, "a third file is over the count");
    assert.equal(readdirSync(join(base, readdirSync(base)[0])).length, 2);
  } finally { await done(); }
});

test("remove deletes one file, dispose deletes the whole folder, and both are safe to repeat", async () => {
  const { base, done } = await scratch();
  try {
    const store = desktop.createAttachmentStore({ baseDir: base });
    const one = store.save({ bytes: PNG, name: "a.png", kind: "image", extension: "png" }).attachment;
    const two = store.save({ bytes: PNG, name: "b.png", kind: "image", extension: "png" }).attachment;
    assert.equal(store.remove(one.id), true);
    assert.equal(existsSync(one.path), false);
    assert.equal(store.remove(one.id), false);
    assert.equal(existsSync(two.path), true);
    store.dispose();
    assert.equal(existsSync(two.path), false);
    assert.deepEqual(readdirSync(base), [], "the folder itself is gone");
    store.dispose();
    assert.equal(store.save({ bytes: PNG, name: "c.png", kind: "image", extension: "png" }).ok, false, "nothing is written after quitting");
  } finally { await done(); }
});

// Library: types and the route logic -------------------------------------------------------------------------------

test("an image is recognized by its bytes, not by what the page claims", () => {
  assert.deepEqual(lib.sniffImage(PNG), { mime: "image/png", extension: "png" });
  assert.deepEqual(lib.sniffImage(JPEG), { mime: "image/jpeg", extension: "jpg" });
  assert.deepEqual(lib.sniffImage(GIF), { mime: "image/gif", extension: "gif" });
  assert.deepEqual(lib.sniffImage(WEBP), { mime: "image/webp", extension: "webp" });
  assert.equal(lib.sniffImage(Buffer.from("<svg onload=alert(1)>")), undefined);
  assert.equal(lib.sniffImage(Buffer.alloc(4)), undefined);
});

test("the prompt names each attached file by its absolute path and leaves images to localImage", () => {
  const attachments = [
    { id: "att_1", path: "/tmp/x/a.png", name: "a.png", kind: "image" },
    { id: "att_2", path: "/tmp/x/notas.md", name: "notas.md", kind: "file" },
  ];
  assert.deepEqual(lib.localImagePaths(attachments), ["/tmp/x/a.png"]);
  assert.equal(lib.promptWithFiles("Resume esto", attachments), "Resume esto\n\nArchivos adjuntos (lee cada uno con tus herramientas):\n- /tmp/x/notas.md (notas.md)");
  assert.equal(lib.promptWithFiles("Hola", [attachments[0]]), "Hola", "images alone add nothing to the text");
});

// Routes ----------------------------------------------------------------------------------------------------------

const STORE_KEY = Symbol.for("click-dot.attachments");
const CONTROL_KEY = Symbol.for("click-dot.control-token");
process.env.PORT ??= "3000";
const TOKEN = "a".repeat(64);

const upload = (body, headers = {}) => new Request("http://127.0.0.1:3000/api/attachments", {
  method: "POST", body,
  headers: { host: "127.0.0.1:3000", origin: "http://127.0.0.1:3000", "content-type": "application/octet-stream", ...headers },
});

async function withStore(run, options = {}) {
  const { base, done } = await scratch();
  const store = desktop.createAttachmentStore({ baseDir: base, ...options });
  const saved = [globalThis[STORE_KEY], globalThis[CONTROL_KEY]];
  globalThis[STORE_KEY] = store;
  delete globalThis[CONTROL_KEY];
  try { await run(store); }
  finally {
    store.dispose();
    for (const [index, key] of [STORE_KEY, CONTROL_KEY].entries()) { if (saved[index] === undefined) delete globalThis[key]; else globalThis[key] = saved[index]; }
    await done();
  }
}

test("an uploaded image answers its id, kind and name but never its path, and a text file is kept as a file", async () => {
  await withStore(async (store) => {
    const image = await lib.handleUploadRequest(upload(PNG, { "x-file-name": encodeURIComponent("captura ñ.png") }));
    assert.equal(image.status, 201);
    const body = await image.json();
    assert.deepEqual(Object.keys(body).sort(), ["bytes", "id", "kind", "name"]);
    assert.equal(body.kind, "image");
    assert.equal(store.resolve(body.id).path.endsWith(".png"), true);
    const file = await (await lib.handleUploadRequest(upload(Buffer.from("# notas"), { "x-file-name": "notas.md" }))).json();
    assert.equal(file.kind, "file");
    const removed = await lib.handleRemoveRequest(new Request("http://127.0.0.1:3000/api/attachments/x", { method: "DELETE", headers: { host: "127.0.0.1:3000", origin: "http://127.0.0.1:3000" } }), file.id);
    assert.equal(removed.status, 200);
    assert.equal(store.resolve(file.id), undefined);
  });
});

test("an SVG or a script claiming to be an image is stored as a plain file, never as localImage", async () => {
  await withStore(async (store) => {
    const body = await (await lib.handleUploadRequest(upload(Buffer.from("<svg onload=x>"), { "content-type": "image/png", "x-file-name": "a.png" }))).json();
    assert.equal(body.kind, "file");
    assert.deepEqual(lib.localImagePaths([store.resolve(body.id)]), []);
  });
});

test("the upload route needs the exact local origin and the control cookie, a store, and a bounded body", async () => {
  await withStore(async () => {
    const foreign = new Request("http://127.0.0.1:3000/api/attachments", { method: "POST", body: PNG, headers: { host: "127.0.0.1:3000", origin: "http://evil.example" } });
    assert.equal((await lib.handleUploadRequest(foreign)).status, 403);
    const noOrigin = new Request("http://127.0.0.1:3000/api/attachments", { method: "POST", body: PNG, headers: { host: "127.0.0.1:3000" } });
    assert.equal((await lib.handleUploadRequest(noOrigin)).status, 403, "a write must send the exact origin");
    globalThis[CONTROL_KEY] = TOKEN;
    assert.equal((await lib.handleUploadRequest(upload(PNG))).status, 403, "without the cookie the published token refuses it");
    const withCookie = await lib.handleUploadRequest(upload(PNG, { cookie: `click_dot_control=${TOKEN}` }));
    assert.equal(withCookie.status, 201);
    delete globalThis[CONTROL_KEY];
    assert.equal((await lib.handleUploadRequest(upload(Buffer.alloc(lib.MAX_UPLOAD_BYTES + 1, 1)))).status, 413);
    assert.equal((await lib.handleUploadRequest(upload(Buffer.alloc(0)))).status, 422, "an empty file is refused by the store");
  });
  const saved = globalThis[STORE_KEY];
  delete globalThis[STORE_KEY];
  try { assert.equal((await lib.handleUploadRequest(upload(PNG))).status, 501, "without the desktop shell nothing can be attached"); }
  finally { if (saved !== undefined) globalThis[STORE_KEY] = saved; }
});

test("message attachments resolve only ids the store issued, once each, at most six", async () => {
  await withStore(async (store) => {
    const ids = [0, 1].map((n) => store.save({ bytes: PNG, name: `${n}.png`, kind: "image", extension: "png" }).attachment.id);
    assert.deepEqual(lib.resolveAttachmentIds(store, undefined), []);
    assert.equal(lib.resolveAttachmentIds(store, ids).length, 2);
    assert.equal(lib.resolveAttachmentIds(store, [ids[0], ids[0]]), undefined, "a repeated id");
    assert.equal(lib.resolveAttachmentIds(store, ["att_0000000000000000"]), undefined, "an unknown id");
    assert.equal(lib.resolveAttachmentIds(store, ["/etc/passwd"]), undefined, "a path is not an id");
    assert.equal(lib.resolveAttachmentIds(store, "x"), undefined);
    assert.equal(lib.resolveAttachmentIds(null, ids), undefined, "no store, no attachments");
    assert.equal(lib.resolveAttachmentIds(store, Array(7).fill(ids[0])), undefined);
  });
});
