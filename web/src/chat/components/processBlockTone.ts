import type { AiopsProcessBlock } from "@/transport/aiopsTransportTypes";

export function toneForStatus(status: AiopsProcessBlock["status"]) {
  switch (status) {
    case "blocked":
      return "border-amber-200 bg-amber-50";
    case "failed":
    case "rejected":
      return "border-red-200 bg-red-50";
    case "running":
      return "border-blue-200 bg-blue-50";
    default:
      return "border-zinc-200 bg-white";
  }
}
