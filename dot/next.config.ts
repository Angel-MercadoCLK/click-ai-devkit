import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  ...(process.env.NEXT_BUILD_DIST_DIR ? { distDir: process.env.NEXT_BUILD_DIST_DIR } : {}),
  experimental: {
    // The packaged app is read-only and replaced on update: never write response caches next to the app.
    isrFlushToDisk: false,
  },
  logging: {
    incomingRequests: {
      ignore: [/\/auth\/callback(?:\?|$)/],
    },
  },
};

export default nextConfig;
