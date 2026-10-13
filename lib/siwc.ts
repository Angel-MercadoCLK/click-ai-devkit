import { createHash, randomBytes } from "node:crypto";
import { createRemoteJWKSet, jwtVerify, type JWTVerifyGetKey } from "jose";
import { createAttempt, type AuthAttempt, type Connection } from "./auth-store.ts";

const AUTHORIZATION_ENDPOINT = "https://auth.openai.com/api/accounts/authorize";
const TOKEN_ENDPOINT = "https://auth.openai.com/api/accounts/oauth/token";
const JWKS = createRemoteJWKSet(new URL("https://auth.openai.com/.well-known/jwks.json"));
const ISSUER = "https://auth.openai.com";
const RESOURCE = "https://api.openai.com/v1";
const INITIAL_CLIENT_ID = "dynamic_agent_client";
const REQUESTED_SCOPES = ["openid", "profile", "email", "offline_access", "resource.invoke", "chatgpt.tokens.use.direct"];
const ATTEMPT_TTL_MS = 10 * 60 * 1000;

const randomBase64Url = (bytes = 32) => randomBytes(bytes).toString("base64url");
const sha256Base64Url = (value: string) => createHash("sha256").update(value).digest("base64url");

export function callbackUri(port = process.env.PORT ?? "3000") {
  if (!/^\d{1,5}$/.test(port) || Number(port) < 1 || Number(port) > 65535) {
    throw new Error("The local callback port is invalid.");
  }
  return `http://127.0.0.1:${Number(port)}/auth/callback`;
}

export async function beginAuthorization() {
  const id = randomBase64Url(24);
  const redirectUri = callbackUri();
  const { hostId, issuedClientId, attempt } = await createAttempt(id, {
    state: randomBase64Url(),
    nonce: randomBase64Url(),
    verifier: randomBase64Url(48),
    expiresAt: Date.now() + ATTEMPT_TTL_MS,
    redirectUri,
  }, INITIAL_CLIENT_ID);
  const params = new URLSearchParams({
    client_id: issuedClientId ?? INITIAL_CLIENT_ID,
    response_type: "code",
    redirect_uri: redirectUri,
    scope: REQUESTED_SCOPES.join(" "),
    resource: RESOURCE,
    state: attempt.state,
    nonce: attempt.nonce,
    code_challenge: sha256Base64Url(attempt.verifier),
    code_challenge_method: "S256",
    ext_agent_host_id: hostId,
  });
  if (!issuedClientId) params.set("agent_name_hint", "Click Dot");
  return { id, url: `${AUTHORIZATION_ENDPOINT}?${params}` };
}

type TokenResponse = {
  access_token?: unknown;
  refresh_token?: unknown;
  id_token?: unknown;
  scope?: unknown;
  expires_in?: unknown;
  client_id?: unknown;
};

async function readTokenResponse(response: Response): Promise<TokenResponse> {
  if (!response.ok) throw new Error("ChatGPT authorization could not be completed.");
  const payload: unknown = await response.json().catch(() => undefined);
  if (!payload || typeof payload !== "object") throw new Error("ChatGPT authorization returned an invalid response.");
  return payload as TokenResponse;
}

export async function exchangeAuthorizationCode(
  code: string,
  attempt: AuthAttempt,
  issuedClientId: string,
  fetcher: typeof fetch = fetch,
  jwks?: JWTVerifyGetKey,
) {
  if (!issuedClientId.trim() || issuedClientId === INITIAL_CLIENT_ID || (issuedClientId !== attempt.expectedClientId && attempt.expectedClientId !== INITIAL_CLIENT_ID)) {
    throw new Error("The local application registration is invalid.");
  }
  if (!code.trim()) throw new Error("The authorization response did not contain a code.");
  const redirectUri = attempt.redirectUri;
  if (redirectUri !== callbackUri()) throw new Error("The local callback address changed during authorization.");
  const form = new URLSearchParams({
    grant_type: "authorization_code",
    code,
    redirect_uri: redirectUri,
    client_id: issuedClientId,
    code_verifier: attempt.verifier,
    resource: RESOURCE,
  });
  const tokenResponse = await readTokenResponse(await fetcher(TOKEN_ENDPOINT, {
    method: "POST",
    headers: { "content-type": "application/x-www-form-urlencoded", accept: "application/json" },
    body: form,
    signal: AbortSignal.timeout(12_000),
    cache: "no-store",
  }));

  const returnedClientId = tokenResponse.client_id === undefined ? issuedClientId : tokenResponse.client_id;
  if (typeof returnedClientId !== "string" || !returnedClientId.trim() || returnedClientId !== returnedClientId.trim()) {
    throw new Error("ChatGPT returned an invalid local application registration.");
  }
  if (returnedClientId !== issuedClientId) {
    throw new Error("ChatGPT did not return the expected local application registration.");
  }
  if (typeof tokenResponse.id_token !== "string" || !tokenResponse.id_token.trim()
    || typeof tokenResponse.access_token !== "string" || !tokenResponse.access_token.trim()) {
    throw new Error("ChatGPT authorization did not return usable credentials.");
  }
  const { payload } = await verifyIdToken(tokenResponse.id_token, { nonce: attempt.nonce, clientId: returnedClientId, jwks });
  const scopes = typeof tokenResponse.scope === "string" ? tokenResponse.scope.split(/\s+/).filter(Boolean) : [];
  if (!scopes.includes("chatgpt.tokens.use.direct")) {
    throw new Error("ChatGPT did not grant direct plan usage. No model access was enabled.");
  }
  const expiresIn = tokenResponse.expires_in;
  if (typeof expiresIn !== "number" || !Number.isFinite(expiresIn) || expiresIn <= 0 || expiresIn > 31_536_000) {
    throw new Error("ChatGPT authorization returned an invalid credential lifetime.");
  }
  const subject = typeof payload.sub === "string" ? payload.sub.trim() : "";
  if (!subject || typeof payload.iss !== "string" || payload.iss !== ISSUER) {
    throw new Error("ChatGPT identity is missing a verified issuer or subject.");
  }
  return {
    clientId: returnedClientId,
    issuer: ISSUER,
    subject,
    accessToken: tokenResponse.access_token,
    refreshToken: typeof tokenResponse.refresh_token === "string" ? tokenResponse.refresh_token : undefined,
    idToken: tokenResponse.id_token,
    email: typeof payload.email === "string" ? payload.email : undefined,
    scopes,
    expiresAt: Date.now() + expiresIn * 1000,
  } satisfies Connection;
}

export function verifyIdToken(token: string, options: { nonce: string; clientId: string; jwks?: JWTVerifyGetKey }) {
  return jwtVerify(token, options.jwks ?? JWKS, {
    issuer: ISSUER,
    audience: options.clientId,
    requiredClaims: ["exp", "iat", "sub", "iss", "aud"],
    algorithms: ["RS256"],
    clockTolerance: 5,
  }).then((result) => {
    if (result.payload.nonce !== options.nonce) throw new Error("The identity token nonce did not match this authorization attempt.");
    if (typeof result.payload.sub !== "string" || !result.payload.sub.trim()) throw new Error("The identity token subject is missing.");
    if (typeof result.payload.iat !== "number" || !Number.isFinite(result.payload.iat)
      || result.payload.iat > Math.floor(Date.now() / 1000) + 5) throw new Error("The identity token issue time is invalid.");
    return result;
  });
}

export function hasDirectUsageScope(connection: Pick<Connection, "scopes" | "expiresAt">) {
  return connection.expiresAt > Date.now() && connection.scopes.includes("chatgpt.tokens.use.direct");
}
