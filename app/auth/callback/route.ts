import { clearPendingCookie, cookieValue, isLocalRequest, noStore, PENDING_COOKIE } from "../../../lib/auth-http.ts";
import { consumeAttempt, saveConnection, saveIssuedClientId } from "../../../lib/auth-store.ts";
import { callbackUri, exchangeAuthorizationCode } from "../../../lib/siwc.ts";

export const runtime = "nodejs";

function uniqueParameter(url: URL, key: string, required = false) {
  const values = url.searchParams.getAll(key);
  if (values.length > 1 || (required && values.length !== 1)) throw new Error("The authorization response is invalid.");
  return values[0];
}

function finish(request: Request, result: "connected" | "authorization-error") {
  return new Response(null, {
    status: 303,
    headers: {
      ...noStore,
      location: new URL(`/?auth=${result}`, callbackUri()).toString(),
      "set-cookie": clearPendingCookie(),
    },
  });
}

export async function handleCallback(request: Request, exchange = exchangeAuthorizationCode) {
  if (!isLocalRequest(request)) return new Response("Request rejected.", { status: 403, headers: noStore });
  const url = new URL(request.url);
  const attemptId = cookieValue(request, PENDING_COOKIE);
  if (!attemptId) return finish(request, "authorization-error");
  try {
    const attempt = await consumeAttempt(attemptId);
    if (!attempt || uniqueParameter(url, "state", true) !== attempt.state || uniqueParameter(url, "error") !== undefined) {
      return finish(request, "authorization-error");
    }
    const code = uniqueParameter(url, "code", true);
    const returnedClientId = uniqueParameter(url, "client_id");
    if (!code) return finish(request, "authorization-error");
    let issuedClientId = attempt.expectedClientId;
    if (attempt.expectedClientId === "dynamic_agent_client") {
      if (!returnedClientId || returnedClientId === "dynamic_agent_client") return finish(request, "authorization-error");
      issuedClientId = returnedClientId;
    } else if (returnedClientId && returnedClientId !== attempt.expectedClientId) {
      return finish(request, "authorization-error");
    }
    if (!await saveIssuedClientId(issuedClientId, attempt.generation)) return finish(request, "authorization-error");
    const connection = await exchange(code, attempt, issuedClientId);
    if (attempt.expectedSubject && connection.subject !== attempt.expectedSubject) return finish(request, "authorization-error");
    if (!await saveConnection(connection, attempt)) return finish(request, "authorization-error");
    return finish(request, "connected");
  } catch {
    return finish(request, "authorization-error");
  }
}

export async function GET(request: Request) {
  return handleCallback(request);
}
