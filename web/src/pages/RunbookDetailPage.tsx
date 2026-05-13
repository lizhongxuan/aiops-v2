import { ArrowRight } from "lucide-react";
import { Link, useParams } from "react-router-dom";

import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { ComplexPageFrame } from "@/pages/complexPageComponents";

export function RunbookDetailPage() {
  const { runbookId = "" } = useParams();

  return (
    <ComplexPageFrame
      kicker="Runbook 兼容"
      title="Runbook 兼容详情"
      description="旧 Runbook 详情路由仍可访问，当前生产操作编排请进入 Runner Workflow。"
      actions={
        <>
          <Button variant="outline" asChild>
            <Link to="/runbooks">返回 Runbook 兼容入口</Link>
          </Button>
          <Button asChild>
            <Link to="/runner"><ArrowRight />前往 Runner Workflow</Link>
          </Button>
        </>
      }
    >
      <Card className="rounded-lg bg-white">
        <CardHeader>
          <CardTitle>请使用 Runner Workflow</CardTitle>
          <CardDescription>Runbook 已不作为当前方案主概念，请使用 Runner Workflow。</CardDescription>
        </CardHeader>
        <CardContent className="space-y-4 text-sm text-slate-600">
          <p>旧 Runbook 标识：{runbookId || "未指定"}</p>
          <p>此页面仅用于保留旧详情链接兼容，不再读取旧 Runbook 详情、匹配测试或动作提案接口。</p>
          <Button asChild>
            <Link to="/runner"><ArrowRight />前往 Runner Workflow</Link>
          </Button>
        </CardContent>
      </Card>
    </ComplexPageFrame>
  );
}
