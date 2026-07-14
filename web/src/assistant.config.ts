import "@assistant-ui/react";

import type { AiopsTransportState } from "./transport/aiopsTransportTypes";

declare module "@assistant-ui/react" {
  namespace Assistant {
    interface ExternalState {
      aiops: AiopsTransportState;
    }

    interface Commands {
      aiopsStop: {
        type: "aiops.stop";
        sessionId?: string;
        turnId?: string;
        reason?: string;
      };
      aiopsRetry: {
        type: "aiops.retry";
        sessionId?: string;
        turnId?: string;
      };
      aiopsApprovalDecision: {
        type: "aiops.approval-decision";
        sessionId?: string;
        turnId?: string;
        approvalId: string;
        decision: string;
      };
      aiopsChoiceAnswer: {
        type: "aiops.choice-answer";
        requestId: string;
        answer: string;
      };
      aiopsMcpAction: {
        type: "aiops.mcp-action";
        surfaceId: string;
        action: string;
        target?: string;
        params?: Record<string, unknown>;
      };
      aiopsMcpRefresh: {
        type: "aiops.mcp-refresh";
        surfaceId: string;
      };
      aiopsMcpPin: {
        type: "aiops.mcp-pin";
        surfaceId: string;
        pinned: boolean;
      };
      aiopsSpecialInputClear: {
        type: "aiops.special-input-clear";
        sessionId?: string;
        resourceKind?: string;
        resourceId?: string;
        canonicalKey?: string;
      };
      aiopsSpecialInputConfirm: {
        type: "aiops.special-input-confirm";
        sessionId?: string;
        resourceKind?: string;
        resourceId?: string;
        canonicalKey?: string;
      };
    }
  }
}

export {};
