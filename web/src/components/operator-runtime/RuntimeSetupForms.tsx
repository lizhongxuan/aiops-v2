import type { RuntimeCollectionKey, RuntimeCollections } from "./operatorRuntimeModels";
import { ActionCatalogForm } from "./ActionCatalogForm";
import { GuardRuleForm } from "./GuardRuleForm";
import { InspectionTemplateForm } from "./InspectionTemplateForm";
import { ManagedResourceForm } from "./ManagedResourceForm";
import { ProblemTypeForm } from "./ProblemTypeForm";
import { WorkflowBindingForm } from "./WorkflowBindingForm";

import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";

export function RuntimeSetupForms({
  busy,
  collections,
  onCreate,
  onCreateRule,
}: {
  busy?: string;
  collections: RuntimeCollections;
  onCreate: (key: RuntimeCollectionKey, payload: unknown) => void;
  onCreateRule: (payload: unknown) => void;
}) {
  return (
    <Card className="rounded-lg bg-white" size="sm">
      <CardHeader className="pb-0">
        <CardTitle>对象配置</CardTitle>
        <CardDescription>按资源顺序配置通用自愈守护规则，PostgreSQL 只是一个示例模板。</CardDescription>
      </CardHeader>
      <CardContent>
        <Tabs defaultValue="resource" className="gap-3">
          <TabsList className="flex h-auto w-full flex-wrap justify-start gap-1 bg-slate-100">
            <TabsTrigger value="resource">受管资源</TabsTrigger>
            <TabsTrigger value="template">巡检模板</TabsTrigger>
            <TabsTrigger value="problem">问题类型</TabsTrigger>
            <TabsTrigger value="action">动作</TabsTrigger>
            <TabsTrigger value="binding">Workflow 绑定</TabsTrigger>
            <TabsTrigger value="rule">守护规则</TabsTrigger>
          </TabsList>
          <TabsContent value="resource">
            <ManagedResourceForm busy={busy} onSubmit={(payload) => onCreate("resources", payload)} />
          </TabsContent>
          <TabsContent value="template">
            <InspectionTemplateForm busy={busy} onSubmit={(payload) => onCreate("inspectionTemplates", payload)} />
          </TabsContent>
          <TabsContent value="problem">
            <ProblemTypeForm busy={busy} onSubmit={(payload) => onCreate("problemTypes", payload)} />
          </TabsContent>
          <TabsContent value="action">
            <ActionCatalogForm busy={busy} onSubmit={(payload) => onCreate("actions", payload)} />
          </TabsContent>
          <TabsContent value="binding">
            <WorkflowBindingForm
              actions={collections.actions}
              busy={busy}
              onSubmit={(payload) => onCreate("workflowBindings", payload)}
            />
          </TabsContent>
          <TabsContent value="rule">
            <GuardRuleForm
              resources={collections.resources}
              inspectionTemplates={collections.inspectionTemplates}
              problemTypes={collections.problemTypes}
              actions={collections.actions}
              workflowBindings={collections.workflowBindings}
              busy={busy}
              onSubmit={onCreateRule}
            />
          </TabsContent>
        </Tabs>
      </CardContent>
    </Card>
  );
}
