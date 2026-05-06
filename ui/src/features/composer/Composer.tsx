import { ChevronDown, LoaderCircle, SendHorizontal, SlidersHorizontal, Sparkles } from "lucide-react";
import type { KeyboardEvent as ReactKeyboardEvent } from "react";

import type { ChatTaskState } from "@/features/chat/useWebChat";
import { useShellStore } from "@/features/shell/useShellStore";

type ComposerProps = {
  activeAgentLabel: string;
  canSend: boolean;
  chatTaskState: ChatTaskState;
  draft: string;
  error: string | null;
  isSending: boolean;
  modelLabel: string;
  onDraftChange: (value: string) => void;
  onReset: () => void;
  onSend: () => void;
  sessionId: string | null;
  setupMessage: string;
  setupRequired: boolean;
};

function buildUserFacingError(error: string | null, chatTaskState: ChatTaskState) {
  const technicalDetail = chatTaskState.technicalDetail ?? error;
  if (!technicalDetail) return null;

  const normalized = technicalDetail.toLowerCase();
  if (normalized.includes("session not found")) {
    return {
      action: "可以新建对话，或从左侧选择另一个历史会话后继续。",
      reason: "这个会话可能已经被删除，或本地记录和网关记录暂时不同步。",
      title: "这个会话已经不可用。",
      technicalDetail,
    };
  }

  if (normalized.includes("workspace not found") || normalized.includes("workspace is required")) {
    return {
      action: "可以刷新页面后重试，或先确认当前工作区是否仍然存在。",
      reason: "当前请求关联的工作区没有被网关识别。",
      title: "没有找到当前工作区。",
      technicalDetail,
    };
  }

  if (normalized.includes("approval") && (normalized.includes("reject") || normalized.includes("denied"))) {
    return {
      action: "可以修改任务要求后重新发送，或选择更低风险的操作方式。",
      reason: "你拒绝了本次需要权限的操作。",
      title: "任务已停止。",
      technicalDetail,
    };
  }

  if (normalized.includes("forbidden") || normalized.includes("required_permission")) {
    return {
      action: "可以切换到有权限的配置，或降低任务需要的操作权限。",
      reason: "当前账号或配置没有执行这个操作的权限。",
      title: "权限不足。",
      technicalDetail,
    };
  }

  return {
    action: chatTaskState.canRetry ? "可以调整任务描述后重试；如果重复出现，再展开技术细节排查。" : "请先处理这个问题，再继续当前任务。",
    reason: "请求过程中出现异常，当前任务没有正常完成。",
    title: "这次请求没有完成。",
    technicalDetail,
  };
}

function InlineErrorPanel({ chatTaskState, error }: { chatTaskState: ChatTaskState; error: string | null }) {
  const userFacingError = buildUserFacingError(error, chatTaskState);
  if (!userFacingError) return null;

  return (
    <div className="mx-5 mt-2 rounded-[18px] border border-[#fed7aa] bg-[#fff7ed] px-4 py-3 text-sm leading-6 text-[#9a3412]">
      <div className="font-medium text-[#7c2d12]">发生了什么：{userFacingError.title}</div>
      <div className="mt-1">可能原因：{userFacingError.reason}</div>
      <div className="mt-1">你可以怎么做：{userFacingError.action}</div>
      <details className="mt-2 text-xs text-[#9a3412]/80">
        <summary className="cursor-pointer font-medium">技术细节</summary>
        <div className="mt-1 break-words">{userFacingError.technicalDetail}</div>
      </details>
    </div>
  );
}

export function Composer({
  activeAgentLabel,
  canSend,
  chatTaskState,
  draft,
  error,
  isSending,
  modelLabel,
  onDraftChange,
  onReset,
  onSend,
  sessionId,
  setupMessage,
  setupRequired,
}: ComposerProps) {
  const openModelSettings = useShellStore((state) => state.openModelSettings);
  const openSettings = useShellStore((state) => state.openSettings);

  function handleKeyDown(event: ReactKeyboardEvent<HTMLTextAreaElement>) {
    if (setupRequired) return;
    if (event.nativeEvent.isComposing) return;
    if (event.key === "Enter" && !event.shiftKey) {
      event.preventDefault();
      onSend();
    }
  }

  return (
    <section className="sticky bottom-0 z-10 shrink-0 bg-white px-5 pb-4 pt-2 sm:px-6 lg:px-8 lg:pb-5">
      <div className="mx-auto w-full max-w-[980px]">
        <div className="overflow-hidden rounded-[30px] border border-[#edf1f5] bg-white shadow-[0_10px_28px_rgba(15,23,42,0.04)]">
          <div className="flex items-center justify-between gap-3 px-5 pb-0 pt-3 text-[13px] text-[#98a2b3]">
            <div className="min-w-0 truncate">
              <span>{activeAgentLabel}</span>
              <span className="mx-2 text-[#d0d5dd]">/</span>
              <span>{sessionId ? `会话 ${sessionId.slice(-8)}` : "新会话"}</span>
            </div>

            <button
              className="text-[13px] font-medium text-[#98a2b3] transition-colors duration-150 hover:text-[#475467]"
              onClick={onReset}
              type="button"
            >
              新对话
            </button>
          </div>

          <div className="px-5 pt-1.5">
            <textarea
              className="min-h-[58px] max-h-[128px] w-full resize-none bg-transparent text-[15px] leading-7 text-ink outline-none placeholder:text-[#b1b8c6] disabled:cursor-not-allowed disabled:text-[#98a2b3]"
              disabled={setupRequired}
              onChange={(event) => onDraftChange(event.target.value)}
              onKeyDown={handleKeyDown}
              placeholder={setupRequired ? "先完成模型配置后再开始对话" : "可以描述任务或提出任何问题"}
              value={draft}
            />
          </div>

          {setupRequired ? <div className="px-5 pt-1 text-sm text-[#475467]">{setupMessage}</div> : null}
          {!setupRequired ? <InlineErrorPanel chatTaskState={chatTaskState} error={error} /> : null}

          <div className="mt-1 flex flex-col gap-2.5 border-t border-[#f2f4f7] px-5 py-2.5 lg:flex-row lg:items-center lg:justify-between">
            <div className="flex flex-wrap items-center gap-2">
              <button
                className={[
                  "chip-button gap-2 px-4 py-2 text-sm",
                  setupRequired ? "bg-[#1f2430] text-white hover:bg-[#111827]" : "text-[#667085]",
                ].join(" ")}
                onClick={openModelSettings}
                type="button"
              >
                <SlidersHorizontal size={15} strokeWidth={2.1} />
                <span>{modelLabel}</span>
                <ChevronDown size={14} strokeWidth={2.1} />
              </button>

              <button
                className="chip-button gap-2 px-4 py-2 text-sm text-[#667085]"
                onClick={() => openSettings("skills")}
                type="button"
              >
                <Sparkles size={15} strokeWidth={2.1} />
                <span>技能</span>
                <ChevronDown size={14} strokeWidth={2.1} />
              </button>
            </div>

            <div className="flex items-center justify-between gap-4">
              <span className="text-xs text-[#98a2b3]">
                {setupRequired ? "先配置模型提供商，再开始当前会话" : isSending ? "正在思考..." : "Enter 发送 · Shift + Enter 换行"}
              </span>

              <button
                aria-label={isSending ? "正在思考" : "发送消息"}
                className="flex h-11 w-11 items-center justify-center rounded-full bg-[#bfc8d8] text-white transition-all duration-150 hover:bg-[#1f2430] disabled:cursor-not-allowed disabled:bg-[#d7dde7]"
                disabled={!canSend || isSending}
                onClick={onSend}
                type="button"
              >
                {isSending ? (
                  <LoaderCircle className="animate-spin" size={17} strokeWidth={2.2} />
                ) : (
                  <SendHorizontal size={17} strokeWidth={2.2} />
                )}
              </button>
            </div>
          </div>
        </div>
      </div>
    </section>
  );
}
