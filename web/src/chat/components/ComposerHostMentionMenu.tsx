import type { HostMentionCandidate } from "../hostMentions";

import { HostMentionChip } from "./HostMentionChip";

export function ComposerHostMentionMenu({ mentions }: { mentions: HostMentionCandidate[] }) {
  if (mentions.length === 0) {
    return null;
  }
  return (
    <div className="flex flex-wrap gap-1.5" data-testid="host-mention-chip-list">
      {mentions.map((mention) => (
        <HostMentionChip key={mention.tokenId} mention={mention} />
      ))}
    </div>
  );
}
