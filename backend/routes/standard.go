package routes

import (
	"encoding/json"
	"net/http"
	"strings"

	"florenbot/services"
)

type StandardHandler struct {
	HTTP *services.HTTPService 
}

func NewStandardHandler(httpService *services.HTTPService) *StandardHandler {
	return &StandardHandler{
		HTTP: httpService,
	}
}

func (h *StandardHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/commands", h.ListCommands)
	mux.HandleFunc("GET /api/v1/commands/{name}", h.GetCommand)
	mux.HandleFunc("POST /api/v1/commands/execute", h.ExecuteCommand)
}

type commandInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type executeCommandRequest struct {
	Name string   `json:"name"`
	Args []string `json:"args,omitempty"`
}

type apiResponse struct {
	OK    bool   `json:"ok"`
	Data  any    `json:"data,omitempty"`
	Error string `json:"error,omitempty"`
}

func writeJSON(w http.ResponseWriter, status int, payload apiResponse) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func (h *StandardHandler) ListCommands(w http.ResponseWriter, r *http.Request) {
	commands := []commandInfo{
		{Name: "ping", Description: "Проверка доступности API"},
		{Name: "help", Description: "Список стандартных команд"},
	}

	writeJSON(w, http.StatusOK, apiResponse{
		OK:   true,
		Data: commands,
	})
}

func (h *StandardHandler) GetCommand(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.PathValue("name"))
	if name == "" {
		writeJSON(w, http.StatusBadRequest, apiResponse{
			OK:    false,
			Error: "command name is required",
		})
		return
	}

	if name != "ping" && name != "help" {
		writeJSON(w, http.StatusNotFound, apiResponse{
			OK:    false,
			Error: "command not found",
		})
		return
	}

	writeJSON(w, http.StatusOK, apiResponse{
		OK: true,
		Data: commandInfo{
			Name:        name,
			Description: "Стандартная команда",
		},
	})
}

func (h *StandardHandler) ExecuteCommand(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	var req executeCommandRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, apiResponse{
			OK:    false,
			Error: "invalid JSON body",
		})
		return
	}

	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		writeJSON(w, http.StatusBadRequest, apiResponse{
			OK:    false,
			Error: "field 'name' is required",
		})
		return
	}

	// Здесь вызывается логика команды (из backend/engine и/или с использованием h.HTTP)
	writeJSON(w, http.StatusOK, apiResponse{
		OK: true,
		Data: map[string]any{
			"command": req.Name,
			"args":    req.Args,
			"status":  "executed",
		},
	})
}