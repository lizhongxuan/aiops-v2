import { Link, useParams } from "react-router-dom";

import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { ComplexPageFrame, EmptyPanel, RiskBadge } from "@/pages/complexPageComponents";
import { asArray, compactText } from "@/pages/complexPagesApi";

const fallbackPostmortem = {
  id: "postmortem-draft",
  rootCause: "待补充",
  timeline: [],
  impact: [],
  actions: [],
  approvals: [],
  verification: [],
  followUps: [],
};

export function PostmortemPage() {
  const { postmortemId = "" } = useParams();
  const postmortem = fallbackPostmortem;
  return (
    <ComplexPageFrame kicker="Postmortem" title="复盘草稿" actions={<Button variant="outline" asChild><Link to="/incidents">事故工作台</Link></Button>}>
      <div className="flex flex-wrap gap-2"><RiskBadge value={compactText(postmortem.id || postmortemId) || "未选择复盘"} /><RiskBadge value="Draft" /></div>
      <section className="grid gap-4 xl:grid-cols-2">
        <Panel title="时间线" items={asArray(postmortem.timeline)} empty="暂无时间线" />
        <Panel title="影响面" items={asArray(postmortem.impact)} empty="暂无影响面" />
        <Card className="rounded-lg bg-white"><CardHeader><CardTitle>根因与促成因素</CardTitle></CardHeader><CardContent><p className="text-sm leading-6 text-slate-700">{postmortem.rootCause || "待补充"}</p></CardContent></Card>
        <Panel title="后续行动" items={asArray(postmortem.actions)} empty="暂无行动项" />
        <Panel title="Approvals" items={asArray(postmortem.approvals)} empty="暂无审批记录" />
        <Panel title="Verification" items={asArray(postmortem.verification)} empty="暂无验证记录" />
        <Panel title="Follow-ups" items={asArray(postmortem.followUps)} empty="暂无后续事项" />
      </section>
    </ComplexPageFrame>
  );
}

function Panel({ title, items, empty }: { title: string; items: Record<string, unknown>[]; empty: string }) {
  return <Card className="rounded-lg bg-white"><CardHeader><CardTitle>{title}</CardTitle></CardHeader><CardContent>{items.length ? <ul className="grid gap-2 text-sm">{items.map((item, index) => <li key={compactText(item.id || item.title || item.name) || index}>{compactText(item.title || item.name || item.summary) || `条目 ${index + 1}`}</li>)}</ul> : <EmptyPanel title={empty} description="待 Agent 或人工补充。" />}</CardContent></Card>;
}
