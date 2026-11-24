package routes

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	helpers "florenbot/backend/engine/helpers"
	std_helpers "florenbot/bot/helpers"
	"florenbot/services"
)

type ModersHandler struct {
	HTTP *services.HTTPService
}

func NewModersHandler(httpService *services.HTTPService) *ModersHandler {
	return &ModersHandler{
		HTTP: httpService,
	}
}

func (h *ModersHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/moders/resolve-target", h.ResolveTarget)
	mux.HandleFunc("POST /api/v1/moders/mute/prepare", h.PrepareMute)
	mux.HandleFunc("GET /api/v1/moders/delete-check", h.CheckDeleteMessage)
}

type moderAPIResponse struct {
	OK    bool   `json:"ok"`
	Data  any    `json:"data,omitempty"`
	Error string `json:"error,omitempty"`
	Code  string `json:"code,omitempty"`
}

type resolveTargetRequest struct {
	ReplyUserID int64  `json:"reply_user_id,omitempty"`
	UsernameArg string `json:"username_arg,omitempty"`
	Reason      string `json:"reason,omitempty"`
}

type prepareMuteRequest struct {
	ReplyUserID int64  `json:"reply_user_id,omitempty"`
	UsernameArg string `json:"username_arg,omitempty"`
	DurationStr string `json:"duration_str"`
	Reason      string `json:"reason,omitempty"`
}

func writeModerJSON(w http.ResponseWriter, status int, payload moderAPIResponse) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func resolveTargetUserID(replyUserID int64, usernameArg string) (int64, string, error) {
	if replyUserID != 0 {
		return replyUserID, "", nil
	}

	rawUsername := strings.TrimSpace(usernameArg)
	if rawUsername == "" {
		return 0, "MISSING_TARGET", strconv.ErrSyntax
	}

	username, err := std_helpers.ParseTelegramUsername(rawUsername)
	if err != nil {
		return 0, "INVALID_USERNAME", err
	}

	targetID := int64(helpers.GetUserIDByUsername(username))
	if targetID == 0 {
		return 0, "USER_NOT_FOUND", strconv.ErrSyntax
	}

	return targetID, "", nil
}

// ResolveTarget — POST /api/v1/moders/resolve-target
func (h *ModersHandler) ResolveTarget(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	var req resolveTargetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeModerJSON(w, http.StatusBadRequest, moderAPIResponse{OK: false, Code: "INVALID_JSON", Error: "invalid JSON body"})
		return
	}

	targetID, errCode, err := resolveTargetUserID(req.ReplyUserID, req.UsernameArg)
	if err != nil {
		code := errCode
		if code == "" {
			code = "RESOLVE_FAILED"
		}
		writeModerJSON(w, http.StatusBadRequest, moderAPIResponse{OK: false, Code: code, Error: "failed to resolve target user"})
		return
	}

	userProfile, err := helpers.GetUserByID(uint64(targetID))
	if err != nil {
		writeModerJSON(w, http.StatusNotFound, moderAPIResponse{OK: false, Code: "DB_USER_NOT_FOUND", Error: "user not found in database"})
		return
	}

	writeModerJSON(w, http.StatusOK, moderAPIResponse{
		OK: true,
		Data: map[string]any{
			"target_user_id": targetID,
			"username":       userProfile.Username,
			"first_name":     userProfile.FirstName,
			"reason":         req.Reason,
		},
	})
}

// PrepareMute — POST /api/v1/moders/mute/prepare
func (h *ModersHandler) PrepareMute(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	var req prepareMuteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeModerJSON(w, http.StatusBadRequest, moderAPIResponse{OK: false, Code: "INVALID_JSON", Error: "invalid JSON body"})
		return
	}

	targetID, errCode, err := resolveTargetUserID(req.ReplyUserID, req.UsernameArg)
	if err != nil {
		code := errCode
		if code == "" {
			code = "RESOLVE_FAILED"
		}
		writeModerJSON(w, http.StatusBadRequest, moderAPIResponse{OK: false, Code: code, Error: "failed to resolve target user"})
		return
	}

	duration, err := std_helpers.ParseDuration(strings.TrimSpace(req.DurationStr))
	if err != nil {
		writeModerJSON(w, http.StatusBadRequest, moderAPIResponse{OK: false, Code: "INVALID_DURATION", Error: "invalid duration format"})
		return
	}

	untilDate := time.Now().Add(duration).Unix()

	displayName := "Пользователь"
	if userProfile, pErr := helpers.GetUserByID(uint64(targetID)); pErr == nil && userProfile.Username != "" {
		displayName = userProfile.Username
	}

	writeModerJSON(w, http.StatusOK, moderAPIResponse{
		OK: true,
		Data: map[string]any{
			"target_user_id": targetID,
			"until_date":     untilDate,
			"duration_str":   strings.TrimSpace(req.DurationStr),
			"display_name":   displayName,
			"reason":         req.Reason,
		},
	})
}

// CheckDeleteMessage — GET /api/v1/moders/delete-check?user_id=...&chat_id=...
func (h *ModersHandler) CheckDeleteMessage(w http.ResponseWriter, r *http.Request) {
	userID, err := parseUserIDParam(r)
	if err != nil {
		writeModerJSON(w, http.StatusBadRequest, moderAPIResponse{OK: false, Code: "INVALID_USER", Error: err.Error()})
		return
	}

	chatID, _, err := parseChatIDParam(r)
	if err != nil {
		writeModerJSON(w, http.StatusBadRequest, moderAPIResponse{OK: false, Code: "INVALID_CHAT", Error: "invalid chat_id"})
		return
	}

	if _, err := helpers.GetUserByID(userID); err != nil {
		writeModerJSON(w, http.StatusNotFound, moderAPIResponse{OK: false, Code: "USER_NOT_FOUND", Error: "user not found"})
		return
	}

	searchID := chatID
	if searchID < 0 {
		searchID = -searchID
	}

	chat, err := helpers.GetChatById(searchID)
	if err != nil || chat == nil {
		writeModerJSON(w, http.StatusNotFound, moderAPIResponse{OK: false, Code: "CHAT_NOT_FOUND", Error: "chat not found"})
		return
	}

	writeModerJSON(w, http.StatusOK, moderAPIResponse{
		OK: true,
		Data: map[string]any{
			"chat_id":   chatID,
			"search_id": searchID,
		},
	})
}