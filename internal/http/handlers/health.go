package handlers

import (
	"net/http"
)

 // Healthz godoc
 // @Summary Liveness probe
 // @Tags platform
 // @Success 200 {string} string "ok"
 // @Router /healthz [get]
func Healthz(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

 // Readyz godoc
 // @Summary Readiness probe
 // @Tags platform
 // @Success 200 {string} string "ready"
 // @Router /readyz [get]
func Readyz(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ready"))
}
