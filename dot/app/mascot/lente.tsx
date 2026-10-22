import { useId } from "react";
import type { CaptureIndicator } from "../../lib/capture-state.ts";
import { LENTE_PALETTES, lenteLook, type LenteState } from "./lente-look.ts";

type LenteProps = {
  state: LenteState;
  /** Width and height in px. */
  size: number;
  /** Capture ring: red while the screen is shared, amber while it is paused. Capture must always be visible. */
  ring?: CaptureIndicator;
};

/**
 * Dot's character: a graphite glossy sphere with a single camera-lens eye, drawn in code. The iris color follows the state;
 * it breathes and blinks (CSS, so `prefers-reduced-motion` stops it), a dashed ring orbits the iris while working, an amber
 * "!" badge asks for attention, and the lid half-closes when done.
 */
export function Lente({ state, size, ring = "off" }: LenteProps) {
  const id = useId().replace(/[^a-zA-Z0-9]/g, "");
  const palette = LENTE_PALETTES[state];
  const { orbit, badge, lid } = lenteLook(state);
  return (
    <svg className={`lente is-${state}${lid === "half" ? " is-half" : ""}`} viewBox="0 0 100 100" width={size} height={size} aria-hidden="true" focusable="false" data-state={state} data-ring={ring}>
      <defs>
        <radialGradient id={`${id}b`} cx="35%" cy="25%" r="80%">
          <stop offset="0%" stopColor="#4A4F5C" />
          <stop offset="60%" stopColor="#1E2027" />
          <stop offset="100%" stopColor="#0D0E12" />
        </radialGradient>
        <radialGradient id={`${id}i`} cx="50%" cy="40%" r="60%">
          <stop offset="0%" stopColor={palette.hi} />
          <stop offset="70%" stopColor={palette.mid} />
          <stop offset="100%" stopColor={palette.lo} />
        </radialGradient>
        <filter id={`${id}g`} x="-60%" y="-60%" width="220%" height="220%">
          <feGaussianBlur stdDeviation="6" />
        </filter>
        <filter id={`${id}s`} x="-60%" y="-60%" width="220%" height="220%">
          <feGaussianBlur stdDeviation="1.8" />
        </filter>
      </defs>
      <circle cx="50" cy="54" r="20" fill={palette.glow} filter={`url(#${id}g)`} />
      <g className="lente-body">
        <circle cx="50" cy="52" r="32" fill={`url(#${id}b)`} />
        <circle cx="50" cy="52" r="31.6" fill="none" stroke="#ffffff" strokeOpacity="0.16" strokeWidth="0.8" />
        <ellipse cx="39" cy="33" rx="11" ry="5" fill="#ffffff" opacity="0.28" filter={`url(#${id}s)`} />
        <circle cx="50" cy="53" r="17.5" fill="#07080A" />
        <g className="lente-iris">
          <circle cx="50" cy="53" r="13.2" fill={`url(#${id}i)`} />
          <circle cx="50" cy="53" r="5.6" fill="#050506" />
          <circle cx="45.8" cy="48.6" r="2.4" fill="#ffffff" opacity="0.9" />
        </g>
        {orbit && <circle className="lente-orbit" cx="50" cy="53" r="15.6" fill="none" stroke={palette.hi} strokeWidth="1.2" strokeDasharray="3 5" strokeLinecap="round" />}
      </g>
      {(ring === "live" || ring === "paused") && <circle className="lente-ring" cx="50" cy="52" r="35" fill="none" strokeWidth="3" />}
      {badge && (
        <g className="lente-badge">
          <circle cx="79" cy="25" r="8.5" fill={palette.badge} stroke="#000000" strokeWidth="2.6" />
          <rect x="77.9" y="19.6" width="2.2" height="6.8" rx="1.1" fill="#1D1306" />
          <circle cx="79" cy="29.6" r="1.3" fill="#1D1306" />
        </g>
      )}
    </svg>
  );
}
