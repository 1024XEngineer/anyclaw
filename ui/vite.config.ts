import path from "node:path";
import fs from "node:fs";
import { fileURLToPath } from "node:url";

import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";

const here = path.dirname(fileURLToPath(import.meta.url));
const outDir = path.resolve(here, "..", "dist", "control-ui");
const repoRoot = path.resolve(here, "..");
const apiTarget = process.env.ANYCLAW_UI_API_TARGET || "http://127.0.0.1:18789";

function readJSON(filePath: string) {
  return JSON.parse(fs.readFileSync(filePath, "utf8")) as Record<string, unknown>;
}

function safeString(value: unknown) {
  return typeof value === "string" ? value.trim() : "";
}

function readStringAlias(source: unknown, ...keys: string[]) {
  if (!source || typeof source !== "object") return "";

  for (const key of keys) {
    const value = (source as Record<string, unknown>)[key];
    if (typeof value === "string" && value.trim() !== "") {
      return value.trim();
    }
  }

  return "";
}

function normalizeBasePath(value: string) {
  let basePath = safeString(value);
  if (basePath === "") {
    return "/dashboard";
  }

  if (!basePath.startsWith("/")) {
    basePath = `/${basePath}`;
  }

  basePath = basePath.replace(/\/+$/, "");
  if (basePath === "" || basePath === "/") {
    return "/dashboard";
  }

  return basePath;
}

function loadBasePath() {
  const explicitConfig = safeString(process.env.ANYCLAW_UI_SNAPSHOT_CONFIG);
  const candidates = explicitConfig
    ? [path.resolve(repoRoot, explicitConfig)]
    : [
        path.join(repoRoot, "anyclaw.json"),
        path.join(repoRoot, "anyclaw.example.json"),
      ];

  for (const candidate of candidates) {
    if (!fs.existsSync(candidate)) continue;

    try {
      const config = readJSON(candidate);
      const gateway =
        config.gateway && typeof config.gateway === "object"
          ? (config.gateway as Record<string, unknown>)
          : undefined;
      const controlUi =
        gateway?.control_ui && typeof gateway.control_ui === "object"
          ? (gateway.control_ui as Record<string, unknown>)
          : undefined;
      const controlUiCamel =
        gateway?.controlUi && typeof gateway.controlUi === "object"
          ? (gateway.controlUi as Record<string, unknown>)
          : undefined;
      const configuredBasePath =
        readStringAlias(gateway, "dashboardPath", "dashboard_path") ||
        readStringAlias(controlUi, "basePath", "base_path") ||
        readStringAlias(controlUiCamel, "basePath", "base_path");

      return normalizeBasePath(configuredBasePath);
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error);
      process.stderr.write(`Skipping invalid UI config ${candidate}: ${message}\n`);
    }
  }

  return "/dashboard";
}

const basePath = loadBasePath();

export default defineConfig({
  base: `${basePath}/`,
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
      "/channel": apiTarget,
      "/approvals": apiTarget,
      "/agents": apiTarget,
      "/channels": apiTarget,
      "/chat": apiTarget,
      "/discovery": apiTarget,
      "/events": apiTarget,
      "/market": apiTarget,
      "/providers": apiTarget,
      "/runtimes": apiTarget,
      "/sessions": apiTarget,
      "/skills": apiTarget,
      "/status": apiTarget,
      "/tasks": apiTarget,
      "/v2": apiTarget,
    },
  },
});
