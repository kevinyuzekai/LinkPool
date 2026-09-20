package api

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
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
	mux.HandleFunc("/api/scheduler", h.scheduler)
	mux.HandleFunc("/api/download", h.download)
	mux.HandleFunc("/api/download/", h.downloadSub)
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

func (h *Handler) scheduler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		writeJSON(w, 200, map[string]any{
			"mode":             string(h.App.SchedulerMode()),
			"effectiveWeights": h.App.Sched.EffectiveWeights(),
		})
		return
	}
	if r.Method != http.MethodPost {
		writeJSON(w, 405, map[string]string{"error": "GET/POST"})
		return
	}
	var body struct {
		Mode string `json:"mode"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}
	mode := h.App.SetSchedulerMode(body.Mode)
	writeJSON(w, 200, map[string]any{
		"mode":             string(mode),
		"effectiveWeights": h.App.Sched.EffectiveWeights(),
	})
}

func (h *Handler) download(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, 200, map[string]any{"jobs": h.App.Downloads.List(), "dir": h.App.Downloads.Dir})
	case http.MethodPost:
		var body struct {
			URL         string `json:"url"`
			Concurrency int    `json:"concurrency"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSON(w, 400, map[string]string{"error": err.Error()})
			return
		}
		if strings.TrimSpace(body.URL) == "" {
			writeJSON(w, 400, map[string]string{"error": "缺少 url"})
			return
		}
		job, err := h.App.Downloads.Start(r.Context(), strings.TrimSpace(body.URL), body.Concurrency)
		if err != nil {
			writeJSON(w, 400, map[string]string{"error": err.Error()})
			return
		}
		snap := job.Snapshot()
		writeJSON(w, 200, map[string]any{"job": snap})
	default:
		writeJSON(w, 405, map[string]string{"error": "GET/POST"})
	}
}

func (h *Handler) downloadSub(w http.ResponseWriter, r *http.Request) {
	// /api/download/{id} or /api/download/{id}/file or /api/download/{id}/cancel
	path := strings.TrimPrefix(r.URL.Path, "/api/download/")
	path = strings.Trim(path, "/")
	if path == "" {
		h.download(w, r)
		return
	}
	parts := strings.Split(path, "/")
	id := parts[0]
	action := ""
	if len(parts) > 1 {
		action = parts[1]
	}

	job, ok := h.App.Downloads.Get(id)
	if !ok {
		writeJSON(w, 404, map[string]string{"error": "任务不存在"})
		return
	}

	switch action {
	case "", "status":
		writeJSON(w, 200, map[string]any{"job": job.Snapshot()})
	case "cancel":
		if r.Method != http.MethodPost {
			writeJSON(w, 405, map[string]string{"error": "POST only"})
			return
		}
		h.App.Downloads.Cancel(id)
		writeJSON(w, 200, map[string]any{"job": job.Snapshot()})
	case "file":
		snap := job.Snapshot()
		if snap.Status != "done" || snap.FilePath == "" {
			writeJSON(w, 400, map[string]string{"error": "文件尚未就绪"})
			return
		}
		f, err := os.Open(snap.FilePath)
		if err != nil {
			writeJSON(w, 500, map[string]string{"error": err.Error()})
			return
		}
		defer f.Close()
		stat, _ := f.Stat()
		name := snap.FileName
		if name == "" {
			name = filepath.Base(snap.FilePath)
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", "attachment; filename=\""+name+"\"")
		if stat != nil {
			http.ServeContent(w, r, name, stat.ModTime(), f)
		} else {
			http.ServeContent(w, r, name, time.Now(), f)
		}
	default:
		writeJSON(w, 404, map[string]string{"error": "unknown action"})
	}
}
