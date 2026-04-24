package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"runner/server/service"
	"runner/server/store/envstore"
)

func TestEnvironmentHandlerCRUD(t *testing.T) {
	t.Parallel()

	svc := service.NewEnvironmentService(envstore.NewFileStore(t.TempDir()))
	router := NewRouter(RouterOptions{
		Environment: NewEnvironmentHandler(svc),
	})

	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/environments", strings.NewReader(`{
		"name":"prod",
		"description":"Production",
		"vars":[]
	}`))
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	router.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body = %s", createRec.Code, createRec.Body.String())
	}

	addVarReq := httptest.NewRequest(http.MethodPost, "/api/v1/environments/prod/vars", strings.NewReader(`{
		"key":"API_TOKEN",
		"value":"secret",
		"description":"main token",
		"sensitive":true
	}`))
	addVarReq.Header.Set("Content-Type", "application/json")
	addVarRec := httptest.NewRecorder()
	router.ServeHTTP(addVarRec, addVarReq)
	if addVarRec.Code != http.StatusCreated {
		t.Fatalf("add var status = %d, body = %s", addVarRec.Code, addVarRec.Body.String())
	}

	updateVarReq := httptest.NewRequest(http.MethodPut, "/api/v1/environments/prod/vars/API_TOKEN", strings.NewReader(`{
		"key":"API_TOKEN",
		"value":"secret-v2",
		"description":"rotated token",
		"sensitive":true
	}`))
	updateVarReq.Header.Set("Content-Type", "application/json")
	updateVarRec := httptest.NewRecorder()
	router.ServeHTTP(updateVarRec, updateVarReq)
	if updateVarRec.Code != http.StatusOK {
		t.Fatalf("update var status = %d, body = %s", updateVarRec.Code, updateVarRec.Body.String())
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/environments/prod", nil)
	getRec := httptest.NewRecorder()
	router.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("get status = %d, body = %s", getRec.Code, getRec.Body.String())
	}

	var payload struct {
		Name string `json:"name"`
		Vars []struct {
			Key   string `json:"key"`
			Value string `json:"value"`
		} `json:"vars"`
	}
	if err := json.Unmarshal(getRec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode get: %v", err)
	}
	if payload.Name != "prod" || len(payload.Vars) != 1 || payload.Vars[0].Value != "secret-v2" {
		t.Fatalf("unexpected payload: %+v", payload)
	}

	deleteVarReq := httptest.NewRequest(http.MethodDelete, "/api/v1/environments/prod/vars/API_TOKEN", nil)
	deleteVarRec := httptest.NewRecorder()
	router.ServeHTTP(deleteVarRec, deleteVarReq)
	if deleteVarRec.Code != http.StatusOK {
		t.Fatalf("delete var status = %d, body = %s", deleteVarRec.Code, deleteVarRec.Body.String())
	}
}
