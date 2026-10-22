import assert from "node:assert/strict";
import { spawn } from "node:child_process";
import { chmod, lstat, mkdir, mkdtemp, rm, rmdir, symlink, unlink } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { pathToFileURL } from "node:url";
import test from "node:test";
import { createLocalJWKSet, generateKeyPair, exportJWK, SignJWT } from "jose";

const temp = await mkdtemp(join(tmpdir(), "click-dot-auth-test-"));
process.env.CLICK_DOT_AUTH_FILE = join(temp, "private", "auth.json");
const store = await import("../lib/auth-store.ts");
const siwc = await import("../lib/siwc.ts");
const startRoute = await import("../app/api/auth/start/route.ts");
const statusRoute = await import("../app/api/auth/status/route.ts");
const disconnectRoute = await import("../app/api/auth/disconnect/route.ts");
const callbackRoute = await import("../app/auth/callback/route.ts");
const { privateKey, publicKey } = await generateKeyPair("RS256");
const jwk = await exportJWK(publicKey);
jwk.kid = "fixture-key";
jwk.alg = "RS256";
const localJwks = createLocalJWKSet({ keys: [jwk] });

test("builds a loopback authorization request with PKCE and a stable install identity", async () => {
  process.env.PORT = "3003";
  const first = await siwc.beginAuthorization();
  const second = await siwc.beginAuthorization();
  const url = new URL(first.url);
  assert.equal(url.origin, "https://auth.openai.com");
  assert.equal(url.searchParams.get("client_id"), "dynamic_agent_client");
  assert.equal(url.searchParams.get("redirect_uri"), "http://127.0.0.1:3003/auth/callback");
  assert.equal(url.searchParams.get("code_challenge_method"), "S256");
  assert.equal(url.searchParams.get("agent_name_hint"), "Click Dot");
  assert.match(url.searchParams.get("ext_agent_host_id"), /^urn:uuid:/);
  assert.equal(url.searchParams.get("ext_agent_host_id"), new URL(second.url).searchParams.get("ext_agent_host_id"));
  assert.notEqual(url.searchParams.get("state"), new URL(second.url).searchParams.get("state"));
  assert.match(url.searchParams.get("scope"), /chatgpt\.tokens\.use\.direct/);
  assert.equal(url.searchParams.get("agent_name_hint"), "Click Dot");
  assert.equal(await store.saveIssuedClientId("issued-client", 0), true);
  const returning = new URL((await siwc.beginAuthorization()).url);
  assert.equal(returning.searchParams.get("client_id"), "issued-client");
  assert.equal(returning.searchParams.has("agent_name_hint"), false);
});

test("uses the issued registration for a no-secret token exchange and trusts token scopes", async () => {
  const attempt = { nonce: "nonce-fixture", verifier: "verifier-fixture", state: "state", expiresAt: Date.now() + 1000,
    redirectUri: "http://127.0.0.1:3003/auth/callback", expectedClientId: "issued-client", generation: 0 };
  // click:allow-secret synthetic test data; not real personal data.
  const idToken = await new SignJWT({ nonce: attempt.nonce, email: "person@example.test", sub: "subject-1" })
    .setProtectedHeader({ alg: "RS256", kid: "fixture-key" })
    .setIssuer("https://auth.openai.com")
    .setAudience("issued-client")
    .setIssuedAt()
    .setExpirationTime("5m")
    .sign(privateKey);
  let requestBody;
  const connection = await siwc.exchangeAuthorizationCode("fixture-code", attempt, "issued-client", async (_url, init) => {
    requestBody = new URLSearchParams(init.body);
    return Response.json({
      access_token: "synthetic-access-token",
      refresh_token: "synthetic-refresh-token",
      id_token: idToken,
      scope: "openid chatgpt.tokens.use.direct",
      expires_in: 300,
    });
  }, localJwks);
  assert.equal(requestBody.get("client_id"), "issued-client");
  assert.equal(requestBody.has("client_secret"), false);
  assert.equal(requestBody.get("resource"), "https://api.openai.com/v1");
  // click:allow-secret synthetic test data; not real personal data.
  assert.equal(connection.email, "person@example.test");
  assert.equal(connection.subject, "subject-1");
  assert.equal(connection.issuer, "https://auth.openai.com");
  assert.equal(siwc.hasDirectUsageScope(connection), true);
  let contactedProvider = false;
  await assert.rejects(siwc.exchangeAuthorizationCode("fixture", { ...attempt, expectedClientId: "dynamic_agent_client" }, "dynamic_agent_client", async () => {
    contactedProvider = true;
    return Response.json({});
  }), /registration is invalid/);
  assert.equal(contactedProvider, false);

  for (const invalidLifetime of [undefined, 0, "not-a-number"]) {
    await assert.rejects(siwc.exchangeAuthorizationCode("fixture", attempt, "issued-client", async () => Response.json({
      access_token: "access", id_token: idToken, scope: "chatgpt.tokens.use.direct", expires_in: invalidLifetime,
    }), localJwks), /credential lifetime/);
  }
});

test("rejects invalid nonce, issuer, audience, expiry, and signature", async (t) => {
  async function sign(claims, key = privateKey) {
    return new SignJWT({ sub: "subject-1", ...claims }).setProtectedHeader({ alg: "RS256", kid: "fixture-key" })
      .setIssuer("https://auth.openai.com").setAudience("client")
      .setIssuedAt().setExpirationTime("5m").sign(key);
  }
  const otherPair = await generateKeyPair("RS256");
  const invalid = [
    ["nonce", await sign({ nonce: "wrong" })],
    ["issuer", await new SignJWT({ nonce: "n" }).setProtectedHeader({ alg: "RS256", kid: "fixture-key" }).setIssuer("https://wrong.example").setAudience("client").setIssuedAt().setExpirationTime("5m").sign(privateKey)],
    ["audience", await new SignJWT({ nonce: "n" }).setProtectedHeader({ alg: "RS256", kid: "fixture-key" }).setIssuer("https://auth.openai.com").setAudience("other").setIssuedAt().setExpirationTime("5m").sign(privateKey)],
    ["expired", await new SignJWT({ nonce: "n" }).setProtectedHeader({ alg: "RS256", kid: "fixture-key" }).setIssuer("https://auth.openai.com").setAudience("client").setIssuedAt().setExpirationTime("-10s").sign(privateKey)],
    ["signature", await sign({ nonce: "n" }, otherPair.privateKey)],
    ["missing-subject", await new SignJWT({ nonce: "n" }).setProtectedHeader({ alg: "RS256", kid: "fixture-key" }).setIssuer("https://auth.openai.com").setAudience("client").setIssuedAt().setExpirationTime("5m").sign(privateKey)],
    ["missing-expiry", await new SignJWT({ nonce: "n", sub: "subject-1" }).setProtectedHeader({ alg: "RS256", kid: "fixture-key" }).setIssuer("https://auth.openai.com").setAudience("client").setIssuedAt().sign(privateKey)],
    ["missing-issue-time", await new SignJWT({ nonce: "n", sub: "subject-1" }).setProtectedHeader({ alg: "RS256", kid: "fixture-key" }).setIssuer("https://auth.openai.com").setAudience("client").setExpirationTime("5m").sign(privateKey)],
    ["wrong-algorithm", await new SignJWT({ nonce: "n", sub: "subject-1" }).setProtectedHeader({ alg: "HS256" }).setIssuer("https://auth.openai.com").setAudience("client").setIssuedAt().setExpirationTime("5m").sign(new Uint8Array(32).fill(7))],
  ];
  for (const [name, token] of invalid) {
    await t.test(name, async () => assert.rejects(siwc.verifyIdToken(token, { nonce: "n", clientId: "client", jwks: localJwks })));
  }
});

test("consumes pending attempts once and rejects expired attempts", async () => {
  const created = await store.createAttempt("once", { state: "state", nonce: "nonce", verifier: "verifier", expiresAt: Date.now() + 5000, redirectUri: "http://127.0.0.1:3003/auth/callback" }, "dynamic_agent_client");
  const attempt = created.attempt;
  assert.deepEqual(await store.consumeAttempt("once"), attempt);
  assert.equal(await store.consumeAttempt("once"), undefined);
  await store.createAttempt("expired", { ...attempt, expiresAt: Date.now() - 1 }, "dynamic_agent_client");
  assert.equal(await store.consumeAttempt("expired"), undefined);
});

test("keeps the auth file private and rejects unsafe storage paths", async () => {
  const path = await store.getStoragePathForTests();
  const info = await lstat(path);
  assert.equal(info.mode & 0o777, 0o600);
  assert.equal(info.isSymbolicLink(), false);
  const linkPath = join(temp, "link.json");
  const linkDir = join(temp, "link-dir");
  const target = join(temp, "target.json");
  await import("node:fs/promises").then(({ writeFile }) => writeFile(target, "{}", { mode: 0o600 }));
  await symlink(target, linkPath);
  process.env.CLICK_DOT_AUTH_FILE = linkPath;
  await assert.rejects(store.getConnection());
  await symlink(join(temp, "private"), linkDir);
  process.env.CLICK_DOT_AUTH_FILE = join(linkDir, "auth.json");
  await assert.rejects(store.getConnection(), /unsafe directory/);
  process.env.CLICK_DOT_AUTH_FILE = path;
  await chmod(join(temp, "private"), 0o755);
  await assert.rejects(store.getConnection(), /not private/);
  await chmod(join(temp, "private"), 0o700);
});

test("does not treat identity-only grants or expired access as plan access", () => {
  assert.equal(siwc.hasDirectUsageScope({ scopes: ["openid", "email"], expiresAt: Date.now() + 60_000 }), false);
  assert.equal(siwc.hasDirectUsageScope({ scopes: ["chatgpt.tokens.use.direct"], expiresAt: Date.now() - 1 }), false);
});

test("requires the exact local origin to start or disconnect and returns no credentials in status", async () => {
  const rejected = await startRoute.POST(new Request("http://127.0.0.1:3003/api/auth/start", {
    method: "POST", headers: { host: "127.0.0.1:3003", origin: "http://localhost:3003" },
  }));
  assert.equal(rejected.status, 403);

  const started = await startRoute.POST(new Request("http://127.0.0.1:3003/api/auth/start", {
    method: "POST", headers: { host: "127.0.0.1:3003", origin: "http://127.0.0.1:3003" },
  }));
  assert.equal(started.status, 200);
  assert.match(started.headers.get("set-cookie"), /HttpOnly/);
  assert.match(started.headers.get("set-cookie"), /SameSite=Lax/);
  assert.equal((await started.json()).url.startsWith("https://auth.openai.com/"), true);

  const statusAttempt = await store.createAttempt("status-auth", { state: "s", nonce: "n", verifier: "v", expiresAt: Date.now() + 5000,
    redirectUri: "http://127.0.0.1:3003/auth/callback" }, "dynamic_agent_client");
  await store.saveConnection({ clientId: statusAttempt.attempt.expectedClientId, issuer: "https://auth.openai.com", subject: "subject-1", accessToken: "secret-access", refreshToken: "secret-refresh", idToken: "secret-id", scopes: ["chatgpt.tokens.use.direct"], expiresAt: Date.now() + 60_000 }, statusAttempt.attempt);
  const status = await statusRoute.GET(new Request("http://127.0.0.1:3003/api/auth/status", { headers: { host: "127.0.0.1:3003" } }));
  const body = await status.json();
  assert.deepEqual(body, { connected: true, email: null });
  assert.doesNotMatch(JSON.stringify(body), /secret/);

  const disconnect = await disconnectRoute.POST(new Request("http://127.0.0.1:3003/api/auth/disconnect", {
    method: "POST", headers: { host: "127.0.0.1:3003", origin: "http://127.0.0.1:3003" },
  }));
  assert.equal(disconnect.status, 200);
  assert.equal(await store.getConnection(), undefined);
});

test("serializes concurrent one-use consumes and prevents pending auth from restoring after disconnect", async () => {
  const created = await store.createAttempt("race", {
    state: "race-state", nonce: "nonce", verifier: "verifier", expiresAt: Date.now() + 5000,
    redirectUri: "http://127.0.0.1:3003/auth/callback",
  }, "dynamic_agent_client");
  const consumed = await Promise.all([store.consumeAttempt("race"), store.consumeAttempt("race")]);
  assert.equal(consumed.filter(Boolean).length, 1);

  const connection = { clientId: "client", issuer: "https://auth.openai.com", subject: "original-subject",
    accessToken: "existing-access", idToken: "existing-id", scopes: ["chatgpt.tokens.use.direct"], expiresAt: Date.now() + 60_000 };
  connection.clientId = created.attempt.expectedClientId;
  await store.saveConnection(connection, created.attempt);
  const loginAttempt = await store.createAttempt("login-before-logout", {
    state: "state", nonce: "nonce", verifier: "verifier", expiresAt: Date.now() + 5000,
    redirectUri: "http://127.0.0.1:3003/auth/callback",
  }, "dynamic_agent_client");
  await store.clearConnection();
  assert.equal(await store.saveConnection({ ...connection, subject: "new-subject" }, loginAttempt.attempt), false);
  assert.equal(await store.getConnection(), undefined);

  const attempts = await Promise.all(Array.from({ length: 12 }, (_, index) => store.createAttempt(`parallel-${index}`, {
    state: `state-${index}`, nonce: "nonce", verifier: "verifier", expiresAt: Date.now() + 5000,
    redirectUri: "http://127.0.0.1:3003/auth/callback",
  }, "dynamic_agent_client")));
  const recovered = await Promise.all(attempts.map((_, index) => store.consumeAttempt(`parallel-${index}`)));
  assert.equal(recovered.filter(Boolean).length, attempts.length);

  const raceAttempt = attempts[0].attempt;
  await Promise.all([
    store.saveConnection({ ...connection, subject: "race-subject" }, raceAttempt),
    store.clearConnection(),
  ]);
  assert.equal(await store.getConnection(), undefined);

  const startLogoutRace = await Promise.all([
    store.createAttempt("start-vs-logout", { state: "race", nonce: "nonce", verifier: "verifier",
      expiresAt: Date.now() + 5000, redirectUri: "http://127.0.0.1:3003/auth/callback" }, "dynamic_agent_client"),
    store.clearConnection(),
  ]);
  const pendingAfterRace = await store.consumeAttempt("start-vs-logout");
  assert.ok(pendingAfterRace === undefined || pendingAfterRace.generation >= startLogoutRace[0].attempt.generation);
});

test("a winning initial connection invalidates competing initial identities atomically", async () => {
  process.env.CLICK_DOT_AUTH_FILE = join(temp, "competing-logins", "auth.json");
  const redirectUri = "http://127.0.0.1:3003/auth/callback";
  const first = await store.createAttempt("initial-a", { state: "a", nonce: "a", verifier: "a", expiresAt: Date.now() + 5000, redirectUri }, "dynamic_agent_client");
  const second = await store.createAttempt("initial-b", { state: "b", nonce: "b", verifier: "b", expiresAt: Date.now() + 5000, redirectUri }, "dynamic_agent_client");
  assert.equal(first.attempt.expectedSubject, undefined);
  assert.equal(second.attempt.expectedSubject, undefined);
  assert.equal(await store.saveIssuedClientId("registered-client", first.attempt.generation), true);
  const connection = { clientId: "registered-client", issuer: "https://auth.openai.com", subject: "verified-subject-a",
    accessToken: "access-a", idToken: "id-a", scopes: ["chatgpt.tokens.use.direct"], expiresAt: Date.now() + 60_000 };
  assert.equal(await store.saveConnection(connection, first.attempt), true);
  assert.equal(await store.saveConnection({ ...connection, subject: "verified-subject-b", accessToken: "access-b" }, second.attempt), false);
  assert.equal((await store.getConnection()).subject, "verified-subject-a");
  assert.equal(await store.consumeAttempt("initial-b"), undefined);
});

test("status is read-only when no credential directory exists", async () => {
  const absent = join(temp, "not-created", "nested", "auth.json");
  process.env.CLICK_DOT_AUTH_FILE = absent;
  const response = await statusRoute.GET(new Request("http://127.0.0.1:3003/api/auth/status", { headers: { host: "127.0.0.1:3003" } }));
  assert.deepEqual(await response.json(), { connected: false });
  await assert.rejects(lstat(join(temp, "not-created")), { code: "ENOENT" });
  process.env.CLICK_DOT_AUTH_FILE = join(temp, "private", "auth.json");
});

test("serializes shared auth-file updates across independent local processes", async () => {
  const sharedFile = join(temp, "cross-process", "auth.json");
  const moduleUrl = pathToFileURL(new URL("../lib/auth-store.ts", import.meta.url).pathname).href;
  const script = `const store = await import(${JSON.stringify(moduleUrl)}); for (let i = 0; i < 20; i++) await store.createAttempt("child-" + process.argv[1] + "-" + i, { state: "s" + i, nonce: "n", verifier: "v", expiresAt: Date.now() + 60000, redirectUri: "http://127.0.0.1:3003/auth/callback" }, "dynamic_agent_client");`;
  const runWorker = (workerId) => new Promise((resolve, reject) => {
    const child = spawn(process.execPath, ["--experimental-strip-types", "--input-type=module", "-e", script, String(workerId)], {
      env: { ...process.env, CLICK_DOT_AUTH_FILE: sharedFile, NODE_ENV: "test" },
      stdio: ["ignore", "ignore", "pipe"],
    });
    let stderr = "";
    child.stderr.setEncoding("utf8").on("data", (chunk) => { stderr += chunk; });
    child.once("error", reject);
    child.once("exit", (code) => code === 0 ? resolve() : reject(new Error(`storage child failed (${code}): ${stderr}`)));
  });
  process.env.CLICK_DOT_AUTH_FILE = sharedFile;
  await Promise.all([runWorker(1), runWorker(2)]);
  const consumed = await Promise.all(Array.from({ length: 40 }, (_, index) =>
    store.consumeAttempt(`child-${index < 20 ? 1 : 2}-${index % 20}`)));
  assert.equal(consumed.filter(Boolean).length, 40);
  await assert.rejects(lstat(`${sharedFile}.lock`), { code: "ENOENT" });
});

test("fails closed on a held lock without reclaiming it, then recovers after explicit cleanup", async () => {
  const path = join(temp, "lock-timeout", "auth.json");
  process.env.CLICK_DOT_AUTH_FILE = path;
  process.env.CLICK_DOT_AUTH_LOCK_TIMEOUT_MS = "120";
  await mkdir(join(temp, "lock-timeout"), { mode: 0o700 });
  const lockPath = `${path}.lock`;
  await mkdir(lockPath, { mode: 0o700 });
  await assert.rejects(store.createAttempt("blocked", { state: "s", nonce: "n", verifier: "v", expiresAt: Date.now() + 60000,
    redirectUri: "http://127.0.0.1:3003/auth/callback" }, "dynamic_agent_client"), /timed out/);
  assert.equal((await lstat(lockPath)).isDirectory(), true);
  await rmdir(lockPath);
  const created = await store.createAttempt("recovered", { state: "s", nonce: "n", verifier: "v", expiresAt: Date.now() + 60000,
    redirectUri: "http://127.0.0.1:3003/auth/callback" }, "dynamic_agent_client");
  assert.equal((await store.consumeAttempt("recovered")).state, created.attempt.state);
  const symlinkLock = `${path}.lock`;
  await symlink(join(temp, "lock-timeout"), symlinkLock);
  await assert.rejects(store.createAttempt("unsafe-lock", { state: "s", nonce: "n", verifier: "v", expiresAt: Date.now() + 60000,
    redirectUri: "http://127.0.0.1:3003/auth/callback" }, "dynamic_agent_client"), /unsafe local account lock/);
  assert.equal((await lstat(symlinkLock)).isSymbolicLink(), true);
  await unlink(symlinkLock);
  delete process.env.CLICK_DOT_AUTH_LOCK_TIMEOUT_MS;
});

test("callback errors are sanitized and redirect to a clean local URL", async () => {
  const response = await callbackRoute.GET(new Request("http://127.0.0.1:3003/auth/callback?code=secret-code&error=private-error", {
    headers: { host: "127.0.0.1:3003", cookie: "click_dot_auth_attempt=unknown" },
  }));
  assert.equal(response.status, 303);
  assert.equal(response.headers.get("location"), "http://127.0.0.1:3003/?auth=authorization-error");
  assert.match(response.headers.get("set-cookie"), /Max-Age=0/);
  assert.equal(response.headers.get("referrer-policy"), "no-referrer");
  assert.doesNotMatch(response.headers.get("location"), /secret-code|private-error/);
});

test("reuses the saved client registration, allows omitted callback client IDs on reconnect, and rejects account changes", async () => {
  process.env.CLICK_DOT_AUTH_FILE = join(temp, "callback-flow", "auth.json");
  const first = await siwc.beginAuthorization();
  const competingInitial = await siwc.beginAuthorization();
  const firstUrl = new URL(first.url);
  const callback = (id, params) => new Request(`http://127.0.0.1:3003/auth/callback?${new URLSearchParams(params)}`, {
    headers: { host: "127.0.0.1:3003", cookie: `click_dot_auth_attempt=${id}` },
  });
  const firstResponse = await callbackRoute.handleCallback(callback(first.id, {
    state: firstUrl.searchParams.get("state"), code: "first-code", client_id: "issued-client",
  }), async (_code, attempt, clientId) => {
    assert.equal(await store.getIssuedClientId(), "issued-client");
    assert.equal(attempt.expectedClientId, "dynamic_agent_client");
    return { clientId, issuer: "https://auth.openai.com", subject: "same-subject", accessToken: "first-access",
    idToken: "synthetic", scopes: ["chatgpt.tokens.use.direct"], expiresAt: Date.now() + 60_000,
    };
  });
  assert.match(firstResponse.headers.get("location"), /auth=connected/);
  let competingExchange = false;
  const competingResponse = await callbackRoute.handleCallback(callback(competingInitial.id, {
    state: new URL(competingInitial.url).searchParams.get("state"), code: "competing-code", client_id: "issued-client",
  }), async () => { competingExchange = true; throw new Error("must not exchange"); });
  assert.equal(competingExchange, false);
  assert.match(competingResponse.headers.get("location"), /auth=authorization-error/);
  assert.equal((await store.getConnection()).subject, "same-subject");

  const second = await siwc.beginAuthorization();
  const secondUrl = new URL(second.url);
  assert.equal(secondUrl.searchParams.get("client_id"), "issued-client");
  assert.equal(secondUrl.searchParams.has("agent_name_hint"), false);
  let calledWith;
  const secondResponse = await callbackRoute.handleCallback(callback(second.id, {
    state: secondUrl.searchParams.get("state"), code: "second-code",
  }), async (_code, _attempt, clientId) => {
    calledWith = clientId;
    return { clientId, issuer: "https://auth.openai.com", subject: "same-subject", accessToken: "second-access",
      idToken: "synthetic", scopes: ["chatgpt.tokens.use.direct"], expiresAt: Date.now() + 60_000 };
  });
  assert.equal(calledWith, "issued-client");
  assert.match(secondResponse.headers.get("location"), /auth=connected/);
  assert.equal((await store.getConnection()).accessToken, "second-access");

  const third = await siwc.beginAuthorization();
  const thirdUrl = new URL(third.url);
  let exchanged = false;
  const mismatch = await callbackRoute.handleCallback(callback(third.id, {
    state: thirdUrl.searchParams.get("state"), code: "mismatch", client_id: "other-client",
  }), async () => { exchanged = true; throw new Error("must not exchange"); });
  assert.equal(exchanged, false);
  assert.match(mismatch.headers.get("location"), /auth=authorization-error/);
  assert.equal((await store.getConnection()).accessToken, "second-access");

  const fourth = await siwc.beginAuthorization();
  const fourthUrl = new URL(fourth.url);
  const changedSubject = await callbackRoute.handleCallback(callback(fourth.id, {
    state: fourthUrl.searchParams.get("state"), code: "changed-account",
  }), async (_code, _attempt, clientId) => ({ clientId, issuer: "https://auth.openai.com", subject: "different-subject",
    accessToken: "must-not-replace", idToken: "synthetic", scopes: ["chatgpt.tokens.use.direct"], expiresAt: Date.now() + 60_000 }));
  assert.match(changedSubject.headers.get("location"), /auth=authorization-error/);
  assert.equal((await store.getConnection()).accessToken, "second-access");

  const fifth = await siwc.beginAuthorization();
  const fifthUrl = new URL(fifth.url);
  let duplicateExchange = false;
  const duplicateState = await callbackRoute.handleCallback(callback(fifth.id, {
    state: [fifthUrl.searchParams.get("state"), "attacker-state"], code: "duplicate-state",
  }), async () => { duplicateExchange = true; throw new Error("must not exchange"); });
  assert.equal(duplicateExchange, false);
  assert.match(duplicateState.headers.get("location"), /auth=authorization-error/);
});

test.after(async () => {
  delete process.env.CLICK_DOT_AUTH_FILE;
  await rm(temp, { recursive: true, force: true });
});
