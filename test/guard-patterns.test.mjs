import assert from "node:assert/strict";
import { test } from "node:test";
import { mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";

const redaction = await import("../desktop/redaction.mts");
const sync = await import("../scripts/sync-guard-patterns.mjs");

const patternsPath = fileURLToPath(new URL("../lib/guard-patterns.json", import.meta.url));
const corpusPath = fileURLToPath(new URL("./fixtures/guard-corpus.json", import.meta.url));
const scriptPath = fileURLToPath(new URL("../scripts/sync-guard-patterns.mjs", import.meta.url));
const generated = JSON.parse(await readFile(patternsPath, "utf8"));
const corpus = JSON.parse(await readFile(corpusPath, "utf8"));
const compiled = redaction.compileGuardPatterns(redaction.parseGuardPatterns(generated));

const SAMPLE_YAML = `categories:
  # comment
  - name: pii
    reason: "blocked: detected pii"
    rules:
      - id: pii/email
        description: "Email addresses."
        pattern: '(?i)\\b[A-Z0-9._%+\\-]+@[A-Z0-9.\\-]+\\.[A-Z]{2,}\\b'
      - id: pii/quote
        description: "Has \\"quotes\\": inside."
        pattern: 'it''s [0-9]{3}'
`;

test("the generated patterns cover every devkit rule and compile as JS regular expressions", () => {
  assert.equal(generated.version, 1);
  assert.equal(compiled.length, 19);
  assert.equal(new Set(generated.rules.map((rule) => rule.id)).size, 19);
  for (const rule of generated.rules) {
    assert.doesNotMatch(rule.source, /^\(\?i\)/, `${rule.id} keeps its (?i) prefix`);
    assert.equal(typeof rule.ignoreCase, "boolean");
  }
  assert.equal(generated.rules.find((rule) => rule.id === "pii/email").ignoreCase, true);
  assert.equal(generated.rules.find((rule) => rule.id === "secrets/aws-access-key-id").ignoreCase, false);
});

test("the golden corpus matches exactly the expected rule ids, negatives included", () => {
  assert.ok(corpus.cases.length >= 45);
  assert.ok(corpus.cases.some((item) => item.rules.length === 0), "the corpus needs negative cases");
  for (const { text, rules } of corpus.cases) {
    const found = [...new Set(redaction.scanText(text, compiled).map((hit) => hit.rule))].sort();
    assert.deepEqual(found, [...rules].sort(), `corpus text: ${text}`);
  }
});

test("every rule is exercised by at least one positive corpus case", () => {
  const covered = new Set(corpus.cases.flatMap((item) => item.rules));
  for (const rule of generated.rules) assert.ok(covered.has(rule.id), `${rule.id} has no positive case`);
});

test("the sync script parses a minimal YAML the way the devkit's does, including quotes and the (?i) flag", () => {
  const built = sync.buildGuardPatterns(SAMPLE_YAML, "sample.yaml");
  assert.deepEqual(built.rules.map((rule) => [rule.id, rule.category, rule.ignoreCase]), [["pii/email", "pii", true], ["pii/quote", "pii", false]]);
  assert.equal(built.rules[1].source, "it's [0-9]{3}");
  assert.equal(built.source.file, "sample.yaml");
  assert.match(built.source.sha256, /^[0-9a-f]{64}$/);
});

test("the sync script fails loudly on anything unexpected", () => {
  const broken = {
    "unknown key": SAMPLE_YAML.replace('        pattern: \'it', '        flavour: x\n        pattern: \'it'),
    "tab indent": SAMPLE_YAML.replace("    reason:", "\treason:"),
    "missing header": SAMPLE_YAML.replace("categories:\n", ""),
    "unquoted reason": SAMPLE_YAML.replace('reason: "blocked: detected pii"', "reason: blocked"),
    "double quoted pattern": SAMPLE_YAML.replace("pattern: 'it''s [0-9]{3}'", 'pattern: "x"'),
    "id outside its category": SAMPLE_YAML.replace("id: pii/quote", "id: other/quote"),
    "duplicate id": SAMPLE_YAML.replace("id: pii/quote", "id: pii/email"),
    "rule without pattern": SAMPLE_YAML.replace("        pattern: 'it''s [0-9]{3}'\n", ""),
    "inline flag other than leading (?i)": SAMPLE_YAML.replace("[0-9]{3}'", "(?s)[0-9]{3}'"),
    "RE2-only syntax": SAMPLE_YAML.replace("it''s [0-9]{3}", "\\\\Aabc"),
    "pattern that does not compile": SAMPLE_YAML.replace("it''s [0-9]{3}", "([0-9]"),
    "empty file": "",
  };
  for (const [name, text] of Object.entries(broken)) {
    assert.throws(() => sync.buildGuardPatterns(text), Error, name);
  }
});

test("the committed JSON is exactly what the script generates from the devkit YAML when it is available", async (t) => {
  const devkit = process.env.CLICK_DEVKIT_PATTERNS;
  if (!devkit) return t.skip("set CLICK_DEVKIT_PATTERNS to the devkit patterns.yaml to check the vendored copy");
  const result = spawnSync(process.execPath, [scriptPath, devkit, "--check"], { encoding: "utf8" });
  assert.equal(result.status, 0, result.stderr);
});

test("the script writes the JSON to the requested path and --check notices drift", async () => {
  const directory = await mkdtemp(join(tmpdir(), "click-dot-guard-"));
  try {
    const yaml = join(directory, "patterns.yaml");
    const out = join(directory, "out.json");
    await writeFile(yaml, SAMPLE_YAML);
    const wrote = spawnSync(process.execPath, [scriptPath, yaml, "--out", out], { encoding: "utf8" });
    assert.equal(wrote.status, 0, wrote.stderr);
    assert.equal(JSON.parse(await readFile(out, "utf8")).rules.length, 2);
    assert.equal(spawnSync(process.execPath, [scriptPath, yaml, "--out", out, "--check"], { encoding: "utf8" }).status, 0);
    await writeFile(yaml, SAMPLE_YAML.replace("Email addresses.", "Changed."));
    assert.equal(spawnSync(process.execPath, [scriptPath, yaml, "--out", out, "--check"], { encoding: "utf8" }).status, 1);
    await writeFile(yaml, "categories:\n  - bogus\n");
    const failed = spawnSync(process.execPath, [scriptPath, yaml, "--out", out], { encoding: "utf8" });
    assert.equal(failed.status, 1);
    assert.match(failed.stderr, /line 2/);
  } finally {
    await rm(directory, { recursive: true, force: true });
  }
});

test("loading the patterns fails closed on a missing, malformed or unexpected file", async () => {
  const directory = await mkdtemp(join(tmpdir(), "click-dot-guard-"));
  try {
    await assert.rejects(redaction.loadGuardPatterns(join(directory, "missing.json")), /could not be read/);
    const cases = {
      "not json": "nope",
      "wrong version": JSON.stringify({ ...generated, version: 2 }),
      "no rules": JSON.stringify({ ...generated, rules: [] }),
      "extra rule key": JSON.stringify({ ...generated, rules: [{ ...generated.rules[0], extra: 1 }] }),
      "bad id": JSON.stringify({ ...generated, rules: [{ ...generated.rules[0], id: "Bad Id" }] }),
      "category mismatch": JSON.stringify({ ...generated, rules: [{ ...generated.rules[0], category: "other" }] }),
      "non boolean flag": JSON.stringify({ ...generated, rules: [{ ...generated.rules[0], ignoreCase: "yes" }] }),
      "uncompilable": JSON.stringify({ ...generated, rules: [{ ...generated.rules[0], source: "([" }] }),
    };
    for (const [name, text] of Object.entries(cases)) {
      const file = join(directory, `${name.replace(/ /g, "-")}.json`);
      await writeFile(file, text);
      await assert.rejects(redaction.loadGuardPatterns(file), Error, name);
    }
    assert.equal((await redaction.loadGuardPatterns(patternsPath)).length, 19);
  } finally {
    await rm(directory, { recursive: true, force: true });
  }
});
