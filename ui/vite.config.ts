import path from "node:path";
import { fileURLToPath } from "node:url";

import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";

const here = path.dirname(fileURLToPath(import.meta.url));
const outDir = path.resolve(here, "..", "dist", "control-ui");
const apiTarget = process.env.ANYCLAW_UI_API_TARGET || "http://127.0.0.1:18789";

export default defineConfig({
  base: "/dashboard/",
  build: {
    emptyOutDir: true,
    outDir,
  },
  plugins: [react()],
  resolve: {
    alias: {
      "@": path.resolve(here, "src"),
    },
  },
  server: {
    host: "127.0.0.1",
    port: 4173,
    proxy: {
      "/agents": apiTarget,
      "/channels": apiTarget,
      "/chat": apiTarget,
      "/discovery": apiTarget,
      "/events": apiTarget,
      "/market": apiTarget,
      "/providers": apiTarget,
      "/runtimes": apiTarget,
      "/skills": apiTarget,
      "/status": apiTarget,
      "/tasks": apiTarget,
      "/v2": apiTarget,
    },
  },
});
