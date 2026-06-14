import { Textarea } from "@/components/ui/textarea";

import type { HostMentionCandidate } from "../hostMentions";
import { ComposerHostMentionMenu } from "./ComposerHostMentionMenu";

export function HostMentionComposer({
  value,
  mentions,
  onChange,
}: {
  value: string;
  mentions: HostMentionCandidate[];
  onChange: (value: string) => void;
}) {
  return (
    <div className="grid gap-2">
      <ComposerHostMentionMenu mentions={mentions} />
      <Textarea
        value={value}
        rows={1}
        className="min-h-12 resize-none text-[16px] leading-7 md:text-[16px]"
        onChange={(event) => onChange(event.currentTarget.value)}
      />
    </div>
  );
}
