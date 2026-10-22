import type { Metadata } from "next";
import { callbackUri } from "../../lib/siwc.ts";
import { Mascot } from "./mascot";

// Rendered per request, like the home page: the canonical origin must come from the running PORT (47321 in the
// desktop app), not from the build environment.
export const dynamic = "force-dynamic";

export const metadata: Metadata = { title: "Click Dot" };

export default function MascotPage() {
  const canonicalOrigin = new URL(callbackUri()).origin;
  return <Mascot canonicalOrigin={canonicalOrigin} />;
}
