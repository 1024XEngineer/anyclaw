#!/usr/bin/env node
import { spawn, spawnSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const here = path.dirname(fileURLToPath(import.meta.url));
const repoRoot = path.resolve(here, "..");
const uiDir = path.join(repoRoot, "ui");

const ACTION_TO_SCRIPT = {
  dev: "dev",
  build: "build",
  preview: "preview",
  test: "test",
};

function usage() {
  process.stderr.write("Usage: node scripts/ui.mjs <install|dev|build|preview|test> [...args]\n");
}

function which(cmd) {
  const pathKey = process.platform === "win32" ? "Path" : "PATH";
  const entries = (process.env[pathKey] ?? process.env.PATH ?? "").split(path.delimiter).filter(Boolean);
  const exts =
    process.platform === "win32"
      ? (process.env.PATHEXT ?? ".EXE;.CMD;.BAT;.COM").split(";").filter(Boolean)
      : [""];
  for (const entry of entries) {
    for (const ext of exts) {
      const candidate = path.join(entry, process.platform === "win32" ? `${cmd}${ext}` : cmd);
      try {
        if (fs.existsSync(candidate)) {
          return candidate;
        }
      } catch {
        // ignore path access errors
      }
    }
  }
  return null;
}

function isWindowsBatchCommand(cmd) {
  return process.platform === "win32" && [".cmd", ".bat", ".com"].includes(path.extname(cmd).toLowerCase());
}

function quoteWindowsCmdArg(value) {
  const input = String(value);
  if (input.length === 0) {
    return '""';
  }
  if (!/[ \t"&()^<>|]/.test(input)) {
    return input;
  }
  return `"${input.replace(/"/g, '""')}"`;
}

function quotePowerShellArg(value) {
  return `'${String(value).replace(/'/g, "''")}'`;
}

function commandInvocation(cmd, args) {
  if (!isWindowsBatchCommand(cmd)) {
    return { command: cmd, args, shell: false };
  }

  const powershell = which("powershell") || "powershell.exe";
  const inner = [`& ${quotePowerShellArg(cmd)}`, ...args.map(quotePowerShellArg)].join(" ");
  return {
    command: powershell,
    args: ["-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", inner],
    shell: false,
  };
}

function run(cmd, args) {
  const invocation = commandInvocation(cmd, args);
  const child = spawn(invocation.command, invocation.args, {
    cwd: uiDir,
    stdio: "inherit",
    env: process.env,
    shell: invocation.shell,
  });
  child.on("error", (err) => {
    console.error(`Failed to launch ${cmd}:`, err);
    process.exit(1);
  });
  child.on("exit", (code) => {
    process.exit(code ?? 1);
  });
}

function runSync(cmd, args, env = process.env) {
  const invocation = commandInvocation(cmd, args);
  const result = spawnSync(invocation.command, invocation.args, {
    cwd: uiDir,
    stdio: "inherit",
    env,
    shell: invocation.shell,
  });
  if (result.error) {
    console.error(`Failed to launch ${cmd}:`, result.error);
    process.exit(1);
  }
  if ((result.status ?? 1) !== 0) {
    process.exit(result.status ?? 1);
  }
}

function hasDeps(kind) {
  if (!fs.existsSync(path.join(uiDir, "node_modules", "vite"))) {
    return false;
  }
  if (kind === "test") {
    if (!fs.existsSync(path.join(uiDir, "node_modules", "vitest"))) {
      return false;
    }
    if (!fs.existsSync(path.join(uiDir, "node_modules", "jsdom"))) {
      return false;
    }
  }
  return true;
}

function main() {
  const [action, ...rest] = process.argv.slice(2);
  if (!action) {
    usage();
    process.exit(2);
  }

  const pnpm = which("pnpm");
  const corepack = which("corepack");
  const pkgRunner = pnpm
    ? { bin: pnpm, prefixArgs: [] }
    : corepack
      ? { bin: corepack, prefixArgs: ["pnpm"] }
      : null;

  if (!pkgRunner) {
    console.error("Missing pnpm/corepack. Please install Node.js with corepack support or pnpm and retry.");
    process.exit(1);
  }

  if (action === "install") {
    run(pkgRunner.bin, [...pkgRunner.prefixArgs, "install", ...rest]);
    return;
  }

  const script = ACTION_TO_SCRIPT[action];
  if (!script) {
    usage();
    process.exit(2);
  }

  if (!hasDeps(action)) {
    runSync(pkgRunner.bin, [...pkgRunner.prefixArgs, "install"], process.env);
  }

  run(pkgRunner.bin, [...pkgRunner.prefixArgs, "run", script, ...rest]);
}

main();
