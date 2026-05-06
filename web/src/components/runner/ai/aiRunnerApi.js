import httpClient from "../../../api/httpClient";

const RUNNER_STUDIO_AI_API_PREFIX = "/api/runner-studio/ai";

export function createAiRunnerApi(client = httpClient) {
  return {
    generateRunnerWorkflowDraft(payload) {
      return client.post(`${RUNNER_STUDIO_AI_API_PREFIX}/draft`, payload);
    },
  };
}

const aiRunnerApi = createAiRunnerApi();

export const draftRunnerWorkflowWithAI = (...args) => aiRunnerApi.generateRunnerWorkflowDraft(...args);

export default aiRunnerApi;
