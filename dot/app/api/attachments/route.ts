import { handleUploadRequest } from "../../../lib/attachments.ts";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

export const POST = handleUploadRequest;
