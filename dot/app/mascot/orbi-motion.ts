// The motion of the Orbi character as plain numbers, so it can be tested without a browser. One engine per drawn
// character: springs for everything that follows or reacts (gaze, hover, state), a blink scheduler with natural random
// intervals, idle glances, the moon's orbit and its peeks, the ring's wobble, the boing of a click, the hop of "needs you",
// and the emotes (sparks, z, rings, sweat) as numbers per frame. The component only reads the frame it returns and writes
// it to the drawing; nothing here touches the DOM.
//
// Reduced motion is a second profile, never "off": Orbi still follows, blinks, breathes, floats a little and the moon keeps
// turning, with a smaller reach, critically damped springs (no bounce), a soft press instead of a boing and no hop or shiver.

import { ORBI_PALETTES, type OrbiPalette, type OrbiState } from "./orbi-look.ts";

export type Vec = { x: number; y: number };
export type SpringState = { x: number; v: number };
export type SpringConfig = { stiffness: number; damping: number };

/** How far a pupil can travel inside its eye (SVG units of the 120 x 120 drawing). */
export const PUPIL_TRAVEL = 3;
/** How far the whole face leans toward the gaze, as a fraction of the pupil travel: a little parallax gives the planet depth. */
const PARALLAX = 0.85;
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
  /** Fraction of the full pupil travel the eyes use when they follow or glance. */
  gaze: number;
  /** Multiplier on the breathing amplitude. */
  breathe: number;
  /** Multiplier on the floating. */
  float: number;
  /** Multiplier on the hop of "needs you"; zero when reduced. */
  hop: number;
  /** Whether springs overshoot. */
  bounce: boolean;
  /** Multiplier on the moon's pace. */
  moon: number;
  /** Multiplier on the boing of a click or a wake-up. */
  boing: number;
  /** How much a hover widens the eyes. */
  widen: number;
  /** Multiplier on the small flourishes: the ring's wobble and the moon's zip. */
  flourish: number;
};

export function motionProfile(reduced: boolean): MotionProfile {
  return reduced
    ? { reduced: true, gaze: 0.55, breathe: 0.35, float: 0.35, hop: 0, bounce: false, moon: 0.45, boing: 0.25, widen: 0.06, flourish: 0.4 }
    : { reduced: false, gaze: 1, breathe: 1, float: 1, hop: 1, bounce: true, moon: 1, boing: 1, widen: 0.14, flourish: 1 };
}

export type SpringKind = "gaze" | "shape" | "state" | "color" | "ring" | "zip";

/** Under-damped for the things that should feel alive, critically damped when reduced and for colors (which never overshoot). */
const BOUNCY: Record<SpringKind, SpringConfig> = {
  gaze: { stiffness: 220, damping: 21 },
  shape: { stiffness: 300, damping: 14 },
  state: { stiffness: 140, damping: 17 },
  color: { stiffness: 120, damping: 2 * Math.sqrt(120) },
  ring: { stiffness: 70, damping: 4.5 },
  zip: { stiffness: 180, damping: 18 },
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

/** A short look away: a pause, how long to hold it and where, as a fraction of the pupil travel. */
export function nextGlance(random: () => number): GlancePlan {
  return { delay: 1600 + random() * 3400, hold: 450 + random() * 750, x: (random() * 2 - 1) * 0.85, y: (random() * 2 - 1) * 0.55 };
}

/** The cursor has to be still this long before the eye starts to look around by itself. */
const IDLE_BEFORE_GLANCE_MS = 2500;

export type RingSpinPlan = { delay: number; amount: number };
/** The ring's little wobble: a long pause, then a kick of some degrees either way. */
export function nextRingSpin(random: () => number): RingSpinPlan {
  const delay = 7000 + random() * 9000;
  const sign = random() < 0.5 ? -1 : 1;
  return { delay, amount: sign * (18 + random() * 10) };
}

export type PeekPlan = { delay: number; hold: number };
/** The moon ducking behind the planet: a long pause, then how long it stays hidden. */
export function nextPeek(random: () => number): PeekPlan {
  return { delay: 9000 + random() * 11000, hold: 1400 + random() * 800 };
}

// --- keyframes -------------------------------------------------------------------------------------------------

export type Keyframe = { t: number; y: number; sx: number; sy: number; ease: "in" | "out" | "smooth" };
export type Pose = { y: number; sx: number; sy: number };

const ease = (kind: Keyframe["ease"], u: number) => (kind === "out" ? 1 - (1 - u) * (1 - u) : kind === "in" ? u * u : u * u * (3 - 2 * u));

/** The pose at `t` ms between keyframes; each segment eases the way its first keyframe says. After the last keyframe it holds. */
export function poseAt(frames: readonly Keyframe[], t: number): Pose {
  const last = frames[frames.length - 1];
  if (t >= last.t) return { y: last.y, sx: last.sx, sy: last.sy };
  if (t <= frames[0].t) return { y: frames[0].y, sx: frames[0].sx, sy: frames[0].sy };
  let index = 0;
  while (frames[index + 1].t <= t) index += 1;
  const a = frames[index];
  const b = frames[index + 1];
  const k = ease(a.ease, (t - a.t) / (b.t - a.t));
  return { y: a.y + (b.y - a.y) * k, sx: a.sx + (b.sx - a.sx) * k, sy: a.sy + (b.sy - a.sy) * k };
}

/** The click: squashed wide and short, stretched tall, a small settle. The prototype's boing, in milliseconds. */
const BOING: readonly Keyframe[] = [
  { t: 0, y: 0, sx: 1, sy: 1, ease: "out" },
  { t: 137, y: 0, sx: 1.18, sy: 0.78, ease: "smooth" },
  { t: 302, y: 0, sx: 0.9, sy: 1.14, ease: "smooth" },
  { t: 440, y: 0, sx: 1.04, sy: 0.97, ease: "smooth" },
  { t: 550, y: 0, sx: 1, sy: 1, ease: "smooth" },
];
/** Reduced motion: a soft press that comes straight back, with no stretch. */
const SOFT_PRESS: readonly Keyframe[] = [
  { t: 0, y: 0, sx: 1, sy: 1, ease: "out" },
  { t: 150, y: 0, sx: 1.04, sy: 0.94, ease: "smooth" },
  { t: 420, y: 0, sx: 1, sy: 1, ease: "smooth" },
];
export const BOING_MS = 550;

/** The scale of a boing `elapsedMs` after it started, with the profile's amplitude and a per-use `amount` (1 for a click). */
export function boingScale(profile: MotionProfile, elapsedMs: number, amount = 1): { x: number; y: number } {
  const pose = poseAt(profile.bounce ? BOING : SOFT_PRESS, elapsedMs);
  const k = profile.bounce ? profile.boing * amount : 1;
  return { x: 1 + (pose.sx - 1) * k, y: 1 + (pose.sy - 1) * k };
}

/** One hop of "needs you", as a fraction of the character's height (negative is up), with the squash of the takeoff and the landing. */
const HOP: readonly Keyframe[] = [
  { t: 0, y: 0, sx: 1, sy: 1, ease: "out" },
  { t: 300, y: -1, sx: 0.96, sy: 1.05, ease: "in" },
  { t: 550, y: 0, sx: 1.08, sy: 0.92, ease: "out" },
  { t: 700, y: -0.21, sx: 1, sy: 1, ease: "in" },
  { t: 1000, y: 0, sx: 1, sy: 1, ease: "smooth" },
  { t: 1400, y: 0, sx: 1, sy: 1, ease: "smooth" },
];
const HOP_PERIOD_MS = 1400;
const HOP_HEIGHT = 0.07;

// --- the state -------------------------------------------------------------------------------------------------

export type StateTargets = {
  /** Degrees per second of the moon's orbit. */
  moon: number;
  /** 1 while the ring glows in a pulse. */
  ringGlow: number;
  badge: number;
  hop: number;
  happy: number;
  closed: number;
  sparks: number;
  zzz: number;
  rings: number;
  sweat: number;
  worry: number;
  /** Brightness of the glow behind the planet. */
  glow: number;
};

const NONE = { ringGlow: 0, badge: 0, hop: 0, happy: 0, closed: 0, sparks: 0, zzz: 0, rings: 0, sweat: 0, worry: 0 };

/** What each state asks of the drawing, matching `orbiLook`. */
export function stateTargets(state: OrbiState): StateTargets {
  switch (state) {
    case "working":
      return { ...NONE, moon: 260, glow: 1.15, ringGlow: 1 };
    case "needs":
      return { ...NONE, moon: 80, glow: 1.1, badge: 1, hop: 1 };
    case "done":
      return { ...NONE, moon: 90, glow: 1.1, happy: 1, sparks: 1 };
    case "error":
      return { ...NONE, moon: 28, glow: 0.8, sweat: 1, worry: 1 };
    case "listening":
      return { ...NONE, moon: 72, glow: 1.15, rings: 1 };
    case "sleeping":
      return { ...NONE, moon: 18, glow: 0.4, closed: 1, zzz: 1 };
    default:
      return { ...NONE, moon: 56, glow: 1 };
  }
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

export function mixPalette(from: OrbiPalette, to: OrbiPalette, t: number): OrbiPalette {
  const mixed = { ...to };
  for (const key of Object.keys(to) as (keyof OrbiPalette)[]) mixed[key] = mixColor(from[key], to[key], t);
  return mixed;
}

// --- the engine ------------------------------------------------------------------------------------------------

export type Emote = { scale: number; opacity: number };

export type Frame = {
  /** Where the pupils are inside the eyes, in SVG units. */
  gazeX: number;
  gazeY: number;
  /** How far the whole face leans toward the gaze, in SVG units. */
  faceX: number;
  faceY: number;
  /** The blink alone, 0 open to 1 closed. */
  blink: number;
  /** How open the eyes are, 1 awake to 0 for the happy or the closed arcs. */
  openness: number;
  happy: number;
  closed: number;
  /** A worried mouth, 0 to 1. */
  worry: number;
  /** Scale of the eyes: a hover widens them. */
  eyeScale: number;
  /** The moon: its angle in degrees, how far it is from the planet's plane (+1 in front, -1 behind), its size and how far out it goes. */
  moonAngle: number;
  moonDepth: number;
  moonScale: number;
  moonReach: number;
  /** The moon's pace right now, degrees per second. */
  moonSpeed: number;
  /** The ring's wobble on top of its tilt, in degrees, and how brightly it pulses (0 to 1). */
  ringTilt: number;
  ringPulse: number;
  /** 0 to 1, a little more while the badge pops in. */
  badge: number;
  /** Squash and stretch of the whole character: the boing, the hop and a hover's lean. */
  squashX: number;
  squashY: number;
  /** The hop as a fraction of the character's size (up is negative); the float; the nervous shiver in SVG units. */
  hopY: number;
  floatY: number;
  shiver: number;
  breathe: number;
  /** Brightness of the glow behind the planet. */
  glow: number;
  /** The emotes, one entry per particle. */
  sparks: Emote[];
  zzz: { x: number; y: number; scale: number; opacity: number }[];
  rings: Emote[];
  sweat: { y: number; opacity: number };
};

export type OrbiEngine = {
  step(dtMs: number): Frame;
  frame(): Frame;
  setState(state: OrbiState): void;
  setReduced(reduced: boolean): void;
  /** The gaze as a vector of length 0 to 1 (see `gazeVector`), or null when there is no cursor to follow. */
  setGaze(gaze: Vec | null): void;
  setHover(hover: boolean): void;
  /** A click: the boing, the moon zipping around and a kick to the ring. */
  press(): void;
  palette(): OrbiPalette;
};

type EngineOptions = { random?: () => number; reduced?: boolean; state?: OrbiState };

const at = (x: number): SpringState => ({ x, v: 0 });
const SPARKS = 4;
const ZIP_MS = 700;
const ZIP_BOOST = 800;
/** A wake-up stretch and a done's little jump are boings with less amplitude. */
const WAKE_BOING = 0.6;
const DONE_BOING = 0.5;
const cycle = (time: number, period: number, offset = 0) => (((time / period + offset) % 1) + 1) % 1;

export function createOrbiEngine({ random = Math.random, reduced = false, state = "idle" }: EngineOptions = {}): OrbiEngine {
  let profile = motionProfile(reduced);
  let current = state;
  let targets = stateTargets(current);
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

  let ringPlan = nextRingSpin(random);
  let nextRingAt = ringPlan.delay;
  let peekPlan = nextPeek(random);
  let nextPeekAt = peekPlan.delay;
  let peekUntil = -1;

  let hovering = false;
  let hover = at(0);
  let boingT: number | null = null;
  let boingAmount = 1;
  let zipUntil = -1;
  let zip = at(0);
  let ringKick = at(0);
  let peek = at(0);
  let moon = at(targets.moon);
  let ringGlow = at(targets.ringGlow);
  let badge = at(targets.badge);
  let hop = at(targets.hop);
  let happy = at(targets.happy);
  let closed = at(targets.closed);
  let sparks = at(targets.sparks);
  let zzz = at(targets.zzz);
  let rings = at(targets.rings);
  let sweat = at(targets.sweat);
  let worry = at(targets.worry);
  let glow = at(targets.glow);
  let mix = at(1);
  let moonAngle = 0;
  let fromPalette: OrbiPalette = ORBI_PALETTES[current];
  let toPalette: OrbiPalette = ORBI_PALETTES[current];

  const cfg = (kind: SpringKind) => springConfig(profile, kind);
  const palette = () => mixPalette(fromPalette, toPalette, clamp(mix.x, 0, 1));
  let last: Frame;

  const depth = () => Math.sin((moonAngle * Math.PI) / 180);
  /** Degrees per second: the state's pace, slower while the moon peeks, plus the zip of a click. */
  const moonSpeed = () => Math.max(0, moon.x) * profile.moon * (1 - 0.55 * clamp(peek.x, 0, 1)) + ZIP_BOOST * profile.flourish * Math.max(0, zip.x);

  function compute(): Frame {
    let gx = gazeX.x;
    let gy = gazeY.x;
    const reach = Math.hypot(gx, gy);
    if (reach > PUPIL_TRAVEL) {
      gx *= PUPIL_TRAVEL / reach;
      gy *= PUPIL_TRAVEL / reach;
    }
    const blink = Math.max(0, ...blinkStarts.map((start) => blinkClosure(time - start)));
    const happyValue = clamp(happy.x, 0, 1);
    const closedValue = clamp(closed.x, 0, 1);
    const d = depth();
    const backness = Math.max(0, -d);
    const boing = boingT === null ? { x: 1, y: 1 } : boingScale(profile, boingT, boingAmount);
    const hopAmount = clamp(hop.x, 0, 1.2) * profile.hop;
    const hopPose = hopAmount > 0 ? poseAt(HOP, time % HOP_PERIOD_MS) : { y: 0, sx: 1, sy: 1 };
    const lean = 1 + 0.04 * hover.x;
    const sleepy = closedValue;
    const sweatPhase = cycle(time, 1800);
    const nervous = profile.bounce ? Math.sin(time / 37) * 0.45 * clamp(sweat.x, 0, 1) : 0;
    return {
      gazeX: gx,
      gazeY: gy,
      faceX: gx * PARALLAX,
      faceY: gy * PARALLAX,
      blink,
      openness: clamp(1 - Math.max(happyValue, closedValue), 0, 1),
      happy: happyValue,
      closed: closedValue,
      worry: clamp(worry.x, 0, 1),
      eyeScale: 1 + profile.widen * hover.x,
      moonAngle,
      moonDepth: d,
      moonScale: 0.8 + 0.28 * ((d + 1) / 2),
      moonReach: 1 - 0.45 * clamp(peek.x, 0, 1.1) * backness,
      moonSpeed: moonSpeed(),
      ringTilt: ringKick.x,
      ringPulse: clamp(ringGlow.x, 0, 1) * (0.5 + 0.5 * Math.sin(time / 300)),
      badge: badge.x,
      squashX: boing.x * (1 + (hopPose.sx - 1) * hopAmount) * lean,
      squashY: boing.y * (1 + (hopPose.sy - 1) * hopAmount) * lean,
      hopY: hopAmount > 0 ? hopPose.y * HOP_HEIGHT * hopAmount + 0 : 0,
      floatY: 0.017 * profile.float * (1 - 0.7 * sleepy) * Math.sin((2 * Math.PI * time) / 3800),
      shiver: nervous,
      breathe: 1 + 0.028 * profile.breathe * (1 + 0.6 * sleepy) * Math.sin((2 * Math.PI * time) / 3100),
      glow: Math.max(0, glow.x),
      sparks: Array.from({ length: SPARKS }, (_, i) => {
        const p = cycle(time, 1200, i / SPARKS);
        return { scale: 0.2 + 1.4 * p, opacity: clamp((1 - p) * sparks.x, 0, 1) };
      }),
      zzz: [0, 1].map((i) => {
        const p = cycle(time, 2400, i * 0.5);
        const fade = p < 0.3 ? p / 0.3 : 1 - (p - 0.3) / 0.7;
        return { x: 10 * p, y: -22 * p, scale: 0.7 + 0.4 * p, opacity: clamp(fade * zzz.x, 0, 1) };
      }),
      rings: [0, 1].map((i) => {
        const p = cycle(time, 1600, i * 0.31);
        return { scale: 1.05 + 0.55 * p, opacity: clamp(0.9 * (1 - p) * rings.x, 0, 1) };
      }),
      sweat: { y: 8 * sweatPhase, opacity: clamp((sweatPhase < 0.15 ? sweatPhase / 0.15 : 1 - (sweatPhase - 0.15) / 0.85) * sweat.x, 0, 1) },
    };
  }

  function currentGaze(): Vec {
    // Asleep it looks at nothing; otherwise it follows the cursor, and looks around by itself once the cursor has been still a while.
    if (current === "sleeping") return { x: 0, y: 0 };
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

  last = compute();

  function kickRing(amount: number) {
    ringKick = { x: amount * profile.flourish, v: 0 };
  }

  function startBoing(amount: number) {
    boingT = 0;
    boingAmount = amount;
  }

  return {
    step(dtMs) {
      const dt = clamp(dtMs, 0, 1000);
      time += dt;
      idleMs += dt;
      const aim = currentGaze();
      gazeX = springStep(gazeX, aim.x * PUPIL_TRAVEL * profile.gaze, cfg("gaze"), dt);
      gazeY = springStep(gazeY, aim.y * PUPIL_TRAVEL * profile.gaze, cfg("gaze"), dt);
      if (time >= nextBlinkAt) {
        blinkStarts = blinkPlan.double ? [time, time + BLINK_MS + DOUBLE_GAP_MS] : [time];
        const span = blinkPlan.double ? 2 * BLINK_MS + DOUBLE_GAP_MS : BLINK_MS;
        blinkPlan = nextBlink(random);
        nextBlinkAt = time + span + blinkPlan.delay;
      }
      blinkStarts = blinkStarts.filter((start) => time - start < BLINK_MS);

      // The ring wobbles now and then by itself, except asleep.
      if (time >= nextRingAt) {
        if (current !== "sleeping") kickRing(ringPlan.amount);
        ringPlan = nextRingSpin(random);
        nextRingAt = time + ringPlan.delay;
      }
      // The moon peeks from behind the planet: when it is due and the moon is behind, it ducks in for a moment. Only while Orbi is calm.
      const calm = current === "idle" || current === "done";
      if (calm && peekUntil < 0 && time >= nextPeekAt && depth() < -0.3) peekUntil = time + peekPlan.hold;
      if (peekUntil >= 0 && time >= peekUntil) {
        peekUntil = -1;
        peekPlan = nextPeek(random);
        nextPeekAt = time + peekPlan.delay;
      }
      peek = springStep(peek, peekUntil >= 0 && calm ? 1 : 0, cfg("state"), dt);

      if (boingT !== null) {
        boingT += dt;
        if (boingT >= BOING_MS) boingT = null;
      }
      zip = springStep(zip, time < zipUntil ? 1 : 0, cfg("zip"), dt);
      ringKick = springStep(ringKick, 0, cfg("ring"), dt);
      hover = springStep(hover, hovering ? 1 : 0, cfg("shape"), dt);
      moon = springStep(moon, targets.moon, cfg("state"), dt);
      ringGlow = springStep(ringGlow, targets.ringGlow, cfg("state"), dt);
      badge = springStep(badge, targets.badge, cfg("state"), dt);
      hop = springStep(hop, targets.hop, cfg("state"), dt);
      happy = springStep(happy, targets.happy, cfg("state"), dt);
      closed = springStep(closed, targets.closed, cfg("state"), dt);
      sparks = springStep(sparks, targets.sparks, cfg("state"), dt);
      zzz = springStep(zzz, targets.zzz, cfg("state"), dt);
      rings = springStep(rings, targets.rings, cfg("state"), dt);
      sweat = springStep(sweat, targets.sweat, cfg("state"), dt);
      worry = springStep(worry, targets.worry, cfg("state"), dt);
      glow = springStep(glow, targets.glow, cfg("state"), dt);
      mix = springStep(mix, 1, cfg("color"), dt);
      moonAngle = (moonAngle + moonSpeed() * (dt / 1000)) % 360;
      last = compute();
      return last;
    },
    frame: () => last,
    setState(next) {
      const previous = current;
      if (next === previous) return;
      current = next;
      targets = stateTargets(current);
      fromPalette = palette();
      toPalette = ORBI_PALETTES[current];
      mix = at(0);
      // Waking up stretches, and finishing is a little jump of joy.
      if (previous === "sleeping") startBoing(WAKE_BOING);
      else if (next === "done") startBoing(DONE_BOING);
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
      startBoing(1);
      zipUntil = time + ZIP_MS;
      kickRing((random() < 0.5 ? -1 : 1) * 26);
    },
    palette,
  };
}
