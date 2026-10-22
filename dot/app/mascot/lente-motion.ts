// The motion of the Lente character as plain numbers, so it can be tested without a browser. One engine per drawn
// character: springs for everything that follows or reacts (gaze, hover, click, state), a blink scheduler with natural
// random intervals, idle glances, breathing and the attention bob. The component only reads the frame it returns and
// writes it to the drawing; nothing here touches the DOM.
//
// Reduced motion is a second profile, never "off": the eye still follows, blinks and breathes, with a smaller reach,
// critically damped springs (no bounce) and no bob.

import { LENTE_PALETTES, type LentePalette, type LenteState } from "./lente-look.ts";

export type Vec = { x: number; y: number };
export type SpringState = { x: number; v: number };
export type SpringConfig = { stiffness: number; damping: number };

/** How far the iris can travel inside its dark socket (SVG units of the 100 x 100 drawing). */
export const IRIS_TRAVEL = 4.2;
/** How far the whole eye follows the gaze, as a fraction of the iris travel: a little parallax gives the sphere depth. */
const PARALLAX = 0.3;
/** A cursor farther than this from every screen is not a position. */
const CURSOR_LIMIT = 100_000;
/** How far away the cursor has to be for the eye to look about three quarters of the way toward it (px). */
const GAZE_REACH = 240;

const clamp = (value: number, min: number, max: number) => Math.min(max, Math.max(min, value));

// --- springs ---------------------------------------------------------------------------------------------------

/** Longest slice the integrator takes in one go; a long frame is split so the spring stays stable. */
const MAX_SLICE_MS = 8;
const REST_DISTANCE = 0.001;
const REST_SPEED = 0.01;

/** Semi-implicit Euler, split into short slices. Settles exactly on the target so values do not drift forever. */
export function springStep(state: SpringState, target: number, config: SpringConfig, dtMs: number): SpringState {
  let { x, v } = state;
  let remaining = Math.max(0, dtMs);
  while (remaining > 0) {
    const slice = Math.min(MAX_SLICE_MS, remaining) / 1000;
    remaining -= MAX_SLICE_MS;
    v += (-config.stiffness * (x - target) - config.damping * v) * slice;
    x += v * slice;
  }
  if (Math.abs(x - target) < REST_DISTANCE && Math.abs(v) < REST_SPEED) return { x: target, v: 0 };
  return { x, v };
}

// --- the profile -----------------------------------------------------------------------------------------------

export type MotionProfile = {
  reduced: boolean;
  /** Fraction of the full iris travel the eye uses when it follows or glances. */
  gaze: number;
  /** Multiplier on the breathing amplitude. */
  breathe: number;
  /** Multiplier on the attention bob. */
  bob: number;
  /** Whether springs overshoot. */
  bounce: boolean;
  /** Degrees per second of the working orbit. */
  orbitSpeed: number;
  /** How much a click squashes the character. */
  press: number;
  /** How much a hover widens the eye. */
  widen: number;
};

export function motionProfile(reduced: boolean): MotionProfile {
  return reduced
    ? { reduced: true, gaze: 0.55, breathe: 0.35, bob: 0, bounce: false, orbitSpeed: 30, press: 0.04, widen: 0.05 }
    : { reduced: false, gaze: 1, breathe: 1, bob: 1, bounce: true, orbitSpeed: 120, press: 0.14, widen: 0.1 };
}

export type SpringKind = "gaze" | "shape" | "state" | "color";

/** Under-damped for the things that should feel alive, critically damped when reduced and for colors (which never overshoot). */
const BOUNCY: Record<SpringKind, SpringConfig> = {
  gaze: { stiffness: 220, damping: 21 },
  shape: { stiffness: 300, damping: 14 },
  state: { stiffness: 140, damping: 17 },
  color: { stiffness: 120, damping: 2 * Math.sqrt(120) },
};

export function springConfig(profile: MotionProfile, kind: SpringKind): SpringConfig {
  const base = BOUNCY[kind];
  return profile.bounce ? base : { stiffness: base.stiffness, damping: 2 * Math.sqrt(base.stiffness) };
}

// --- gaze, blinking and glancing -------------------------------------------------------------------------------

/** A cursor position from the desktop shell: two finite numbers, anything else is ignored. */
export function readCursor(value: unknown): Vec | undefined {
  if (!value || typeof value !== "object") return undefined;
  const { x, y } = value as Record<string, unknown>;
  if (typeof x !== "number" || typeof y !== "number" || !Number.isFinite(x) || !Number.isFinite(y)) return undefined;
  if (Math.abs(x) > CURSOR_LIMIT || Math.abs(y) > CURSOR_LIMIT) return undefined;
  return { x, y };
}

/** From the eye toward the cursor: the direction, with a length that grows with the distance and never reaches 1. */
export function gazeVector(cursor: Vec | null, eye: Vec, reach = GAZE_REACH): Vec {
  if (!cursor) return { x: 0, y: 0 };
  const dx = cursor.x - eye.x;
  const dy = cursor.y - eye.y;
  const distance = Math.hypot(dx, dy);
  if (distance < 2) return { x: 0, y: 0 };
  const strength = 1 - Math.exp(-distance / reach);
  return { x: (dx / distance) * strength, y: (dy / distance) * strength };
}

export type BlinkPlan = { delay: number; double: boolean };
const BLINK_MIN_MS = 2200;
const BLINK_SPREAD_MS = 4300;
const DOUBLE_BLINK_CHANCE = 0.2;
/** A whole blink: it closes fast and opens a little slower. */
export const BLINK_MS = 180;
export const BLINK_CLOSE_SHARE = 0.38;
/** The pause between the two blinks of a double blink. */
const DOUBLE_GAP_MS = 90;

/** The wait before a blink (natural, random) and whether it is a double one. */
export function nextBlink(random: () => number): BlinkPlan {
  return { delay: BLINK_MIN_MS + random() * BLINK_SPREAD_MS, double: random() < DOUBLE_BLINK_CHANCE };
}

/** How closed the eye is, 0 to 1, this long after a blink started. */
export function blinkClosure(elapsedMs: number): number {
  if (elapsedMs <= 0 || elapsedMs >= BLINK_MS) return 0;
  const peak = BLINK_MS * BLINK_CLOSE_SHARE;
  return elapsedMs <= peak ? elapsedMs / peak : 1 - (elapsedMs - peak) / (BLINK_MS - peak);
}

export type GlancePlan = { delay: number; hold: number; x: number; y: number };

/** A short look away: a pause, how long to hold it and where, as a fraction of the iris travel. */
export function nextGlance(random: () => number): GlancePlan {
  return { delay: 1600 + random() * 3400, hold: 450 + random() * 750, x: (random() * 2 - 1) * 0.85, y: (random() * 2 - 1) * 0.55 };
}

/** The cursor has to be still this long before the eye starts to look around by itself. */
const IDLE_BEFORE_GLANCE_MS = 2500;

// --- the state -------------------------------------------------------------------------------------------------

export type StateTargets = { orbit: number; badge: number; lid: number; bob: number; dilate: number };

/** What each Dot state asks of the drawing, matching `lenteLook`; the iris also dilates while Dot listens. */
export function stateTargets(state: LenteState, listening: boolean): StateTargets {
  return {
    orbit: state === "working" ? 1 : 0,
    badge: state === "approval" ? 1 : 0,
    lid: state === "done" ? 0.5 : 0,
    bob: state === "approval" ? 1 : 0,
    dilate: listening ? 1 : state === "working" ? 0.3 : 0,
  };
}

// --- colors ----------------------------------------------------------------------------------------------------

type Rgba = [number, number, number, number];

function parseColor(color: string): Rgba | undefined {
  const hex = /^#([0-9a-f]{6})$/i.exec(color.trim());
  if (hex) {
    const value = parseInt(hex[1], 16);
    return [(value >> 16) & 255, (value >> 8) & 255, value & 255, 1];
  }
  const rgba = /^rgba?\(\s*(\d+(?:\.\d+)?)\s*,\s*(\d+(?:\.\d+)?)\s*,\s*(\d+(?:\.\d+)?)\s*(?:,\s*(\d*\.?\d+)\s*)?\)$/i.exec(color.trim());
  if (rgba) return [Number(rgba[1]), Number(rgba[2]), Number(rgba[3]), rgba[4] === undefined ? 1 : Number(rgba[4])];
  return undefined;
}

const hexByte = (value: number) => Math.round(clamp(value, 0, 255)).toString(16).padStart(2, "0");

/** Blends two colors (hex or rgba) channel by channel. A value that is not a color yields the other end. */
export function mixColor(from: string, to: string, t: number): string {
  const a = parseColor(from);
  const b = parseColor(to);
  if (!a || !b) return b ? to : from;
  const k = clamp(t, 0, 1);
  const channel = (i: number) => a[i] + (b[i] - a[i]) * k;
  if (a[3] === 1 && b[3] === 1 && /^#/.test(from.trim()) && /^#/.test(to.trim())) return `#${hexByte(channel(0))}${hexByte(channel(1))}${hexByte(channel(2))}`;
  if (k === 1) return to;
  if (k === 0) return from;
  const alpha = Math.round(channel(3) * 1000) / 1000;
  return `rgba(${Math.round(channel(0))}, ${Math.round(channel(1))}, ${Math.round(channel(2))}, ${alpha})`;
}

export function mixPalette(from: LentePalette, to: LentePalette, t: number): LentePalette {
  return { hi: mixColor(from.hi, to.hi, t), mid: mixColor(from.mid, to.mid, t), lo: mixColor(from.lo, to.lo, t), glow: mixColor(from.glow, to.glow, t), badge: mixColor(from.badge, to.badge, t) };
}

// --- the engine ------------------------------------------------------------------------------------------------

export type Frame = {
  /** Where the iris is inside the socket, in SVG units. */
  gazeX: number;
  gazeY: number;
  /** How far the whole eye leans toward the gaze, in SVG units. */
  shiftX: number;
  shiftY: number;
  /** The blink alone, 0 open to 1 closed. */
  blink: number;
  /** The state's lid (0, or 0.5 when done). */
  lid: number;
  /** Blink and lid together: how closed the eye is. */
  closure: number;
  /** Scale of the eye: hover widens it. */
  eyeScale: number;
  /** Scale of the pupil: it dilates while Dot listens. */
  pupil: number;
  orbitAngle: number;
  orbitOpacity: number;
  /** 0 to 1, a little more while the badge pops in. */
  badge: number;
  /** Squash and stretch of the whole character. */
  squashX: number;
  squashY: number;
  /** Attention bob as a fraction of the character's size, zero or negative (up). */
  bobY: number;
  breathe: number;
};

export type LenteEngine = {
  step(dtMs: number): Frame;
  frame(): Frame;
  setState(state: LenteState, listening?: boolean): void;
  setReduced(reduced: boolean): void;
  /** The gaze as a vector of length 0 to 1 (see `gazeVector`), or null when there is no cursor to follow. */
  setGaze(gaze: Vec | null): void;
  setHover(hover: boolean): void;
  press(): void;
  release(): void;
  palette(): LentePalette;
};

type EngineOptions = { random?: () => number; reduced?: boolean; state?: LenteState; listening?: boolean };

const at = (x: number): SpringState => ({ x, v: 0 });

export function createLenteEngine({ random = Math.random, reduced = false, state = "idle", listening = false }: EngineOptions = {}): LenteEngine {
  let profile = motionProfile(reduced);
  let current = state;
  let isListening = listening;
  let targets = stateTargets(current, isListening);
  let time = 0;

  let gazeTarget: Vec = { x: 0, y: 0 };
  let idleMs = 0;
  let gazeX = at(0);
  let gazeY = at(0);

  let blinkPlan = nextBlink(random);
  let nextBlinkAt = blinkPlan.delay;
  let blinkStarts: number[] = [];

  let glancePlan: GlancePlan | null = null;
  let nextGlanceAt: number | null = null;
  let glance: { x: number; y: number; until: number } | null = null;

  let hovering = false;
  let pressed = false;
  let hover = at(0);
  let press = at(0);
  let orbit = at(targets.orbit);
  let badge = at(targets.badge);
  let lid = at(targets.lid);
  let bob = at(targets.bob);
  let dilate = at(targets.dilate);
  let mix = at(1);
  let orbitAngle = 0;
  let fromPalette: LentePalette = LENTE_PALETTES[current];
  let toPalette: LentePalette = LENTE_PALETTES[current];

  const cfg = (kind: SpringKind) => springConfig(profile, kind);
  const palette = () => mixPalette(fromPalette, toPalette, clamp(mix.x, 0, 1));
  let last: Frame = compute();

  function compute(): Frame {
    const travel = IRIS_TRAVEL;
    let gx = gazeX.x;
    let gy = gazeY.x;
    const reach = Math.hypot(gx, gy);
    if (reach > travel) {
      gx *= travel / reach;
      gy *= travel / reach;
    }
    const blink = Math.max(0, ...blinkStarts.map((start) => blinkClosure(time - start)));
    const lidValue = clamp(lid.x, 0, 1);
    const sinceBob = (time % 1300) / 1300;
    return {
      gazeX: gx,
      gazeY: gy,
      shiftX: gx * PARALLAX,
      shiftY: gy * PARALLAX,
      blink,
      lid: lidValue,
      closure: lidValue + (1 - lidValue) * blink,
      eyeScale: 1 + profile.widen * hover.x,
      pupil: 1 + 0.35 * dilate.x + 0.012 * Math.sin(time / 650),
      orbitAngle,
      orbitOpacity: clamp(orbit.x, 0, 1),
      badge: badge.x,
      squashX: 1 + profile.press * 0.55 * press.x,
      squashY: 1 - profile.press * press.x,
      bobY: profile.bob > 0 ? -clamp(bob.x, 0, 1.2) * 0.03 * profile.bob * (0.5 - 0.5 * Math.cos(2 * Math.PI * sinceBob)) : 0,
      breathe: 1 + 0.028 * profile.breathe * Math.sin((2 * Math.PI * time) / 3800),
    };
  }

  function currentGaze(): Vec {
    if (idleMs < IDLE_BEFORE_GLANCE_MS) {
      glance = null;
      nextGlanceAt = null;
      return gazeTarget;
    }
    if (glance && time < glance.until) return glance;
    glance = null;
    if (nextGlanceAt === null) {
      glancePlan = nextGlance(random);
      nextGlanceAt = time + glancePlan.delay;
    }
    if (glancePlan && time >= nextGlanceAt) {
      glance = { x: glancePlan.x, y: glancePlan.y, until: time + glancePlan.hold };
      glancePlan = nextGlance(random);
      nextGlanceAt = glance.until + glancePlan.delay;
      return glance;
    }
    return gazeTarget;
  }

  return {
    step(dtMs) {
      const dt = clamp(dtMs, 0, 1000);
      time += dt;
      idleMs += dt;
      const aim = currentGaze();
      gazeX = springStep(gazeX, aim.x * IRIS_TRAVEL * profile.gaze, cfg("gaze"), dt);
      gazeY = springStep(gazeY, aim.y * IRIS_TRAVEL * profile.gaze, cfg("gaze"), dt);
      if (time >= nextBlinkAt) {
        blinkStarts = blinkPlan.double ? [time, time + BLINK_MS + DOUBLE_GAP_MS] : [time];
        const span = blinkPlan.double ? 2 * BLINK_MS + DOUBLE_GAP_MS : BLINK_MS;
        blinkPlan = nextBlink(random);
        nextBlinkAt = time + span + blinkPlan.delay;
      }
      blinkStarts = blinkStarts.filter((start) => time - start < BLINK_MS);
      hover = springStep(hover, hovering ? 1 : 0, cfg("shape"), dt);
      press = springStep(press, pressed ? 1 : 0, cfg("shape"), dt);
      orbit = springStep(orbit, targets.orbit, cfg("state"), dt);
      badge = springStep(badge, targets.badge, cfg("state"), dt);
      lid = springStep(lid, targets.lid, cfg("state"), dt);
      bob = springStep(bob, targets.bob, cfg("state"), dt);
      dilate = springStep(dilate, targets.dilate, cfg("state"), dt);
      mix = springStep(mix, 1, cfg("color"), dt);
      orbitAngle = (orbitAngle + profile.orbitSpeed * clamp(orbit.x, 0, 1) * (dt / 1000)) % 360;
      last = compute();
      return last;
    },
    frame: () => last,
    setState(next, nextListening = false) {
      const changed = next !== current;
      current = next;
      isListening = nextListening;
      targets = stateTargets(current, isListening);
      if (!changed) return;
      fromPalette = palette();
      toPalette = LENTE_PALETTES[current];
      mix = at(0);
    },
    setReduced(next) {
      profile = motionProfile(next);
    },
    setGaze(gaze) {
      const next = gaze ?? { x: 0, y: 0 };
      if (Math.hypot(next.x - gazeTarget.x, next.y - gazeTarget.y) > 0.01) idleMs = 0;
      gazeTarget = next;
    },
    setHover(next) {
      hovering = next;
    },
    press() {
      pressed = true;
    },
    release() {
      pressed = false;
    },
    palette,
  };
}
