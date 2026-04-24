/**
 * WebSocket event types for the ReAct loop lifecycle.
 */
export const WS_EVENTS = {
  LOOP_STARTED: 'loop_started',
  ITERATION_STARTED: 'iteration_started',
  TOOL_STARTED: 'tool_started',
  TOOL_COMPLETED: 'tool_completed',
  WAITING_USER: 'waiting_user',
  WAITING_APPROVAL: 'waiting_approval',
  LOOP_COMPLETED: 'loop_completed',
  LOOP_FAILED: 'loop_failed',
};
