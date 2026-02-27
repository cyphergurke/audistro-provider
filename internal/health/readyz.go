package health

import (
	"context"
	"encoding/json"
	"net/http"
)

type ReadinessChecker func(context.Context) error

type ReadinessResponse struct {
	Status string `json:"status"`
	Reason string `json:"reason,omitempty"`
}

func ReadyHandler(checker ReadinessChecker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		resp := ReadinessResponse{Status: "ready"}
		statusCode := http.StatusOK

		if checker != nil {
			if err := checker(r.Context()); err != nil {
				resp.Status = "not_ready"
				resp.Reason = err.Error()
				statusCode = http.StatusServiceUnavailable
			}
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		_ = json.NewEncoder(w).Encode(resp)
	}
}
