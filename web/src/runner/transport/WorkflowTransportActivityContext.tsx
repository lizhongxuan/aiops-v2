import { useAssistantTransportState } from "@assistant-ui/react";
import { createContext, useContext, type PropsWithChildren } from "react";

import type { AiopsTransportState } from "@/transport/aiopsTransportTypes";
import {
  selectWorkflowTransportActivity,
  type WorkflowTransportActivity,
} from "./workflowTransportSelectors";

const IDLE_WORKFLOW_TRANSPORT_ACTIVITY: WorkflowTransportActivity = {
  busy: false,
  canStop: false,
  canRetry: false,
};

const WorkflowTransportActivityContext = createContext<WorkflowTransportActivity>(IDLE_WORKFLOW_TRANSPORT_ACTIVITY);

/**
 * Read-only projection of the route's single AssistantTransport owner. It does
 * not copy or advance task state; consumers outside a Workflow transport route
 * safely see the idle value.
 */
export function WorkflowTransportActivityProvider({ children }: PropsWithChildren) {
  const state = useAssistantTransportState() as AiopsTransportState;
  return (
    <WorkflowTransportActivityContext.Provider value={selectWorkflowTransportActivity(state)}>
      {children}
    </WorkflowTransportActivityContext.Provider>
  );
}

export function useWorkflowTransportActivity() {
  return useContext(WorkflowTransportActivityContext);
}
