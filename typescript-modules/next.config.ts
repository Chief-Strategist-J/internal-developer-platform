import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  reactStrictMode: true,
  output: "standalone",
  env: {
    BACKEND_URL: process.env["BACKEND_URL"] || process.env["NEXT_PUBLIC_BACKEND_URL"] || ""
  }
};

export default nextConfig;
