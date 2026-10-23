import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { test } from "node:test";

process.env.PORT = "3011";

const look = await import("../app/mascot/orbi-look.ts");
const island = await import("../app/mascot/island-state.ts");
const choices = await import("../app/mascot/agent-choices.ts");
const windowMode = await import("../lib/window-mode.ts");

const read = (path) => readFile(new URL(`../${path}`, import.meta.url), "utf8");
const session = (overrides = {}) => ({ id: "s1", project: "alpha", state: "trabajando", action: "Editando agent-card.tsx +12 −3", updatedAt: 10, ...overrides });
const pending = (overrides = {}) => ({ id: "p1", kind: "claude-code", tool: "Bash", summary: "npm test", detail: "npm test", project: "alpha", alwaysOffered: true, questions: null, ...overrides });

// --- the Orbi character ----------------------------------------------------------------------------------------

test("Orbi has seven states, each with a palette: the planet's own colors, muted when worried and dimmed when asleep", () => {
  assert.deepEqual([...look.ORBI_STATES], ["idle", "working", "needs", "done", "error", "listening", "sleeping"]);
  assert.equal(look.ORBI_PALETTES.idle.b0, "#B8FFF0");
  assert.equal(look.ORBI_PALETTES.idle.b1, "#3FD2C7");
  assert.equal(look.ORBI_PALETTES.idle.b2, "#4A6CFF");
  assert.equal(look.ORBI_PALETTES.idle.b3, "#7B4DFF");
  assert.deepEqual([look.ORBI_PALETTES.idle.r0, look.ORBI_PALETTES.idle.r1, look.ORBI_PALETTES.idle.r2], ["#FFB86B", "#FF6FB5", "#A78BFF"]);
  for (const state of look.ORBI_STATES) {
    for (const key of ["b0", "b1", "b2", "b3", "glow", "r0", "r1", "r2"]) assert.ok(look.ORBI_PALETTES[state][key], `${state}.${key}`);
  }
  assert.notEqual(look.ORBI_PALETTES.error.b1, look.ORBI_PALETTES.idle.b1, "an error does not look like the others");
  assert.equal(look.ORBI_PALETTES.working.b1, look.ORBI_PALETTES.idle.b1, "working keeps the planet's colors");
});

test("each state shows only its own emotes: the badge, the hop, the sparks, the z, the rings, the sweat, the glowing ring", () => {
  const flags = (state) => {
    const { eyes, badge, hop, sparks, zzz, rings, sweat, worry, ringGlow, moon } = look.orbiLook(state, 76);
    return { eyes, badge, hop, sparks, zzz, rings, sweat, worry, ringGlow, moon };
  };
  const calm = { eyes: "open", badge: false, hop: false, sparks: false, zzz: false, rings: false, sweat: false, worry: false, ringGlow: false };
  assert.deepEqual(flags("idle"), { ...calm, moon: "normal" });
  assert.deepEqual(flags("working"), { ...calm, ringGlow: true, moon: "fast" });
  assert.deepEqual(flags("needs"), { ...calm, badge: true, hop: true, moon: "normal" });
  assert.deepEqual(flags("done"), { ...calm, eyes: "happy", sparks: true, moon: "normal" });
  assert.deepEqual(flags("error"), { ...calm, sweat: true, worry: true, moon: "slow" });
  assert.deepEqual(flags("listening"), { ...calm, rings: true, moon: "normal" });
  assert.deepEqual(flags("sleeping"), { ...calm, eyes: "closed", zzz: true, moon: "slow" });
});

test("an unknown state falls back to idle instead of breaking the character", () => {
  assert.equal(look.readOrbiState("sleepy"), "idle");
  assert.equal(look.readOrbiState(undefined), "idle");
  for (const state of look.ORBI_STATES) assert.equal(look.readOrbiState(state), state);
});

test("below 40 px the drawing is simplified to stay legible on the black island: no sparks or z, a thicker ring, a bigger moon and bigger eyes", () => {
  assert.equal(look.SMALL_ORBI_SIZE, 40);
  for (const size of [28, 34, 39]) {
    assert.equal(look.orbiLook("done", size).sparks, false, `${size}: no sparks`);
    assert.equal(look.orbiLook("sleeping", size).zzz, false, `${size}: no z`);
    assert.equal(look.orbiLook("needs", size).badge, true, `${size}: the badge stays`);
    assert.equal(look.orbiLook("listening", size).rings, true, `${size}: the rings stay`);
    assert.equal(look.orbiLook("done", size).eyes, "happy");
    assert.equal(look.orbiLook("idle", size).small, true);
  }
  for (const size of [40, 76, 96]) {
    assert.equal(look.orbiLook("done", size).sparks, true, `${size}: sparks`);
    assert.equal(look.orbiLook("sleeping", size).zzz, true, `${size}: z`);
    assert.equal(look.orbiLook("idle", size).small, false);
  }
  const small = look.orbiGeometry(34);
  const big = look.orbiGeometry(76);
  assert.ok(small.ring.width > big.ring.width && small.moon.r > big.moon.r && small.eyes.rx > big.eyes.rx && small.planetR > big.planetR);
  // In pixels at the pill's 34 px: a ring you can see, eyes you can read, a moon that is more than a speck.
  const px = (units) => (units * 34) / 120;
  assert.ok(px(small.ring.width) >= 2.2, "the ring is at least about 2 px thick");
  assert.ok(px(small.eyes.ry * 2) >= 6, "the eyes are at least 6 px tall");
  assert.ok(px(small.eyes.rx * 2) >= 4.5);
  assert.ok(px(small.moon.r * 2) >= 3, "the moon is at least 3 px");
  assert.ok(px(small.mouth.stroke) >= 0.9);
  assert.ok(look.orbiLook("idle", 28).geometry.eyes.rx === small.eyes.rx, "28 and 34 share the small drawing");
});

test("everything Orbi draws stays inside the capture halo, and the halo fits the 120 x 120 box, at both sizes", () => {
  // The farthest a tilted ellipse (plus its stroke or its moon) gets from the planet's center.
  const farthest = ({ cx, cy, rx, ry, tilt }, extra, center) => {
    const angle = (tilt * Math.PI) / 180;
    let far = 0;
    for (let t = 0; t < 360; t += 2) {
      const x = rx * Math.cos((t * Math.PI) / 180);
      const y = ry * Math.sin((t * Math.PI) / 180);
      const px = cx + x * Math.cos(angle) - y * Math.sin(angle);
      const py = cy + x * Math.sin(angle) + y * Math.cos(angle);
      far = Math.max(far, Math.hypot(px - center.cx, py - center.cy));
    }
    return far + extra;
  };
  for (const size of [28, 34, 76, 96]) {
    const g = look.orbiGeometry(size);
    const inner = g.halo.r - g.halo.stroke / 2;
    const ring = farthest(g.ring, g.ring.width / 2, g);
    const moon = farthest(g.moon, g.moon.r * 1.1, g);
    assert.ok(ring < inner, `${size}: the ring stays inside the halo (${ring.toFixed(1)} < ${inner})`);
    assert.ok(moon < inner, `${size}: the moon stays inside the halo (${moon.toFixed(1)} < ${inner})`);
    assert.ok(g.planetR < inner);
    assert.ok(g.halo.r + g.halo.stroke / 2 <= 60 + 1e-9, `${size}: the halo fits the box`);
    assert.ok(g.cy + g.halo.r + g.halo.stroke / 2 <= 120 && g.cy - g.halo.r - g.halo.stroke / 2 >= 0);
  }
  assert.ok((look.orbiGeometry(34).halo.stroke * 34) / 120 >= 1.8, "the halo is at least about 2 px thick at the pill's size");
});

test("Orbi falls asleep after about three minutes without anything going on and wakes with any activity", () => {
  assert.equal(look.SLEEP_AFTER_MS, 3 * 60 * 1000);
  const base = { quiet: true, lastActivityAt: 1000 };
  assert.deepEqual(look.sleepStatus({ ...base, now: 1000 }), { asleep: false, checkInMs: look.SLEEP_AFTER_MS });
  assert.deepEqual(look.sleepStatus({ ...base, now: 1000 + 100_000 }), { asleep: false, checkInMs: look.SLEEP_AFTER_MS - 100_000 });
  assert.deepEqual(look.sleepStatus({ ...base, now: 1000 + look.SLEEP_AFTER_MS }), { asleep: true, checkInMs: null });
  assert.equal(look.sleepStatus({ ...base, now: 1000 + 10 * look.SLEEP_AFTER_MS }).asleep, true);
  // Activity (hover, click, anything going on) moves the start and wakes it.
  assert.equal(look.sleepStatus({ quiet: true, lastActivityAt: 1000 + look.SLEEP_AFTER_MS, now: 1000 + look.SLEEP_AFTER_MS + 5 }).asleep, false);
  // While something is going on it never sleeps, however long ago the last activity was.
  assert.deepEqual(look.sleepStatus({ quiet: false, lastActivityAt: 0, now: 10 * look.SLEEP_AFTER_MS }), { asleep: false, checkInMs: null });
});

test("the character is drawn in code and alive: an SVG component driven by one animation-frame loop that stops when the island hides, with a gentler profile for reduced motion and no image assets", async () => {
  const source = await read("app/mascot/orbi.tsx");
  assert.match(source, /<svg/);
  assert.match(source, /export function Orbi/);
  assert.doesNotMatch(source, /<img|\.png|\.jpg|\.svg"|url\(\s*["']?http/i);
  assert.match(source, /requestAnimationFrame/);
  assert.match(source, /cancelAnimationFrame/, "the loop stops when the character goes away");
  assert.match(source, /visibilitychange/, "and while the page is hidden");
  assert.match(source, /prefers-reduced-motion: reduce/, "the profile follows the Windows animation setting, live");
  assert.match(source, /subscribeCursor/);
  assert.match(source, /createOrbiEngine/);
  const css = await read("app/globals.css");
  assert.match(css, /\.orbi\s*\{/);
  assert.doesNotMatch(css, /@keyframes orbi-(float|boing|hop|moon|blink|spark|zzz|ring)/, "the motion lives in the engine, so the Windows animation setting cannot freeze it");
  assert.doesNotMatch(css, /animation:[^;]*orbi-(float|boing|hop|moon|blink|spark|zzz|ring)/);
  // What stays in CSS is the capture halo's pulse, and it is the only orbi animation, gated like before.
  const animated = css.split("@media").filter((block) => /animation:[^;]*orbi-/.test(block));
  assert.equal(animated.length, 1);
  assert.match(animated[0], /^\s*\(prefers-reduced-motion: no-preference\)/);
  assert.match(animated[0], /orbi-pulse/);
  assert.doesNotMatch(css, /\.orbi[^{]*\{[^}]*transition:\s*all/);
});

test("the character moves with transforms and opacity only, so it never lays out again", async () => {
  const source = await read("app/mascot/orbi.tsx");
  assert.doesNotMatch(source, /\.style\.(width|height|top|left|margin|padding)\b/);
  for (const property of ['"transform"', '"opacity"']) assert.ok(source.includes(property), property);
  assert.match(source, /will-change|translate3d/);
});

test("the capture halo is drawn only while the screen is shared or paused, red or amber, and never replaces the ring", async () => {
  const source = await read("app/mascot/orbi.tsx");
  assert.match(source, /ring === "live" \|\| ring === "paused"/);
  assert.match(source, /orbi-halo/);
  assert.match(source, /data-ring=\{ring\}/);
  const css = await read("app/globals.css");
  assert.match(css, /\.orbi-halo\s*\{[^}]*stroke:\s*#FF5F52/);
  assert.match(css, /\[data-ring="paused"\] \.orbi-halo\s*\{[^}]*stroke:\s*#FFB340/);
  // The halo is the capture signal; it is a separate element from Orbi's own gradient ring.
  assert.doesNotMatch(source, /className="orbi-halo"[^>]*url\(#/);
});

test("the old Spark and the Lente are gone and nothing imports them", async () => {
  for (const gone of ["app/mascot/spark.tsx", "app/mascot/lente.tsx", "app/mascot/lente-look.ts", "app/mascot/lente-motion.ts", "test/lente-motion.test.mjs"]) await assert.rejects(read(gone), gone);
  for (const file of ["app/mascot/mascot.tsx", "app/mascot/agent-card.tsx", "app/mascot/island-state.ts", "app/mascot/cursor-store.ts", "app/globals.css"]) {
    assert.doesNotMatch(await read(file), /lente|Lente|\.spark|spark-/, file);
  }
});

test("the packaged smoke test keeps finding what it looks for: the toggle, the menu and its labels, and the connect button", async () => {
  const source = await read("app/mascot/mascot.tsx");
  for (const label of ["Compartir mi pantalla", "Voz", "Mensaje", "Conectar con ChatGPT"]) assert.ok(source.includes(`>${label}<`) || source.includes(`"${label}"`) || source.includes(`{${JSON.stringify(label)}}`) || source.includes(label), label);
  assert.match(source, /<span>Compartir mi pantalla<\/span>/);
  assert.match(source, /<span>\{voice\.listening \? "Escuchando… \(cancelar\)" : "Voz"\}<\/span>/);
  assert.match(source, /<span>Mensaje<\/span>/);
  assert.match(source, /Conectar con ChatGPT/);
  assert.match(source, /className="mascot-root"/);
  assert.match(source, /role="button"/);
  assert.match(source, /aria-expanded=\{expanded\}/);
  assert.match(source, /className="mascot-menu island-card"/);
  assert.match(source, /className=\{`menu-button/);
  // Orbi is decoration: it adds nothing the smoke script could mistake for a button or a label.
  const orbi = await read("app/mascot/orbi.tsx");
  assert.match(orbi, /aria-hidden="true"/);
  assert.doesNotMatch(orbi, /role="button"|<button|aria-expanded|<text/);
});

test("the island shows Orbi at its three sizes and puts it to sleep and wakes it with the pointer", async () => {
  const source = await read("app/mascot/mascot.tsx");
  for (const size of [34, 28, 92]) assert.match(source, new RegExp(`<Orbi state=\\{orbiState\\} size=\\{${size}\\} ring=\\{view\\.ring\\} />`), `size ${size}`);
  assert.match(source, /useSleep/);
  assert.match(source, /wake\(\)/);
  const hook = await read("app/mascot/use-sleep.ts");
  assert.match(hook, /sleepStatus/);
  assert.match(hook, /clearTimeout/, "the timer is cleaned up");
  const card = await read("app/mascot/agent-card.tsx");
  assert.match(card, /<Orbi state="needs" size=\{96\} \/>/);
});

// --- what the island shows -------------------------------------------------------------------------------------

test("the character follows the agent: needing the user wins, then listening, working, a failure, a finish, sleeping, and idle otherwise", () => {
  const state = (overrides) => island.orbiStateFor({ attention: "idle", listening: false, ...overrides });
  assert.equal(state({ attention: "needs" }), "needs");
  assert.equal(state({ attention: "needs", listening: true }), "needs");
  assert.equal(state({ attention: "needs", failed: true, asleep: true }), "needs");
  assert.equal(state({ listening: true }), "listening");
  assert.equal(state({ attention: "working", listening: true }), "listening");
  assert.equal(state({ attention: "working" }), "working");
  assert.equal(state({ attention: "working", failed: true }), "working");
  assert.equal(state({ failed: true, finished: true }), "error");
  assert.equal(state({ finished: true }), "done");
  assert.equal(state({ asleep: true }), "sleeping");
  assert.equal(state({ asleep: true, finished: true }), "done", "news wakes it");
  assert.equal(state({}), "idle");
});

test("a background task that just failed or timed out is a failure worth showing; one already failed when first seen is not", () => {
  const task = (overrides = {}) => ({ id: "t1", project: "alpha", state: "running", latestLine: null, startedAt: 1, ...overrides });
  assert.equal(island.newlyFailed({ t1: "running" }, [task({ state: "failed" })]), true);
  assert.equal(island.newlyFailed({ t1: "running" }, [task({ state: "timed_out" })]), true);
  assert.equal(island.newlyFailed({ t1: "running" }, [task({ state: "done" })]), false);
  assert.equal(island.newlyFailed({ t1: "running" }, [task({ state: "cancelled" })]), false, "cancelled by the user is not an error");
  assert.equal(island.newlyFailed({}, [task({ state: "failed" })]), false, "already failed when first seen");
  assert.equal(island.newlyFailed({ t1: "failed" }, [task({ state: "failed" })]), false);
  assert.deepEqual(island.taskStates([task(), task({ id: "t2", state: "failed" })]), { t1: "running", t2: "failed" });
});

test("the island rests as the thin bar, becomes the pill when something is going on or the pointer is on it", () => {
  assert.equal(island.restingMode({ active: false, hovered: false }), "collapsed");
  assert.equal(island.restingMode({ active: false, hovered: true }), "compact");
  assert.equal(island.restingMode({ active: true, hovered: false }), "compact");
  assert.equal(island.restingMode({ active: true, hovered: true }), "compact");
});

test("something is going on while an agent works or needs the user, the screen is shared, Voz listens, or a session just finished", () => {
  const quiet = { attention: "idle", sharing: false, listening: false, finishedRecently: false };
  assert.equal(island.isActive(quiet), false);
  for (const change of [{ attention: "working" }, { attention: "needs" }, { sharing: true }, { listening: true }, { finishedRecently: true }]) {
    assert.equal(island.isActive({ ...quiet, ...change }), true, JSON.stringify(change));
  }
});

test("closing a card lands on the resting state, never flashes the thin bar while something is going on", () => {
  assert.equal(island.settleMode("collapsed", "compact"), "compact");
  assert.equal(island.settleMode("collapsed", "collapsed"), "collapsed");
  for (const mode of ["menu", "chat", "approval", "compact"]) assert.equal(island.settleMode(mode, "collapsed"), mode);
});

test("the resting size waits only until a new request's card is up, so a card put away never hides the pill", () => {
  // No request: the bar or pill follows what is going on.
  assert.equal(island.restingTarget({ mode: "collapsed", resting: "compact", pendingId: null, raisedFor: null }), "compact");
  assert.equal(island.restingTarget({ mode: "compact", resting: "compact", pendingId: null, raisedFor: null }), null);
  // A new request whose card is not up yet: leave the window to the card.
  assert.equal(island.restingTarget({ mode: "collapsed", resting: "compact", pendingId: "r1", raisedFor: null }), null);
  // The card was put away by hand while the screen is shared: the pill (with its capture ring) comes back.
  assert.equal(island.restingTarget({ mode: "collapsed", resting: "compact", pendingId: "r1", raisedFor: "r1" }), "compact");
  // Open cards and panels are never resized by the resting rule.
  for (const mode of ["approval", "menu", "chat"]) assert.equal(island.restingTarget({ mode, resting: "compact", pendingId: null, raisedFor: null }), null);
});

test("a card counts as raised only once the window is in the approval mode, so its arrival never races the pill", () => {
  // The commit where the request arrives: still collapsed, not raised, so the resting rule leaves the window alone.
  const arriving = island.raisedCardFor(null, "collapsed", "r1");
  assert.equal(arriving, null);
  assert.equal(island.restingTarget({ mode: "collapsed", resting: "compact", pendingId: "r1", raisedFor: arriving }), null);
  // The card is up; put away later, the pill comes back.
  const raised = island.raisedCardFor(arriving, "approval", "r1");
  assert.equal(raised, "r1");
  assert.equal(island.raisedCardFor(raised, "collapsed", "r1"), "r1");
});

test("an edit line splits into its words and the +N and −M counts", () => {
  assert.deepEqual(island.splitAction("Editando agent-card.tsx +12 −3"), { text: "Editando agent-card.tsx", added: 12, removed: 3 });
  assert.deepEqual(island.splitAction("Escribiendo notes.md +40"), { text: "Escribiendo notes.md", added: 40, removed: null });
  assert.deepEqual(island.splitAction("Ejecutando npm test"), { text: "Ejecutando npm test", added: null, removed: null });
  assert.deepEqual(island.splitAction(null), { text: "", added: null, removed: null });
  assert.deepEqual(island.splitAction("Editando a +b.ts +1 −0"), { text: "Editando a +b.ts", added: 1, removed: 0 });
});

test("the pill says what Claude Code is doing, with the latest working session first", () => {
  const base = { sessions: [], pending: null, listening: false, finishedRecently: false };
  assert.equal(island.compactLine({ ...base, pending: pending() }), "Dot necesita tu permiso");
  assert.equal(island.compactLine({ ...base, pending: pending({ questions: [{ question: "Q?", header: "H", multiSelect: false, options: [] }] }) }), "Dot necesita tu respuesta");
  assert.equal(
    island.compactLine({ ...base, sessions: [session({ id: "a", updatedAt: 1, action: "Leyó viejo.ts" }), session({ id: "b", updatedAt: 9 }), session({ id: "c", state: "listo", updatedAt: 99 })] }),
    "Editando agent-card.tsx +12 −3",
  );
  assert.equal(island.compactLine({ ...base, sessions: [session({ action: null })] }), "Trabajando en alpha");
  assert.equal(island.compactLine({ ...base, sessions: [session({ state: "te necesita" })] }), "Claude Code te necesita en alpha");
  assert.equal(island.compactLine({ ...base, listening: true }), "Escuchando…");
  assert.equal(island.compactLine({ ...base, sessions: [session({ state: "listo" })], finishedRecently: true }), "Listo en alpha");
  assert.equal(island.compactLine(base), "Dot");
});

test("the activity count and the in-progress summary are short Spanish labels", () => {
  assert.equal(island.activityCountLabel([]), "");
  assert.equal(island.activityCountLabel([session()]), "1 en curso");
  assert.equal(island.activityCountLabel([session(), session({ id: "b" })]), "2 en curso");
  assert.equal(island.inProgressLabel([]), "Todo tranquilo");
  assert.equal(island.inProgressLabel([session({ state: "listo" })]), "Todo listo");
  assert.equal(island.inProgressLabel([session(), session({ id: "b", state: "te necesita" }), session({ id: "c", state: "listo" })]), "2 en curso");
});

test("the dots show each session's state, at most five", () => {
  assert.deepEqual(island.sessionDots([session(), session({ id: "b", state: "listo" }), session({ id: "c", state: "te necesita" })]), ["trabajando", "listo", "te necesita"]);
  assert.equal(island.sessionDots(Array.from({ length: 9 }, (_, index) => session({ id: String(index) }))).length, 5);
});

test("a session that went from working to ready is news; nothing else is", () => {
  assert.equal(island.newlyFinished({ s1: "trabajando" }, [session({ state: "listo" })]), true);
  assert.equal(island.newlyFinished({ s1: "te necesita" }, [session({ state: "listo" })]), true);
  assert.equal(island.newlyFinished({ s1: "listo" }, [session({ state: "listo" })]), false);
  assert.equal(island.newlyFinished({}, [session({ state: "listo" })]), false, "a session that was already finished when the page first looked is not news");
  assert.equal(island.newlyFinished({ s1: "trabajando" }, [session()]), false);
  assert.deepEqual(island.sessionStates([session(), session({ id: "b", state: "listo" })]), { s1: "trabajando", b: "listo" });
});

test("the approval header names who is asking", () => {
  assert.equal(island.approvalTitle(pending()), "Dot necesita tu permiso");
  assert.equal(island.approvalTitle(pending({ kind: "dot-tool" })), "Dot necesita tu permiso");
  assert.equal(island.approvalTitle(pending({ questions: [{ question: "Q?", header: "H", multiSelect: false, options: [] }] })), "Dot necesita tu respuesta");
});

// --- keyboard: Y allows and N denies while an approval card is shown ------------------------------------------------

const ctx = (overrides = {}) => ({ pending: pending(), cardShown: true, busy: false, focused: true, ...overrides });
const key = (value, overrides = {}) => ({ key: value, ...overrides });

test("Y allows and N denies, in either case", () => {
  assert.equal(choices.approvalKeyChoice(key("y"), ctx()), "allow");
  assert.equal(choices.approvalKeyChoice(key("Y"), ctx()), "allow");
  assert.equal(choices.approvalKeyChoice(key("n"), ctx()), "deny");
  assert.equal(choices.approvalKeyChoice(key("N"), ctx()), "deny");
  assert.equal(choices.approvalKeyChoice(key("y"), ctx({ pending: pending({ kind: "dot-tool" }) })), "allow");
});

test("the keys do nothing without a visible permission card, without focus, while busy, or on a question", () => {
  assert.equal(choices.approvalKeyChoice(key("y"), ctx({ pending: null })), null);
  assert.equal(choices.approvalKeyChoice(key("y"), ctx({ cardShown: false })), null);
  assert.equal(choices.approvalKeyChoice(key("y"), ctx({ focused: false })), null, "only while the island has focus");
  assert.equal(choices.approvalKeyChoice(key("n"), ctx({ busy: true })), null);
  const question = pending({ questions: [{ question: "Q?", header: "H", multiSelect: false, options: [{ label: "Yes", description: "" }] }] });
  assert.equal(choices.approvalKeyChoice(key("y"), ctx({ pending: question })), null, "a question is answered by choosing an option, not by Y");
});

test("the keys never fire while typing, with a modifier, held down, or for any other key", () => {
  for (const tagName of ["INPUT", "TEXTAREA", "SELECT"]) assert.equal(choices.approvalKeyChoice(key("y", { target: { tagName } }), ctx()), null, tagName);
  assert.equal(choices.approvalKeyChoice(key("y", { target: { tagName: "DIV", isContentEditable: true } }), ctx()), null);
  assert.equal(choices.approvalKeyChoice(key("y", { target: { tagName: "BUTTON" } }), ctx()), "allow", "a focused button is not a text field");
  for (const modifier of ["ctrlKey", "metaKey", "altKey"]) assert.equal(choices.approvalKeyChoice(key("y", { [modifier]: true }), ctx()), null, modifier);
  assert.equal(choices.approvalKeyChoice(key("y", { repeat: true }), ctx()), null);
  for (const other of ["a", "Enter", "Escape", " ", "yes", "Tab"]) assert.equal(choices.approvalKeyChoice(key(other), ctx()), null, other);
});

test("the page wires the keys to the decision only while the card is shown, and adds no global shortcut", async () => {
  const mascot = await read("app/mascot/mascot.tsx");
  assert.match(mascot, /approvalKeyChoice\(/);
  assert.match(mascot, /document\.hasFocus\(\)/);
  const main = await read("desktop/main.mts");
  assert.equal((main.match(/globalShortcut\.register/g) ?? []).length, 1, "only Ctrl+Shift+Space is global");
  const card = await read("app/mascot/agent-card.tsx");
  assert.match(card, /<kbd[^>]*>N<\/kbd>/);
  assert.match(card, /<kbd[^>]*>Y<\/kbd>/);
});

// --- the page is the island ------------------------------------------------------------------------------------

test("the smoke test's selectors still find the island: the only role=button, collapsed, and the labels in the menu buttons", async () => {
  const mascot = await read("app/mascot/mascot.tsx");
  assert.equal((mascot.match(/role="button"/g) ?? []).length, 1);
  assert.match(mascot, /aria-expanded=\{mode !== "collapsed" && mode !== "compact" \? true : false\}|aria-expanded=\{expanded\}/);
  assert.match(mascot, /className="mascot-root"|className=\{?["`]mascot-root/);
  assert.match(mascot, /className="mascot-menu/);
  for (const label of ["Compartir mi pantalla", "Voz", "Mensaje", "Conectar con ChatGPT"]) assert.ok(mascot.includes(label), label);
  // The Mensaje button's text is exactly "Mensaje": the smoke matches textContent.trim() === 'Mensaje' inside `.mascot-menu button`.
  assert.match(mascot, /<span>Mensaje<\/span>/);
});

test("the island is fixed: no dragging, no drag region, and the only pointer work is hovering the bar and clicking", async () => {
  const mascot = await read("app/mascot/mascot.tsx");
  assert.doesNotMatch(mascot, /setPointerCapture|dragTarget|exceedsDragThreshold|moveTo/);
  assert.match(mascot, /onPointerEnter/);
  assert.match(mascot, /onPointerLeave/);
  assert.equal(windowMode.exceedsDragThreshold, undefined);
  assert.equal(windowMode.dragTarget, undefined);
  assert.doesNotMatch(await read("app/globals.css"), /app-region/);
});

test("the capture indicator stays on screen in every state the island can be in while the screen is shared", async () => {
  const mascot = await read("app/mascot/mascot.tsx");
  // The pill and the expanded head both carry the sharing text and the pause control; the bar is never rest while sharing.
  assert.match(mascot, /className=\{`island-capture/);
  assert.match(mascot, /view\.sharingText/);
  assert.match(mascot, /view\.pause/);
  assert.match(mascot, /sharing/);
  assert.equal(island.isActive({ attention: "idle", sharing: true, listening: false, finishedRecently: false }), true);
  assert.match(mascot, /isActive\(/);
  assert.match(mascot, /restingMode\(/);
});

test("fonts are local stacks only: no network font in the markup or the styles", async () => {
  const css = await read("app/globals.css");
  const layout = await read("app/layout.tsx");
  for (const [name, source] of [["css", css], ["layout", layout], ["mascot", await read("app/mascot/mascot.tsx")]]) {
    assert.doesNotMatch(source, /googleapis|gstatic|fonts\.com|@import\s+url|<link[^>]+stylesheet/i, name);
  }
  assert.match(css, /"Segoe UI Variable"/);
  assert.match(css, /"Segoe UI"/);
  assert.match(css, /"Cascadia Mono"/);
  assert.match(css, /Consolas/);
  assert.doesNotMatch(layout, /next\/font/);
});

test("the island uses the canvas tokens: black island with radius 32 below, cards #141519 with radius 24, white primary pills", async () => {
  const css = await read("app/globals.css");
  for (const token of ["#000000", "#141519", "rgba(255, 255, 255, 0.07)", "#F5F5F7", "#A1A1AA", "#7FA6FF", "#FFB340", "#FFC864", "#7FE0A6", "#FF8A7A"]) {
    assert.ok(css.toLowerCase().includes(token.toLowerCase()), token);
  }
  assert.match(css, /border-radius:\s*0 0 32px 32px/);
  assert.match(css, /border-radius:\s*0 0 22px 22px/);
  assert.match(css, /border-radius:\s*24px/);
});

test("the packaged smoke script is untouched and still names the selectors the island keeps", async () => {
  const script = await readFile(new URL("../../scripts/dot-package-smoke.ps1", import.meta.url), "utf8");
  assert.ok(script.includes(`.mascot-root [role="button"][aria-expanded="false"]`));
  assert.ok(script.includes(".mascot-menu button"));
  for (const label of ["Compartir mi pantalla", "Voz", "Mensaje", "Conectar con ChatGPT"]) assert.ok(script.includes(label), label);
});
