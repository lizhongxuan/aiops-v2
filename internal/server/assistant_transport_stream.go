package server

import (
	"encoding/json"
	"io"
	"net/http"
)

const (
	assistantTransportStreamOpSet        = "set"
	assistantTransportStreamOpAppendText = "append-text"
)

type assistantTransportStreamStateOp struct {
	Type  string `json:"type"`
	Path  []any  `json:"path"`
	Value any    `json:"value"`
}

type assistantTransportStreamEncoder struct {
	writer  io.Writer
	flusher http.Flusher
}

func newAssistantTransportStreamEncoder(w io.Writer) *assistantTransportStreamEncoder {
	encoder := &assistantTransportStreamEncoder{writer: w}
	if flusher, ok := w.(http.Flusher); ok {
		encoder.flusher = flusher
	}
	return encoder
}

func (e *assistantTransportStreamEncoder) WriteStateOps(ops []assistantTransportStreamStateOp) error {
	payload, err := json.Marshal(ops)
	if err != nil {
		return err
	}
	if _, err := io.WriteString(e.writer, "aui-state:"+string(payload)+"\n"); err != nil {
		return err
	}
	e.flush()
	return nil
}

func (e *assistantTransportStreamEncoder) WriteError(message string) error {
	payload, err := json.Marshal(message)
	if err != nil {
		return err
	}
	if _, err := io.WriteString(e.writer, "3:"+string(payload)+"\n"); err != nil {
		return err
	}
	e.flush()
	return nil
}

func (e *assistantTransportStreamEncoder) flush() {
	if e.flusher != nil {
		e.flusher.Flush()
	}
}
