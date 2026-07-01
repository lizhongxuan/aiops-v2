export type CapabilityGroupTitle = "常用" | "资产" | "能力" | "文件";

export type CapabilityGroup = {
  title: CapabilityGroupTitle;
};

export const defaultCapabilityGroups: CapabilityGroup[] = [
  { title: "常用" },
  { title: "资产" },
  { title: "能力" },
  { title: "文件" },
];

const capabilityKinds = new Set(["skill", "plugin", "mcp_server", "connector"]);

export function capabilityGroupForKind(kind: unknown): CapabilityGroupTitle {
  return capabilityKinds.has(String(kind)) ? "能力" : "常用";
}
