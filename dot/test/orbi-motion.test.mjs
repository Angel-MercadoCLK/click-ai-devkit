import assert from "node:assert/strict";
import { test } from "node:test";

const motion = await import("../app/mascot/orbi-motion.ts");
const look = await import("../app/mascot/orbi-look.ts");

/** A deterministic random source that cycles through a fixed list. */
const sequence = (...values) => {
  let index = 0;
  return () => values[index++ % values.length];
};
const run = (engine, ms, dt = 16) => {
  let frame = engine.frame();
  for (let t = 0; t < ms; t += dt) frame = engine.step(dt);
  return frame;
};

// --- springs ---------------------------------------------------------------------------------------------------

test("a spring settles on its target and a critically damped one never overshoots", () => {
  const config = motion.springConfig(motion.motionProfile(true), "gaze");
  let spring = { x: 0, v: 0 };
  let peak = 0;
  for (let t = 0; t < 2000; t += 8) {
    spring = motion.springStep(spring, 1, config, 8);
    peak = Math.max(peak, spring.x);
  }
  assert.ok(Math.abs(spring.x - 1) < 0.001);
  assert.ok(peak <= 1 + 1e-6, "no overshoot without bounce");
});

test("the normal profile's springs bounce a little, the reduced profile's never do", () => {
  for (const kind of ["gaze", "shape", "state"]) {
    const bouncy = motion.springConfig(motion.motionProfile(false), kind);
    const calm = motion.springConfig(motion.motionProfile(true), kind);
    assert.ok(bouncy.damping < 2 * Math.sqrt(bouncy.stiffness), `${kind} is under-damped`);
    assert.ok(calm.damping >= 2 * Math.sqrt(calm.stiffness) - 1e-9, `${kind} is critically damped when reduced`);
  }
});

test("a spring is stable even after a long frame (the page was in the background)", () => {
  const config = motion.springConfig(motion.motionProfile(false), "shape");
  const spring = motion.springStep({ x: 0, v: 0 }, 1, config, 5000);
  assert.ok(Number.isFinite(spring.x) && Number.isFinite(spring.v));
  assert.ok(Math.abs(spring.x) < 3);
});

// --- gaze ------------------------------------------------------------------------------------------------------

test("the gaze points from the eye toward the cursor and grows with distance without ever leaving the unit circle", () => {
  const eye = { x: 100, y: 100 };
  assert.deepEqual(motion.gazeVector(null, eye), { x: 0, y: 0 });
  assert.deepEqual(motion.gazeVector({ x: 100, y: 100 }, eye), { x: 0, y: 0 });
  const right = motion.gazeVector({ x: 400, y: 100 }, eye);
  assert.ok(right.x > 0 && Math.abs(right.y) < 1e-9);
  const down = motion.gazeVector({ x: 100, y: 1000 }, eye);
  assert.ok(down.y > 0 && Math.abs(down.x) < 1e-9);
  const near = Math.hypot(...Object.values(motion.gazeVector({ x: 130, y: 100 }, eye)));
  const far = Math.hypot(...Object.values(motion.gazeVector({ x: 1900, y: 100 }, eye)));
  assert.ok(near < far && far < 1 && near > 0);
  const diagonal = motion.gazeVector({ x: -3000, y: -3000 }, eye);
  assert.ok(diagonal.x < 0 && diagonal.y < 0 && Math.hypot(diagonal.x, diagonal.y) < 1);
});

test("a cursor position from the shell is accepted only as two finite numbers", () => {
  assert.deepEqual(motion.readCursor({ x: 12, y: -4 }), { x: 12, y: -4 });
  for (const bad of [null, undefined, 4, "x", {}, { x: 1 }, { x: "1", y: 2 }, { x: Number.NaN, y: 0 }, { x: 0, y: Infinity }, { x: 1e9, y: 0 }]) {
    assert.equal(motion.readCursor(bad), undefined, JSON.stringify(bad));
  }
});

// --- blinking and glancing -------------------------------------------------------------------------------------

test("blinks come at natural random intervals and now and then twice", () => {
  const single = motion.nextBlink(sequence(0, 0.9));
  const double = motion.nextBlink(sequence(1, 0.05));
  assert.ok(single.delay >= 2000 && single.delay <= 7000 && single.double === false);
  assert.ok(double.delay >= 2000 && double.delay <= 7000 && double.double === true);
  const delays = Array.from({ length: 200 }, (_, i) => motion.nextBlink(sequence(((i * 37) % 100) / 100, ((i * 53) % 100) / 100)));
  const doubles = delays.filter((plan) => plan.double).length;
  assert.ok(doubles > 10 && doubles < 80, `double blinks are the exception (${doubles}/200)`);
  assert.ok(new Set(delays.map((plan) => Math.round(plan.delay))).size > 50, "the interval varies");
});

test("a blink closes the eye quickly and opens it a little slower, and is over after its duration", () => {
  assert.equal(motion.blinkClosure(-5), 0);
  assert.equal(motion.blinkClosure(0), 0);
  const peakAt = motion.BLINK_MS * (motion.BLINK_CLOSE_SHARE);
  assert.ok(Math.abs(motion.blinkClosure(peakAt) - 1) < 1e-9);
  assert.ok(motion.blinkClosure(peakAt / 2) > 0 && motion.blinkClosure(peakAt / 2) < 1);
  assert.equal(motion.blinkClosure(motion.BLINK_MS), 0);
  assert.equal(motion.blinkClosure(motion.BLINK_MS + 1000), 0);
  assert.ok(peakAt < motion.BLINK_MS / 2, "closing is faster than opening");
});

test("a glance picks a short look away inside a small box, after a pause", () => {
  for (let i = 0; i < 50; i += 1) {
    const plan = motion.nextGlance(sequence(((i * 31) % 100) / 100, ((i * 17) % 100) / 100, ((i * 7) % 100) / 100, ((i * 13) % 100) / 100));
    assert.ok(plan.delay >= 1000 && plan.delay <= 6000);
    assert.ok(plan.hold >= 300 && plan.hold <= 1500);
    assert.ok(Math.abs(plan.x) <= 1 && Math.abs(plan.y) <= 1);
  }
});

// --- the profile -----------------------------------------------------------------------------------------------

test("reduced motion is gentler and without bounces, never zero", () => {
  const full = motion.motionProfile(false);
  const calm = motion.motionProfile(true);
  assert.equal(full.reduced, false);
  assert.equal(calm.reduced, true);
  assert.equal(calm.bounce, false);
  assert.equal(full.bounce, true);
  assert.equal(calm.hop, 0, "no hopping");
  for (const key of ["gaze", "breathe", "float", "moon", "boing", "widen", "flourish"]) {
    assert.ok(calm[key] > 0, `${key} is not zero when reduced`);
    assert.ok(calm[key] < full[key], `${key} is smaller when reduced`);
  }
});

// --- colors ----------------------------------------------------------------------------------------------------

test("colors mix channel by channel for hex and rgba values, clamped to the two ends", () => {
  assert.equal(motion.mixColor("#000000", "#FFFFFF", 0), "#000000");
  assert.equal(motion.mixColor("#000000", "#FFFFFF", 1), "#ffffff");
  assert.equal(motion.mixColor("#000000", "#FFFFFF", 0.5), "#808080");
  assert.equal(motion.mixColor("#000000", "#FFFFFF", -3), "#000000");
  assert.equal(motion.mixColor("#000000", "#FFFFFF", 9), "#ffffff");
  assert.equal(motion.mixColor("rgba(0, 0, 0, 0)", "rgba(200, 100, 50, 1)", 0.5), "rgba(100, 50, 25, 0.5)");
  assert.equal(motion.mixColor("#000000", "rgba(255, 255, 255, 0.5)", 1), "rgba(255, 255, 255, 0.5)");
  assert.equal(motion.mixColor("not a color", "#FFFFFF", 0.5), "#FFFFFF");
});

test("a palette mixes every field and ends exactly on the target palette", () => {
  const from = look.ORBI_PALETTES.idle;
  const to = look.ORBI_PALETTES.error;
  assert.deepEqual(Object.keys(motion.mixPalette(from, to, 0.5)).sort(), Object.keys(from).sort());
  assert.equal(motion.mixPalette(from, to, 0).b1.toLowerCase(), from.b1.toLowerCase());
  assert.equal(motion.mixPalette(from, to, 1).b1.toLowerCase(), to.b1.toLowerCase());
  assert.equal(motion.mixPalette(from, to, 1).glow, to.glow);
  assert.notEqual(motion.mixPalette(from, to, 0.5).b1.toLowerCase(), from.b1.toLowerCase());
});

// --- the state -------------------------------------------------------------------------------------------------

test("the state targets match the drawing for every state", () => {
  for (const state of look.ORBI_STATES) {
    const flags = look.orbiLook(state, 76);
    const targets = motion.stateTargets(state);
    assert.equal(targets.badge === 1, flags.badge, `${state} badge`);
    assert.equal(targets.hop === 1, flags.hop, `${state} hop`);
    assert.equal(targets.sparks === 1, flags.sparks, `${state} sparks`);
    assert.equal(targets.zzz === 1, flags.zzz, `${state} zzz`);
    assert.equal(targets.rings === 1, flags.rings, `${state} rings`);
    assert.equal(targets.sweat === 1, flags.sweat, `${state} sweat`);
    assert.equal(targets.worry === 1, flags.worry, `${state} worry`);
    assert.equal(targets.ringGlow === 1, flags.ringGlow, `${state} ring glow`);
    assert.equal(targets.happy === 1, flags.eyes === "happy", `${state} happy eyes`);
    assert.equal(targets.closed === 1, flags.eyes === "closed", `${state} closed eyes`);
  }
  const idle = motion.stateTargets("idle");
  assert.equal(idle.badge + idle.hop + idle.sparks + idle.zzz + idle.rings + idle.sweat + idle.worry + idle.ringGlow + idle.happy + idle.closed, 0);
});

test("the moon turns at the pace of the state: fast while working, slow when asleep, never stopped", () => {
  const speed = (state) => motion.stateTargets(state).moon;
  assert.ok(speed("working") > 3 * speed("idle"));
  assert.ok(speed("idle") > 2 * speed("sleeping"));
  for (const state of look.ORBI_STATES) assert.ok(speed(state) > 0, `${state} keeps turning`);
});

test("every state springs in from the one before: the color, the badge and the eyes move a little every frame", () => {
  const engine = motion.createOrbiEngine({ random: sequence(0.5), state: "idle" });
  run(engine, 800);
  engine.setState("needs");
  let previous = engine.frame();
  let biggest = 0;
  const colors = [];
  for (let t = 0; t < 1000; t += 16) {
    const frame = engine.step(16);
    biggest = Math.max(biggest, Math.abs(frame.badge - previous.badge));
    previous = frame;
    colors.push(engine.palette().glow);
  }
  assert.ok(biggest < 0.25, `the badge pops in smoothly (biggest step ${biggest})`);
  assert.ok(new Set(colors).size > 5, "the color passes through in-between values");
  assert.equal(colors.at(-1), look.ORBI_PALETTES.needs.glow);
  assert.ok(previous.badge > 0.95);
  engine.setState("done");
  const done = run(engine, 1500);
  assert.ok(done.happy > 0.95 && done.badge < 0.05 && done.openness < 0.1, "happy eyes replace the open ones");
  engine.setState("error");
  const error = run(engine, 1500);
  assert.ok(error.worry > 0.95 && error.happy < 0.05);
  assert.equal(engine.palette().b1.toLowerCase(), look.ORBI_PALETTES.error.b1.toLowerCase(), "the body turns muted");
});

// --- the eyes --------------------------------------------------------------------------------------------------

test("the pupils follow the gaze they are given, springing toward it instead of jumping", () => {
  const engine = motion.createOrbiEngine({ random: sequence(0.5) });
  engine.setGaze({ x: 1, y: 0 });
  const first = engine.step(16);
  assert.ok(first.gazeX > 0 && first.gazeX < 1, "starts moving but has not arrived");
  const settled = run(engine, 1500);
  assert.ok(settled.gazeX > 0.85 * motion.PUPIL_TRAVEL, "reaches the edge of its range");
  assert.ok(Math.abs(settled.gazeY) < 0.2);
  assert.ok(settled.faceX > 0, "the whole face leans a little toward the cursor too");
  engine.setGaze({ x: -1, y: 0 });
  assert.ok(engine.step(16).gazeX > 0, "turning around carries the old motion for a moment");
  assert.ok(run(engine, 1500).gazeX < -0.85 * motion.PUPIL_TRAVEL);
});

test("the pupils never leave the eyes, whatever the gaze", () => {
  const engine = motion.createOrbiEngine({ random: sequence(0.5) });
  for (const gaze of [{ x: 1, y: 0 }, { x: 0, y: 1 }, { x: -0.7, y: 0.7 }, { x: 5, y: 5 }]) {
    engine.setGaze(gaze);
    for (let t = 0; t < 1200; t += 16) {
      const frame = engine.step(16);
      assert.ok(Math.hypot(frame.gazeX, frame.gazeY) <= motion.PUPIL_TRAVEL + 1e-6, "inside the travel range");
    }
  }
});

test("the reduced-motion eyes follow with a smaller reach and no overshoot", () => {
  const full = motion.createOrbiEngine({ random: sequence(0.5), reduced: false });
  const calm = motion.createOrbiEngine({ random: sequence(0.5), reduced: true });
  full.setGaze({ x: 1, y: 0 });
  calm.setGaze({ x: 1, y: 0 });
  let calmPeak = 0;
  for (let t = 0; t < 3000; t += 16) {
    full.step(16);
    calmPeak = Math.max(calmPeak, calm.step(16).gazeX);
  }
  const settledFull = full.frame().gazeX;
  const settledCalm = calm.frame().gazeX;
  assert.ok(settledCalm > 1 && settledCalm < settledFull);
  assert.ok(calmPeak <= settledCalm + 1e-6, "no bounce");
});

test("the eyes blink by themselves, also under reduced motion, and sometimes twice in a row", () => {
  for (const reduced of [false, true]) {
    const engine = motion.createOrbiEngine({ random: sequence(0.1, 0.1, 0.5, 0.9), reduced });
    let closed = 0;
    let blinks = 0;
    let wasClosed = false;
    for (let t = 0; t < 30000; t += 16) {
      const isClosed = engine.step(16).blink > 0.6;
      if (isClosed && !wasClosed) blinks += 1;
      wasClosed = isClosed;
      if (isClosed) closed += 1;
    }
    assert.ok(blinks >= 4, `reduced=${reduced}: blinks ${blinks}`);
    assert.ok(closed > 0);
  }
  const engine = motion.createOrbiEngine({ random: sequence(0, 0, 0.5) });
  const starts = [];
  let was = false;
  for (let t = 0; t < 6000; t += 8) {
    const isClosed = engine.step(8).blink > 0.6;
    if (isClosed && !was) starts.push(t);
    was = isClosed;
  }
  assert.ok(starts.length >= 2 && starts[1] - starts[0] < 500, "a double blink is two quick closings");
});

test("when the cursor is still the eyes glance around on their own, and follow again as soon as the cursor moves", () => {
  const engine = motion.createOrbiEngine({ random: sequence(0.9, 0.9, 0.9, 0.9, 0.1, 0.9, 0.1, 0.1) });
  engine.setGaze({ x: 0, y: 0 });
  let moved = 0;
  for (let t = 0; t < 12000; t += 16) {
    engine.setGaze({ x: 0, y: 0 });
    if (Math.abs(engine.step(16).gazeX) > 0.5) moved += 1;
  }
  assert.ok(moved > 20, "glanced away from the center while nothing moved");
  engine.setGaze({ x: -1, y: 0.2 });
  assert.ok(run(engine, 1500).gazeX < -2);
});

test("a hover widens the eyes and releasing settles them back", () => {
  const engine = motion.createOrbiEngine({ random: sequence(0.5) });
  const rest = run(engine, 800).eyeScale;
  engine.setHover(true);
  assert.ok(run(engine, 800).eyeScale > rest + 0.08);
  engine.setHover(false);
  assert.ok(Math.abs(run(engine, 2000).eyeScale - rest) < 0.01);
});

// --- reactions -------------------------------------------------------------------------------------------------

const extremes = (engine, ms, dt = 8) => {
  let minY = Infinity;
  let maxY = -Infinity;
  let maxX = -Infinity;
  for (let t = 0; t < ms; t += dt) {
    const frame = engine.step(dt);
    minY = Math.min(minY, frame.squashY);
    maxY = Math.max(maxY, frame.squashY);
    maxX = Math.max(maxX, frame.squashX);
  }
  return { minY, maxY, maxX };
};

test("a click is a boing: squashed wide and short, then stretched tall, then settled; reduced motion only presses softly", () => {
  const engine = motion.createOrbiEngine({ random: sequence(0.5) });
  run(engine, 600);
  engine.press();
  const boing = extremes(engine, 700);
  assert.ok(boing.minY < 0.85 && boing.maxX > 1.1, "squashed: wider and shorter");
  assert.ok(boing.maxY > 1.08, "stretched on the way back");
  assert.ok(Math.abs(run(engine, 1500).squashY - 1) < 0.01, "and back to normal");

  const calm = motion.createOrbiEngine({ random: sequence(0.5), reduced: true });
  run(calm, 600);
  calm.press();
  const soft = extremes(calm, 700);
  assert.ok(soft.minY < 1 && soft.minY > 0.9, "a soft press");
  assert.ok(soft.maxY <= 1 + 1e-6, "no stretch without bounce");
});

test("pressing again in the middle of a boing starts a fresh one instead of piling up", () => {
  const engine = motion.createOrbiEngine({ random: sequence(0.5) });
  run(engine, 600);
  engine.press();
  run(engine, 300);
  engine.press();
  const again = extremes(engine, 700);
  assert.ok(again.minY < 0.85);
  assert.ok(again.minY > 0.5, "never crushed");
  assert.ok(Math.abs(run(engine, 1500).squashY - 1) < 0.01);
});

test("a click sends the moon zipping around, and it settles back to its pace", () => {
  const engine = motion.createOrbiEngine({ random: sequence(0.5) });
  const before = run(engine, 800).moonSpeed;
  engine.press();
  const during = run(engine, 160).moonSpeed;
  assert.ok(during > 4 * before, `zips (${during} vs ${before})`);
  assert.ok(Math.abs(run(engine, 3000).moonSpeed - before) < before * 0.1, "back to its pace");
  const calm = motion.createOrbiEngine({ random: sequence(0.5), reduced: true });
  const calmBefore = run(calm, 800).moonSpeed;
  calm.press();
  const calmDuring = run(calm, 160).moonSpeed;
  assert.ok(calmDuring > calmBefore && calmDuring < during, "a gentler zip when reduced");
});

test("needing the user is a hop with the badge; reduced motion keeps the badge and drops the hop", () => {
  const hop = (reduced) => {
    const engine = motion.createOrbiEngine({ random: sequence(0.5), state: "needs", reduced });
    let min = 0;
    let max = 0;
    let frame;
    for (let t = 0; t < 4000; t += 16) {
      frame = engine.step(16);
      min = Math.min(min, frame.hopY);
      max = Math.max(max, frame.hopY);
    }
    return { min, max, badge: frame.badge };
  };
  const full = hop(false);
  assert.ok(full.min < -0.03 && full.max <= 0, "jumps up and lands");
  assert.ok(full.badge > 0.95);
  const calm = hop(true);
  assert.equal(calm.min, 0);
  assert.equal(calm.max, 0);
  assert.ok(calm.badge > 0.95, "still asks for attention");
});

// --- the emotes ------------------------------------------------------------------------------------------------

test("done throws sparks that grow and fade and keep coming, and nothing sparks otherwise", () => {
  const engine = motion.createOrbiEngine({ random: sequence(0.5), state: "done" });
  run(engine, 1200);
  const seen = new Set();
  let peak = 0;
  for (let t = 0; t < 2400; t += 16) {
    const frame = engine.step(16);
    assert.equal(frame.sparks.length, 4);
    for (const spark of frame.sparks) {
      seen.add(spark.scale.toFixed(2));
      peak = Math.max(peak, spark.opacity);
      assert.ok(spark.opacity >= 0 && spark.opacity <= 1 && spark.scale > 0);
    }
  }
  assert.ok(seen.size > 20 && peak > 0.7);
  const quiet = motion.createOrbiEngine({ random: sequence(0.5), state: "idle" });
  for (let t = 0; t < 2400; t += 16) assert.ok(quiet.step(16).sparks.every((spark) => spark.opacity === 0));
});

test("sleeping closes the eyes, floats a z and slows everything down; waking opens the eyes with a stretch", () => {
  const engine = motion.createOrbiEngine({ random: sequence(0.5), state: "sleeping" });
  let asleep = run(engine, 2500);
  assert.ok(asleep.closed > 0.95 && asleep.openness < 0.05);
  let zMax = 0;
  const heights = new Set();
  for (let t = 0; t < 2600; t += 16) {
    asleep = engine.step(16);
    for (const z of asleep.zzz) {
      zMax = Math.max(zMax, z.opacity);
      heights.add(z.y.toFixed(1));
    }
  }
  assert.ok(zMax > 0.7 && heights.size > 10, "a z rises and fades");
  assert.ok(asleep.moonSpeed < motion.stateTargets("idle").moon / 2);
  assert.ok(asleep.glow < 0.7, "the glow dims");
  engine.setState("idle");
  const stretch = extremes(engine, 600);
  assert.ok(stretch.minY < 0.95, "a little stretch as it wakes");
  const awake = run(engine, 2000);
  assert.ok(awake.closed < 0.02 && awake.openness > 0.95);
  assert.ok(awake.zzz.every((z) => z.opacity === 0));
});

test("an error looks worried: a muted face, a nervous shiver and a sweat drop that slides down; reduced motion does not shiver", () => {
  const trace = (reduced) => {
    const engine = motion.createOrbiEngine({ random: sequence(0.5), state: "error", reduced });
    run(engine, 1500);
    let shiver = 0;
    const drops = new Set();
    let peak = 0;
    for (let t = 0; t < 2400; t += 16) {
      const frame = engine.step(16);
      shiver = Math.max(shiver, Math.abs(frame.shiver));
      drops.add(frame.sweat.y.toFixed(1));
      peak = Math.max(peak, frame.sweat.opacity);
    }
    return { shiver, drops: drops.size, peak, worry: engine.frame().worry };
  };
  const full = trace(false);
  assert.ok(full.shiver > 0.1 && full.drops > 10 && full.peak > 0.7 && full.worry > 0.95);
  const calm = trace(true);
  assert.equal(calm.shiver, 0);
  assert.ok(calm.drops > 10 && calm.peak > 0.7, "the drop still slides");
});

test("listening sends rings out from the planet, one after the other, and working makes the ring glow in a pulse", () => {
  const listening = motion.createOrbiEngine({ random: sequence(0.5), state: "listening" });
  run(listening, 1200);
  const scales = new Set();
  let opacity = 0;
  for (let t = 0; t < 3200; t += 16) {
    const frame = listening.step(16);
    assert.equal(frame.rings.length, 2);
    for (const ring of frame.rings) {
      scales.add(ring.scale.toFixed(2));
      opacity = Math.max(opacity, ring.opacity);
    }
  }
  assert.ok(scales.size > 20 && opacity > 0.6);
  const working = motion.createOrbiEngine({ random: sequence(0.5), state: "working" });
  run(working, 1200);
  const glows = [];
  for (let t = 0; t < 2000; t += 16) glows.push(working.step(16).ringPulse);
  assert.ok(Math.max(...glows) - Math.min(...glows) > 0.5, "the glow pulses");
  const idle = motion.createOrbiEngine({ random: sequence(0.5) });
  assert.equal(run(idle, 1200).ringPulse, 0);
});

// --- idle behaviors --------------------------------------------------------------------------------------------

test("the ring gives a little wobble now and then by itself, smaller and without overshoot when reduced", () => {
  const wobble = (reduced) => {
    const engine = motion.createOrbiEngine({ random: sequence(0.5), reduced });
    let peak = 0;
    let crossed = false;
    let last = 0;
    for (let t = 0; t < 60000; t += 16) {
      const tilt = engine.step(16).ringTilt;
      peak = Math.max(peak, Math.abs(tilt));
      if (last !== 0 && Math.sign(tilt) !== Math.sign(last) && tilt !== 0) crossed = true;
      if (tilt !== 0) last = tilt;
    }
    return { peak, crossed };
  };
  const full = wobble(false);
  assert.ok(full.peak > 8 && full.peak < 45, `a small spin (${full.peak})`);
  assert.equal(full.crossed, true, "it overshoots and wobbles back");
  const calm = wobble(true);
  assert.ok(calm.peak > 2 && calm.peak < full.peak);
  assert.equal(calm.crossed, false);
});

test("now and then the moon ducks behind the planet to peek, and never while Orbi is busy or asleep", () => {
  const lowest = (state) => {
    const engine = motion.createOrbiEngine({ random: sequence(0.5), state });
    let min = 1;
    for (let t = 0; t < 90000; t += 16) min = Math.min(min, engine.step(16).moonReach);
    return min;
  };
  const idle = lowest("idle");
  assert.ok(idle < 0.9 && idle >= 0.5, `ducks behind (${idle})`);
  assert.equal(lowest("working"), 1);
  assert.equal(lowest("sleeping"), 1);
});

test("the moon goes around the planet: behind it half of the turn and in front the other half, a little smaller when far", () => {
  const engine = motion.createOrbiEngine({ random: sequence(0.5) });
  let front = 0;
  let back = 0;
  let nearest = 0;
  let farthest = Infinity;
  for (let t = 0; t < 12000; t += 16) {
    const frame = engine.step(16);
    assert.ok(frame.moonDepth >= -1 && frame.moonDepth <= 1);
    if (frame.moonDepth > 0) front += 1;
    else back += 1;
    nearest = Math.max(nearest, frame.moonScale);
    farthest = Math.min(farthest, frame.moonScale);
  }
  assert.ok(front > 100 && back > 100);
  assert.ok(nearest > farthest + 0.15);
});

// --- never static ----------------------------------------------------------------------------------------------

test("it is never static: in every state and with every setting something is always moving", () => {
  for (const reduced of [false, true]) {
    for (const state of look.ORBI_STATES) {
      const engine = motion.createOrbiEngine({ random: sequence(0.5), reduced, state });
      run(engine, 1500);
      const breathing = new Set();
      const moon = new Set();
      for (let t = 0; t < 4000; t += 16) {
        const frame = engine.step(16);
        breathing.add(frame.breathe.toFixed(4));
        moon.add(frame.moonAngle.toFixed(1));
      }
      assert.ok(breathing.size > 20, `${state}, reduced=${reduced}: breathing changes the frame`);
      assert.ok(moon.size > 20, `${state}, reduced=${reduced}: the moon keeps turning`);
    }
  }
});

test("it floats up and down gently, and less when reduced", () => {
  const range = (reduced) => {
    const engine = motion.createOrbiEngine({ random: sequence(0.5), reduced });
    let min = 0;
    let max = 0;
    for (let t = 0; t < 8000; t += 16) {
      const frame = engine.step(16);
      min = Math.min(min, frame.floatY);
      max = Math.max(max, frame.floatY);
    }
    return max - min;
  };
  const full = range(false);
  const calm = range(true);
  assert.ok(full > 0.02 && full < 0.09);
  assert.ok(calm > 0 && calm < full);
});

test("changing the motion preference at run time re-tunes the same engine", () => {
  const engine = motion.createOrbiEngine({ random: sequence(0.5) });
  engine.setReduced(true);
  engine.setGaze({ x: 1, y: 0 });
  const calm = run(engine, 2000).gazeX;
  engine.setReduced(false);
  assert.ok(run(engine, 2000).gazeX > calm);
});
