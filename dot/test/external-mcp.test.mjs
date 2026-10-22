import assert from "node:assert/strict";
import { test } from "node:test";

const external = await import("../desktop/external-mcp.mts");
const toml = await import("../desktop/external-mcp-toml.mts");
const { SHIM_SAFE_ARG } = await import("../desktop/codex-app-server.mts");

const present = (...names) => async (path) => names.some((name) => path.toLowerCase().endsWith(name));
const WIN = { platform: "win32", env: { PATH: "C:\\tools;C:\\node", PATHEXT: ".COM;.EXE;.BAT;.CMD", LOCALAPPDATA: "C:\\Users\\u\\AppData\\Local" }, listDir: async () => [] };
const EXT = "extensions\\mmlmfjhmonkocbjadbfplnigmagldckm";
const NIX = { platform: "linux", env: { PATH: "/usr/bin:/opt/bin" } };

test("literal values hold only shim-safe characters and refuse everything else", () => {
  assert.equal(toml.tomlLiteral("@playwright/mcp@0.0.83"), "'@playwright/mcp@0.0.83'");
  assert.equal(toml.tomlLiteralArray(["-y", "a.b"]), "['-y','a.b']");
  for (const bad of ["has space", "it's", 'say "x"', "a%b", "a&b", "a|b", "a<b", "a>b", "a^b", "a!b", "", "line\nbreak"]) {
    assert.throws(() => toml.tomlLiteral(bad), `${JSON.stringify(bad)} must be refused`);
  }
});

test("findOnPath looks through PATH with the Windows extensions, and returns null when nothing matches", async () => {
  assert.equal(await external.findOnPath(["npx"], { ...WIN, exists: present("c:\\node\\npx.cmd") }), "C:\\node\\npx.cmd");
  assert.equal(await external.findOnPath(["uvx", "uv"], { ...WIN, exists: present("c:\\tools\\uv.exe") }), "C:\\tools\\uv.exe");
  assert.equal(await external.findOnPath(["npx"], { ...NIX, exists: present("/opt/bin/npx") }), "/opt/bin/npx");
  assert.equal(await external.findOnPath(["npx"], { ...NIX, exists: present() }), null);
  assert.equal(await external.findOnPath(["npx"], { platform: "linux", env: {}, exists: present("npx") }), null, "no PATH means nothing is found");
});

test("with npx and uvx on Windows both servers are added, pinned, safe for a .cmd shim and approval 'writes'", async () => {
  const found = await external.detectExternalMcp({ ...WIN, listDir: async () => ["Default"], exists: present("npx.cmd", "uvx.exe", `default\\${EXT}`) });
  assert.equal(found.info.chrome.state, "ready");
  assert.equal(found.info.computer.state, "ready");
  const lines = found.overrides;
  for (const line of lines) assert.ok(SHIM_SAFE_ARG.test(line) && !/["\s%&|<>^!]/.test(line), `${line} must be shim safe`);
  assert.ok(lines.includes("mcp_servers.chrome.command='npx'"));
  assert.ok(lines.includes(`mcp_servers.chrome.args=['-y','@playwright/mcp@${external.PLAYWRIGHT_MCP_VERSION}','--extension']`));
  assert.ok(lines.includes("mcp_servers.chrome.default_tools_approval_mode='writes'"));
  assert.ok(lines.includes("mcp_servers.computer.command='uvx'"));
  assert.ok(lines.includes(`mcp_servers.computer.args=['windows-mcp@${external.WINDOWS_MCP_VERSION}','serve','--control-policy','strict']`));
  assert.ok(lines.includes("mcp_servers.computer.default_tools_approval_mode='writes'"));
  assert.ok(lines.includes("mcp_servers.computer.env.ANONYMIZED_TELEMETRY='false'"));
  const enabled = lines.find((line) => line.startsWith("mcp_servers.computer.enabled_tools="));
  for (const tool of ["Click", "Type", "Scroll", "Move", "Shortcut", "Screenshot", "Snapshot"]) assert.ok(enabled.includes(`'${tool}'`), `${tool} is an input or screen tool`);
  for (const tool of ["PowerShell", "FileSystem", "Registry", "Process", "Clipboard", "App", "Scrape", "Notification"]) assert.ok(!enabled.includes(`'${tool}'`), `${tool} must stay off`);
  assert.deepEqual(found.env, { ANONYMIZED_TELEMETRY: "false" });
});

test("without uv the computer server is not added and the card says what is missing; Chrome still is", async () => {
  const found = await external.detectExternalMcp({ ...WIN, exists: present("npx.cmd") });
  assert.equal(found.info.chrome.state, "missing", "the Playwright extension is not installed in any profile");
  assert.equal(found.info.computer.state, "missing");
  assert.match(found.info.computer.hint, /uv/);
  assert.ok(found.overrides.every((line) => !line.startsWith("mcp_servers.computer.")));
  assert.equal(found.info.chrome.link, external.CHROME_EXTENSION_URL);
});

test("without Node nothing is added for Chrome, and off Windows the mouse and keyboard server is unsupported", async () => {
  const none = await external.detectExternalMcp({ ...WIN, exists: present() });
  assert.deepEqual(none.overrides, []);
  assert.equal(none.info.chrome.state, "missing");
  assert.match(none.info.chrome.hint, /Node/);
  const linux = await external.detectExternalMcp({ ...NIX, exists: present("/usr/bin/npx", "/usr/bin/uvx") });
  assert.equal(linux.info.computer.state, "unsupported");
  assert.ok(linux.overrides.every((line) => !line.startsWith("mcp_servers.computer.")));
  assert.equal(linux.info.chrome.state, "ready");
});

test("only uv on PATH runs the server through `uv tool run`", async () => {
  const found = await external.detectExternalMcp({ ...WIN, exists: present("uv.exe") });
  assert.ok(found.overrides.includes("mcp_servers.computer.command='uv'"));
  assert.ok(found.overrides.includes(`mcp_servers.computer.args=['tool','run','windows-mcp@${external.WINDOWS_MCP_VERSION}','serve','--control-policy','strict']`));
});

test("the availability is published for the status route, and a malformed publication reads as nothing", () => {
  const target = {};
  const info = { chrome: { state: "ready", hint: "h" }, computer: { state: "missing", hint: "m" } };
  external.installExternalMcp(target, info);
  assert.deepEqual(Reflect.get(target, Symbol.for(external.EXTERNAL_MCP_KEY)), info);
  assert.throws(() => external.installExternalMcp({}, { chrome: 1 }));
});

test("Chrome is ready only when the Playwright extension is installed in a Chrome profile; otherwise the card links to it", async () => {
  const inProfile = await external.detectExternalMcp({ ...WIN, listDir: async () => ["System Profile", "Profile 2", "Default"], exists: present("npx.cmd", `profile 2\\${EXT}`) });
  assert.equal(inProfile.info.chrome.state, "ready");
  assert.match(inProfile.info.chrome.hint, /listo/i);
  const notInstalled = await external.detectExternalMcp({ ...WIN, listDir: async () => ["Default"], exists: present("npx.cmd") });
  assert.equal(notInstalled.info.chrome.state, "missing");
  assert.match(notInstalled.info.chrome.hint, /instala la extensión/);
  assert.equal(notInstalled.info.chrome.link, external.CHROME_EXTENSION_URL);
  assert.ok(notInstalled.overrides.some((line) => line.startsWith("mcp_servers.chrome.")), "the server is still added, so installing the extension needs no restart of the config");
  const noChromeData = await external.detectExternalMcp({ ...WIN, listDir: async () => { throw new Error("ENOENT"); }, exists: present("npx.cmd") });
  assert.equal(noChromeData.info.chrome.state, "missing");
});
