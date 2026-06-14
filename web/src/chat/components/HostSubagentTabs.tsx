import { cn } from "@/lib/utils";

export type HostSubagentTabId =
  | "task"
  | "conversation"
  | "prompt"
  | "tools"
  | "mcp-skills"
  | "approval"
  | "evidence"
  | "receipts";

const tabs: Array<{ id: HostSubagentTabId; label: string }> = [
  { id: "task", label: "任务" },
  { id: "conversation", label: "对话" },
  { id: "prompt", label: "Prompt" },
  { id: "tools", label: "工具" },
  { id: "mcp-skills", label: "MCP/Skills" },
  { id: "approval", label: "审核" },
  { id: "evidence", label: "证据" },
  { id: "receipts", label: "回执" },
];

type HostSubagentTabsProps = {
  activeTab: HostSubagentTabId;
  onTabChange: (tab: HostSubagentTabId) => void;
};

export function HostSubagentTabs({ activeTab, onTabChange }: HostSubagentTabsProps) {
  return (
    <div className="border-b border-zinc-200 px-4 py-2">
      <div className="flex min-w-0 gap-1 overflow-x-auto" role="tablist" aria-label="主机 Agent 详情页签">
        {tabs.map((tab) => (
          <button
            key={tab.id}
            type="button"
            role="tab"
            aria-selected={activeTab === tab.id}
            data-testid={`host-subagent-tab-${tab.id}`}
            className={cn(
              "rounded-md px-2.5 py-1 text-xs font-medium text-zinc-500 hover:bg-zinc-50 hover:text-zinc-800",
              activeTab === tab.id && "bg-zinc-100 text-zinc-900",
            )}
            onClick={() => onTabChange(tab.id)}
          >
            {tab.label}
          </button>
        ))}
      </div>
    </div>
  );
}
