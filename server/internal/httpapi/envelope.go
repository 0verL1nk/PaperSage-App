package httpapi

import (
	"encoding/json"
	"net/http"
)

// Envelope is the standard API response envelope: {code, data, info}.
// Field order matches backend/api/response.py.
type Envelope struct {
	Code int    `json:"code"`
	Data any    `json:"data"`
	Info string `json:"info"`
}

// Success creates a success envelope (code 200).
func Success(data any, info string) Envelope {
	return Envelope{Code: http.StatusOK, Data: data, Info: info}
}

// Failure creates an error envelope with null data.
func Failure(code int, info string) Envelope {
	return Envelope{Code: code, Data: nil, Info: info}
}

// WriteEnvelope writes the envelope as a JSON response with the given status.
func WriteEnvelope(w http.ResponseWriter, status int, env Envelope) error {
	body, err := json.Marshal(env)
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, err = w.Write(body)
	return err
}
