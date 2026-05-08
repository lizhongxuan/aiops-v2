import { createContext, useContext } from "react";

export type SessionTargetContextValue = {
  targetValue: string;
  targetKind: "all" | "host" | "label" | "k8s";
  targetLabel: string;
  targetDescription: string;
  hostId?: string;
  metadata: Record<string, string>;
};

const defaultSessionTarget: SessionTargetContextValue = {
  targetValue: "all",
  targetKind: "all",
  targetLabel: "全部主机",
  targetDescription: "全部主机上下文",
  metadata: {
    "aiops.target.kind": "all",
    "aiops.target.label": "全部主机",
  },
};

export const SessionTargetContext = createContext<SessionTargetContextValue>(defaultSessionTarget);

export function useSessionTargetContext() {
  return useContext(SessionTargetContext);
}
