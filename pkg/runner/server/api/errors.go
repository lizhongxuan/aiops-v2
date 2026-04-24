package api

import (
	"errors"
	"net/http"

	"runner/server/service"
)

func writeServiceError(w http.ResponseWriter, err error) {
	if err == nil {
		return
	}
	switch {
	case errors.Is(err, service.ErrInvalid):
		writeJSONError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, service.ErrNotFound):
		writeJSONError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, service.ErrAlreadyExists):
		writeJSONError(w, http.StatusConflict, err.Error())
	case errors.Is(err, service.ErrConflict):
		writeJSONError(w, http.StatusConflict, err.Error())
	case errors.Is(err, service.ErrQueueFull):
		writeJSONError(w, http.StatusServiceUnavailable, err.Error())
	case errors.Is(err, service.ErrUnavailable):
		writeJSONError(w, http.StatusServiceUnavailable, err.Error())
	default:
		writeJSONError(w, http.StatusInternalServerError, err.Error())
	}
}
