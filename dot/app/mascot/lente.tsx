"use client";

import { useEffect, useId, useRef } from "react";
import type { CaptureIndicator } from "../../lib/capture-state.ts";
import { latestCursor, subscribeCursor } from "./cursor-store.ts";
import { LENTE_PALETTES, type LenteState } from "./lente-look.ts";
import { createLenteEngine, gazeVector, type Frame, type LenteEngine } from "./lente-motion.ts";

type LenteProps = {
  state: LenteState;
  /** Width and height in px. */
  size: number;
  /** Capture ring: red while the screen is shared, amber while it is paused. Capture must always be visible. */
  ring?: CaptureIndicator;
  /** Voz is listening: the iris dilates a little. */
  listening?: boolean;
};

/** Where the eye sits in the drawing, as a fraction of its height: the cursor is measured from here. */
const EYE_CENTER_Y = 0.53;
/** The blink closes the eye to this thin slit, never to nothing. */
const BLINK_DEPTH = 0.9;

type Parts = {
  svg: SVGSVGElement;
  glow: SVGCircleElement;
  eye: SVGGElement;
  iris: SVGGElement;
  pupil: SVGCircleElement;
  orbit: SVGCircleElement;
  badge: SVGGElement;
  stops: SVGStopElement[];
};

/** Writes one style value only when it changed, so a settled character costs almost nothing per frame. */
function put(element: Element, cache: Map<string, string>, name: string, property: "transform" | "opacity", value: string) {
  const key = `${name}:${property}`;
  if (cache.get(key) === value) return;
  cache.set(key, value);
  (element as SVGElement | HTMLElement).style[property] = value;
}

const fixed = (value: number) => (Math.abs(value) < 0.0005 ? 0 : Number(value.toFixed(3)));

function apply(parts: Parts, frame: Frame, size: number, cache: Map<string, string>) {
  const { gazeX, gazeY, shiftX, shiftY, closure, eyeScale, pupil, orbitAngle, orbitOpacity, badge, squashX, squashY, bobY, breathe } = frame;
  // The whole character: squash and stretch, bob and breathing on the root element, which the browser moves on the compositor.
  put(parts.svg, cache, "svg", "transform", `translate3d(0, ${fixed(bobY * size)}px, 0) scale(${fixed(squashX * breathe)}, ${fixed(squashY * breathe)})`);
  // The eye: a little parallax toward the gaze, widened by a hover, thinned by a blink or the done lid.
  put(parts.eye, cache, "eye", "transform", `translate(${fixed(shiftX)}px, ${fixed(shiftY)}px) scale(${fixed(eyeScale)}, ${fixed(eyeScale * (1 - BLINK_DEPTH * closure))})`);
  put(parts.iris, cache, "iris", "transform", `translate(${fixed(gazeX)}px, ${fixed(gazeY)}px)`);
  put(parts.pupil, cache, "pupil", "transform", `scale(${fixed(pupil)})`);
  put(parts.orbit, cache, "orbit", "transform", `rotate(${fixed(orbitAngle)}deg)`);
  put(parts.orbit, cache, "orbit", "opacity", String(fixed(orbitOpacity)));
  const popped = Math.max(0, badge);
  put(parts.badge, cache, "badge", "transform", `scale(${fixed(popped)})`);
  put(parts.badge, cache, "badge", "opacity", String(fixed(Math.min(1, popped))));
}

function applyPalette(parts: Parts, engine: LenteEngine, cache: Map<string, string>) {
  const palette = engine.palette();
  const colors = [palette.hi, palette.mid, palette.lo, palette.glow];
  const signature = colors.join("|");
  if (cache.get("palette") === signature) return;
  cache.set("palette", signature);
  parts.stops.forEach((stop, index) => stop.setAttribute("stop-color", colors[index]));
  parts.glow.setAttribute("fill", palette.glow);
  parts.orbit.setAttribute("stroke", palette.hi);
}

/**
 * Dot's character: a graphite glossy sphere with a single camera-lens eye, drawn in code and alive. A small engine
 * (`lente-motion.ts`) drives it from one requestAnimationFrame loop: the iris follows the cursor with a spring (the desktop
 * shell pushes the position), the eye blinks at random and sometimes twice, glances around when nothing moves, breathes,
 * widens on hover and squashes on a click; the state changes (color, orbit, "!" badge, half lid) spring instead of jumping.
 * Reduced motion keeps the blinking, the breathing and the following with a smaller reach and no bounce. The loop exists
 * only while the character is mounted, so a hidden or collapsed island runs nothing.
 */
export function Lente({ state, size, ring = "off", listening = false }: LenteProps) {
  const id = useId().replace(/[^a-zA-Z0-9]/g, "");
  const initial = useRef(LENTE_PALETTES[state]).current;
  const svgRef = useRef<SVGSVGElement>(null);
  const engineRef = useRef<LenteEngine | null>(null);

  useEffect(() => {
    const svg = svgRef.current;
    if (!svg) return;
    const find = <T extends Element>(selector: string) => svg.querySelector(selector) as T;
    const parts: Parts = {
      svg,
      glow: find<SVGCircleElement>(".lente-glow"),
      eye: find<SVGGElement>(".lente-eye"),
      iris: find<SVGGElement>(".lente-iris"),
      pupil: find<SVGCircleElement>(".lente-pupil"),
      orbit: find<SVGCircleElement>(".lente-orbit"),
      badge: find<SVGGElement>(".lente-badge"),
      stops: Array.from(svg.querySelectorAll<SVGStopElement>("[data-iris-stop]")),
    };
    const reducedQuery = window.matchMedia("(prefers-reduced-motion: reduce)");
    const engine = createLenteEngine({ reduced: reducedQuery.matches, state, listening });
    engineRef.current = engine;
    const cache = new Map<string, string>();

    const follow = (cursor: { x: number; y: number } | undefined) => {
      if (!cursor) return;
      const rect = svg.getBoundingClientRect();
      engine.setGaze(gazeVector(cursor, { x: rect.left + rect.width / 2, y: rect.top + rect.height * EYE_CENTER_Y }));
    };
    follow(latestCursor());
    const unsubscribe = subscribeCursor(follow);

    const onReduced = (event: MediaQueryListEvent) => engine.setReduced(event.matches);
    reducedQuery.addEventListener("change", onReduced);

    // Hovering the character's surroundings widens the eye; pressing the character itself squashes it.
    const host = svg.parentElement;
    const onEnter = () => engine.setHover(true);
    const onLeave = () => engine.setHover(false);
    const onDown = (event: PointerEvent) => {
      if (event.target instanceof Node && svg.contains(event.target)) engine.press();
    };
    const onUp = () => engine.release();
    host?.addEventListener("pointerenter", onEnter);
    host?.addEventListener("pointerleave", onLeave);
    host?.addEventListener("pointerdown", onDown);
    window.addEventListener("pointerup", onUp);
    window.addEventListener("pointercancel", onUp);

    let frame = 0;
    let last = performance.now();
    const tick = (now: number) => {
      const next = engine.step(Math.min(now - last, 50));
      last = now;
      apply(parts, next, size, cache);
      applyPalette(parts, engine, cache);
      frame = window.requestAnimationFrame(tick);
    };
    frame = window.requestAnimationFrame(tick);

    return () => {
      window.cancelAnimationFrame(frame);
      unsubscribe();
      reducedQuery.removeEventListener("change", onReduced);
      host?.removeEventListener("pointerenter", onEnter);
      host?.removeEventListener("pointerleave", onLeave);
      host?.removeEventListener("pointerdown", onDown);
      window.removeEventListener("pointerup", onUp);
      window.removeEventListener("pointercancel", onUp);
      engineRef.current = null;
    };
    // The engine is built once per mount; the state and the size reach it below and through the frame loop.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [size]);

  useEffect(() => {
    engineRef.current?.setState(state, listening);
  }, [state, listening]);

  return (
    <svg ref={svgRef} className={`lente is-${state}`} viewBox="0 0 100 100" width={size} height={size} aria-hidden="true" focusable="false" data-state={state} data-ring={ring}>
      <defs>
        <radialGradient id={`${id}b`} cx="35%" cy="25%" r="80%">
          <stop offset="0%" stopColor="#4A4F5C" />
          <stop offset="60%" stopColor="#1E2027" />
          <stop offset="100%" stopColor="#0D0E12" />
        </radialGradient>
        <radialGradient id={`${id}i`} cx="50%" cy="40%" r="60%">
          <stop data-iris-stop offset="0%" stopColor={initial.hi} />
          <stop data-iris-stop offset="70%" stopColor={initial.mid} />
          <stop data-iris-stop offset="100%" stopColor={initial.lo} />
        </radialGradient>
        <filter id={`${id}g`} x="-60%" y="-60%" width="220%" height="220%">
          <feGaussianBlur stdDeviation="6" />
        </filter>
        <filter id={`${id}s`} x="-60%" y="-60%" width="220%" height="220%">
          <feGaussianBlur stdDeviation="1.8" />
        </filter>
      </defs>
      <circle className="lente-glow" cx="50" cy="54" r="20" fill={initial.glow} filter={`url(#${id}g)`} />
      <g className="lente-body">
        <circle cx="50" cy="52" r="32" fill={`url(#${id}b)`} />
        <circle cx="50" cy="52" r="31.6" fill="none" stroke="#ffffff" strokeOpacity="0.16" strokeWidth="0.8" />
        <ellipse cx="39" cy="33" rx="11" ry="5" fill="#ffffff" opacity="0.28" filter={`url(#${id}s)`} />
        <g className="lente-eye">
          <circle cx="50" cy="53" r="17.5" fill="#07080A" />
          <g className="lente-iris">
            <circle cx="50" cy="53" r="13.2" fill={`url(#${id}i)`} />
            <circle className="lente-pupil" cx="50" cy="53" r="5.6" fill="#050506" />
          </g>
          <circle cx="45.8" cy="48.6" r="2.4" fill="#ffffff" opacity="0.9" />
          <circle className="lente-orbit" cx="50" cy="53" r="15.6" fill="none" stroke={initial.hi} strokeWidth="1.2" strokeDasharray="3 5" strokeLinecap="round" opacity="0" />
        </g>
      </g>
      {(ring === "live" || ring === "paused") && <circle className="lente-ring" cx="50" cy="52" r="35" fill="none" strokeWidth="3" />}
      <g className="lente-badge" opacity="0">
        <circle cx="79" cy="25" r="8.5" fill="#FFB340" stroke="#000000" strokeWidth="2.6" />
        <rect x="77.9" y="19.6" width="2.2" height="6.8" rx="1.1" fill="#1D1306" />
        <circle cx="79" cy="29.6" r="1.3" fill="#1D1306" />
      </g>
    </svg>
  );
}
