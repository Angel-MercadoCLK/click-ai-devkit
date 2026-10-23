"use client";

import { useEffect, useId, useRef } from "react";
import type { CaptureIndicator } from "../../lib/capture-state.ts";
import { latestCursor, subscribeCursor } from "./cursor-store.ts";
import { ORBI_PALETTES, orbiLook, type OrbiGeometry, type OrbiState } from "./orbi-look.ts";
import { createOrbiEngine, gazeVector, type Frame, type OrbiEngine } from "./orbi-motion.ts";

type OrbiProps = {
  state: OrbiState;
  /** Width and height in px. Below 40 the drawing is simplified and bolder so it reads on the black island. */
  size: number;
  /** Capture halo: red while the screen is shared, amber while it is paused. Capture must always be visible, apart from Orbi's own ring. */
  ring?: CaptureIndicator;
};

/** The blink closes the eyes to this thin slit, never to nothing. */
const BLINK_DEPTH = 0.92;
const MOON_COLOR = "#FFE27A";
const INK = "#14183A";

type Parts = {
  svg: SVGSVGElement;
  glow: SVGElement;
  face: SVGElement;
  eyes: SVGElement;
  pupils: SVGElement;
  happy: SVGElement;
  closed: SVGElement;
  smile: SVGElement;
  worry: SVGElement;
  ringBack: SVGElement;
  ringFront: SVGElement;
  shineBack: SVGElement;
  shineFront: SVGElement;
  moonBack: SVGElement;
  moonFront: SVGElement;
  badge: SVGElement | null;
  sweat: SVGElement | null;
  sparks: SVGElement[];
  zzz: SVGElement[];
  rings: SVGElement[];
  stops: SVGStopElement[];
  glowFill: SVGElement;
};

/** Writes one style value only when it changed, so a settled character costs almost nothing per frame. */
function put(element: Element | null, cache: Map<string, string>, name: string, property: "transform" | "opacity", value: string) {
  if (!element) return;
  const key = `${name}:${property}`;
  if (cache.get(key) === value) return;
  cache.set(key, value);
  (element as SVGElement | HTMLElement).style[property] = value;
}

const fixed = (value: number) => (Math.abs(value) < 0.0005 ? 0 : Number(value.toFixed(3)));
const unit = (value: number) => String(fixed(Math.min(1, Math.max(0, value))));

function apply(parts: Parts, frame: Frame, geometry: OrbiGeometry, size: number, cache: Map<string, string>) {
  const k = size / 120;
  // The whole character: float, hop, shiver, squash and stretch and breathing on the root element, which the browser moves on the compositor.
  put(parts.svg, cache, "svg", "transform", `translate3d(${fixed(frame.shiver * k)}px, ${fixed((frame.floatY + frame.hopY) * size)}px, 0) scale(${fixed(frame.squashX * frame.breathe)}, ${fixed(frame.squashY * frame.breathe)})`);
  put(parts.glow, cache, "glow", "opacity", unit(frame.glow));
  // The face leans a little toward the gaze, the pupils move inside the eyes, the eyes widen on a hover and blink.
  put(parts.face, cache, "face", "transform", `translate(${fixed(frame.faceX)}px, ${fixed(frame.faceY)}px)`);
  put(parts.eyes, cache, "eyes", "transform", `scale(${fixed(frame.eyeScale)}, ${fixed(frame.eyeScale * (1 - BLINK_DEPTH * frame.blink))})`);
  put(parts.eyes, cache, "eyes", "opacity", unit(frame.openness));
  put(parts.pupils, cache, "pupils", "transform", `translate(${fixed(frame.gazeX)}px, ${fixed(frame.gazeY)}px)`);
  put(parts.happy, cache, "happy", "opacity", unit(frame.happy));
  put(parts.closed, cache, "closed", "opacity", unit(frame.closed));
  put(parts.smile, cache, "smile", "opacity", unit(1 - frame.worry));
  put(parts.worry, cache, "worry", "opacity", unit(frame.worry));
  // The ring: its own wobble, and a brighter pulse while working.
  const tilt = `rotate(${fixed(geometry.ring.tilt + frame.ringTilt)}deg)`;
  put(parts.ringBack, cache, "ringBack", "transform", tilt);
  put(parts.ringFront, cache, "ringFront", "transform", tilt);
  put(parts.shineBack, cache, "shineBack", "opacity", unit(frame.ringPulse * 0.55));
  put(parts.shineFront, cache, "shineFront", "opacity", unit(frame.ringPulse * 0.55));
  // The moon goes around on a tilted ellipse: behind the planet on one half, in front on the other, smaller when far.
  const angle = (frame.moonAngle * Math.PI) / 180;
  const tiltRad = (geometry.moon.tilt * Math.PI) / 180;
  const x = geometry.moon.rx * frame.moonReach * Math.cos(angle);
  const y = geometry.moon.ry * frame.moonReach * Math.sin(angle);
  const px = geometry.moon.cx + x * Math.cos(tiltRad) - y * Math.sin(tiltRad);
  const py = geometry.moon.cy + x * Math.sin(tiltRad) + y * Math.cos(tiltRad);
  const place = `translate(${fixed(px)}px, ${fixed(py)}px) scale(${fixed(frame.moonScale)})`;
  const inFront = frame.moonDepth > 0;
  put(parts.moonBack, cache, "moonBack", "transform", place);
  put(parts.moonFront, cache, "moonFront", "transform", place);
  put(parts.moonBack, cache, "moonBack", "opacity", inFront ? "0" : "1");
  put(parts.moonFront, cache, "moonFront", "opacity", inFront ? "1" : "0");
  const popped = Math.max(0, frame.badge);
  put(parts.badge, cache, "badge", "transform", `scale(${fixed(popped)})`);
  put(parts.badge, cache, "badge", "opacity", unit(popped));
  put(parts.sweat, cache, "sweat", "transform", `translate(0px, ${fixed(frame.sweat.y)}px)`);
  put(parts.sweat, cache, "sweat", "opacity", unit(frame.sweat.opacity));
  parts.sparks.forEach((spark, i) => {
    put(spark, cache, `spark${i}`, "transform", `scale(${fixed(frame.sparks[i].scale)})`);
    put(spark, cache, `spark${i}`, "opacity", unit(frame.sparks[i].opacity));
  });
  parts.zzz.forEach((z, i) => {
    put(z, cache, `zzz${i}`, "transform", `translate(${fixed(frame.zzz[i].x)}px, ${fixed(frame.zzz[i].y)}px) scale(${fixed(frame.zzz[i].scale)})`);
    put(z, cache, `zzz${i}`, "opacity", unit(frame.zzz[i].opacity));
  });
  parts.rings.forEach((ring, i) => {
    put(ring, cache, `rings${i}`, "transform", `scale(${fixed(frame.rings[i].scale)})`);
    put(ring, cache, `rings${i}`, "opacity", unit(frame.rings[i].opacity));
  });
}

function applyPalette(parts: Parts, engine: OrbiEngine, cache: Map<string, string>) {
  const palette = engine.palette();
  const colors = [palette.b0, palette.b1, palette.b2, palette.b3, palette.r0, palette.r1, palette.r2];
  const signature = `${colors.join("|")}|${palette.glow}`;
  if (cache.get("palette") === signature) return;
  cache.set("palette", signature);
  parts.stops.forEach((stop, index) => stop.setAttribute("stop-color", colors[index]));
  parts.glowFill.setAttribute("fill", palette.glow);
}

/** A little four-pointed sparkle, centered on (0, 0). */
const SPARKLE = "M0 -7 L2 -2 L7 0 L2 2 L0 7 L-2 2 L-7 0 L-2 -2 Z";
/** Where the sparks of done fly, around the planet. */
const SPARK_SPOTS: readonly [number, number][] = [[17, 34], [104, 38], [20, 92], [102, 88]];
/** The drop that slides down a worried face. */
const DROP = "M0 0 q-4.6 7.4 0 10.4 q4.6 -3 0 -10.4 Z";

/**
 * Dot's character: Orbi, a little planet with a tilted ring that passes behind and in front of it, a moon going around,
 * big eyes and a small smile, drawn in code and alive. A small engine (`orbi-motion.ts`) drives it from one requestAnimationFrame
 * loop: the pupils follow the cursor with a spring (the desktop shell pushes the position), the eyes blink at random,
 * glance around when nothing moves, widen on a hover; a click is a boing and the moon zips around; now and then the ring
 * wobbles and the moon ducks behind the planet; the emotes of each state (the glowing ring while working, the hop and the "!"
 * when Dot needs you, sparks when done, rings while listening, a sweat drop on an error, a floating z asleep) spring in
 * instead of jumping. Reduced motion keeps all of it alive with a smaller reach and no bounce, hop or shiver. The loop
 * exists only while the character is mounted and the page is visible, so a hidden or collapsed island runs nothing.
 */
export function Orbi({ state, size, ring = "off" }: OrbiProps) {
  const id = useId().replace(/[^a-zA-Z0-9]/g, "");
  const { geometry: g, small } = orbiLook(state, size);
  const initial = useRef(ORBI_PALETTES[state]).current;
  const svgRef = useRef<SVGSVGElement>(null);
  const engineRef = useRef<OrbiEngine | null>(null);
  const stateRef = useRef(state);
  stateRef.current = state;

  useEffect(() => {
    const svg = svgRef.current;
    if (!svg) return;
    const part = (name: string) => svg.querySelector<SVGElement>(`[data-part="${name}"]`);
    const all = (name: string) => Array.from(svg.querySelectorAll<SVGElement>(`[data-part^="${name}"]`));
    const parts: Parts = {
      svg,
      glow: part("glow")!,
      glowFill: part("glow")!,
      face: part("face")!,
      eyes: part("eyes")!,
      pupils: part("pupils")!,
      happy: part("happy")!,
      closed: part("closed")!,
      smile: part("smile")!,
      worry: part("worry")!,
      ringBack: part("ring-back")!,
      ringFront: part("ring-front")!,
      shineBack: part("shine-back")!,
      shineFront: part("shine-front")!,
      moonBack: part("moon-back")!,
      moonFront: part("moon-front")!,
      badge: part("badge"),
      sweat: part("sweat"),
      sparks: all("spark"),
      zzz: all("zzz"),
      rings: all("rings"),
      stops: Array.from(svg.querySelectorAll<SVGStopElement>("[data-stop]")),
    };
    const reducedQuery = window.matchMedia("(prefers-reduced-motion: reduce)");
    const engine = createOrbiEngine({ reduced: reducedQuery.matches, state: stateRef.current });
    engineRef.current = engine;
    const cache = new Map<string, string>();

    const follow = (cursor: { x: number; y: number } | undefined) => {
      if (!cursor) return;
      const rect = svg.getBoundingClientRect();
      engine.setGaze(gazeVector(cursor, { x: rect.left + rect.width / 2, y: rect.top + (rect.height * g.eyes.y) / 120 }));
    };
    follow(latestCursor());
    const unsubscribe = subscribeCursor(follow);

    const onReduced = (event: MediaQueryListEvent) => engine.setReduced(event.matches);
    reducedQuery.addEventListener("change", onReduced);

    // Hovering the character's surroundings widens the eyes; pressing the character itself is a boing.
    const host = svg.parentElement;
    const onEnter = () => engine.setHover(true);
    const onLeave = () => engine.setHover(false);
    const onDown = (event: PointerEvent) => {
      if (event.target instanceof Node && svg.contains(event.target)) engine.press();
    };
    host?.addEventListener("pointerenter", onEnter);
    host?.addEventListener("pointerleave", onLeave);
    host?.addEventListener("pointerdown", onDown);

    let frame = 0;
    let last = performance.now();
    const tick = (now: number) => {
      const next = engine.step(Math.min(now - last, 50));
      last = now;
      apply(parts, next, g, size, cache);
      applyPalette(parts, engine, cache);
      frame = window.requestAnimationFrame(tick);
    };
    const start = () => {
      if (frame) return;
      last = performance.now();
      frame = window.requestAnimationFrame(tick);
    };
    const stop = () => {
      window.cancelAnimationFrame(frame);
      frame = 0;
    };
    // A page that is not visible draws nothing, so the loop rests with it.
    const onVisibility = () => (document.visibilityState === "hidden" ? stop() : start());
    document.addEventListener("visibilitychange", onVisibility);
    if (document.visibilityState !== "hidden") start();

    return () => {
      stop();
      document.removeEventListener("visibilitychange", onVisibility);
      unsubscribe();
      reducedQuery.removeEventListener("change", onReduced);
      host?.removeEventListener("pointerenter", onEnter);
      host?.removeEventListener("pointerleave", onLeave);
      host?.removeEventListener("pointerdown", onDown);
      engineRef.current = null;
    };
    // The engine is built once per mount and per size; the state reaches it below.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [size]);

  useEffect(() => {
    engineRef.current?.setState(state);
  }, [state]);

  const ringBack = `M ${g.ring.cx - g.ring.rx} ${g.ring.cy} A ${g.ring.rx} ${g.ring.ry} 0 0 1 ${g.ring.cx + g.ring.rx} ${g.ring.cy}`;
  const ringFront = `M ${g.ring.cx + g.ring.rx} ${g.ring.cy} A ${g.ring.rx} ${g.ring.ry} 0 0 1 ${g.ring.cx - g.ring.rx} ${g.ring.cy}`;
  const ringOrigin = `${g.ring.cx}px ${g.ring.cy}px`;
  const ringStyle = { transformOrigin: ringOrigin, transform: `rotate(${g.ring.tilt}deg)` };
  const { eyes: e, mouth: m, badge: b } = g;
  const left = g.cx - e.dx;
  const right = g.cx + e.dx;
  const arc = (cx: number, drop: number) => `M ${cx - e.rx * 0.9} ${e.y + drop} q ${e.rx * 0.9} ${-e.ry * 0.9} ${e.rx * 1.8} 0`;
  const sleepArc = (cx: number) => `M ${cx - e.rx * 0.9} ${e.y - 1} q ${e.rx * 0.9} ${e.ry * 0.5} ${e.rx * 1.8} 0`;
  const stroke = small ? 4.2 : 3.2;
  const kb = b.r / 11;
  const fillBox = { transformBox: "fill-box", transformOrigin: "center" } as const;
  const ringWidth = g.ring.width;
  // The emotes are always in the drawing (invisible until their state springs them in), so a state can come and go; the sparks and the z are left out at the small size.
  const rings = [0, 1];
  const sparks = small ? [] : SPARK_SPOTS;
  const zzz = small ? [] : [0, 1];
  const halo = ring === "live" || ring === "paused";

  return (
    <svg ref={svgRef} className={`orbi is-${state}`} viewBox="0 0 120 120" width={size} height={size} aria-hidden="true" focusable="false" data-state={state} data-ring={ring} data-size={small ? "small" : "regular"}>
      <defs>
        <radialGradient id={`${id}b`} cx="36%" cy="30%" r="80%">
          <stop data-stop offset="0%" stopColor={initial.b0} />
          <stop data-stop offset="40%" stopColor={initial.b1} />
          <stop data-stop offset="80%" stopColor={initial.b2} />
          <stop data-stop offset="100%" stopColor={initial.b3} />
        </radialGradient>
        <linearGradient id={`${id}r`} gradientUnits="userSpaceOnUse" x1={g.ring.cx - g.ring.rx} y1={g.ring.cy} x2={g.ring.cx + g.ring.rx} y2={g.ring.cy}>
          <stop data-stop offset="0%" stopColor={initial.r0} />
          <stop data-stop offset="50%" stopColor={initial.r1} />
          <stop data-stop offset="100%" stopColor={initial.r2} />
        </linearGradient>
        <filter id={`${id}g`} x="-80%" y="-80%" width="260%" height="260%">
          <feGaussianBlur stdDeviation={small ? 8 : 9} />
        </filter>
      </defs>
      {halo && <circle className="orbi-halo" cx={g.cx} cy={g.cy} r={g.halo.r} strokeWidth={g.halo.stroke} />}
      <ellipse data-part="glow" cx={g.cx} cy={g.cy + 4} rx={g.planetR + 6} ry={g.planetR + 2} fill={initial.glow} filter={`url(#${id}g)`} />
      <g data-part="ring-back" style={ringStyle}>
        <path d={ringBack} fill="none" stroke={`url(#${id}r)`} strokeWidth={ringWidth} strokeLinecap="round" opacity="0.92" />
        <path data-part="shine-back" d={ringBack} fill="none" stroke="#FFFFFF" strokeWidth={ringWidth} strokeLinecap="round" opacity="0" />
      </g>
      <g data-part="moon-back" opacity="0">
        <circle r={g.moon.r} fill={MOON_COLOR} />
        <circle cx={-g.moon.r * 0.3} cy={-g.moon.r * 0.3} r={g.moon.r * 0.38} fill="#FFF6C4" opacity="0.85" />
      </g>
      <circle cx={g.cx} cy={g.cy} r={g.planetR} fill={`url(#${id}b)`} />
      <ellipse cx={g.cx - g.planetR * 0.36} cy={g.cy - g.planetR * 0.4} rx={g.planetR * 0.34} ry={g.planetR * 0.19} fill="#FFFFFF" opacity={small ? 0.55 : 0.5} transform={`rotate(-24 ${g.cx - g.planetR * 0.36} ${g.cy - g.planetR * 0.4})`} />
      <g data-part="face">
        {!small && (
          <>
            <ellipse cx={left - 7.5} cy={e.y + e.ry - 0.5} rx="4.4" ry="2.6" fill="#FF6FB5" opacity="0.32" />
            <ellipse cx={right + 7.5} cy={e.y + e.ry - 0.5} rx="4.4" ry="2.6" fill="#FF6FB5" opacity="0.32" />
          </>
        )}
        <g data-part="eyes" style={fillBox}>
          <ellipse cx={left} cy={e.y} rx={e.rx} ry={e.ry} fill="#FFFFFF" />
          <ellipse cx={right} cy={e.y} rx={e.rx} ry={e.ry} fill="#FFFFFF" />
          <g data-part="pupils">
            <circle cx={left + 0.5} cy={e.y + 1} r={e.pupil} fill={INK} />
            <circle cx={right + 0.5} cy={e.y + 1} r={e.pupil} fill={INK} />
            <circle cx={left + 0.5 + e.pupil * 0.38} cy={e.y + 1 - e.pupil * 0.42} r={e.shine} fill="#FFFFFF" />
            <circle cx={right + 0.5 + e.pupil * 0.38} cy={e.y + 1 - e.pupil * 0.42} r={e.shine} fill="#FFFFFF" />
          </g>
        </g>
        <g data-part="happy" fill="none" stroke={INK} strokeWidth={stroke} strokeLinecap="round" opacity="0">
          <path d={arc(left, 2)} />
          <path d={arc(right, 2)} />
        </g>
        <g data-part="closed" fill="none" stroke={INK} strokeWidth={small ? 3.6 : 2.8} strokeLinecap="round" opacity="0">
          <path d={sleepArc(left)} />
          <path d={sleepArc(right)} />
        </g>
        <path data-part="smile" d={`M ${g.cx - m.width / 2} ${m.y} q ${m.width / 2} ${m.width / 2} ${m.width} 0`} fill="none" stroke={INK} strokeWidth={m.stroke} strokeLinecap="round" />
        <path data-part="worry" d={`M ${g.cx - m.width / 2} ${m.y + 1} q ${m.width / 4} ${-m.width / 3} ${m.width / 2} 0 t ${m.width / 2} 0`} fill="none" stroke={INK} strokeWidth={m.stroke} strokeLinecap="round" strokeLinejoin="round" opacity="0" />
        <g transform={`translate(${g.cx - e.dx - e.rx - 6} ${e.y - e.ry - 4}) scale(${small ? 1.7 : 1.35})`}>
          <g data-part="sweat" opacity="0">
            <path d={DROP} fill="#7FC8FF" />
            <circle cx="-1.3" cy="6" r="1.2" fill="#FFFFFF" opacity="0.8" />
          </g>
        </g>
      </g>
      <g data-part="ring-front" style={ringStyle}>
        <path d={ringFront} fill="none" stroke={`url(#${id}r)`} strokeWidth={ringWidth} strokeLinecap="round" opacity="0.95" />
        <path data-part="shine-front" d={ringFront} fill="none" stroke="#FFFFFF" strokeWidth={ringWidth} strokeLinecap="round" opacity="0" />
      </g>
      <g data-part="moon-front" opacity="0">
        <circle r={g.moon.r} fill={MOON_COLOR} />
        <circle cx={-g.moon.r * 0.3} cy={-g.moon.r * 0.3} r={g.moon.r * 0.38} fill="#FFF6C4" opacity="0.85" />
      </g>
      {rings.map((i) => (
        <circle key={`rings${i}`} data-part={`rings${i}`} cx={g.cx} cy={g.cy} r={g.planetR * 0.95} fill="none" stroke="#3FD2C7" strokeWidth={small ? 3.6 : 3} opacity="0" style={fillBox} />
      ))}
      {sparks.map(([x, y], i) => (
        <g key={`spark${i}`} transform={`translate(${x} ${y})`}>
          <path data-part={`spark${i}`} d={SPARKLE} fill={MOON_COLOR} opacity="0" style={fillBox} />
        </g>
      ))}
      {zzz.map((i) => (
        <g key={`zzz${i}`} transform={`translate(${96 - i * 7} ${34 + i * 9}) scale(${i === 0 ? 1 : 0.75})`}>
          <path data-part={`zzz${i}`} d="M-3.6 -4 h7.2 l-7.2 8 h7.2" fill="none" stroke="#C8CBD6" strokeWidth="2.2" strokeLinecap="round" strokeLinejoin="round" opacity="0" style={fillBox} />
        </g>
      ))}
      <g data-part="badge" opacity="0" style={fillBox}>
        <circle cx={b.cx} cy={b.cy} r={b.r} fill="#FFD23F" stroke="#000000" strokeWidth={small ? 3.4 : 3} />
        <rect x={b.cx - 1.4 * kb} y={b.cy - 7 * kb} width={2.8 * kb} height={9 * kb} rx={1.4 * kb} fill="#1D1306" />
        <circle cx={b.cx} cy={b.cy + 5.8 * kb} r={1.8 * kb} fill="#1D1306" />
      </g>
    </svg>
  );
}
