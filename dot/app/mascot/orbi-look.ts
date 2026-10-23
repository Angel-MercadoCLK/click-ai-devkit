// How Orbi looks in each state: the palettes, the few flags the drawing needs, the drawing's measurements (a simpler,
// bolder one below 40 px so it stays legible on the black island) and when it falls asleep. Plain data and plain
// functions so it can be tested without a browser.

export type OrbiState = "idle" | "working" | "needs" | "done" | "error" | "listening" | "sleeping";
export const ORBI_STATES: readonly OrbiState[] = ["idle", "working", "needs", "done", "error", "listening", "sleeping"];

/** Anything that is not a known state is shown as idle. */
export function readOrbiState(value: unknown): OrbiState {
  return ORBI_STATES.find((state) => state === value) ?? "idle";
}

/**
 * The colors that change with the state: the planet's four gradient stops (`b0` highlight to `b3` edge), the glow behind
 * it and the three stops of the ring's gradient. The planet keeps its colors in every state but two: a worried Orbi turns
 * muted and a sleeping one dims.
 */
export type OrbiPalette = { b0: string; b1: string; b2: string; b3: string; glow: string; r0: string; r1: string; r2: string };

const PLANET = { b0: "#B8FFF0", b1: "#3FD2C7", b2: "#4A6CFF", b3: "#7B4DFF" } as const;
const RING = { r0: "#FFB86B", r1: "#FF6FB5", r2: "#A78BFF" } as const;

export const ORBI_PALETTES: Record<OrbiState, OrbiPalette> = {
  idle: { ...PLANET, ...RING, glow: "rgba(63, 167, 255, 0.45)" },
  working: { ...PLANET, ...RING, glow: "rgba(63, 200, 255, 0.62)" },
  needs: { ...PLANET, ...RING, glow: "rgba(255, 190, 70, 0.55)" },
  done: { ...PLANET, ...RING, glow: "rgba(110, 235, 170, 0.55)" },
  error: { b0: "#F4DAD6", b1: "#CB8E98", b2: "#8C6EA0", b3: "#5F4D88", r0: "#CBA783", r1: "#BA8A9E", r2: "#9D95C8", glow: "rgba(224, 112, 132, 0.4)" },
  listening: { ...PLANET, ...RING, glow: "rgba(63, 210, 199, 0.6)" },
  sleeping: { b0: "#8FCBC3", b1: "#2F9C98", b2: "#3A50B8", b3: "#5B3CC2", r0: "#C99A66", r1: "#C45A92", r2: "#8472C8", glow: "rgba(63, 167, 255, 0.2)" },
};

/** Below this size in px the drawing is simplified: no sparks or z, a thicker ring, a bigger moon, bigger eyes. */
export const SMALL_ORBI_SIZE = 40;

export type OrbiGeometry = {
  /** The planet's center in the 120 x 120 box. */
  cx: number;
  cy: number;
  planetR: number;
  /** The tilted ring: its center, radii, stroke and tilt (degrees, negative is counter-clockwise). */
  ring: { cx: number; cy: number; rx: number; ry: number; width: number; tilt: number };
  /** The moon and the ellipse it travels on. */
  moon: { cx: number; cy: number; rx: number; ry: number; r: number; tilt: number };
  eyes: { y: number; dx: number; rx: number; ry: number; pupil: number; shine: number };
  mouth: { y: number; width: number; stroke: number };
  /** The capture halo around everything: its own signal, never part of the ring. */
  halo: { r: number; stroke: number };
  /** The "!" badge when Dot needs the user. */
  badge: { cx: number; cy: number; r: number };
};

const REGULAR: OrbiGeometry = {
  cx: 60,
  cy: 60,
  planetR: 32,
  ring: { cx: 60, cy: 71, rx: 46, ry: 13.5, width: 6, tilt: -14 },
  moon: { cx: 60, cy: 63, rx: 46, ry: 19, r: 5, tilt: -14 },
  eyes: { y: 58, dx: 10, rx: 7.2, ry: 8.8, pupil: 4.4, shine: 1.5 },
  mouth: { y: 72, width: 8, stroke: 2.4 },
  halo: { r: 57, stroke: 3 },
  badge: { cx: 97, cy: 24, r: 11 },
};

const SMALL: OrbiGeometry = {
  cx: 60,
  cy: 60,
  planetR: 36,
  ring: { cx: 60, cy: 74, rx: 41, ry: 12, width: 9, tilt: -14 },
  moon: { cx: 60, cy: 65, rx: 42, ry: 17, r: 7.5, tilt: -14 },
  eyes: { y: 54, dx: 12, rx: 9.6, ry: 11.6, pupil: 5.8, shine: 2.1 },
  mouth: { y: 70, width: 11, stroke: 3.6 },
  halo: { r: 56.7, stroke: 6.5 },
  badge: { cx: 96, cy: 25, r: 13.5 },
};

export const orbiGeometry = (size: number): OrbiGeometry => (size < SMALL_ORBI_SIZE ? SMALL : REGULAR);

export type OrbiLook = {
  /** Drawn at the small, bolder size. */
  small: boolean;
  /** Open eyes, the happy arcs of done, or the closed arcs of sleeping. */
  eyes: "open" | "happy" | "closed";
  /** The yellow "!" badge: Dot needs the user. */
  badge: boolean;
  /** Hopping with the badge. */
  hop: boolean;
  /** Sparks around the planet (done); never at the small size. */
  sparks: boolean;
  /** The floating "z" (sleeping); never at the small size. */
  zzz: boolean;
  /** Pulsing rings (Voz is listening). */
  rings: boolean;
  /** A sweat drop (error). */
  sweat: boolean;
  /** A worried mouth instead of the smile (error). */
  worry: boolean;
  /** The ring glows in a pulse (working). */
  ringGlow: boolean;
  /** How fast the moon goes around. */
  moon: "slow" | "normal" | "fast";
  geometry: OrbiGeometry;
};

export function orbiLook(state: OrbiState, size: number): OrbiLook {
  const small = size < SMALL_ORBI_SIZE;
  return {
    small,
    eyes: state === "done" ? "happy" : state === "sleeping" ? "closed" : "open",
    badge: state === "needs",
    hop: state === "needs",
    sparks: state === "done" && !small,
    zzz: state === "sleeping" && !small,
    rings: state === "listening",
    sweat: state === "error",
    worry: state === "error",
    ringGlow: state === "working",
    moon: state === "working" ? "fast" : state === "sleeping" || state === "error" ? "slow" : "normal",
    geometry: orbiGeometry(size),
  };
}

// --- sleeping --------------------------------------------------------------------------------------------------

/** Orbi falls asleep after this long with nothing going on. */
export const SLEEP_AFTER_MS = 3 * 60 * 1000;

/**
 * Whether Orbi is asleep and when to look again. `quiet` is true while nothing is going on (no agent, no capture, no voice,
 * nothing new); any activity, a hover or a click moves `lastActivityAt` and wakes it. While something is going on it never sleeps.
 */
export function sleepStatus({ quiet, lastActivityAt, now }: { quiet: boolean; lastActivityAt: number; now: number }): { asleep: boolean; checkInMs: number | null } {
  if (!quiet) return { asleep: false, checkInMs: null };
  const left = SLEEP_AFTER_MS - (now - lastActivityAt);
  return left <= 0 ? { asleep: true, checkInMs: null } : { asleep: false, checkInMs: left };
}
