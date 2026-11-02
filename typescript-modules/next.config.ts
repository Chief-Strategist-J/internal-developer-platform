import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  reactStrictMode: true,
  output: "standalone",
  env: {
    BACKEND_URL: process.env["BACKEND_URL"] || "http://127.0.0.1:8080"
  }
};

export default nextConfig;
