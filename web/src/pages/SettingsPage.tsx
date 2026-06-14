import { ArrowRight, BookOpen, Bot, Boxes, Cable, KeyRound, LayoutGrid, Server, Settings, Wrench } from "lucide-react";
import { Link } from "react-router-dom";

import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { SettingsPageFrame } from "@/pages/settingsComponents";

const primaryEntries = [
  { title: "LLM 配置", description: "模型接入、API Key、模型选择", path: "/settings/llm", icon: KeyRound },
  { title: "Hosts", description: "主机清单、标签、会话与接入状态", path: "/settings/hosts", icon: Server },
  { title: "运维手册", description: "已验证手册、待审核候选、执行记录", path: "/settings/ops-manuals", icon: BookOpen },
  { title: "Agent Profile", description: "System prompt、权限、skills、MCP", path: "/settings/agent", icon: Bot },
  { title: "Skills", description: "Skill catalog 和默认激活策略", path: "/settings/skills", icon: Wrench },
  { title: "MCP Catalog", description: "Agent MCP 绑定和权限策略", path: "/settings/mcp", icon: Cable },
];

const toolEntries = [
  { title: "Capability Center", description: "能力绑定与调试", path: "/capability-center", icon: LayoutGrid },
  { title: "UI Cards", description: "卡片协议调试", path: "/ui-cards", icon: Boxes },
  { title: "Script Configs", description: "脚本配置实验区", path: "/script-configs", icon: Settings },
  { title: "Generator", description: "草稿生成器", path: "/generator", icon: Wrench },
];

export function SettingsPage() {
  return (
    <SettingsPageFrame
      title="设置"
      description="把模型、主机、运维手册和 Agent Profile 收敛到一个入口。React 版本直接消费现有后端 API，不再挂旧 Vue 设置页。"
    >
      <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
        {primaryEntries.map((entry) => (
          <Link key={entry.path} to={entry.path} className="block">
            <Card className="h-full rounded-lg bg-white transition hover:border-slate-300 hover:shadow-sm">
              <CardHeader>
                <div className="flex items-center justify-between gap-3">
                  <entry.icon className="h-4 w-4 text-slate-500" />
                  <ArrowRight className="h-4 w-4 text-slate-400" />
                </div>
                <CardTitle>{entry.title}</CardTitle>
                <CardDescription>{entry.description}</CardDescription>
              </CardHeader>
            </Card>
          </Link>
        ))}
      </div>

      <section className="grid gap-3 lg:grid-cols-[280px_1fr]">
        <Card className="rounded-lg bg-white">
          <CardHeader>
            <Badge variant="secondary" className="w-fit rounded-md">
              Developer Tools
            </Badge>
            <CardTitle>开发工具</CardTitle>
            <CardDescription>这些入口保留给配置和排障，不作为事故处理主路径。</CardDescription>
          </CardHeader>
        </Card>
        <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-4">
          {toolEntries.map((entry) => (
            <Link key={entry.path} to={entry.path}>
              <Card className="h-full rounded-lg bg-white">
                <CardContent className="flex items-start gap-3 pt-1">
                  <entry.icon className="mt-0.5 h-4 w-4 shrink-0 text-slate-500" />
                  <span>
                    <span className="block font-medium text-slate-900">{entry.title}</span>
                    <span className="block text-xs leading-5 text-slate-500">{entry.description}</span>
                  </span>
                </CardContent>
              </Card>
            </Link>
          ))}
        </div>
      </section>
    </SettingsPageFrame>
  );
}
