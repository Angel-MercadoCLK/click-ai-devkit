import assert from "node:assert/strict";
import { test } from "node:test";

const motion = await import("../app/mascot/lente-motion.ts");
const look = await import("../app/mascot/lente-look.ts");

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
  assert.ok(calm.gaze > 0 && calm.gaze < full.gaze, "still follows, with a smaller reach");
  assert.ok(calm.breathe > 0 && calm.breathe < full.breathe, "still breathes");
  assert.ok(calm.orbitSpeed > 0 && calm.orbitSpeed < full.orbitSpeed);
  assert.equal(calm.bounce, false);
  assert.equal(full.bounce, true);
  assert.equal(calm.bob, 0);
  assert.ok(calm.press < full.press);
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
  const from = look.LENTE_PALETTES.idle;
  const to = look.LENTE_PALETTES.working;
  assert.deepEqual(Object.keys(motion.mixPalette(from, to, 0.5)).sort(), Object.keys(from).sort());
  assert.equal(motion.mixPalette(from, to, 0).mid.toLowerCase(), from.mid.toLowerCase());
  assert.equal(motion.mixPalette(from, to, 1).mid.toLowerCase(), to.mid.toLowerCase());
  assert.equal(motion.mixPalette(from, to, 1).glow, to.glow);
});

// --- the engine ------------------------------------------------------------------------------------------------

test("the state targets match the drawing: orbit while working, badge and bob when Dot needs you, half lid when done", () => {
  const states = Object.fromEntries(look.LENTE_STATES.map((state) => [state, motion.stateTargets(state, false)]));
  assert.equal(states.working.orbit, 1);
  assert.equal(states.approval.badge, 1);
  assert.equal(states.approval.bob, 1);
  assert.equal(states.done.lid, 0.5);
  for (const state of look.LENTE_STATES) {
    const lens = look.lenteLook(state);
    assert.equal(motion.stateTargets(state, false).orbit === 1, lens.orbit, `${state} orbit`);
    assert.equal(motion.stateTargets(state, false).badge === 1, lens.badge, `${state} badge`);
    assert.equal(motion.stateTargets(state, false).lid === 0.5, lens.lid === "half", `${state} lid`);
  }
  assert.equal(states.idle.orbit + states.idle.badge + states.idle.lid + states.idle.bob, 0);
});

test("the iris dilates when Dot listens", () => {
  assert.ok(motion.stateTargets("working", true).dilate > motion.stateTargets("working", false).dilate);
  assert.ok(motion.stateTargets("idle", true).dilate > motion.stateTargets("idle", false).dilate);
  const listening = motion.createLenteEngine({ random: sequence(0.5), state: "working", listening: true });
  const quiet = motion.createLenteEngine({ random: sequence(0.5), state: "working", listening: false });
  assert.ok(run(listening, 1200).pupil > run(quiet, 1200).pupil + 0.1);
});

test("the eye follows the gaze it is given, springing toward it instead of jumping", () => {
  const engine = motion.createLenteEngine({ random: sequence(0.5) });
  engine.setGaze({ x: 1, y: 0 });
  const first = engine.step(16);
  assert.ok(first.gazeX > 0 && first.gazeX < 1, "starts moving but has not arrived");
  const settled = run(engine, 1500);
  assert.ok(settled.gazeX > 2.5, "reaches the edge of its range");
  assert.ok(Math.abs(settled.gazeY) < 0.2);
  engine.setGaze({ x: -1, y: 0 });
  assert.ok(engine.step(16).gazeX > 0, "turning around carries the old motion for a moment");
  assert.ok(run(engine, 1500).gazeX < -2.5);
});

test("the iris never leaves the dark socket, whatever the gaze", () => {
  const engine = motion.createLenteEngine({ random: sequence(0.5) });
  for (const gaze of [{ x: 1, y: 0 }, { x: 0, y: 1 }, { x: -0.7, y: 0.7 }, { x: 5, y: 5 }]) {
    engine.setGaze(gaze);
    for (let t = 0; t < 1200; t += 16) {
      const frame = engine.step(16);
      assert.ok(Math.hypot(frame.gazeX, frame.gazeY) <= motion.IRIS_TRAVEL + 1e-6, "inside the travel range");
    }
  }
});

test("the reduced-motion eye follows with a smaller reach and no overshoot", () => {
  const full = motion.createLenteEngine({ random: sequence(0.5), reduced: false });
  const calm = motion.createLenteEngine({ random: sequence(0.5), reduced: true });
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

test("the eye blinks by itself, also under reduced motion, and sometimes twice in a row", () => {
  for (const reduced of [false, true]) {
    const engine = motion.createLenteEngine({ random: sequence(0.1, 0.1, 0.5, 0.9), reduced });
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
  const engine = motion.createLenteEngine({ random: sequence(0, 0, 0.5) });
  const starts = [];
  let was = false;
  for (let t = 0; t < 6000; t += 8) {
    const isClosed = engine.step(8).blink > 0.6;
    if (isClosed && !was) starts.push(t);
    was = isClosed;
  }
  assert.ok(starts.length >= 2 && starts[1] - starts[0] < 500, "a double blink is two quick closings");
});

test("when the cursor is still the eye glances around on its own, and follows again as soon as the cursor moves", () => {
  const engine = motion.createLenteEngine({ random: sequence(0.9, 0.9, 0.9, 0.9, 0.1, 0.9, 0.1, 0.1) });
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

test("hovering widens the eye and releasing settles it back", () => {
  const engine = motion.createLenteEngine({ random: sequence(0.5) });
  const rest = run(engine, 800).eyeScale;
  engine.setHover(true);
  assert.ok(run(engine, 800).eyeScale > rest + 0.04);
  engine.setHover(false);
  assert.ok(Math.abs(run(engine, 2000).eyeScale - rest) < 0.01);
});

test("a click squashes the character and lets it spring back with a stretch, while reduced motion only presses it", () => {
  const engine = motion.createLenteEngine({ random: sequence(0.5) });
  run(engine, 600);
  engine.press();
  const pressed = run(engine, 200);
  assert.ok(pressed.squashY < 0.95 && pressed.squashX > 1.02, "squashed: wider and shorter");
  engine.release();
  let stretch = 0;
  for (let t = 0; t < 600; t += 8) stretch = Math.max(stretch, engine.step(8).squashY);
  assert.ok(stretch > 1.01, "overshoots into a stretch");
  assert.ok(Math.abs(run(engine, 1500).squashY - 1) < 0.01);

  const calm = motion.createLenteEngine({ random: sequence(0.5), reduced: true });
  run(calm, 600);
  calm.press();
  assert.ok(run(calm, 300).squashY < 1);
  calm.release();
  let calmStretch = 0;
  for (let t = 0; t < 600; t += 8) calmStretch = Math.max(calmStretch, calm.step(8).squashY);
  assert.ok(calmStretch <= 1 + 1e-6, "no stretch without bounce");
});

test("a state change is a spring, never a jump: the color, the orbit, the badge and the lid move a little every frame", () => {
  const engine = motion.createLenteEngine({ random: sequence(0.5), state: "idle" });
  run(engine, 800);
  engine.setState("working", false);
  let previous = engine.frame();
  let biggest = 0;
  const palettes = [];
  for (let t = 0; t < 1000; t += 16) {
    const frame = engine.step(16);
    biggest = Math.max(biggest, Math.abs(frame.orbitOpacity - previous.orbitOpacity));
    previous = frame;
    palettes.push(engine.palette().mid);
  }
  assert.ok(biggest < 0.2, `orbit fades in smoothly (biggest step ${biggest})`);
  assert.ok(new Set(palettes).size > 5, "the color passes through in-between values");
  assert.equal(palettes.at(-1).toLowerCase(), look.LENTE_PALETTES.working.mid.toLowerCase());
  assert.ok(previous.orbitOpacity > 0.95);
  engine.setState("approval", false);
  const approval = run(engine, 1200);
  assert.ok(approval.badge > 0.9 && approval.orbitOpacity < 0.05);
  engine.setState("done", false);
  const done = run(engine, 1500);
  assert.ok(Math.abs(done.lid - 0.5) < 0.05, "the lid half closes for done");
  assert.ok(done.closure >= done.lid);
});

test("needing the user adds an attention bob that reduced motion leaves out", () => {
  const bob = (reduced) => {
    const engine = motion.createLenteEngine({ random: sequence(0.5), state: "approval", reduced });
    let min = 0;
    let max = 0;
    for (let t = 0; t < 4000; t += 16) {
      const frame = engine.step(16);
      min = Math.min(min, frame.bobY);
      max = Math.max(max, frame.bobY);
    }
    return { min, max };
  };
  const full = bob(false);
  assert.ok(full.min < -0.005 && full.max <= 0);
  const calm = bob(true);
  assert.equal(calm.min, 0);
  assert.equal(calm.max, 0);
});

test("the character is never static: it breathes with every setting, and the orbit turns while working", () => {
  for (const reduced of [false, true]) {
    const engine = motion.createLenteEngine({ random: sequence(0.5), reduced });
    const values = new Set();
    for (let t = 0; t < 4000; t += 16) values.add(engine.step(16).breathe.toFixed(4));
    assert.ok(values.size > 20, `reduced=${reduced}: breathing changes the frame`);
  }
  const working = motion.createLenteEngine({ random: sequence(0.5), state: "working" });
  const a = run(working, 1000).orbitAngle;
  const b = run(working, 1000).orbitAngle;
  assert.ok(b > a);
});

test("changing the motion preference at run time re-tunes the same engine", () => {
  const engine = motion.createLenteEngine({ random: sequence(0.5) });
  engine.setReduced(true);
  engine.setGaze({ x: 1, y: 0 });
  const calm = run(engine, 2000).gazeX;
  engine.setReduced(false);
  assert.ok(run(engine, 2000).gazeX > calm);
});
