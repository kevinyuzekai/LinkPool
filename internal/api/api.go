package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/kevinyuzekai/LinkPool/internal/app"
)

// Handler serves JSON API for the embedded UI.
type Handler struct {
	App *app.App
}

func (h *Handler) Mount(mux *http.ServeMux) {
	mux.HandleFunc("/api/adapters", h.adapters)
	mux.HandleFunc("/api/adapters/update", h.updateAdapters)
	mux.HandleFunc("/api/start", h.start)
	mux.HandleFunc("/api/stop", h.stop)
	mux.HandleFunc("/api/status", h.status)
	mux.HandleFunc("/api/bypass", h.bypass)
	mux.HandleFunc("/api/diag", h.diag)
	mux.HandleFunc("/api/listen", h.listen)
	mux.HandleFunc("/api/stats/reset", h.resetStats)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func (h *Handler) adapters(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost || r.URL.Query().Get("refresh") == "1" {
		list, err := h.App.RefreshAdapters()
		if err != nil {
			writeJSON(w, 500, map[string]any{"error": err.Error(), "adapters": list})
			return
		}
		writeJSON(w, 200, map[string]any{"adapters": list})
		return
	}
	list, err := h.App.RefreshAdapters()
	if err != nil {
		writeJSON(w, 200, map[string]any{"adapters": list, "warning": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"adapters": list})
}

func (h *Handler) updateAdapters(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, 405, map[string]string{"error": "POST only"})
		return
	}
	var body struct {
		Adapters []app.AdapterUpdate `json:"adapters"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}
	list := h.App.UpdateAdapters(body.Adapters)
	writeJSON(w, 200, map[string]any{"adapters": list})
}

func (h *Handler) start(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, 405, map[string]string{"error": "POST only"})
		return
	}
	var body struct {
		SysProxy bool `json:"sysProxy"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	err := h.App.StartProxy(body.SysProxy)
	if err != nil {
		writeJSON(w, 500, map[string]any{"error": err.Error(), "status": h.App.Status()})
		return
	}
	writeJSON(w, 200, h.App.Status())
}

func (h *Handler) stop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, 405, map[string]string{"error": "POST only"})
		return
	}
	_ = h.App.StopProxy()
	writeJSON(w, 200, h.App.Status())
}

func (h *Handler) status(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, h.App.Status())
}

func (h *Handler) bypass(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		writeJSON(w, 200, map[string]string{"text": h.App.BypassText()})
		return
	}
	if r.Method != http.MethodPost {
		writeJSON(w, 405, map[string]string{"error": "GET/POST"})
		return
	}
	var body struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}
	h.App.SetBypassText(body.Text)
	writeJSON(w, 200, map[string]string{"text": h.App.BypassText()})
}

func (h *Handler) diag(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	results := h.App.Diagnose(ctx)
	writeJSON(w, 200, map[string]any{"results": results, "at": time.Now().Format(time.RFC3339)})
}

func (h *Handler) listen(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, 405, map[string]string{"error": "POST only"})
		return
	}
	var body struct {
		HTTP  string `json:"http"`
		SOCKS string `json:"socks"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}
	if err := h.App.SetListenAddrs(body.HTTP, body.SOCKS); err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, h.App.Status())
}

func (h *Handler) resetStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, 405, map[string]string{"error": "POST only"})
		return
	}
	h.App.Stats.Reset()
	writeJSON(w, 200, h.App.Status())
}
