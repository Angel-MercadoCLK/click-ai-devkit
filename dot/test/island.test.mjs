import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { test } from "node:test";

process.env.PORT = "3011";

const look = await import("../app/mascot/lente-look.ts");
const island = await import("../app/mascot/island-state.ts");
const choices = await import("../app/mascot/agent-choices.ts");
const windowMode = await import("../lib/window-mode.ts");

const read = (path) => readFile(new URL(`../${path}`, import.meta.url), "utf8");
const session = (overrides = {}) => ({ id: "s1", project: "alpha", state: "trabajando", action: "Editando agent-card.tsx +12 −3", updatedAt: 10, ...overrides });
const pending = (overrides = {}) => ({ id: "p1", kind: "claude-code", tool: "Bash", summary: "npm test", detail: "npm test", project: "alpha", alwaysOffered: true, questions: null, ...overrides });

// --- the Lente character ---------------------------------------------------------------------------------------

test("Lente has four states with the canvas palettes: pearl, working blue, needs-you amber and done green", () => {
  assert.deepEqual([...look.LENTE_STATES], ["idle", "working", "approval", "done"]);
  assert.equal(look.LENTE_PALETTES.idle.mid, "#DCE5F3");
  assert.equal(look.LENTE_PALETTES.working.mid, "#7FA6FF");
  assert.equal(look.LENTE_PALETTES.approval.mid, "#FFC864");
  assert.equal(look.LENTE_PALETTES.done.mid, "#7FE0A6");
  for (const state of look.LENTE_STATES) {
    for (const key of ["hi", "mid", "lo", "glow", "badge"]) assert.ok(look.LENTE_PALETTES[state][key], `${state}.${key}`);
    assert.equal(look.LENTE_PALETTES[state].badge, "#FFB340");
  }
});

test("only working shows the dashed orbit ring, only approval the amber badge, and only done half-closes the lid", () => {
  assert.deepEqual(look.lenteLook("idle"), { orbit: false, badge: false, lid: "open" });
  assert.deepEqual(look.lenteLook("working"), { orbit: true, badge: false, lid: "open" });
  assert.deepEqual(look.lenteLook("approval"), { orbit: false, badge: true, lid: "open" });
  assert.deepEqual(look.lenteLook("done"), { orbit: false, badge: false, lid: "half" });
});

test("an unknown state falls back to idle instead of breaking the character", () => {
  assert.equal(look.readLenteState("sleepy"), "idle");
  assert.equal(look.readLenteState(undefined), "idle");
  for (const state of look.LENTE_STATES) assert.equal(look.readLenteState(state), state);
});

test("the character is drawn in code and alive: an SVG component driven by one animation-frame loop, with a gentler profile for reduced motion and no image assets", async () => {
  const source = await read("app/mascot/lente.tsx");
  assert.match(source, /<svg/);
  assert.match(source, /export function Lente/);
  assert.doesNotMatch(source, /<img|\.png|\.jpg|\.svg"|url\(\s*["']?http/i);
  assert.match(source, /requestAnimationFrame/);
  assert.match(source, /cancelAnimationFrame/, "the loop stops when the character goes away");
  assert.match(source, /prefers-reduced-motion: reduce/, "the profile follows the Windows animation setting, live");
  assert.match(source, /subscribeCursor/);
  const css = await read("app/globals.css");
  for (const selector of [".lente-body", ".lente-eye", ".lente-iris", ".lente-orbit", ".lente-badge"]) assert.ok(css.includes(selector), selector);
  // The old CSS loops were gated off by "Animation effects" in Windows, which left the character static. Nothing of that is left.
  assert.doesNotMatch(css, /@keyframes lente-(breathe|blink|orbit|nudge)/);
  assert.doesNotMatch(css, /animation:[^;]*lente-(breathe|blink|orbit|nudge)/);
  // What stays in CSS is the capture ring's pulse, and it is the only lente animation, gated like before.
  const animated = css.split("@media").filter((block) => /animation:[^;]*lente-/.test(block));
  assert.equal(animated.length, 1);
  assert.match(animated[0], /^\s*\(prefers-reduced-motion: no-preference\)/);
  assert.match(animated[0], /lente-pulse/);
  assert.doesNotMatch(css, /\.lente[^{]*\{[^}]*transition:\s*all/);
});

test("the character moves with transforms and opacity only, so it never lays out again", async () => {
  const source = await read("app/mascot/lente.tsx");
  assert.doesNotMatch(source, /\.style\.(width|height|top|left|margin|padding)\b/);
  for (const property of ['"transform"', '"opacity"']) assert.ok(source.includes(property), property);
  assert.match(source, /will-change|translate3d/);
});

test("the old Spark is gone and nothing imports it", async () => {
  await assert.rejects(read("app/mascot/spark.tsx"));
  assert.doesNotMatch(await read("app/mascot/mascot.tsx"), /Spark|spark/);
  assert.doesNotMatch(await read("app/globals.css"), /\.spark|spark-/);
});

// --- what the island shows -------------------------------------------------------------------------------------

test("the character follows the agent: needing the user wins, then working, listening works like working, then done", () => {
  assert.equal(island.lenteStateFor({ attention: "needs", listening: false, sessions: [] }), "approval");
  assert.equal(island.lenteStateFor({ attention: "needs", listening: true, sessions: [] }), "approval");
  assert.equal(island.lenteStateFor({ attention: "working", listening: false, sessions: [] }), "working");
  assert.equal(island.lenteStateFor({ attention: "idle", listening: true, sessions: [] }), "working");
  assert.equal(island.lenteStateFor({ attention: "idle", listening: false, sessions: [session({ state: "listo" })] }), "done");
  assert.equal(island.lenteStateFor({ attention: "idle", listening: false, sessions: [] }), "idle");
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

test("the session count and the in-progress summary are short Spanish labels", () => {
  assert.equal(island.sessionCountLabel(0), "");
  assert.equal(island.sessionCountLabel(1), "1 sesión");
  assert.equal(island.sessionCountLabel(2), "2 sesiones");
  assert.equal(island.inProgressLabel([]), "Sin sesiones");
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
