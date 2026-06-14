import { AlertCircle, CheckCircle2, Loader2 } from "lucide-react";
import type { PropsWithChildren, ReactNode } from "react";

import { useRegisterAppShellPageChrome } from "@/app/AppShellChromeContext";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { cn } from "@/lib/utils";

export function SettingsPageFrame({
  title,
  description,
  actions,
  children,
}: PropsWithChildren<{ title: string; description: string; actions?: ReactNode }>) {
  useRegisterAppShellPageChrome({ title, description, actions: actions || null });

  return (
    <section className="h-full overflow-y-auto bg-slate-50 px-4 py-5 text-slate-900 lg:px-6">
      <div className="mx-auto flex w-full max-w-7xl flex-col gap-4">
        {children}
      </div>
    </section>
  );
}

export function StatGrid({ items }: { items: Array<{ label: string; value: ReactNode; tone?: "ok" | "warn" | "bad" }> }) {
  return (
    <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-4">
      {items.map((item) => (
        <Card key={item.label} size="sm" className="rounded-lg bg-white">
          <CardHeader className="pb-1">
            <CardDescription className="text-xs font-medium uppercase tracking-normal text-slate-500">{item.label}</CardDescription>
            <CardTitle
              className={cn(
                "break-words text-lg",
                item.tone === "ok" && "text-emerald-700",
                item.tone === "warn" && "text-amber-700",
                item.tone === "bad" && "text-red-700",
              )}
            >
              {item.value}
            </CardTitle>
          </CardHeader>
        </Card>
      ))}
    </div>
  );
}

export function Field({ label, hint, children }: PropsWithChildren<{ label: string; hint?: string }>) {
  return (
    <label className="grid gap-1.5 text-sm">
      <span className="font-medium text-slate-700">{label}</span>
      {children}
      {hint ? <span className="text-xs leading-5 text-slate-500">{hint}</span> : null}
    </label>
  );
}

export function SelectField({
  value,
  onChange,
  options,
  "aria-label": ariaLabel,
  "data-testid": dataTestId,
}: {
  value: string;
  onChange: (value: string) => void;
  options: Array<{ label: string; value: string }>;
  "aria-label"?: string;
  "data-testid"?: string;
}) {
  return (
    <select
      aria-label={ariaLabel}
      data-testid={dataTestId}
      value={value}
      onChange={(event) => onChange(event.target.value)}
      className="h-8 w-full rounded-lg border border-input bg-white px-2.5 text-sm outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50"
    >
      {options.map((option) => (
        <option key={option.value} value={option.value}>
          {option.label}
        </option>
      ))}
    </select>
  );
}

export function StatusAlert({ type, title, message }: { type: "success" | "error" | "info"; title: string; message: string }) {
  const Icon = type === "success" ? CheckCircle2 : AlertCircle;
  return (
    <Alert variant={type === "error" ? "destructive" : "default"} className="rounded-lg bg-white">
      <Icon className="h-4 w-4" />
      <AlertTitle>{title}</AlertTitle>
      <AlertDescription className="whitespace-pre-wrap break-words">{message}</AlertDescription>
    </Alert>
  );
}

export function LoadingState({ label = "Loading" }: { label?: string }) {
  return (
    <div className="flex min-h-[220px] items-center justify-center text-sm text-slate-500">
      <Loader2 className="mr-2 h-4 w-4 animate-spin" />
      {label}
    </div>
  );
}

export function EmptyState({ title, description }: { title: string; description: string }) {
  return (
    <Card className="rounded-lg bg-white">
      <CardContent className="py-8 text-center">
        <div className="font-medium text-slate-900">{title}</div>
        <p className="mt-1 text-sm text-slate-500">{description}</p>
      </CardContent>
    </Card>
  );
}

export function ToneBadge({ children, tone = "default" }: PropsWithChildren<{ tone?: "default" | "success" | "warning" | "danger" }>) {
  const variant = tone === "danger" ? "destructive" : tone === "default" ? "secondary" : "outline";
  return (
    <Badge
      variant={variant}
      className={cn(
        "rounded-md",
        tone === "success" && "border-emerald-200 bg-emerald-50 text-emerald-700",
        tone === "warning" && "border-amber-200 bg-amber-50 text-amber-700",
      )}
    >
      {children}
    </Badge>
  );
}

export function ConfirmButton({
  children,
  confirm,
  onConfirm,
  ...props
}: PropsWithChildren<React.ComponentProps<typeof Button> & { confirm: string; onConfirm: () => void }>) {
  return (
    <Button
      {...props}
      onClick={(event) => {
        props.onClick?.(event);
        if (!event.defaultPrevented && window.confirm(confirm)) {
          onConfirm();
        }
      }}
    >
      {children}
    </Button>
  );
}
