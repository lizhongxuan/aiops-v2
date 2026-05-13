import { ArrowRight } from "lucide-react";
import { Link } from "react-router-dom";

import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { ComplexPageFrame } from "@/pages/complexPageComponents";

export function RunbookCatalogPage() {
  return (
    <ComplexPageFrame
      kicker="Runbook 兼容"
      title="Runbook 兼容入口"
      description="旧 Runbook 路由仍可访问，当前生产操作编排请进入 Runner Workflow。"
      actions={
        <Button asChild>
          <Link to="/runner"><ArrowRight />前往 Runner Workflow</Link>
        </Button>
      }
    >
      <Card className="rounded-lg bg-white">
        <CardHeader>
          <CardTitle>请使用 Runner Workflow</CardTitle>
          <CardDescription>Runbook 已不作为当前方案主概念，请使用 Runner Workflow。</CardDescription>
        </CardHeader>
        <CardContent className="space-y-4 text-sm text-slate-600">
          <p>此页面仅用于保留旧链接兼容，不再读取旧 Runbook 目录或匹配接口。</p>
          <Button asChild>
            <Link to="/runner"><ArrowRight />前往 Runner Workflow</Link>
          </Button>
        </CardContent>
      </Card>
    </ComplexPageFrame>
  );
}
