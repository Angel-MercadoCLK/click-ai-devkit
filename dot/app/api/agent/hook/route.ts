import { handleHookRequest } from "../../../../lib/agent-hook.ts";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";
// A permission request is held until the user decides: up to 105 s, and the relay itself gives up at 110 s.
export const maxDuration = 120;

export const POST = handleHookRequest;
