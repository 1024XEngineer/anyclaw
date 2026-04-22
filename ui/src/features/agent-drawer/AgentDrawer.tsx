import { Bot, FolderKanban, ShieldCheck, Sparkles, Waypoints, X } from "lucide-react";

import { useShellStore } from "@/features/shell/useShellStore";
import { useWorkspaceOverview } from "@/features/workspace/useWorkspaceOverview";

type AgentDrawerProps = {
  onClose: () => void;
};

export function AgentDrawer({ onClose }: AgentDrawerProps) {
  const { data } = useWorkspaceOverview();
  const openSettings = useShellStore((state) => state.openSettings);
  const runtimeProfile = data.runtimeProfile;
  const activeAgent = data.localAgents.find((agent) => agent.active) ?? data.localAgents[0];
  const highlights = [
    { label: "模型", value: runtimeProfile.model, icon: Sparkles },
    { label: "权限", value: runtimeProfile.permission, icon: ShieldCheck },
    { label: "工作空间", value: runtimeProfile.workspace, icon: FolderKanban },
  ];

  return (
    <section
      aria-labelledby="agent-drawer-title"
      aria-modal="true"
      className="shell-panel pointer-events-auto flex h-full w-full max-w-[560px] flex-col border-l border-white/80 shadow-[0_28px_90px_rgba(27,24,20,0.24)]"
      role="dialog"
    >
      <header className="flex items-start justify-between gap-4 border-b border-skin px-5 py-5 sm:px-6">
        <div className="space-y-2">
          <div className="inline-flex items-center gap-2 rounded-full bg-[#f3f6fb] px-3 py-1.5 text-sm text-[#5f7597]">
            <Bot size={15} strokeWidth={2.1} />
            Round 3 · Agent 实时摘要
          </div>
          <div>
            <h2
              className="text-[30px] font-semibold tracking-[-0.04em] text-ink"
              id="agent-drawer-title"
            >
              当前 Agent
            </h2>
            <p className="mt-2 text-sm leading-7 text-mute">
              这里已经开始读取运行配置、Provider 摘要和网关状态。后续如果继续做第四轮，可以把这里扩展成真正的 Agent 管理面板。
            </p>
          </div>
        </div>

        <button
          aria-label="关闭 Agent 抽屉"
          className="flex h-12 w-12 shrink-0 items-center justify-center rounded-full bg-white/80 text-[#556274] shadow-soft transition-transform duration-150 hover:-translate-y-0.5"
          onClick={onClose}
          type="button"
        >
          <X size={18} strokeWidth={2.2} />
        </button>
      </header>

      <div className="flex min-h-0 flex-1 flex-col overflow-y-auto px-5 py-5 sm:px-6">
        <section className="rounded-[30px] border border-white/80 bg-white/74 p-5 shadow-soft">
          <div className="flex items-start gap-4">
            <span className="flex h-16 w-16 shrink-0 items-center justify-center rounded-[24px] bg-[radial-gradient(circle_at_30%_28%,#ffe3a5,transparent_28%),linear-gradient(140deg,#ffb36f,#ff6c3c)] text-white shadow-[0_16px_30px_rgba(255,123,67,0.22)]">
              <Bot size={28} strokeWidth={2.1} />
            </span>
            <div className="min-w-0">
              <div className="inline-flex rounded-full bg-[#f5f7fb] px-3 py-1 text-xs font-medium text-[#5b6f8b]">
                {runtimeProfile.title}
              </div>
              <h3 className="mt-3 text-[30px] font-semibold tracking-[-0.04em] text-ink">
                {runtimeProfile.name}
              </h3>
              <p className="mt-3 text-sm leading-7 text-mute">{runtimeProfile.description}</p>
            </div>
          </div>
        </section>

        <section className="mt-5 grid gap-4">
          {highlights.map(({ label, value, icon: Icon }) => (
            <article
              key={label}
              className="rounded-[26px] border border-white/75 bg-white/72 p-5 shadow-soft"
            >
              <div className="flex items-center gap-3">
                <span className="flex h-11 w-11 items-center justify-center rounded-2xl bg-[#f3f6fb] text-[#607699]">
                  <Icon size={19} strokeWidth={2.1} />
                </span>
                <div>
                  <div className="text-sm text-mute">{label}</div>
                  <div className="text-[24px] font-semibold tracking-[-0.04em] text-ink">{value}</div>
                </div>
              </div>
            </article>
          ))}
        </section>

        <section className="mt-5 rounded-[30px] border border-white/75 bg-white/72 p-5 shadow-soft">
          <div className="flex items-center justify-between gap-4">
            <div>
              <h3 className="text-[24px] font-semibold tracking-[-0.04em] text-ink">运行时摘要</h3>
              <p className="mt-2 text-sm leading-7 text-mute">这里汇总当前网关、会话、工具和运行时缓存的状态。</p>
            </div>
            <div className="rounded-full bg-[#f5f7fb] px-3 py-1 text-xs text-[#5b6f8b]">
              {runtimeProfile.orchestrator}
            </div>
          </div>

          <div className="mt-4 space-y-3 text-sm">
            <div className="flex items-center justify-between gap-4 rounded-[20px] bg-[#f8fafc] px-4 py-3">
              <span className="text-mute">Provider</span>
              <span className="font-medium text-ink">
                {runtimeProfile.providerLabel} · {runtimeProfile.provider}
              </span>
            </div>
            <div className="flex items-center justify-between gap-4 rounded-[20px] bg-[#f8fafc] px-4 py-3">
              <span className="text-mute">网关</span>
              <span className="font-medium text-ink">
                {runtimeProfile.gatewayOnline ? "在线" : "离线"} · {runtimeProfile.address}
              </span>
            </div>
            <div className="flex items-center justify-between gap-4 rounded-[20px] bg-[#f8fafc] px-4 py-3">
              <span className="text-mute">会话 / 运行时</span>
              <span className="font-medium text-ink">
                {runtimeProfile.sessions} / {runtimeProfile.runtimes}
              </span>
            </div>
          </div>
        </section>

        <section className="mt-5 rounded-[30px] border border-white/75 bg-white/72 p-5 shadow-soft">
          <div className="flex items-center justify-between gap-4">
            <div>
              <h3 className="text-[24px] font-semibold tracking-[-0.04em] text-ink">已挂接能力</h3>
              <p className="mt-2 text-sm leading-7 text-mute">这里优先展示当前 Agent 对应的技能与主要渠道入口。</p>
            </div>
          </div>

          <div className="mt-4 flex flex-wrap gap-2">
            {data.localSkills.slice(0, 6).map((skill) => (
              <span
                key={skill.name}
                className="rounded-full border border-white/80 bg-white/88 px-3 py-1.5 text-xs text-[#5d5344]"
              >
                {skill.name}
              </span>
            ))}
          </div>

          <div className="mt-5 flex flex-wrap gap-2">
            {data.priorityChannels.slice(0, 4).map((channel) => (
              <span
                key={channel.slug}
                className="inline-flex items-center gap-2 rounded-full bg-[#edf2fa] px-3 py-1.5 text-xs text-[#577098]"
              >
                <Waypoints size={12} strokeWidth={2.2} />
                {channel.name}
              </span>
            ))}
          </div>
        </section>

        <section className="mt-5 rounded-[30px] border border-white/75 bg-white/72 p-5 shadow-soft">
          <div className="text-sm font-medium uppercase tracking-[0.16em] text-[#64748b]">当前配置角色</div>
          <div className="mt-3 text-[24px] font-semibold tracking-[-0.04em] text-ink">
            {activeAgent?.role || runtimeProfile.title}
          </div>
          <p className="mt-3 text-sm leading-7 text-mute">
            {activeAgent?.summary || "当前 Agent 的描述会在这里展示。"}
          </p>
        </section>

        <footer className="mt-5 flex flex-col gap-3 sm:flex-row">
          <button
            className="shell-button h-12 justify-center px-5 text-base font-medium"
            onClick={() => {
              onClose();
              openSettings("skills");
            }}
            type="button"
          >
            查看 Skill
          </button>
          <button
            className="chip-button justify-center px-5"
            onClick={() => {
              onClose();
              openSettings("channels");
            }}
            type="button"
          >
            查看渠道
          </button>
        </footer>
      </div>
    </section>
  );
}
