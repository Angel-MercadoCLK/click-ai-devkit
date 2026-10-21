#!/usr/bin/env node
// Builds the Click Dot Windows package and zips it as the release asset
//   click-dot_<version>_windows_amd64.zip
// with the unpacked app at the zip root (`Click Dot.exe` next to its resources).
//
// Contract with the Go installer (odd/tasks/click-dot-distribution.md, L5):
//   - <version> is the release tag without the leading "v".
//   - The zip root holds the unpacked app directly; `Click Dot.exe` is at the root.
//
// Usage (from anywhere in the repository):
//   node scripts/package-dot.mjs --version 1.2.3 [--out <dir>] [--skip-build]
//
// The zip is written to <out> (default: dot/release, which Dot's .gitignore already ignores). The
// output is deterministic for a given input tree: entries are sorted, timestamps are fixed and the
// writer does not depend on any `zip` binary, so it runs the same on Linux, macOS and Windows
// runners. Only Node (>= 24, as required by dot/package.json) is needed.

import { spawnSync } from "node:child_process";
import { createHash } from "node:crypto";
import {
  closeSync,
  existsSync,
  mkdirSync,
  openSync,
  readdirSync,
  readFileSync,
  rmSync,
  statSync,
  writeSync,
} from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { crc32, deflateRawSync } from "node:zlib";

const EXE_NAME = "Click Dot.exe";
const MAX_ENTRIES = 0xfffe; // plain zip (no zip64): fail loudly rather than write a broken archive
const MAX_SIZE = 0xfffffffe;

const here = path.dirname(fileURLToPath(import.meta.url));
const dotDir = path.resolve(here, "..", "dot");

function fail(message) {
  console.error(`package-dot: ${message}`);
  process.exit(1);
}

function parseArgs(argv) {
  const opts = { version: "", out: path.join(dotDir, "release"), skipBuild: false };
  for (let i = 0; i < argv.length; i++) {
    const arg = argv[i];
    if (arg === "--version") opts.version = argv[++i] ?? "";
    else if (arg === "--out") opts.out = path.resolve(argv[++i] ?? "");
    else if (arg === "--skip-build") opts.skipBuild = true;
    else fail(`unknown argument: ${arg}`);
  }
  opts.version = opts.version.replace(/^v/, "");
  if (!/^\d+\.\d+\.\d+(?:[-+][0-9A-Za-z.-]+)?$/.test(opts.version)) {
    fail(`--version must look like 1.2.3 (optionally with a leading "v" and a prerelease suffix); got "${opts.version}"`);
  }
  return opts;
}

function buildPackage() {
  const result = spawnSync("npm", ["run", "desktop:pack:win"], {
    cwd: dotDir,
    stdio: "inherit",
    // npm is npm.cmd on Windows, which needs a shell to be spawned.
    shell: process.platform === "win32",
  });
  if (result.status !== 0) fail(`"npm run desktop:pack:win" failed (exit ${result.status ?? result.signal})`);
}

function listFiles(root) {
  const files = [];
  const walk = (dir) => {
    for (const entry of readdirSync(dir, { withFileTypes: true })) {
      const full = path.join(dir, entry.name);
      if (entry.isDirectory()) walk(full);
      else if (entry.isFile()) files.push(full);
      else fail(`unsupported entry in package (symlink or special file): ${full}`);
    }
  };
  walk(root);
  const rel = files.map((f) => ({ full: f, name: path.relative(root, f).split(path.sep).join("/") }));
  rel.sort((a, b) => (a.name < b.name ? -1 : a.name > b.name ? 1 : 0));
  return rel;
}

function u16(n) {
  const b = Buffer.alloc(2);
  b.writeUInt16LE(n);
  return b;
}
function u32(n) {
  const b = Buffer.alloc(4);
  b.writeUInt32LE(n >>> 0);
  return b;
}

function writeZip(zipPath, files) {
  if (files.length > MAX_ENTRIES) fail(`too many files for a plain zip: ${files.length}`);
  const DOS_TIME = 0; // 00:00:00
  const DOS_DATE = (0 << 9) | (1 << 5) | 1; // 1980-01-01
  const fd = openSync(zipPath, "w");
  let offset = 0;
  const central = [];
  const write = (buf) => {
    writeSync(fd, buf);
    offset += buf.length;
  };
  try {
    for (const file of files) {
      const data = readFileSync(file.full);
      if (data.length > MAX_SIZE) fail(`file too large for a plain zip: ${file.name}`);
      const crc = crc32(data);
      const deflated = data.length === 0 ? data : deflateRawSync(data, { level: 9 });
      const useDeflate = data.length > 0 && deflated.length < data.length;
      const body = useDeflate ? deflated : data;
      const method = useDeflate ? 8 : 0;
      const nameBuf = Buffer.from(file.name, "utf8");
      const flags = 0x0800; // UTF-8 names
      const localOffset = offset;
      write(
        Buffer.concat([
          u32(0x04034b50), u16(20), u16(flags), u16(method), u16(DOS_TIME), u16(DOS_DATE),
          u32(crc), u32(body.length), u32(data.length), u16(nameBuf.length), u16(0), nameBuf,
        ]),
      );
      write(body);
      const mode = statSync(file.full).mode & 0o111 ? 0o100755 : 0o100644;
      central.push(
        Buffer.concat([
          u32(0x02014b50), u16((3 << 8) | 20), u16(20), u16(flags), u16(method), u16(DOS_TIME), u16(DOS_DATE),
          u32(crc), u32(body.length), u32(data.length), u16(nameBuf.length), u16(0), u16(0), u16(0), u16(0),
          u32((mode << 16) >>> 0), u32(localOffset), nameBuf,
        ]),
      );
    }
    const centralStart = offset;
    for (const entry of central) write(entry);
    const centralSize = offset - centralStart;
    if (offset > MAX_SIZE) fail("archive too large for a plain zip");
    write(
      Buffer.concat([
        u32(0x06054b50), u16(0), u16(0), u16(central.length), u16(central.length),
        u32(centralSize), u32(centralStart), u16(0),
      ]),
    );
  } finally {
    closeSync(fd);
  }
}

const opts = parseArgs(process.argv.slice(2));
if (!opts.skipBuild) buildPackage();

const unpacked = path.join(dotDir, "release", "win-unpacked");
if (!existsSync(path.join(unpacked, EXE_NAME))) {
  fail(`${EXE_NAME} not found in ${unpacked}; run without --skip-build`);
}

const assetName = `click-dot_${opts.version}_windows_amd64.zip`;
const zipPath = path.join(opts.out, assetName);
mkdirSync(opts.out, { recursive: true });
rmSync(zipPath, { force: true });

const files = listFiles(unpacked);
if (!files.some((f) => f.name === EXE_NAME)) fail(`${EXE_NAME} is not at the package root`);
writeZip(zipPath, files);

const sha256 = createHash("sha256").update(readFileSync(zipPath)).digest("hex");
console.log(`package-dot: wrote ${zipPath}`);
console.log(`package-dot: ${files.length} files, ${statSync(zipPath).size} bytes`);
console.log(`package-dot: sha256 ${sha256}  ${assetName}`);
