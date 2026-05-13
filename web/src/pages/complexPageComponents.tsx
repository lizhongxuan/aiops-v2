import type { PropsWithChildren, ReactNode } from "react";

import { useRegisterAppShellPageChrome } from "@/app/AppShellChromeContext";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { cn } from "@/lib/utils";

export function ComplexPageFrame({
  kicker,
  title,
  description,
  actions,
  children,
}: PropsWithChildren<{ kicker: string; title: string; description?: string; actions?: ReactNode }>) {
  useRegisterAppShellPageChrome({ title, description: description || kicker, actions: actions || null });

  return (
    <section className="h-full overflow-y-auto bg-slate-50 px-4 py-5 text-slate-900 lg:px-6">
      <div className="mx-auto flex w-full max-w-7xl flex-col gap-4">
        {children}
      </div>
    </section>
  );
}

export function MetricStrip({ items }: { items: Array<{ label: string; value: ReactNode; tone?: "ok" | "warn" | "bad" }> }) {
  return (
    <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-4">
      {items.map((item) => (
        <Card key={item.label} size="sm" className="rounded-lg bg-white">
          <CardHeader className="pb-1">
            <CardDescription className="text-xs font-medium uppercase tracking-normal text-slate-500">{item.label}</CardDescription>
            <CardTitle className={cn("text-lg", item.tone === "ok" && "text-emerald-700", item.tone === "warn" && "text-amber-700", item.tone === "bad" && "text-red-700")}>{item.value}</CardTitle>
          </CardHeader>
        </Card>
      ))}
    </div>
  );
}

export function RiskBadge({ value }: { value?: string }) {
  const text = value || "unknown";
  const tone = text.toLowerCase();
  return (
    <Badge
      variant={tone.includes("high") || tone.includes("sev1") ? "destructive" : "outline"}
      className={cn(
        "rounded-md bg-white",
        (tone.includes("low") || tone.includes("ok")) && "border-emerald-200 bg-emerald-50 text-emerald-700",
        (tone.includes("medium") || tone.includes("sev2") || tone.includes("warning")) && "border-amber-200 bg-amber-50 text-amber-700",
      )}
    >
      {text}
    </Badge>
  );
}

export function EmptyPanel({ title, description }: { title: string; description: string }) {
  return (
    <Card className="rounded-lg bg-white">
      <CardContent className="py-8 text-center">
        <div className="font-medium text-slate-900">{title}</div>
        <p className="mt-1 text-sm text-slate-500">{description}</p>
      </CardContent>
    </Card>
  );
}

export function KeyValueList({ items }: { items: Array<{ label: string; value?: ReactNode }> }) {
  return (
    <dl className="grid gap-2 text-sm">
      {items.map((item) => (
        <div key={item.label} className="flex items-start justify-between gap-3 border-b border-slate-100 pb-2 last:border-0">
          <dt className="text-slate-500">{item.label}</dt>
          <dd className="text-right font-medium text-slate-900">{item.value || "-"}</dd>
        </div>
      ))}
    </dl>
  );
}
