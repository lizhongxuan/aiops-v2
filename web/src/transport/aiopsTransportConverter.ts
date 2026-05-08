import type {
  AssistantTransportConnectionMetadata,
  ThreadMessage,
} from "@assistant-ui/react";

import type {
  AiopsTranscriptBlock,
  AiopsTransportState,
  AiopsTransportTurn,
} from "./aiopsTransportTypes";

type ConverterResult = {
  messages: ThreadMessage[];
  isRunning: boolean;
  state: AiopsTransportState;
};

export function createAiopsTransportConverter() {
  return (
    state: AiopsTransportState,
    connectionMetadata: AssistantTransportConnectionMetadata,
  ): ConverterResult => {
    const messages = orderedTurnMessages(state).concat(
      optimisticPendingUserMessages(connectionMetadata),
    );

    return {
      messages,
      isRunning: isAiopsTransportRunning(state) || connectionMetadata.isSending,
      state,
    };
  };
}

export function isAiopsTransportRunning(state: AiopsTransportState) {
  if (state.status === "working" || state.status === "blocked") {
    return true;
  }
  return Object.keys(state.runtimeLiveness.activeTurns || {}).length > 0;
}

function orderedTurnMessages(state: AiopsTransportState) {
  return state.turnOrder.flatMap((turnId) => {
    const turn = state.turns[turnId];
    if (!turn) {
      return [];
    }
    const messages: ThreadMessage[] = [];
    const userMessage = toUserThreadMessage(turn);
    if (userMessage) {
      messages.push(userMessage);
    }
    const assistantMessage = toAssistantThreadMessage(turn, state.lastError);
    if (assistantMessage) {
      messages.push(assistantMessage);
    }
    return messages;
  });
}

function toUserThreadMessage(turn: AiopsTransportTurn): ThreadMessage | null {
  if (!turn.user?.text) {
    return null;
  }
  return {
    id: turn.user.id || `${turn.id}:user`,
    role: "user",
    createdAt: parseDate(turn.user.createdAt || turn.startedAt),
    content: [{ type: "text", text: turn.user.text }],
    attachments: [],
    metadata: { custom: { turnId: turn.id, source: "aiops.transport.user" } },
  };
}

function toAssistantThreadMessage(turn: AiopsTransportTurn, lastError?: string): ThreadMessage | null {
  if (!shouldShowAssistantMessage(turn)) {
    return null;
  }
  const transcript = transcriptForAssistantMessage(turn, lastError);
  return {
    id: `${turn.id}:assistant`,
    role: "assistant",
    createdAt: parseDate(turn.startedAt || turn.completedAt),
    content: [],
    status: assistantMessageStatus(turn, lastError),
    metadata: {
      unstable_annotations: [],
      unstable_data: [],
      steps: [],
      custom: {
        source: "aiops.transport.assistant",
        aiops: {
          turnId: turn.id,
          turnStatus: turn.status,
          turnStartedAt: turn.startedAt,
          turnCompletedAt: turn.completedAt,
          turnUpdatedAt: turn.updatedAt || turn.completedAt || turn.startedAt,
          blocks: transcript.blocks,
          blockOrder: transcript.blockOrder,
          blocksById: transcript.blocksById,
        },
      },
    },
  };
}

function shouldShowAssistantMessage(turn: AiopsTransportTurn) {
  if (orderedTurnBlocks(turn).length > 0) {
    return true;
  }
  if (turn.status === "failed" || turn.status === "canceled") {
    return true;
  }
  return turn.status === "submitted" || turn.status === "working" || turn.status === "blocked";
}

function transcriptForAssistantMessage(turn: AiopsTransportTurn, lastError?: string) {
  const blocks = orderedTurnBlocks(turn);
  if (blocks.length > 0) {
    return {
      blocks,
      blockOrder: turn.blockOrder || blocks.map((block) => block.id),
      blocksById: turn.blocksById || blocksByIdFromBlocks(blocks),
    };
  }
  if (turn.status !== "failed") {
    return {
      blocks,
      blockOrder: [],
      blocksById: {},
    };
  }
  const timestamp = turn.updatedAt || turn.completedAt || turn.startedAt;
  const errorBlock = {
    id: `${turn.id}:error`,
    type: "text",
    text: {
      role: "assistant",
      text: lastError || "turn failed",
      status: "completed",
    },
    createdAt: timestamp,
    updatedAt: timestamp,
  } satisfies AiopsTranscriptBlock;
  return {
    blocks: [errorBlock],
    blockOrder: [errorBlock.id],
    blocksById: { [errorBlock.id]: errorBlock },
  };
}

function orderedTurnBlocks(turn: AiopsTransportTurn): AiopsTranscriptBlock[] {
  return (turn.blockOrder || [])
    .map((id) => turn.blocksById?.[id])
    .filter((block): block is AiopsTranscriptBlock => Boolean(block));
}

function blocksByIdFromBlocks(blocks: AiopsTranscriptBlock[]) {
  return Object.fromEntries(blocks.map((block) => [block.id, block]));
}

function optimisticPendingUserMessages(connectionMetadata: AssistantTransportConnectionMetadata) {
  return connectionMetadata.pendingCommands.flatMap((command, index) => {
    if (command.type !== "add-message" || command.message.role !== "user") {
      return [];
    }
    const text = command.message.parts
      .filter((part) => part.type === "text")
      .map((part) => part.text)
      .join("\n")
      .trim();
    if (!text) {
      return [];
    }
    return [
      {
        id: `optimistic:add-message:${index}`,
        role: "user" as const,
        createdAt: new Date(),
        content: [{ type: "text" as const, text }],
        attachments: [],
        metadata: { custom: { optimistic: true } },
      } satisfies ThreadMessage,
    ];
  });
}

function assistantMessageStatus(turn: AiopsTransportTurn, lastError?: string): ThreadMessage["status"] {
  switch (turn.status) {
    case "completed":
      return { type: "complete", reason: "stop" };
    case "blocked":
      return { type: "requires-action", reason: "interrupt" };
    case "failed":
      return { type: "incomplete", reason: "error", error: lastError || "turn failed" };
    case "canceled":
      return { type: "incomplete", reason: "cancelled" };
    default:
      return { type: "running" };
  }
}

function parseDate(value?: string) {
  if (!value) {
    return new Date(0);
  }
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) {
    return new Date(0);
  }
  return parsed;
}
