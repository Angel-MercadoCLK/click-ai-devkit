// How the Lente character looks in each state: the palettes from the design canvas and the few flags the drawing needs.
// Plain data so it can be tested without a browser.

export type LenteState = "idle" | "working" | "approval" | "done";
export const LENTE_STATES: readonly LenteState[] = ["idle", "working", "approval", "done"];

export type LentePalette = { hi: string; mid: string; lo: string; glow: string; badge: string };

/** The iris color follows the state: pearl at rest, blue while working, amber when Dot needs the user, green when done. */
export const LENTE_PALETTES: Record<LenteState, LentePalette> = {
  idle: { hi: "#FFFFFF", mid: "#DCE5F3", lo: "#8FA6C9", glow: "rgba(160, 190, 235, 0.45)", badge: "#FFB340" },
  working: { hi: "#E4EEFF", mid: "#7FA6FF", lo: "#3A5FD9", glow: "rgba(90, 135, 255, 0.55)", badge: "#FFB340" },
  approval: { hi: "#FFF3DA", mid: "#FFC864", lo: "#E58A12", glow: "rgba(255, 170, 50, 0.55)", badge: "#FFB340" },
  done: { hi: "#E6FBEE", mid: "#7FE0A6", lo: "#23A35E", glow: "rgba(60, 200, 120, 0.5)", badge: "#FFB340" },
};

export type LenteLook = {
  /** The dashed ring that orbits the iris while working. */
  orbit: boolean;
  /** The amber "!" badge when Dot needs the user. */
  badge: boolean;
  /** The happy, half-closed lid of the done state. */
  lid: "open" | "half";
};

export function lenteLook(state: LenteState): LenteLook {
  return { orbit: state === "working", badge: state === "approval", lid: state === "done" ? "half" : "open" };
}

/** Anything that is not a known state is shown as idle. */
export function readLenteState(value: unknown): LenteState {
  return LENTE_STATES.find((state) => state === value) ?? "idle";
}
