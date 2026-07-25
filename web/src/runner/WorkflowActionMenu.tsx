import {
  BookOpen,
  Copy,
  Download,
  Ellipsis,
  Keyboard,
  Trash2,
  Upload,
} from "lucide-react";
import {
  Fragment,
  forwardRef,
  useEffect,
  useState,
  type ComponentProps,
} from "react";

import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
  Sheet,
  SheetClose,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
  SheetTrigger,
} from "@/components/ui/sheet";

export type WorkflowActionMenuItemId =
  | "import"
  | "export"
  | "manual"
  | "shortcuts"
  | "duplicate"
  | "delete";

export type WorkflowActionMenuGroupId = "file" | "assist" | "management";

export type WorkflowActionMenuItem = {
  id: WorkflowActionMenuItemId;
  label: string;
  group: WorkflowActionMenuGroupId;
  danger?: boolean;
  disabled?: boolean;
  onSelect: () => void;
};

export type WorkflowActionMenuGroup = {
  id: WorkflowActionMenuGroupId;
  label: string;
  items: WorkflowActionMenuItem[];
};

const GROUPS: ReadonlyArray<{ id: WorkflowActionMenuGroupId; label: string }> = [
  { id: "file", label: "文件" },
  { id: "assist", label: "辅助" },
  { id: "management", label: "管理" },
];

const ALLOWED_ITEM_IDS = new Set<WorkflowActionMenuItemId>([
  "import",
  "export",
  "manual",
  "shortcuts",
  "duplicate",
  "delete",
]);

export function groupWorkflowActionMenuItems(
  items: readonly WorkflowActionMenuItem[],
): WorkflowActionMenuGroup[] {
  return GROUPS.flatMap((group) => {
    const available = items.filter((item) =>
      ALLOWED_ITEM_IDS.has(item.id) && item.group === group.id);
    return available.length > 0 ? [{ ...group, items: available }] : [];
  });
}

export function WorkflowActionMenu({
  items,
  disabled = false,
}: {
  items: readonly WorkflowActionMenuItem[];
  disabled?: boolean;
}) {
  const groups = groupWorkflowActionMenuItems(items);
  const mobile = useMobileActionSheet();
  if (groups.length === 0) return null;

  if (mobile) {
    return (
      <Sheet>
        <SheetTrigger asChild>
          <ActionMenuTrigger disabled={disabled} />
        </SheetTrigger>
        <SheetContent
          side="bottom"
          className="workflow-action-menu-sheet gap-0 rounded-t-2xl pb-[max(1rem,env(safe-area-inset-bottom))]"
          data-testid="workflow-action-menu"
          aria-label="更多工作流操作"
        >
          <SheetHeader className="border-b border-slate-200 pr-14">
            <SheetTitle>更多工作流操作</SheetTitle>
            <SheetDescription>文件、辅助与管理操作</SheetDescription>
          </SheetHeader>
          <div className="workflow-action-menu-sheet__groups" role="menu" aria-label="更多工作流操作">
            {groups.map((group) => (
              <section key={group.id} aria-labelledby={`workflow-action-menu-group-${group.id}`}>
                <h3 id={`workflow-action-menu-group-${group.id}`}>{group.label}</h3>
                {group.items.map((item) => (
                  <SheetClose key={item.id} asChild>
                    <button
                      type="button"
                      role="menuitem"
                      disabled={item.disabled}
                      data-action={item.id}
                      data-danger={item.danger ? "true" : undefined}
                      onClick={item.onSelect}
                    >
                      {menuIcon(item.id)}
                      <span>{item.label}</span>
                    </button>
                  </SheetClose>
                ))}
              </section>
            ))}
          </div>
        </SheetContent>
      </Sheet>
    );
  }

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild disabled={disabled}>
        <ActionMenuTrigger disabled={disabled} />
      </DropdownMenuTrigger>
      <DropdownMenuContent
        align="end"
        sideOffset={8}
        className="workflow-action-menu__content min-w-56"
        data-testid="workflow-action-menu"
        aria-label="更多工作流操作"
      >
        {groups.map((group, index) => (
          <Fragment key={group.id}>
            {index > 0 ? <DropdownMenuSeparator /> : null}
            <DropdownMenuLabel>{group.label}</DropdownMenuLabel>
            <DropdownMenuGroup>
              {group.items.map((item) => (
                <DropdownMenuItem
                  key={item.id}
                  disabled={item.disabled}
                  variant={item.danger ? "destructive" : "default"}
                  className="min-h-9 w-full min-w-0 gap-2.5 px-2.5 py-2"
                  onSelect={item.onSelect}
                  data-action={item.id}
                >
                  {menuIcon(item.id)}
                  <span className="min-w-0 flex-1 whitespace-nowrap text-left">{item.label}</span>
                </DropdownMenuItem>
              ))}
            </DropdownMenuGroup>
          </Fragment>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

const ActionMenuTrigger = forwardRef<HTMLButtonElement, ComponentProps<"button">>(
  function ActionMenuTrigger({ disabled = false, ...props }, ref) {
    return (
      <button
        ref={ref}
        type="button"
        disabled={disabled}
        className="workflow-action-menu-trigger inline-flex h-9 w-9 shrink-0 items-center justify-center rounded-lg border border-slate-300 bg-white text-slate-700 hover:bg-slate-50 disabled:cursor-not-allowed disabled:opacity-50"
        data-testid="workflow-action-menu-trigger"
        aria-label="更多工作流操作"
        title="更多工作流操作"
        {...props}
      >
        <Ellipsis className="h-4 w-4" aria-hidden="true" />
      </button>
    );
  },
);

function useMobileActionSheet() {
  const query = "(max-width: 820px)";
  const [mobile, setMobile] = useState(() =>
    typeof window !== "undefined" && typeof window.matchMedia === "function"
      ? window.matchMedia(query).matches
      : false);

  useEffect(() => {
    if (typeof window.matchMedia !== "function") return undefined;
    const media = window.matchMedia(query);
    const update = () => setMobile(media.matches);
    update();
    media.addEventListener?.("change", update);
    return () => media.removeEventListener?.("change", update);
  }, []);
  return mobile;
}

function menuIcon(id: WorkflowActionMenuItemId) {
  const className = "h-4 w-4";
  if (id === "import") return <Upload className={className} aria-hidden="true" />;
  if (id === "export") return <Download className={className} aria-hidden="true" />;
  if (id === "manual") return <BookOpen className={className} aria-hidden="true" />;
  if (id === "shortcuts") return <Keyboard className={className} aria-hidden="true" />;
  if (id === "duplicate") return <Copy className={className} aria-hidden="true" />;
  return <Trash2 className={className} aria-hidden="true" />;
}
