import { engineHandlers } from "../../../../lib/engine-http.ts";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

export const POST = (request: Request) => engineHandlers.recheck(request);
