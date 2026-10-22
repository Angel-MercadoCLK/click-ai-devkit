// Vendors the click-ai-devkit memory-guard patterns into Dot as generated JSON.
//
//   node scripts/sync-guard-patterns.mjs <path to click-ai-devkit/internal/guard/patterns.yaml> [--out <file>] [--check]
//
// The YAML has a small, fixed shape, so it is parsed here line by line without a YAML dependency. Anything the
// parser does not recognise is an error: a silently dropped rule would mean personal data left the machine.
// Dot and the devkit must agree on what a pattern matches. Both sides are meant to run the same golden corpus,
// test/fixtures/guard-corpus.json. The Dot side is test/guard-patterns.test.mjs; the devkit side is
// internal/guard/dot_corpus_test.go, which runs the same corpus against the embedded Go patterns.
import { createHash } from "node:crypto";
import { readFileSync, writeFileSync } from "node:fs";
import { fileURLToPath } from "node:url";

const defaultOutput = fileURLToPath(new URL("../lib/guard-patterns.json", import.meta.url));

function fail(line, message) {
  throw new Error(`patterns.yaml line ${line}: ${message}`);
}

function doubleQuoted(token, line, field) {
  if (token.length < 2 || !token.startsWith('"') || !token.endsWith('"')) fail(line, `${field} must be a double-quoted string`);
  try {
    const value = JSON.parse(token);
    if (typeof value !== "string") throw new Error();
    return value;
  } catch {
    fail(line, `${field} is not a valid double-quoted string`);
  }
}

function singleQuoted(token, line, field) {
  if (token.length < 2 || !token.startsWith("'") || !token.endsWith("'")) fail(line, `${field} must be a single-quoted string`);
  const body = token.slice(1, -1);
  if (/(?<!')'(?!')/.test(body.replace(/''/g, ""))) fail(line, `${field} has an unescaped quote`);
  return body.replace(/''/g, "'");
}

/** Returns the categories and rules found in the YAML text; throws on any line that does not fit the shape. */
export function parsePatternsYaml(text) {
  const categories = [];
  let sawHeader = false;
  let category;
  let rule;
  let expectRules = false;
  const lines = text.replace(/^﻿/, "").split(/\r?\n/);
  const closeRule = (line) => {
    if (!rule) return;
    if (rule.description === undefined || rule.pattern === undefined) fail(line, `rule ${rule.id} needs a description and a pattern`);
    category.rules.push(rule);
    rule = undefined;
  };
  const closeCategory = (line) => {
    closeRule(line);
    if (!category) return;
    if (category.reason === undefined || category.rules.length === 0) fail(line, `category ${category.name} needs a reason and at least one rule`);
    categories.push(category);
    category = undefined;
  };
  lines.forEach((raw, index) => {
    const line = index + 1;
    if (/\t/.test(raw)) fail(line, "tabs are not allowed");
    if (raw.trim() === "" || raw.trimStart().startsWith("#")) return;
    if (raw === "categories:") {
      if (sawHeader) fail(line, "duplicate categories header");
      sawHeader = true;
      return;
    }
    if (!sawHeader) fail(line, "expected the categories header first");
    let match;
    if ((match = /^ {2}- name: ([a-z0-9-]+)$/.exec(raw))) {
      closeCategory(line);
      if (categories.some((existing) => existing.name === match[1])) fail(line, `duplicate category ${match[1]}`);
      category = { name: match[1], reason: undefined, rules: [] };
      expectRules = false;
    } else if ((match = /^ {4}reason: (.+)$/.exec(raw)) && category && category.reason === undefined && !rule) {
      category.reason = doubleQuoted(match[1], line, "reason");
    } else if (raw === "    rules:" && category && !expectRules && category.rules.length === 0) {
      expectRules = true;
    } else if ((match = /^ {6}- id: ([a-z0-9-]+\/[a-z0-9-]+)$/.exec(raw)) && category && expectRules) {
      closeRule(line);
      if (!match[1].startsWith(`${category.name}/`)) fail(line, `rule id ${match[1]} must start with ${category.name}/`);
      rule = { id: match[1], description: undefined, pattern: undefined };
    } else if ((match = /^ {8}description: (.+)$/.exec(raw)) && rule && rule.description === undefined) {
      rule.description = doubleQuoted(match[1], line, "description");
    } else if ((match = /^ {8}pattern: (.+)$/.exec(raw)) && rule && rule.pattern === undefined) {
      rule.pattern = singleQuoted(match[1], line, "pattern");
    } else {
      fail(line, "unexpected content");
    }
  });
  closeCategory(lines.length);
  if (!sawHeader || categories.length === 0) fail(1, "no categories found");
  return categories;
}

/** Converts one RE2 pattern into JS: a leading (?i) becomes the ignoreCase flag; other RE2-only syntax is refused. */
export function convertPattern(id, pattern) {
  let source = pattern;
  let ignoreCase = false;
  if (source.startsWith("(?i)")) {
    ignoreCase = true;
    source = source.slice(4);
  }
  if (/\(\?[a-zA-Z-]+[:)]/.test(source)) throw new Error(`${id}: inline flags are only supported as a leading (?i)`);
  if (/\\[AzQEpPC]|\[\[:/.test(source)) throw new Error(`${id}: RE2-only syntax is not supported in JS`);
  if (source === "") throw new Error(`${id}: empty pattern`);
  try {
    new RegExp(source, ignoreCase ? "gi" : "g");
  } catch (error) {
    throw new Error(`${id}: does not compile in JS (${error.message})`);
  }
  return { source, ignoreCase };
}

export function buildGuardPatterns(yamlText, sourceName = "internal/guard/patterns.yaml") {
  const rules = [];
  const seen = new Set();
  for (const category of parsePatternsYaml(yamlText)) {
    for (const rule of category.rules) {
      if (seen.has(rule.id)) throw new Error(`duplicate rule id ${rule.id}`);
      seen.add(rule.id);
      rules.push({ id: rule.id, category: category.name, ...convertPattern(rule.id, rule.pattern) });
    }
  }
  return {
    version: 1,
    generatedBy: "scripts/sync-guard-patterns.mjs",
    source: { file: sourceName, sha256: createHash("sha256").update(yamlText).digest("hex") },
    rules,
  };
}

function main(argv) {
  const args = argv.slice(2);
  let input;
  let output = defaultOutput;
  let check = false;
  for (let index = 0; index < args.length; index += 1) {
    if (args[index] === "--check") check = true;
    else if (args[index] === "--out") output = args[++index];
    else if (!input && !args[index].startsWith("--")) input = args[index];
    else throw new Error("Usage: node scripts/sync-guard-patterns.mjs <patterns.yaml> [--out <file>] [--check]");
  }
  if (!input || !output) throw new Error("Usage: node scripts/sync-guard-patterns.mjs <patterns.yaml> [--out <file>] [--check]");
  const generated = `${JSON.stringify(buildGuardPatterns(readFileSync(input, "utf8")), null, 2)}\n`;
  if (check) {
    if (readFileSync(output, "utf8") !== generated) throw new Error(`${output} is out of date; run this script without --check.`);
    console.log(`${output} is up to date.`);
    return;
  }
  writeFileSync(output, generated);
  console.log(`Wrote ${output}.`);
}

if (process.argv[1] && fileURLToPath(import.meta.url) === process.argv[1]) {
  try {
    main(process.argv);
  } catch (error) {
    console.error(error instanceof Error ? error.message : String(error));
    process.exit(1);
  }
}
