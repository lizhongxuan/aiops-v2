import { createContext, useContext } from "react";

import type { SessionKind } from "@/pages/settingsApi";

type SessionWorkspaceContextValue = {
  kind: SessionKind;
  title: string;
  activeSessionId: string;
  activeSessionLabel: string;
  llmLabel: string;
  llmConfigured: boolean;
  busy: boolean;
  composerDisabledReason: string;
  composerFocusNonce: number;
  createSession: () => void;
  clearContext: () => void;
  refreshContext: () => void;
};

const noop = () => {};

const defaultSessionWorkspaceContext: SessionWorkspaceContextValue = {
  kind: "single_host",
  title: "",
  activeSessionId: "",
  activeSessionLabel: "未创建",
  llmLabel: "LLM 未配置",
  llmConfigured: false,
  busy: false,
  composerDisabledReason: "正在初始化会话",
  composerFocusNonce: 0,
  createSession: noop,
  clearContext: noop,
  refreshContext: noop,
};

export const SessionWorkspaceContext = createContext<SessionWorkspaceContextValue>(defaultSessionWorkspaceContext);

export function useSessionWorkspaceContext() {
  return useContext(SessionWorkspaceContext);
}
