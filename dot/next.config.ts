import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  ...(process.env.NEXT_BUILD_DIST_DIR ? { distDir: process.env.NEXT_BUILD_DIST_DIR } : {}),
  logging: {
    incomingRequests: {
      ignore: [/\/auth\/callback(?:\?|$)/],
    },
  },
};

export default nextConfig;
