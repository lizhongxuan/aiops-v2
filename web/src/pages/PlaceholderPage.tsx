import { Badge } from "@/components/ui/badge";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";

type PlaceholderPageProps = {
  title: string;
  description: string;
  routePath: string;
};

export function PlaceholderPage({ title, description, routePath }: PlaceholderPageProps) {
  return (
    <div className="h-full overflow-y-auto px-4 py-6 lg:px-6 lg:py-8">
      <div className="mx-auto flex w-full max-w-6xl flex-col gap-6">
        <Card>
          <CardHeader>
            <Badge variant="secondary" className="w-fit">
              Migration Placeholder
            </Badge>
            <CardTitle className="mt-2 text-2xl">{title}</CardTitle>
            <CardDescription className="max-w-2xl text-sm leading-6">{description}</CardDescription>
          </CardHeader>
          <CardContent>
            <div className="inline-flex rounded-md border border-slate-200 bg-slate-50 px-3 py-1.5 text-xs text-slate-600">{routePath}</div>
          </CardContent>
        </Card>
        <section className="grid gap-4 lg:grid-cols-2">
          <Card>
            <CardHeader>
              <CardTitle className="text-sm">Status</CardTitle>
            </CardHeader>
            <CardContent className="text-sm leading-6 text-slate-600">
              This route is mounted in the React shell and ready for page-by-page migration onto AssistantTransport and shadcn/ui.
            </CardContent>
          </Card>
          <Card>
            <CardHeader>
              <CardTitle className="text-sm">Scope</CardTitle>
            </CardHeader>
            <CardContent className="text-sm leading-6 text-slate-600">
              This route now belongs to the React shell. Historical implementations are not part of the active entry path.
            </CardContent>
          </Card>
        </section>
      </div>
    </div>
  );
}
