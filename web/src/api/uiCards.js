import httpClient from "./httpClient";

const UI_CARDS_API_PREFIX = "/api/v1/ui-cards";

function encodePathSegment(value) {
  return encodeURIComponent(String(value));
}

export function createUiCardsClient(client = httpClient) {
  return {
    fetchUiCards() {
      return client.get(UI_CARDS_API_PREFIX);
    },

    getUiCard(id) {
      return client.get(`${UI_CARDS_API_PREFIX}/${encodePathSegment(id)}`);
    },

    createUiCard(payload) {
      return client.post(UI_CARDS_API_PREFIX, payload);
    },

    updateUiCard(id, payload) {
      return client.put(`${UI_CARDS_API_PREFIX}/${encodePathSegment(id)}`, payload);
    },

    deleteUiCard(id) {
      return client.delete(`${UI_CARDS_API_PREFIX}/${encodePathSegment(id)}`);
    },

    previewUiCard(id, payload) {
      return client.post(`${UI_CARDS_API_PREFIX}/${encodePathSegment(id)}/preview`, payload);
    },

    validateUiCard(id, payload) {
      return client.post(`${UI_CARDS_API_PREFIX}/${encodePathSegment(id)}/validate`, payload);
    },

    createUiCardVersion(id, payload = {}) {
      return client.post(`${UI_CARDS_API_PREFIX}/${encodePathSegment(id)}/versions`, payload);
    },

    updateUiCardStatus(id, status) {
      return client.put(`${UI_CARDS_API_PREFIX}/${encodePathSegment(id)}/status`, { status });
    },
  };
}

const uiCardsClient = createUiCardsClient();

export const fetchUiCards = (...args) => uiCardsClient.fetchUiCards(...args);
export const getUiCard = (...args) => uiCardsClient.getUiCard(...args);
export const createUiCard = (...args) => uiCardsClient.createUiCard(...args);
export const updateUiCard = (...args) => uiCardsClient.updateUiCard(...args);
export const deleteUiCard = (...args) => uiCardsClient.deleteUiCard(...args);
export const previewUiCard = (...args) => uiCardsClient.previewUiCard(...args);
export const validateUiCard = (...args) => uiCardsClient.validateUiCard(...args);
export const createUiCardVersion = (...args) => uiCardsClient.createUiCardVersion(...args);
export const updateUiCardStatus = (...args) => uiCardsClient.updateUiCardStatus(...args);

export default uiCardsClient;
