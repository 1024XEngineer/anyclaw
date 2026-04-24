#!/usr/bin/env node
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const here = path.dirname(fileURLToPath(import.meta.url));
const uiRoot = path.resolve(here, "..");
const repoRoot = path.resolve(uiRoot, "..");
const outFile = path.join(uiRoot, "src", "generated", "workspaceSnapshot.generated.ts");

function readJSON(filePath) {
  return JSON.parse(fs.readFileSync(filePath, "utf8"));
}

function existingDir(dirPath) {
  return fs.existsSync(dirPath) && fs.statSync(dirPath).isDirectory();
}

function loadWorkspaceConfig() {
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
      return {
        config: readJSON(candidate),
        configPath: candidate,
      };
    } catch (error) {
      process.stderr.write(`Skipping invalid snapshot config ${candidate}: ${error.message}\n`);
    }
  }

  return {
    config: {},
    configPath: path.join(repoRoot, "anyclaw.json"),
  };
}

function safeString(value) {
  return typeof value === "string" ? value.trim() : "";
}

function readStringAlias(source, ...keys) {
  if (!source || typeof source !== "object") return "";

  for (const key of keys) {
    const value = source[key];
    if (typeof value === "string" && value.trim() !== "") {
      return value.trim();
    }
  }

  return "";
}

function safeArray(value) {
  return Array.isArray(value) ? value : [];
}

function safeBoolean(value, fallback = false) {
  return typeof value === "boolean" ? value : fallback;
}

function stripWrappingQuotes(value) {
  if (value.length >= 2 && value.startsWith('"') && value.endsWith('"')) {
    return value.slice(1, -1).trim();
  }
  if (value.length >= 2 && value.startsWith("'") && value.endsWith("'")) {
    return value.slice(1, -1).trim();
  }
  return value;
}

function normalizeSkillDescription(value) {
  return compactText(
    stripWrappingQuotes(safeString(value))
      .replace(/`([^`]+)`/g, "$1")
      .replace(/\[([^\]]+)\]\(([^)]+)\)/g, "$1")
      .replace(/\s+/g, " ")
      .trim(),
  );
}

function compactText(value, maxLength = 160) {
  const text = safeString(value).replace(/\s+/g, " ");
  if (text.length <= maxLength) return text;
  return `${text.slice(0, maxLength - 1).trimEnd()}…`;
}

function humanizeKey(key) {
  if (!key) return "";
  return key
    .split(/[-_]/g)
    .filter(Boolean)
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join(" ");
}

function channelConfigured(entry, keys) {
  if (!entry || typeof entry !== "object") return false;
  return keys.every((key) => {
    const value = entry[key];
    return typeof value === "string" && value.trim() !== "";
  });
}

function channelConfiguredWithAliases(entry, keyGroups) {
  if (!entry || typeof entry !== "object") return false;
  return keyGroups.every((group) =>
    group.some((key) => {
      const value = entry[key];
      return typeof value === "string" && value.trim() !== "";
    }),
  );
}

function resolveConfigPath(configPath, value, fallback) {
  const configured = safeString(value) || safeString(fallback);
  if (configured === "") return "";

  const normalized = path.normalize(configured);
  if (path.isAbsolute(normalized)) {
    return normalized;
  }

  return path.resolve(path.dirname(configPath), normalized);
}

function readConfiguredDir(config, configPath, sectionKey, fallbackDir, nestedAliases = [], rootAliases = []) {
  const section = config?.[sectionKey];
  const nestedValue = readStringAlias(section, ...nestedAliases);
  const rootValue = readStringAlias(config, ...rootAliases);
  return resolveConfigPath(configPath, nestedValue || rootValue, fallbackDir);
}

function normalizeBasePath(value) {
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

function readGatewayBasePath(config) {
  const gateway = config?.gateway;
  const basePath =
    readStringAlias(gateway, "dashboardPath", "dashboard_path") ||
    readStringAlias(gateway?.control_ui, "basePath", "base_path") ||
    readStringAlias(gateway?.controlUi, "basePath", "base_path");

  return normalizeBasePath(basePath);
}

function readSkillManifests(skillsDir) {
  if (!existingDir(skillsDir)) {
    return [];
  }
  return fs
    .readdirSync(skillsDir, { withFileTypes: true })
    .filter((entry) => entry.isDirectory())
    .map((entry) => {
      const manifestPath = path.join(skillsDir, entry.name, "skill.json");
      const manifest = fs.existsSync(manifestPath) ? readJSON(manifestPath) : {};

      return {
        name: safeString(manifest.name) || entry.name,
        description: normalizeSkillDescription(manifest.description),
        source: safeString(manifest.source) || "local",
        registry: safeString(manifest.registry),
        version: safeString(manifest.version),
        installCommand: safeString(manifest.install_command),
      };
    })
    .sort((left, right) => left.name.localeCompare(right.name));
}

function readExtensionManifests(extensionsDir) {
  if (!existingDir(extensionsDir)) {
    return [];
  }
  return fs
    .readdirSync(extensionsDir, { withFileTypes: true })
    .filter((entry) => entry.isDirectory())
    .map((entry) => {
      const manifestPath = path.join(extensionsDir, entry.name, "anyclaw.extension.json");
      const manifest = fs.existsSync(manifestPath) ? readJSON(manifestPath) : {};

      return {
        id: safeString(manifest.id) || entry.name,
        name: safeString(manifest.name) || humanizeKey(entry.name),
        description: compactText(manifest.description),
        kind: safeString(manifest.kind) || "extension",
        channels: safeArray(manifest.channels).map((item) => safeString(item)).filter(Boolean),
      };
    })
    .sort((left, right) => left.name.localeCompare(right.name));
}

function buildMainAgent(agentConfig) {
  return {
    name: safeString(agentConfig?.name) || "AnyClaw",
    description: compactText(agentConfig?.description),
    role: "main",
    permissionLevel: safeString(agentConfig?.permission_level) || "limited",
    workingDir: safeString(agentConfig?.working_dir),
    providerRef: safeString(agentConfig?.provider_ref),
    defaultModel: safeString(agentConfig?.default_model),
    enabled: true,
    active: true,
    skills: safeArray(agentConfig?.skills)
      .filter((skill) => safeBoolean(skill?.enabled, true))
      .map((skill) => safeString(skill?.name))
      .filter(Boolean),
  };
}

function buildAgentProfiles(config) {
  const mainAgent = buildMainAgent(config.agent);
  const profiles = safeArray(config.agent?.profiles).map((profile) => ({
    name: safeString(profile.name),
    description: compactText(profile.description),
    role: safeString(profile.role) || "profile",
    permissionLevel: safeString(profile.permission_level) || "limited",
    workingDir: safeString(profile.working_dir),
    providerRef: safeString(profile.provider_ref),
    defaultModel: safeString(profile.default_model),
    enabled: safeBoolean(profile.enabled, true),
    active: safeString(config.agent?.active_profile) !== ""
      ? safeString(config.agent?.active_profile) === safeString(profile.name)
      : false,
    skills: safeArray(profile.skills)
      .filter((skill) => safeBoolean(skill?.enabled, true))
      .map((skill) => safeString(skill?.name))
      .filter(Boolean),
  }));

  if (profiles.length === 0) return [mainAgent];

  const hasActive = profiles.some((profile) => profile.active);
  return [
    {
      ...mainAgent,
      active: !hasActive,
    },
    ...profiles,
  ];
}

function buildProviders(config) {
  const defaultProviderRef = safeString(config.llm?.default_provider_ref);

  return safeArray(config.providers).map((provider) => ({
    id: safeString(provider.id) || safeString(provider.name),
    name: safeString(provider.name) || safeString(provider.id),
    type: safeString(provider.type),
    provider: safeString(provider.provider),
    defaultModel: safeString(provider.default_model),
    enabled: safeBoolean(provider.enabled, true),
    isDefault: safeString(provider.id) !== "" && safeString(provider.id) === defaultProviderRef,
    capabilitiesCount: safeArray(provider.capabilities).length,
  }));
}

function buildConfiguredChannels(config) {
  const channels = config.channels ?? {};

  return [
    {
      key: "wechat",
      enabled: safeBoolean(channels.wechat?.enabled),
      configured: channelConfiguredWithAliases(channels.wechat, [
        ["app_id", "appid"],
        ["app_secret", "secret"],
        ["token", "verify_token"],
      ]),
    },
    {
      key: "feishu",
      enabled: safeBoolean(channels.feishu?.enabled),
      configured: channelConfiguredWithAliases(channels.feishu, [
        ["app_id", "appId"],
        ["app_secret", "appSecret"],
      ]),
    },
    {
      key: "telegram",
      enabled: safeBoolean(channels.telegram?.enabled),
      configured: channelConfigured(channels.telegram, ["bot_token", "chat_id"]),
    },
    {
      key: "slack",
      enabled: safeBoolean(channels.slack?.enabled),
      configured: channelConfigured(channels.slack, ["bot_token", "app_token", "default_channel"]),
    },
    {
      key: "discord",
      enabled: safeBoolean(channels.discord?.enabled),
      configured: channelConfigured(channels.discord, ["bot_token", "default_channel", "guild_id", "public_key"]),
    },
    {
      key: "whatsapp",
      enabled: safeBoolean(channels.whatsapp?.enabled),
      configured: channelConfigured(channels.whatsapp, ["access_token", "phone_number_id", "verify_token"]),
    },
    {
      key: "signal",
      enabled: safeBoolean(channels.signal?.enabled),
      configured: channelConfigured(channels.signal, ["number", "default_recipient", "bearer_token"]),
    },
  ];
}

function buildSnapshot() {
  const { config, configPath } = loadWorkspaceConfig();
  const skills = readSkillManifests(
    readConfiguredDir(config, configPath, "skills", path.join(repoRoot, "skills"), ["skillsDir", "dir", "path"], [
      "skillsDir",
      "skills_dir",
    ]),
  );
  const extensions = readExtensionManifests(path.join(repoRoot, "extensions"));

  return {
    generatedAt: new Date().toISOString(),
    agent: {
      name: safeString(config.agent?.name) || "AnyClaw",
      description: compactText(config.agent?.description),
      permissionLevel: safeString(config.agent?.permission_level) || "limited",
      workDir: safeString(config.agent?.work_dir),
      workingDir: safeString(config.agent?.working_dir),
      language: safeString(config.agent?.lang) || "CN",
      activeProfile: safeString(config.agent?.active_profile),
    },
    orchestrator: {
      enabled: safeBoolean(config.orchestrator?.enabled),
      maxConcurrentAgents: Number(config.orchestrator?.max_concurrent_agents) || 0,
      retryCount: Number(config.orchestrator?.max_retries) || 0,
      timeoutSeconds: Number(config.orchestrator?.timeout_seconds) || 0,
      namedAgents: safeArray(config.orchestrator?.agent_names).length,
      subAgents: safeArray(config.orchestrator?.sub_agents).length,
    },
    gateway: {
      basePath: readGatewayBasePath(config),
      host: safeString(config.gateway?.host) || "127.0.0.1",
      port: Number(config.gateway?.port) || 18789,
      bind: safeString(config.gateway?.bind) || "loopback",
    },
    providers: buildProviders(config),
    agents: buildAgentProfiles(config),
    skills,
    extensions,
    configuredChannels: buildConfiguredChannels(config),
    security: {
      mentionGate: safeBoolean(config.channels?.security?.mention_gate),
      pairingEnabled: safeBoolean(config.channels?.security?.pairing_enabled),
      defaultDenyDM: safeBoolean(config.channels?.security?.default_deny_dm),
    },
  };
}

const snapshot = buildSnapshot();
const content = `/* eslint-disable */
// This file is auto-generated by ui/scripts/generate-workspace-snapshot.mjs.
// Do not edit manually.

export const workspaceSnapshot = ${JSON.stringify(snapshot, null, 2)} as const;

export type WorkspaceSnapshot = typeof workspaceSnapshot;
`;

fs.mkdirSync(path.dirname(outFile), { recursive: true });
fs.writeFileSync(outFile, content, "utf8");
process.stdout.write(`Generated workspace snapshot -> ${path.relative(uiRoot, outFile)}\n`);
