import { readFile } from "node:fs/promises";

// Local personal-data redaction for screen images. Everything here is pure except `loadGuardPatterns`.
// OCR words go in, padded rectangles come out; `redactBitmap` then pixelates those rectangles. The OCR text
// itself is never logged or returned: only rule ids and boxes leave `findSensitiveRegions`.

export type GuardRule = { id: string; category: string; source: string; ignoreCase: boolean };
export type CompiledGuardRule = { id: string; category: string; regex: RegExp };
export type OcrWord = { text: string; x: number; y: number; w: number; h: number; line: number };
export type Rect = { x: number; y: number; w: number; h: number };
export type SensitiveRegion = Rect & { rules: string[] };

/** A longer line is scanned in overlapping windows, so a very long OCR line cannot stall the matcher. */
export const MAX_LINE_CHARS = 500;
const WINDOW_OVERLAP_CHARS = 120;
const MAX_RULES = 200;
const MAX_RULE_SOURCE_CHARS = 2_000;
export const MIN_BLOCK_PIXELS = 8;

const ruleKeys = ["id", "category", "source", "ignoreCase"];

/** Strictly validates the generated lib/guard-patterns.json. Anything unexpected throws, so nothing is sent. */
export function parseGuardPatterns(value: unknown): GuardRule[] {
  if (!value || typeof value !== "object" || Array.isArray(value)) throw new Error("The guard patterns file is invalid.");
  const document = value as Record<string, unknown>;
  if (document.version !== 1 || !Array.isArray(document.rules)) throw new Error("The guard patterns file is invalid.");
  if (document.rules.length < 1 || document.rules.length > MAX_RULES) throw new Error("The guard patterns file is invalid.");
  const seen = new Set<string>();
  return document.rules.map((item): GuardRule => {
    if (!item || typeof item !== "object" || Array.isArray(item)) throw new Error("The guard patterns file is invalid.");
    const rule = item as Record<string, unknown>;
    if (Object.keys(rule).length !== ruleKeys.length || ruleKeys.some((key) => !(key in rule))) throw new Error("The guard patterns file is invalid.");
    const { id, category, source, ignoreCase } = rule;
    if (typeof id !== "string" || !/^[a-z0-9-]+\/[a-z0-9-]+$/.test(id) || seen.has(id)) throw new Error("The guard patterns file is invalid.");
    if (typeof category !== "string" || !id.startsWith(`${category}/`)) throw new Error("The guard patterns file is invalid.");
    if (typeof source !== "string" || source.length < 1 || source.length > MAX_RULE_SOURCE_CHARS || typeof ignoreCase !== "boolean") {
      throw new Error("The guard patterns file is invalid.");
    }
    seen.add(id);
    return { id, category, source, ignoreCase };
  });
}

export function compileGuardPatterns(rules: GuardRule[]): CompiledGuardRule[] {
  try {
    return rules.map((rule) => ({ id: rule.id, category: rule.category, regex: new RegExp(rule.source, rule.ignoreCase ? "gi" : "g") }));
  } catch {
    throw new Error("The guard patterns could not be compiled.");
  }
}

/** Reads and compiles the patterns. Any failure throws: the caller must treat that as "send nothing". */
export async function loadGuardPatterns(file: string): Promise<CompiledGuardRule[]> {
  let parsed: unknown;
  try {
    parsed = JSON.parse(await readFile(file, "utf8"));
  } catch {
    throw new Error("The guard patterns could not be read.");
  }
  return compileGuardPatterns(parseGuardPatterns(parsed));
}

type Hit = { rule: string; start: number; end: number };

function scanWindow(text: string, offset: number, rules: CompiledGuardRule[], hits: Hit[]) {
  for (const rule of rules) {
    for (const match of text.matchAll(rule.regex)) {
      if (match[0].length === 0 || match.index === undefined) continue;
      hits.push({ rule: rule.id, start: offset + match.index, end: offset + match.index + match[0].length });
    }
  }
}

/** Every match of every rule in `text`, as character spans. */
export function scanText(text: string, rules: CompiledGuardRule[]): Hit[] {
  const hits: Hit[] = [];
  if (text.length <= MAX_LINE_CHARS) {
    scanWindow(text, 0, rules, hits);
    return hits;
  }
  const step = MAX_LINE_CHARS - WINDOW_OVERLAP_CHARS;
  for (let start = 0; start < text.length; start += step) {
    scanWindow(text.slice(start, start + MAX_LINE_CHARS), start, rules, hits);
    if (start + MAX_LINE_CHARS >= text.length) break;
  }
  return hits;
}

type LineText = { text: string; words: OcrWord[]; starts: number[] };

function buildLines(words: OcrWord[]): LineText[] {
  const byLine = new Map<number, OcrWord[]>();
  for (const word of words) {
    if (typeof word.text !== "string" || word.text.length === 0) continue;
    if (![word.x, word.y, word.w, word.h, word.line].every(Number.isFinite) || word.w <= 0 || word.h <= 0) continue;
    const list = byLine.get(word.line);
    if (list) list.push(word);
    else byLine.set(word.line, [word]);
  }
  return [...byLine.entries()]
    .sort((a, b) => a[0] - b[0])
    .map(([, lineWords]) => {
      const starts: number[] = [];
      let text = "";
      for (const word of lineWords) {
        if (text) text += " ";
        starts.push(text.length);
        text += word.text;
      }
      return { text, words: lineWords, starts };
    });
}

type Box = { left: number; top: number; right: number; bottom: number };

function union(words: OcrWord[]): Box {
  return {
    left: Math.min(...words.map((word) => word.x)),
    top: Math.min(...words.map((word) => word.y)),
    right: Math.max(...words.map((word) => word.x + word.w)),
    bottom: Math.max(...words.map((word) => word.y + word.h)),
  };
}

function pad(box: Box, image: { width: number; height: number }): Rect | undefined {
  const margin = Math.max(3, Math.round((box.bottom - box.top) * 0.35));
  const left = Math.max(0, Math.floor(box.left - margin));
  const top = Math.max(0, Math.floor(box.top - margin));
  const right = Math.min(image.width, Math.ceil(box.right + margin));
  const bottom = Math.min(image.height, Math.ceil(box.bottom + margin));
  if (right <= left || bottom <= top) return undefined;
  return { x: left, y: top, w: right - left, h: bottom - top };
}

const overlaps = (a: Rect, b: Rect) => a.x < b.x + b.w && b.x < a.x + a.w && a.y < b.y + b.h && b.y < a.y + a.h;

function mergeOverlapping(regions: SensitiveRegion[]): SensitiveRegion[] {
  const merged = [...regions];
  let changed = true;
  while (changed) {
    changed = false;
    outer: for (let i = 0; i < merged.length; i += 1) {
      for (let j = i + 1; j < merged.length; j += 1) {
        if (!overlaps(merged[i], merged[j])) continue;
        const left = Math.min(merged[i].x, merged[j].x);
        const top = Math.min(merged[i].y, merged[j].y);
        const right = Math.max(merged[i].x + merged[i].w, merged[j].x + merged[j].w);
        const bottom = Math.max(merged[i].y + merged[i].h, merged[j].y + merged[j].h);
        const rules = [...new Set([...merged[i].rules, ...merged[j].rules])].sort();
        merged.splice(j, 1);
        merged[i] = { x: left, y: top, w: right - left, h: bottom - top, rules };
        changed = true;
        break outer;
      }
    }
  }
  return merged;
}

/**
 * Finds the screen areas that hold text matching a guard rule. Each OCR line is scanned, and so is every pair of
 * neighbouring lines (joined by a space) so a value wrapped across two lines is still found. A match covers every
 * word it touches; the result is one padded rectangle per OCR line involved, clamped to the image, with
 * overlapping rectangles merged.
 */
export function findSensitiveRegions(words: OcrWord[], image: { width: number; height: number }, rules: CompiledGuardRule[]): SensitiveRegion[] {
  const lines = buildLines(words);
  const found: SensitiveRegion[] = [];
  const cover = (candidates: Array<{ line: LineText; offset: number }>, text: string) => {
    for (const hit of scanText(text, rules)) {
      for (const { line, offset } of candidates) {
        const covered = line.words.filter((_, index) => {
          const start = offset + line.starts[index];
          return start < hit.end && start + line.words[index].text.length > hit.start;
        });
        if (covered.length === 0) continue;
        const rect = pad(union(covered), image);
        if (rect) found.push({ ...rect, rules: [hit.rule] });
      }
    }
  };
  for (let index = 0; index < lines.length; index += 1) {
    cover([{ line: lines[index], offset: 0 }], lines[index].text);
    const next = lines[index + 1];
    if (next) {
      cover([{ line: lines[index], offset: 0 }, { line: next, offset: lines[index].text.length + 1 }], `${lines[index].text} ${next.text}`);
    }
  }
  return mergeOverlapping(found);
}

/**
 * Pixelates each rectangle in a raw 4-byte-per-pixel bitmap and returns a copy. A block is at least as tall as the
 * rectangle (and never under 8 pixels) and at least as wide, so no glyph survives; every block becomes one flat
 * colour. Pixels outside the rectangles are copied untouched. Rectangles are clamped to the image.
 */
export function redactBitmap(bitmap: Uint8Array, width: number, height: number, rects: Rect[]): Buffer {
  if (!Number.isInteger(width) || !Number.isInteger(height) || width < 1 || height < 1 || bitmap.length !== width * height * 4) {
    throw new Error("The bitmap does not match its size.");
  }
  const out = Buffer.from(bitmap);
  for (const rect of rects) {
    if (![rect.x, rect.y, rect.w, rect.h].every(Number.isFinite)) throw new Error("A redaction rectangle is invalid.");
    const left = Math.max(0, Math.floor(rect.x));
    const top = Math.max(0, Math.floor(rect.y));
    const right = Math.min(width, Math.ceil(rect.x + rect.w));
    const bottom = Math.min(height, Math.ceil(rect.y + rect.h));
    if (right <= left || bottom <= top) continue;
    const rectWidth = right - left;
    const rectHeight = bottom - top;
    const block = Math.max(rectHeight, MIN_BLOCK_PIXELS);
    // Equal-sized blocks that are never smaller than `block`, so no thin sliver keeps a trace of a glyph edge.
    const columns = Math.max(1, Math.floor(rectWidth / block));
    const rows = Math.max(1, Math.floor(rectHeight / block));
    for (let row = 0; row < rows; row += 1) {
      const y0 = top + Math.floor((row * rectHeight) / rows);
      const y1 = top + Math.floor(((row + 1) * rectHeight) / rows);
      for (let column = 0; column < columns; column += 1) {
        const x0 = left + Math.floor((column * rectWidth) / columns);
        const x1 = left + Math.floor(((column + 1) * rectWidth) / columns);
        const sums = [0, 0, 0, 0];
        for (let y = y0; y < y1; y += 1) {
          for (let x = x0; x < x1; x += 1) {
            const at = (y * width + x) * 4;
            for (let channel = 0; channel < 4; channel += 1) sums[channel] += bitmap[at + channel];
          }
        }
        const count = (y1 - y0) * (x1 - x0);
        const fill = sums.map((sum) => Math.round(sum / count));
        for (let y = y0; y < y1; y += 1) {
          for (let x = x0; x < x1; x += 1) {
            const at = (y * width + x) * 4;
            for (let channel = 0; channel < 4; channel += 1) out[at + channel] = fill[channel];
          }
        }
      }
    }
  }
  return out;
}
