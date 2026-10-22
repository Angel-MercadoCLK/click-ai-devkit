import { handleDecisionRequest } from "../../../../lib/agent-hook.ts";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

export const POST = handleDecisionRequest;
