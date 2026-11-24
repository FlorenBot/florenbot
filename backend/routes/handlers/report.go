package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"

	helpers "florenbot/backend/engine/helpers"
	std_helpers "florenbot/bot/helpers"
	"florenbot/services"
)

type ReportHandler struct {
	HTTP *services.HTTPService
}

func NewReportHandler(httpService *services.HTTPService) *ReportHandler {
	return &ReportHandler{
		HTTP: httpService,
	}
}

func (h *ReportHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/reports", h.CreateReport)
	mux.HandleFunc("DELETE /api/v1/reports", h.DeleteReport)
}

type reportRequest struct {
	UserID uint64 `json:"user_id"`
	ChatID int64  `json:"chat_id"`
	Text   string `json:"text,omitempty"`
}

type reportAPIResponse struct {
	OK    bool   `json:"ok"`
	Data  any    `json:"data,omitempty"`
	Error string `json:"error,omitempty"`
	Code  string `json:"code,omitempty"`
}

func writeReportJSON(w http.ResponseWriter, status int, payload reportAPIResponse) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func decodeReportRequest(r *http.Request) (reportRequest, error) {
	defer r.Body.Close()
	var req reportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return req, err
	}
	return req, nil
}

// CreateReport — POST /api/v1/reports
func (h *ReportHandler) CreateReport(w http.ResponseWriter, r *http.Request) {
	req, err := decodeReportRequest(r)
	if err != nil {
		writeReportJSON(w, http.StatusBadRequest, reportAPIResponse{OK: false, Code: "INVALID_JSON", Error: "invalid JSON body"})
		return
	}
	debugType := isDebugEnabled()

	if req.UserID == 0 {
		writeReportJSON(w, http.StatusBadRequest, reportAPIResponse{OK: false, Code: "INVALID_USER", Error: "user_id is required"})
		return
	}

	if debugType {
		log.Printf("Создание отчета для user_id=%d: %s\n", req.UserID, req.Text)
	}

	existsUser, err := helpers.GetUserByID(req.UserID)
	if err != nil {
		if debugType {
			log.Printf("Ошибка получения пользователя: %v", err)
		}
		writeReportJSON(w, http.StatusNotFound, reportAPIResponse{OK: false, Code: "USER_NOT_FOUND", Error: "user not found"})
		return
	}
	if existsUser.ID == 0 {
		writeReportJSON(w, http.StatusNotFound, reportAPIResponse{OK: false, Code: "USER_NOT_FOUND", Error: "user not found"})
		return
	}

	text := strings.TrimSpace(req.Text)
	if text == "" {
		writeReportJSON(w, http.StatusBadRequest, reportAPIResponse{OK: false, Code: "EMPTY_TEXT", Error: "report text is required"})
		return
	}
	if debugType {
		log.Printf("Текст отчета: %s\n", text)
	}

	exists, err := helpers.HasReport(req.UserID)
	if err != nil {
		if debugType {
			log.Printf("Ошибка проверки наличия отчета: %v", err)
		}
		writeReportJSON(w, http.StatusInternalServerError, reportAPIResponse{OK: false, Code: "CHECK_FAILED", Error: "failed to check report existence"})
		return
	}
	if exists {
		writeReportJSON(w, http.StatusConflict, reportAPIResponse{OK: false, Code: "REPORT_EXISTS", Error: "user already has an active report"})
		return
	}

	if err := helpers.CreateReport(req.UserID, text); err != nil {
		if debugType {
			log.Printf("Ошибка создания отчета: %v", err)
		}
		writeReportJSON(w, http.StatusInternalServerError, reportAPIResponse{OK: false, Code: "CREATE_FAILED", Error: "failed to create report"})
		return
	}

	writeReportJSON(w, http.StatusCreated, reportAPIResponse{
		OK: true,
		Data: map[string]any{
			"user_id": req.UserID,
			"created": true,
		},
	})
}

// DeleteReport — DELETE /api/v1/reports?user_id=...&chat_id=...
func (h *ReportHandler) DeleteReport(w http.ResponseWriter, r *http.Request) {
	debugType := isDebugEnabled()

	userID, err := parseUserIDParam(r)
	if err != nil {
		writeReportJSON(w, http.StatusBadRequest, reportAPIResponse{OK: false, Code: "INVALID_USER", Error: err.Error()})
		return
	}

	rawChatID := strings.TrimSpace(r.URL.Query().Get("chat_id"))
	if rawChatID == "" {
		writeReportJSON(w, http.StatusBadRequest, reportAPIResponse{OK: false, Code: "INVALID_CHAT", Error: "chat_id is required"})
		return
	}
	chatID, err := strconv.ParseInt(rawChatID, 10, 64)
	if err != nil {
		writeReportJSON(w, http.StatusBadRequest, reportAPIResponse{OK: false, Code: "INVALID_CHAT", Error: "invalid chat_id"})
		return
	}

	existsUser, err := helpers.GetUserByID(userID)
	if err != nil {
		if debugType {
			log.Printf("Ошибка получения пользователя: %v", err)
		}
		writeReportJSON(w, http.StatusNotFound, reportAPIResponse{OK: false, Code: "USER_NOT_FOUND", Error: "user not found"})
		return
	}
	if existsUser.ID == 0 {
		writeReportJSON(w, http.StatusNotFound, reportAPIResponse{OK: false, Code: "USER_NOT_FOUND", Error: "user not found"})
		return
	}

	parsedChatID := std_helpers.ParseChatID(uint64(chatID))
	memberRole, err := helpers.GetMemberRole(userID, uint64(parsedChatID))
	if err != nil || !std_helpers.IsUserOwnerOrCreator(&memberRole) {
		writeReportJSON(w, http.StatusForbidden, reportAPIResponse{OK: false, Code: "FORBIDDEN_ROLE", Error: "insufficient permissions or role check error"})
		return
	}
	if !std_helpers.IsUserAdmin(&memberRole) {
		writeReportJSON(w, http.StatusForbidden, reportAPIResponse{OK: false, Code: "NOT_ADMIN_OWNER", Error: "only owner/admin can delete reports"})
		return
	}

	exists, err := helpers.HasReport(userID)
	if err != nil {
		if debugType {
			log.Printf("Ошибка проверки наличия отчета: %v", err)
		}
		writeReportJSON(w, http.StatusInternalServerError, reportAPIResponse{OK: false, Code: "CHECK_FAILED", Error: "failed to check report existence"})
		return
	}
	if !exists {
		writeReportJSON(w, http.StatusNotFound, reportAPIResponse{OK: false, Code: "REPORT_NOT_FOUND", Error: "user has no report"})
		return
	}

	if err := helpers.DeleteReport(userID); err != nil {
		if debugType {
			log.Printf("Ошибка удаления отчета: %v", err)
		}
		writeReportJSON(w, http.StatusInternalServerError, reportAPIResponse{OK: false, Code: "DELETE_FAILED", Error: "failed to delete report"})
		return
	}

	writeReportJSON(w, http.StatusOK, reportAPIResponse{
		OK: true,
		Data: map[string]any{
			"user_id": userID,
			"deleted": true,
		},
	})
}