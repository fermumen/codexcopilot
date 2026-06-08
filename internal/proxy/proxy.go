package proxy

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"github.com/fermumen/codexcopilot/internal/auth"
	"github.com/fermumen/codexcopilot/internal/copilot"
)

const SyntheticAttachmentPrompt = "The user attached an image."

var unsupportedToolTypes = map[string]bool{
	"image_generation": true,
	"image_tool":       true,
}

var hopByHopHeaders = map[string]bool{
	"connection":          true,
	"content-length":      true,
	"host":                true,
	"keep-alive":          true,
	"proxy-authenticate":  true,
	"proxy-authorization": true,
	"te":                  true,
	"trailer":             true,
	"transfer-encoding":   true,
	"upgrade":             true,
	"accept-encoding":     true,
}

func hasImage(value any) bool {
	switch v := value.(type) {
	case map[string]any:
		if kind, _ := v["type"].(string); kind == "input_image" || kind == "image_url" {
			return true
		}
		for _, item := range v {
			if hasImage(item) {
				return true
			}
		}
	case []any:
		for _, item := range v {
			if hasImage(item) {
				return true
			}
		}
	}
	return false
}

func bodyHasImage(body []byte) bool {
	var payload any
	if err := json.Unmarshal(body, &payload); err != nil {
		return false
	}
	return hasImage(payload)
}

func stripUnsupportedTools(body []byte) []byte {
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return body
	}
	tools, ok := payload["tools"].([]any)
	if !ok {
		return body
	}
	filtered := tools[:0]
	removed := false
	for _, rawTool := range tools {
		tool, ok := rawTool.(map[string]any)
		if !ok {
			filtered = append(filtered, rawTool)
			continue
		}
		kind, _ := tool["type"].(string)
		if unsupportedToolTypes[kind] {
			removed = true
			continue
		}
		filtered = append(filtered, rawTool)
	}
	if !removed {
		return body
	}
	payload["tools"] = filtered
	data, err := json.Marshal(payload)
	if err != nil {
		return body
	}
	return data
}

func isSyntheticAttachmentMessage(message any) bool {
	msg, ok := message.(map[string]any)
	if !ok || msg["role"] != "user" {
		return false
	}
	switch content := msg["content"].(type) {
	case string:
		return content == SyntheticAttachmentPrompt
	case []any:
		for _, rawPart := range content {
			part, ok := rawPart.(map[string]any)
			if !ok {
				continue
			}
			kind, _ := part["type"].(string)
			text, _ := part["text"].(string)
			if (kind == "text" || kind == "input_text") && text == SyntheticAttachmentPrompt {
				return true
			}
		}
	}
	return false
}

func messagesInitiator(messages any) string {
	list, ok := messages.([]any)
	if !ok || len(list) == 0 {
		return "user"
	}
	last := list[len(list)-1]
	msg, ok := last.(map[string]any)
	if !ok {
		return "agent"
	}
	if isSyntheticAttachmentMessage(msg) {
		return "agent"
	}
	if msg["role"] == "user" {
		return "user"
	}
	return "agent"
}

func responsesInitiator(payload map[string]any) string {
	input, ok := payload["input"]
	if !ok {
		return "user"
	}
	return messagesInitiator(input)
}

func Initiator(body []byte) string {
	var payload any
	if err := json.Unmarshal(body, &payload); err != nil {
		return "user"
	}
	obj, ok := payload.(map[string]any)
	if !ok {
		return "user"
	}
	if _, ok := obj["input"]; ok {
		return responsesInitiator(obj)
	}
	if messages, ok := obj["messages"]; ok {
		return messagesInitiator(messages)
	}
	return "user"
}

func upstreamPath(path string) string {
	parsed, err := url.Parse(path)
	if err != nil {
		return path
	}
	rawPath := parsed.Path
	var upstream string
	switch {
	case rawPath == "/health" || rawPath == "/v1/health":
		upstream = rawPath
	case rawPath == "/v1/models":
		upstream = "/models"
	case strings.HasPrefix(rawPath, "/v1/"):
		upstream = strings.TrimPrefix(rawPath, "/v1")
	default:
		upstream = rawPath
	}
	if parsed.RawQuery != "" {
		upstream += "?" + parsed.RawQuery
	}
	return upstream
}

type Server struct {
	Auth auth.Auth
	Log  *log.Logger
}

func New(a auth.Auth) *Server {
	return &Server{Auth: a, Log: log.Default()}
}

func (s *Server) ListenAndServe(addr string) error {
	server := &http.Server{
		Addr:              addr,
		Handler:           s,
		ReadHeaderTimeout: 30 * time.Second,
	}
	return server.ListenAndServe()
}

func cors(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "authorization,content-type,openai-beta")
	w.Header().Set("Access-Control-Allow-Methods", "GET,POST,OPTIONS")
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	data, _ := json.Marshal(payload)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(data)
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	cors(w)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method == http.MethodGet {
		s.handleGet(w, r)
		return
	}
	if r.Method == http.MethodPost {
		s.handlePost(w, r)
		return
	}
	writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": map[string]any{"message": "method not allowed"}})
}

func (s *Server) handleGet(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/health", "/v1/health":
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	case "/models", "/v1/models":
		models, err := copilot.FetchModels(s.Auth)
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]any{"error": map[string]any{"message": err.Error()}})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"object": "list", "data": models})
	default:
		writeJSON(w, http.StatusNotFound, map[string]any{"error": map[string]any{"message": "unknown path " + r.URL.Path}})
	}
}

func (s *Server) handlePost(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": map[string]any{"message": err.Error()}})
		return
	}
	body = stripUnsupportedTools(body)
	base, err := url.Parse(copilot.APIBase(s.Auth))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": map[string]any{"message": err.Error()}})
		return
	}
	target, err := url.Parse(upstreamPath(r.URL.RequestURI()))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": map[string]any{"message": err.Error()}})
		return
	}
	upstream := base.ResolveReference(target)
	req, err := http.NewRequest(http.MethodPost, upstream.String(), bytes.NewReader(body))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": map[string]any{"message": err.Error()}})
		return
	}
	for key, values := range copilot.Headers(s.Auth, Initiator(body), bodyHasImage(body)) {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}
	for key, values := range r.Header {
		lower := strings.ToLower(key)
		if hopByHopHeaders[lower] || lower == "authorization" || lower == "x-initiator" {
			continue
		}
		req.Header.Del(key)
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": map[string]any{"message": err.Error()}})
		return
	}
	defer res.Body.Close()
	for key, values := range res.Header {
		if hopByHopHeaders[strings.ToLower(key)] {
			continue
		}
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	w.WriteHeader(res.StatusCode)
	_, _ = io.Copy(w, res.Body)
}

func DumpRequest(r *http.Request) ([]byte, error) {
	return httputil.DumpRequest(r, true)
}
