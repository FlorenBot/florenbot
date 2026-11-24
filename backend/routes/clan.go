package routes

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	cache "florenbot/backend/engine/cache"
	helpers "florenbot/backend/engine/helpers"
	structs "florenbot/backend/engine/structs"
	helper "florenbot/backend/engine/helpers"
	std_helpers "florenbot/bot/helpers"
	"florenbot/services"
	"gorm.io/gorm"
)

const createClanCost = 1000.0

type ClanHandler struct {
	HTTP *services.HTTPService
}

func NewClanHandler(httpService *services.HTTPService) *ClanHandler {
	return &ClanHandler{
		HTTP: httpService,
	}
}

func (h *ClanHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/clans", h.ListClans)
	mux.HandleFunc("POST /api/v1/clans", h.CreateClan)
	mux.HandleFunc("GET /api/v1/clans/me", h.GetMyClan)
	mux.HandleFunc("DELETE /api/v1/clans/me", h.DeleteMyClan)
	mux.HandleFunc("GET /api/v1/clans/me/role", h.GetMyRole)
	mux.HandleFunc("GET /api/v1/clans/me/members", h.GetMyMembers)
	mux.HandleFunc("POST /api/v1/clans/me/kick", h.KickMember)
	mux.HandleFunc("POST /api/v1/clans/me/ban", h.BanMember)
	mux.HandleFunc("POST /api/v1/clans/join", h.JoinByInvite)
	mux.HandleFunc("POST /api/v1/clans/leave", h.LeaveClan)

	mux.HandleFunc("GET /api/v1/clans/me/invite-code", h.GetInviteCode)
	mux.HandleFunc("POST /api/v1/clans/me/invite-code", h.CreateInviteCode)
	mux.HandleFunc("DELETE /api/v1/clans/me/invite-code", h.DeleteInviteCode)
	mux.HandleFunc("POST /api/v1/clans/me/invite-code/revoke", h.RevokeInviteCode)
	mux.HandleFunc("POST /api/v1/clans/me/invite-code/expire", h.SetInviteExpire)
	mux.HandleFunc("DELETE /api/v1/clans/me/invite-code/expire", h.DeleteInviteExpire)
}

type clanRequest struct {
	UserID    uint64 `json:"user_id"`
	Username  string `json:"username,omitempty"`
	FirstName string `json:"first_name,omitempty"`
	ChatType  string `json:"chat_type,omitempty"`
	Name      string `json:"name,omitempty"`
	TargetID  uint64 `json:"target_id,omitempty"`
	Reason    string `json:"reason,omitempty"`
	Code      string `json:"code,omitempty"`
	Duration  string `json:"duration,omitempty"`
}

type clanAPIResponse struct {
	OK    bool   `json:"ok"`
	Data  any    `json:"data,omitempty"`
	Error string `json:"error,omitempty"`
	Code  string `json:"code,omitempty"`
}

func writeClanJSON(w http.ResponseWriter, status int, payload clanAPIResponse) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func isGroupChat(chatType string) bool {
	return chatType == "supergroup" || chatType == "group"
}

func parseUserIDParam(r *http.Request) (uint64, error) {
	raw := strings.TrimSpace(r.URL.Query().Get("user_id"))
	if raw == "" {
		return 0, errors.New("user_id is required")
	}
	id, err := strconv.ParseUint(raw, 10, 64)
	if err != nil || id == 0 {
		return 0, errors.New("invalid user_id")
	}
	return id, nil
}

func decodeClanRequest(r *http.Request) (clanRequest, error) {
	defer r.Body.Close()
	var req clanRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return req, err
	}
	return req, nil
}

// GetMyClan — GET /api/v1/clans/me?user_id=...
func (h *ClanHandler) GetMyClan(w http.ResponseWriter, r *http.Request) {
	userID, err := parseUserIDParam(r)
	if err != nil {
		writeClanJSON(w, http.StatusBadRequest, clanAPIResponse{OK: false, Code: "INVALID_USER", Error: err.Error()})
		return
	}
	debugType := isDebugEnabled()

	clanID, err := helpers.GetUserClanID(userID)
	if err != nil {
		if debugType {
			log.Printf("Ошибка получения клана: %v", err)
		}
		writeClanJSON(w, http.StatusNotFound, clanAPIResponse{OK: false, Code: "NOT_IN_CLAN", Error: "user is not in a clan"})
		return
	}

	clan, err := helpers.GetClanByID(clanID)
	if err != nil {
		if debugType {
			log.Printf("Ошибка получения клана: %v", err)
		}
		writeClanJSON(w, http.StatusNotFound, clanAPIResponse{OK: false, Code: "CLAN_NOT_FOUND", Error: "clan not found"})
		return
	}

	count, _ := helpers.GetClanMemberCount(clanID)

	writeClanJSON(w, http.StatusOK, clanAPIResponse{
		OK: true,
		Data: map[string]any{
			"id":           clan.ID,
			"name":         clan.Name,
			"owner_name":   clan.OwnerName,
			"member_count": count,
		},
	})
}

// CreateClan — POST /api/v1/clans
func (h *ClanHandler) CreateClan(w http.ResponseWriter, r *http.Request) {
	req, err := decodeClanRequest(r)
	if err != nil {
		writeClanJSON(w, http.StatusBadRequest, clanAPIResponse{OK: false, Code: "INVALID_JSON", Error: "invalid JSON body"})
		return
	}
	debugType := isDebugEnabled()

	if req.UserID == 0 {
		writeClanJSON(w, http.StatusBadRequest, clanAPIResponse{OK: false, Code: "INVALID_USER", Error: "user_id is required"})
		return
	}

	_, err = helpers.GetUserClanID(req.UserID)
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			if debugType {
				log.Printf("Ошибка получения клана из БД: %v", err)
			}
			writeClanJSON(w, http.StatusInternalServerError, clanAPIResponse{OK: false, Code: "DB_ERROR", Error: "database error"})
			return
		}
	} else {
		writeClanJSON(w, http.StatusConflict, clanAPIResponse{OK: false, Code: "ALREADY_IN_CLAN", Error: "user is already in a clan"})
		return
	}

	balance, err := cache.GetBalance(req.UserID, req.Username)
	if err != nil {
		writeClanJSON(w, http.StatusUnauthorized, clanAPIResponse{OK: false, Code: "UNAUTHORIZED", Error: "user is not authorized"})
		return
	}

	if balance < createClanCost {
		writeClanJSON(w, http.StatusPaymentRequired, clanAPIResponse{OK: false, Code: "INSUFFICIENT_FUNDS", Error: "insufficient funds, required 1000"})
		return
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		writeClanJSON(w, http.StatusBadRequest, clanAPIResponse{OK: false, Code: "INVALID_NAME", Error: "clan name is required"})
		return
	}

	if err := helpers.CreateClan(name, req.FirstName, req.UserID); err != nil {
		writeClanJSON(w, http.StatusInternalServerError, clanAPIResponse{OK: false, Code: "CREATE_FAILED", Error: "failed to create clan"})
		return
	}

	writeClanJSON(w, http.StatusCreated, clanAPIResponse{
		OK: true,
		Data: map[string]any{
			"name": name,
		},
	})
}

// GetMyMembers — GET /api/v1/clans/me/members?user_id=...&filter=active|noactive
func (h *ClanHandler) GetMyMembers(w http.ResponseWriter, r *http.Request) {
	userID, err := parseUserIDParam(r)
	if err != nil {
		writeClanJSON(w, http.StatusBadRequest, clanAPIResponse{OK: false, Code: "INVALID_USER", Error: err.Error()})
		return
	}
	debugType := isDebugEnabled()

	clanID, err := helpers.GetUserClanID(userID)
	if err != nil {
		if debugType {
			log.Printf("Ошибка получения клана для members: %v", err)
		}
		writeClanJSON(w, http.StatusNotFound, clanAPIResponse{OK: false, Code: "NOT_IN_CLAN", Error: "user is not in a clan"})
		return
	}

	filter := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("filter")))
	if filter == "" {
		filter = "active"
	}
	if filter != "active" && filter != "noactive" {
		writeClanJSON(w, http.StatusBadRequest, clanAPIResponse{OK: false, Code: "INVALID_FILTER", Error: "filter must be active or noactive"})
		return
	}

	members, err := helpers.GetClanMembersWithDetails(uint64(clanID))
	if err != nil {
		log.Printf("[Clan Error] Не удалось получить участников клана для ID %d: %v", clanID, err)
		writeClanJSON(w, http.StatusInternalServerError, clanAPIResponse{OK: false, Code: "MEMBERS_ERROR", Error: "failed to load clan members"})
		return
	}

	if filter == "active" {
		writeClanJSON(w, http.StatusOK, clanAPIResponse{
			OK: true,
			Data: map[string]any{
				"filter":  "active",
				"total":   len(members),
				"members": members,
			},
		})
		return
	}

	inactive := make([]any, 0)
	for _, member := range members {
		if member.MessageCount == 5 {
			inactive = append(inactive, member)
		}
	}

	writeClanJSON(w, http.StatusOK, clanAPIResponse{
		OK: true,
		Data: map[string]any{
			"filter":  "noactive",
			"total":   len(members),
			"count":   len(inactive),
			"members": inactive,
		},
	})
}

// GetMyRole — GET /api/v1/clans/me/role?user_id=...
func (h *ClanHandler) GetMyRole(w http.ResponseWriter, r *http.Request) {
	userID, err := parseUserIDParam(r)
	if err != nil {
		writeClanJSON(w, http.StatusBadRequest, clanAPIResponse{OK: false, Code: "INVALID_USER", Error: err.Error()})
		return
	}
	debugType := isDebugEnabled()

	if _, err := helpers.GetUserClanID(userID); err != nil {
		if debugType {
			log.Printf("Ошибка получения клана: %v", err)
		}
		writeClanJSON(w, http.StatusNotFound, clanAPIResponse{OK: false, Code: "NOT_IN_CLAN", Error: "user is not in a clan"})
		return
	}

	role, err := helpers.GetUserClanRole(userID)
	if err != nil {
		if debugType {
			log.Printf("Ошибка получения роли: %v", err)
		}
		writeClanJSON(w, http.StatusInternalServerError, clanAPIResponse{OK: false, Code: "ROLE_ERROR", Error: "failed to get role"})
		return
	}

	writeClanJSON(w, http.StatusOK, clanAPIResponse{
		OK: true,
		Data: map[string]any{
			"role": role,
		},
	})
}

// KickMember — POST /api/v1/clans/me/kick
func (h *ClanHandler) KickMember(w http.ResponseWriter, r *http.Request) {
	req, err := decodeClanRequest(r)
	if err != nil {
		writeClanJSON(w, http.StatusBadRequest, clanAPIResponse{OK: false, Code: "INVALID_JSON", Error: "invalid JSON body"})
		return
	}
	debugType := isDebugEnabled()

	clanID, err := helpers.GetUserClanID(req.UserID)
	if err != nil {
		if debugType {
			log.Printf("Ошибка получения клана: %v", err)
		}
		writeClanJSON(w, http.StatusNotFound, clanAPIResponse{OK: false, Code: "NOT_IN_CLAN", Error: "user is not in a clan"})
		return
	}

	role, err := helpers.GetUserClanRole(req.UserID)
	if err != nil {
		writeClanJSON(w, http.StatusInternalServerError, clanAPIResponse{OK: false, Code: "ROLE_ERROR", Error: "failed to get role"})
		return
	}
	if role != "admin" && role != "owner" {
		writeClanJSON(w, http.StatusForbidden, clanAPIResponse{OK: false, Code: "FORBIDDEN", Error: "insufficient permissions"})
		return
	}

	if req.TargetID == 0 {
		writeClanJSON(w, http.StatusBadRequest, clanAPIResponse{OK: false, Code: "NO_TARGET", Error: "target_id is required"})
		return
	}

	reason := strings.TrimSpace(req.Reason)
	if len(reason) < 3 || len(reason) > 100 {
		writeClanJSON(w, http.StatusBadRequest, clanAPIResponse{OK: false, Code: "INVALID_REASON", Error: "reason must be between 3 and 100 characters"})
		return
	}

	if _, err := helpers.GetUserClanID(req.TargetID); err != nil {
		writeClanJSON(w, http.StatusNotFound, clanAPIResponse{OK: false, Code: "TARGET_NOT_IN_CLAN", Error: "target user is not in a clan"})
		return
	}

	if err := helpers.KickClanUser(clanID, req.TargetID); err != nil {
		if debugType {
			log.Printf("Ошибка кикнуть пользователя: %v", err)
		}
		writeClanJSON(w, http.StatusInternalServerError, clanAPIResponse{OK: false, Code: "KICK_FAILED", Error: "failed to kick user"})
		return
	}

	writeClanJSON(w, http.StatusOK, clanAPIResponse{
		OK: true,
		Data: map[string]any{
			"target_id": req.TargetID,
			"reason":    reason,
		},
	})
}

// ListClans — GET /api/v1/clans
func (h *ClanHandler) ListClans(w http.ResponseWriter, r *http.Request) {
	debugType := isDebugEnabled()

	clans, err := helpers.GetClans()
	if err != nil {
		if debugType {
			log.Printf("Ошибка получения кланов: %v", err)
		}
		writeClanJSON(w, http.StatusInternalServerError, clanAPIResponse{OK: false, Code: "LIST_ERROR", Error: "failed to get clans"})
		return
	}

	writeClanJSON(w, http.StatusOK, clanAPIResponse{
		OK: true,
		Data: map[string]any{
			"clans": clans,
		},
	})
}

// BanMember — POST /api/v1/clans/me/ban
func (h *ClanHandler) BanMember(w http.ResponseWriter, r *http.Request) {
	req, err := decodeClanRequest(r)
	if err != nil {
		writeClanJSON(w, http.StatusBadRequest, clanAPIResponse{OK: false, Code: "INVALID_JSON", Error: "invalid JSON body"})
		return
	}
	debugType := isDebugEnabled()

	clanID, err := helpers.GetUserClanID(req.UserID)
	if err != nil {
		writeClanJSON(w, http.StatusNotFound, clanAPIResponse{OK: false, Code: "NOT_IN_CLAN", Error: "user is not in a clan"})
		return
	}

	role, err := helpers.GetUserClanRole(req.UserID)
	if err != nil {
		writeClanJSON(w, http.StatusInternalServerError, clanAPIResponse{OK: false, Code: "ROLE_ERROR", Error: "failed to get role"})
		return
	}
	if role != "admin" && role != "owner" {
		writeClanJSON(w, http.StatusForbidden, clanAPIResponse{OK: false, Code: "FORBIDDEN", Error: "insufficient permissions"})
		return
	}

	if req.TargetID == 0 {
		writeClanJSON(w, http.StatusBadRequest, clanAPIResponse{OK: false, Code: "NO_TARGET", Error: "target_id is required"})
		return
	}

	reason := strings.TrimSpace(req.Reason)
	if len(reason) < 3 || len(reason) > 100 {
		writeClanJSON(w, http.StatusBadRequest, clanAPIResponse{OK: false, Code: "INVALID_REASON", Error: "reason must be between 3 and 100 characters"})
		return
	}

	if _, err := helpers.GetUserClanID(req.TargetID); err != nil {
		writeClanJSON(w, http.StatusNotFound, clanAPIResponse{OK: false, Code: "TARGET_NOT_IN_CLAN", Error: "target user is not in a clan"})
		return
	}

	if err := helpers.BlockMemberClan(clanID, req.TargetID, reason); err != nil {
		if debugType {
			log.Printf("Ошибка блокировки пользователя: %v", err)
		}
		writeClanJSON(w, http.StatusInternalServerError, clanAPIResponse{OK: false, Code: "BAN_FAILED", Error: "failed to ban user"})
		return
	}

	writeClanJSON(w, http.StatusOK, clanAPIResponse{
		OK: true,
		Data: map[string]any{
			"target_id": req.TargetID,
			"reason":    reason,
		},
	})
}

// DeleteMyClan — DELETE /api/v1/clans/me?user_id=...
func (h *ClanHandler) DeleteMyClan(w http.ResponseWriter, r *http.Request) {
	userID, err := parseUserIDParam(r)
	if err != nil {
		writeClanJSON(w, http.StatusBadRequest, clanAPIResponse{OK: false, Code: "INVALID_USER", Error: err.Error()})
		return
	}
	debugType := isDebugEnabled()

	if _, err := helpers.GetUserClanID(userID); err != nil {
		writeClanJSON(w, http.StatusNotFound, clanAPIResponse{OK: false, Code: "NOT_IN_CLAN", Error: "user is not in a clan"})
		return
	}

	clan, err := helpers.GetClanByOwnerID(userID)
	if err != nil {
		writeClanJSON(w, http.StatusForbidden, clanAPIResponse{OK: false, Code: "NOT_OWNER", Error: "user is not the clan owner"})
		return
	}

	if err := helpers.DeleteClan(uint64(clan.ID)); err != nil {
		if debugType {
			log.Printf("Ошибка удаления клана: %v", err)
		}
		writeClanJSON(w, http.StatusInternalServerError, clanAPIResponse{OK: false, Code: "DELETE_FAILED", Error: "failed to delete clan"})
		return
	}

	writeClanJSON(w, http.StatusOK, clanAPIResponse{
		OK: true,
		Data: map[string]any{
			"deleted_clan_id": clan.ID,
		},
	})
}

// JoinByInvite — POST /api/v1/clans/join
func (h *ClanHandler) JoinByInvite(w http.ResponseWriter, r *http.Request) {
	req, err := decodeClanRequest(r)
	if err != nil {
		writeClanJSON(w, http.StatusBadRequest, clanAPIResponse{OK: false, Code: "INVALID_JSON", Error: "invalid JSON body"})
		return
	}
	debugType := isDebugEnabled()

	if isGroupChat(req.ChatType) {
		writeClanJSON(w, http.StatusBadRequest, clanAPIResponse{OK: false, Code: "GROUP_NOT_ALLOWED", Error: "command is not available in groups"})
		return
	}

	if _, err := helpers.GetUserClanID(req.UserID); err == nil {
		writeClanJSON(w, http.StatusConflict, clanAPIResponse{OK: false, Code: "ALREADY_IN_CLAN", Error: "user is already in a clan"})
		return
	} else if err.Error() != "sql: no rows in result set" && !errors.Is(err, gorm.ErrRecordNotFound) {
		writeClanJSON(w, http.StatusInternalServerError, clanAPIResponse{OK: false, Code: "DB_ERROR", Error: "failed to check clan status"})
		return
	}

	code := strings.TrimSpace(req.Code)
	if code == "" {
		writeClanJSON(w, http.StatusBadRequest, clanAPIResponse{OK: false, Code: "INVALID_CODE", Error: "invite code is required"})
		return
	}

	clan, err := helpers.GetClanByInviteCode(code)
	if err != nil {
		if debugType {
			log.Printf("Ошибка при приглашении в клан: %v", err)
		}
		writeClanJSON(w, http.StatusNotFound, clanAPIResponse{OK: false, Code: "INVALID_INVITE", Error: "invite code not found"})
		return
	}

	clanID := uint64(clan.ID)
	if err := helpers.CheckBlacklist(clanID, req.UserID); err != nil {
		writeClanJSON(w, http.StatusForbidden, clanAPIResponse{OK: false, Code: "BLACKLISTED", Error: "user is blacklisted"})
		return
	}

	if err := helpers.JoinClan(clanID, req.UserID); err != nil {
		writeClanJSON(w, http.StatusInternalServerError, clanAPIResponse{OK: false, Code: "JOIN_FAILED", Error: "failed to join clan"})
		return
	}

	ownerIDRaw, _ := helpers.GetClanOwnerID(clanID)

	writeClanJSON(w, http.StatusOK, clanAPIResponse{
		OK: true,
		Data: map[string]any{
			"clan_id":  clanID,
			"owner_id": int64(ownerIDRaw),
		},
	})
}

// RevokeInviteCode — POST /api/v1/clans/me/invite-code/revoke
func (h *ClanHandler) RevokeInviteCode(w http.ResponseWriter, r *http.Request) {
	req, err := decodeClanRequest(r)
	if err != nil {
		writeClanJSON(w, http.StatusBadRequest, clanAPIResponse{OK: false, Code: "INVALID_JSON", Error: "invalid JSON body"})
		return
	}

	if isGroupChat(req.ChatType) {
		writeClanJSON(w, http.StatusBadRequest, clanAPIResponse{OK: false, Code: "GROUP_NOT_ALLOWED", Error: "command is not available in groups"})
		return
	}

	clanID, err := helpers.GetUserClanID(req.UserID)
	if err != nil {
		writeClanJSON(w, http.StatusNotFound, clanAPIResponse{OK: false, Code: "NOT_IN_CLAN", Error: "user is not in a clan"})
		return
	}

	role, err := helpers.GetUserClanRole(req.UserID)
	if err != nil {
		writeClanJSON(w, http.StatusInternalServerError, clanAPIResponse{OK: false, Code: "ROLE_ERROR", Error: "failed to get role"})
		return
	}
	if role != "owner" && role != "admin" {
		writeClanJSON(w, http.StatusForbidden, clanAPIResponse{OK: false, Code: "FORBIDDEN", Error: "insufficient permissions"})
		return
	}

	if err := helpers.RevokeInviteCode(clanID); err != nil {
		writeClanJSON(w, http.StatusInternalServerError, clanAPIResponse{OK: false, Code: "REVOKE_FAILED", Error: "failed to revoke invite code"})
		return
	}

	newCode, err := helpers.GetClanInviteCode(clanID)
	if err != nil {
		writeClanJSON(w, http.StatusInternalServerError, clanAPIResponse{OK: false, Code: "CODE_ERROR", Error: "failed to get new invite code"})
		return
	}

	writeClanJSON(w, http.StatusOK, clanAPIResponse{
		OK: true,
		Data: map[string]any{
			"invite_code": newCode,
		},
	})
}

// DeleteInviteCode — DELETE /api/v1/clans/me/invite-code?user_id=...&chat_type=...
func (h *ClanHandler) DeleteInviteCode(w http.ResponseWriter, r *http.Request) {
	userID, err := parseUserIDParam(r)
	if err != nil {
		writeClanJSON(w, http.StatusBadRequest, clanAPIResponse{OK: false, Code: "INVALID_USER", Error: err.Error()})
		return
	}
	chatType := r.URL.Query().Get("chat_type")
	if isGroupChat(chatType) {
		writeClanJSON(w, http.StatusBadRequest, clanAPIResponse{OK: false, Code: "GROUP_NOT_ALLOWED", Error: "command is not available in groups"})
		return
	}

	clanID, err := helpers.GetUserClanID(userID)
	if err != nil {
		writeClanJSON(w, http.StatusNotFound, clanAPIResponse{OK: false, Code: "NOT_IN_CLAN", Error: "user is not in a clan"})
		return
	}

	role, err := helpers.GetUserClanRole(userID)
	if err != nil {
		writeClanJSON(w, http.StatusInternalServerError, clanAPIResponse{OK: false, Code: "ROLE_ERROR", Error: "failed to get role"})
		return
	}
	if role != "owner" && role != "admin" {
		writeClanJSON(w, http.StatusForbidden, clanAPIResponse{OK: false, Code: "FORBIDDEN", Error: "insufficient permissions"})
		return
	}

	code, err := helpers.GetClanInviteCode(clanID)
	if err != nil || code == "" {
		writeClanJSON(w, http.StatusNotFound, clanAPIResponse{OK: false, Code: "NO_INVITE_CODE", Error: "invite code not found"})
		return
	}

	if err := helpers.DeleteInviteCode(clanID); err != nil {
		writeClanJSON(w, http.StatusInternalServerError, clanAPIResponse{OK: false, Code: "DELETE_CODE_FAILED", Error: "failed to delete invite code"})
		return
	}

	writeClanJSON(w, http.StatusOK, clanAPIResponse{
		OK:   true,
		Data: map[string]any{"deleted": true},
	})
}

// CreateInviteCode — POST /api/v1/clans/me/invite-code
func (h *ClanHandler) CreateInviteCode(w http.ResponseWriter, r *http.Request) {
	req, err := decodeClanRequest(r)
	if err != nil {
		writeClanJSON(w, http.StatusBadRequest, clanAPIResponse{OK: false, Code: "INVALID_JSON", Error: "invalid JSON body"})
		return
	}

	if isGroupChat(req.ChatType) {
		writeClanJSON(w, http.StatusBadRequest, clanAPIResponse{OK: false, Code: "GROUP_NOT_ALLOWED", Error: "command is not available in groups"})
		return
	}

	clanID, err := helpers.GetUserClanID(req.UserID)
	if err != nil {
		writeClanJSON(w, http.StatusNotFound, clanAPIResponse{OK: false, Code: "NOT_IN_CLAN", Error: "user is not in a clan"})
		return
	}

	role, err := helpers.GetUserClanRole(req.UserID)
	if err != nil {
		writeClanJSON(w, http.StatusInternalServerError, clanAPIResponse{OK: false, Code: "ROLE_ERROR", Error: "failed to get role"})
		return
	}
	if role != "owner" && role != "admin" {
		writeClanJSON(w, http.StatusForbidden, clanAPIResponse{OK: false, Code: "FORBIDDEN", Error: "insufficient permissions"})
		return
	}

	if _, err := helpers.GetClanInviteCode(clanID); err != nil {
		writeClanJSON(w, http.StatusInternalServerError, clanAPIResponse{OK: false, Code: "CODE_CHECK_FAILED", Error: "failed to check invite code"})
		return
	}

	inviteCode := std_helpers.GenerateCode()
	if err := helpers.CreateInviteCode(clanID, inviteCode); err != nil {
		writeClanJSON(w, http.StatusInternalServerError, clanAPIResponse{OK: false, Code: "CREATE_CODE_FAILED", Error: "failed to create invite code"})
		return
	}

	writeClanJSON(w, http.StatusCreated, clanAPIResponse{
		OK: true,
		Data: map[string]any{
			"invite_code": inviteCode,
		},
	})
}

// SetInviteExpire — POST /api/v1/clans/me/invite-code/expire
func (h *ClanHandler) SetInviteExpire(w http.ResponseWriter, r *http.Request) {
	req, err := decodeClanRequest(r)
	if err != nil {
		writeClanJSON(w, http.StatusBadRequest, clanAPIResponse{OK: false, Code: "INVALID_JSON", Error: "invalid JSON body"})
		return
	}

	if isGroupChat(req.ChatType) {
		writeClanJSON(w, http.StatusBadRequest, clanAPIResponse{OK: false, Code: "GROUP_NOT_ALLOWED", Error: "command is not available in groups"})
		return
	}

	duration, err := std_helpers.ParseDuration(strings.TrimSpace(req.Duration))
	if err != nil {
		writeClanJSON(w, http.StatusBadRequest, clanAPIResponse{OK: false, Code: "INVALID_DURATION", Error: "invalid duration"})
		return
	}

	clanID, err := helpers.GetUserClanID(req.UserID)
	if err != nil {
		writeClanJSON(w, http.StatusNotFound, clanAPIResponse{OK: false, Code: "NOT_IN_CLAN", Error: "user is not in a clan"})
		return
	}

	role, err := helpers.GetUserClanRole(req.UserID)
	if err != nil {
		writeClanJSON(w, http.StatusInternalServerError, clanAPIResponse{OK: false, Code: "ROLE_ERROR", Error: "failed to get role"})
		return
	}
	if role != "owner" && role != "admin" {
		writeClanJSON(w, http.StatusForbidden, clanAPIResponse{OK: false, Code: "FORBIDDEN", Error: "insufficient permissions"})
		return
	}

	code, err := helpers.GetClanInviteCode(clanID)
	if err != nil {
		writeClanJSON(w, http.StatusInternalServerError, clanAPIResponse{OK: false, Code: "CODE_CHECK_FAILED", Error: "failed to check invite code"})
		return
	}
	if code == "" {
		writeClanJSON(w, http.StatusNotFound, clanAPIResponse{OK: false, Code: "NO_INVITE_CODE", Error: "clan has no invite code"})
		return
	}

	rdb := cache.GetRedis()
	if rdb == nil {
		writeClanJSON(w, http.StatusServiceUnavailable, clanAPIResponse{OK: false, Code: "REDIS_UNAVAILABLE", Error: "redis is unavailable"})
		return
	}

	timerData := structs.ClanInviteTimer{
		ClanID: int64(clanID),
		UserID: int64(req.UserID),
	}
	data, _ := json.Marshal(timerData)
	redisKey := "clan_invite:" + strconv.FormatInt(int64(clanID), 10)

	if err := rdb.Set(context.Background(), redisKey, data, duration).Err(); err != nil {
		log.Printf("Ошибка записи в Redis: %v", err)
		writeClanJSON(w, http.StatusInternalServerError, clanAPIResponse{OK: false, Code: "REDIS_ERROR", Error: "failed to set invite timer"})
		return
	}

	expiresAt := time.Now().Add(duration).Format("2006-01-02 15:04:05")
	writeClanJSON(w, http.StatusOK, clanAPIResponse{
		OK: true,
		Data: map[string]any{
			"expires_at": expiresAt,
		},
	})
}

// DeleteInviteExpire — DELETE /api/v1/clans/me/invite-code/expire?user_id=...&chat_type=...
func (h *ClanHandler) DeleteInviteExpire(w http.ResponseWriter, r *http.Request) {
	userID, err := parseUserIDParam(r)
	if err != nil {
		writeClanJSON(w, http.StatusBadRequest, clanAPIResponse{OK: false, Code: "INVALID_USER", Error: err.Error()})
		return
	}
	if isGroupChat(r.URL.Query().Get("chat_type")) {
		writeClanJSON(w, http.StatusBadRequest, clanAPIResponse{OK: false, Code: "GROUP_NOT_ALLOWED", Error: "command is not available in groups"})
		return
	}

	clanID, err := helpers.GetUserClanID(userID)
	if err != nil {
		writeClanJSON(w, http.StatusNotFound, clanAPIResponse{OK: false, Code: "NOT_IN_CLAN", Error: "user is not in a clan"})
		return
	}

	role, err := helpers.GetUserClanRole(userID)
	if err != nil {
		writeClanJSON(w, http.StatusInternalServerError, clanAPIResponse{OK: false, Code: "ROLE_ERROR", Error: "failed to get role"})
		return
	}
	if role != "owner" && role != "admin" {
		writeClanJSON(w, http.StatusForbidden, clanAPIResponse{OK: false, Code: "FORBIDDEN", Error: "insufficient permissions"})
		return
	}

	code, err := helpers.GetClanInviteCode(clanID)
	if err != nil || code == "" {
		writeClanJSON(w, http.StatusNotFound, clanAPIResponse{OK: false, Code: "NO_INVITE_CODE", Error: "invite code not found"})
		return
	}

	rdb := cache.GetRedis()
	if rdb == nil {
		writeClanJSON(w, http.StatusServiceUnavailable, clanAPIResponse{OK: false, Code: "REDIS_UNAVAILABLE", Error: "redis is unavailable"})
		return
	}

	redisKey := "clan_invite:" + strconv.FormatInt(int64(clanID), 10)
	deletedCount, err := rdb.Del(context.Background(), redisKey).Result()
	if err != nil {
		writeClanJSON(w, http.StatusInternalServerError, clanAPIResponse{OK: false, Code: "REDIS_ERROR", Error: "failed to delete invite timer"})
		return
	}
	if deletedCount == 0 {
		writeClanJSON(w, http.StatusNotFound, clanAPIResponse{OK: false, Code: "NO_ACTIVE_TIMER", Error: "no active timer"})
		return
	}

	writeClanJSON(w, http.StatusOK, clanAPIResponse{
		OK:   true,
		Data: map[string]any{"deleted": true},
	})
}

// LeaveClan — POST /api/v1/clans/leave
func (h *ClanHandler) LeaveClan(w http.ResponseWriter, r *http.Request) {
	req, err := decodeClanRequest(r)
	if err != nil {
		writeClanJSON(w, http.StatusBadRequest, clanAPIResponse{OK: false, Code: "INVALID_JSON", Error: "invalid JSON body"})
		return
	}
	debugType := isDebugEnabled()

	clanID, err := helpers.GetUserClanID(req.UserID)
	if err != nil {
		writeClanJSON(w, http.StatusNotFound, clanAPIResponse{OK: false, Code: "NOT_IN_CLAN", Error: "user is not in a clan"})
		return
	}

	role, err := helpers.GetUserClanRole(req.UserID)
	if err != nil {
		writeClanJSON(w, http.StatusInternalServerError, clanAPIResponse{OK: false, Code: "ROLE_ERROR", Error: "failed to get role"})
		return
	}
	if role == "owner" {
		writeClanJSON(w, http.StatusConflict, clanAPIResponse{OK: false, Code: "OWNER_CANNOT_LEAVE", Error: "clan owner cannot leave"})
		return
	}

	ownerIDRaw, err := helpers.GetClanOwnerID(clanID)
	if err != nil {
		writeClanJSON(w, http.StatusInternalServerError, clanAPIResponse{OK: false, Code: "OWNER_ERROR", Error: "failed to get clan owner"})
		return
	}

	if err := helpers.LeaveClan(clanID, req.UserID); err != nil {
		if debugType {
			log.Printf("Ошибка при выходе из клана: %v", err)
		}
		writeClanJSON(w, http.StatusInternalServerError, clanAPIResponse{OK: false, Code: "LEAVE_FAILED", Error: "failed to leave clan"})
		return
	}

	writeClanJSON(w, http.StatusOK, clanAPIResponse{
		OK: true,
		Data: map[string]any{
			"owner_id": int64(ownerIDRaw),
		},
	})
}

// GetInviteCode — GET /api/v1/clans/me/invite-code?user_id=...
func (h *ClanHandler) GetInviteCode(w http.ResponseWriter, r *http.Request) {
	userID, err := parseUserIDParam(r)
	if err != nil {
		writeClanJSON(w, http.StatusBadRequest, clanAPIResponse{OK: false, Code: "INVALID_USER", Error: err.Error()})
		return
	}

	clanID, err := helpers.GetUserClanID(userID)
	if err != nil {
		writeClanJSON(w, http.StatusNotFound, clanAPIResponse{OK: false, Code: "NOT_IN_CLAN", Error: "user is not in a clan"})
		return
	}

	role, err := helpers.GetUserClanRole(userID)
	if err != nil {
		writeClanJSON(w, http.StatusInternalServerError, clanAPIResponse{OK: false, Code: "ROLE_ERROR", Error: "failed to get role"})
		return
	}
	if role != "owner" && role != "admin" {
		writeClanJSON(w, http.StatusForbidden, clanAPIResponse{OK: false, Code: "FORBIDDEN", Error: "insufficient permissions"})
		return
	}

	inviteCode, err := helpers.GetClanInviteCode(clanID)
	if err != nil {
		writeClanJSON(w, http.StatusInternalServerError, clanAPIResponse{OK: false, Code: "CODE_ERROR", Error: "failed to get invite code"})
		return
	}
	if inviteCode == "" {
		writeClanJSON(w, http.StatusNotFound, clanAPIResponse{OK: false, Code: "NO_INVITE_CODE", Error: "invite code not found"})
		return
	}

	writeClanJSON(w, http.StatusOK, clanAPIResponse{
		OK: true,
		Data: map[string]any{
			"invite_code": inviteCode,
		},
	})
}