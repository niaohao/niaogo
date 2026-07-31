package gm

import (
	"net/http"
	"strconv"
	"time"

	"niaohao/server/internal/store"
)

type topLevelReq struct {
	UserID int64  `json:"userId"`
	Score  int    `json:"score"`  // set 用绝对值
	Delta  int    `json:"delta"`  // add 用增量
	Admin  string `json:"admin"`
}

func (s *Server) loadOps(uid int64) store.UserOpsState {
	st := store.UserOpsState{}
	if s.cfg.Store == nil {
		return store.NormalizeUserOps(st, time.Now())
	}
	raw, _ := s.cfg.Store.GetUserOps(uid)
	return store.NormalizeUserOps(raw, time.Now())
}

func (s *Server) saveOps(uid int64, st store.UserOpsState) error {
	if s.cfg.Store == nil {
		return nil
	}
	return s.cfg.Store.SetUserOps(uid, store.NormalizeUserOps(st, time.Now()))
}

// handleTopLevelGet GET /api/top-level?uid=
func (s *Server) handleTopLevelGet(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Store == nil {
		writeErr(w, 500, "store 未就绪")
		return
	}
	uid, _ := strconv.ParseInt(r.URL.Query().Get("uid"), 10, 64)
	if uid <= 0 {
		writeErr(w, 400, "需要 uid")
		return
	}
	u, err := s.cfg.Store.FindByUserID(uid)
	if err != nil || u == nil {
		writeErr(w, 404, "用户不存在")
		return
	}
	score := store.ClampTopLevel(s.loadOps(uid).CurTopLevel)
	writeJSON(w, map[string]any{"ok": true, "userId": uid, "curTopLevel": score})
}

// handleTopLevelSet POST /api/top-level/set  {userId, score}
func (s *Server) handleTopLevelSet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, 405, "POST only")
		return
	}
	var req topLevelReq
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if s.cfg.Store == nil || req.UserID <= 0 {
		writeErr(w, 400, "需要 userId 且 Store 就绪")
		return
	}
	u, err := s.cfg.Store.FindByUserID(req.UserID)
	if err != nil || u == nil {
		writeErr(w, 404, "用户不存在")
		return
	}
	st := s.loadOps(req.UserID)
	before := st.CurTopLevel
	st.CurTopLevel = store.ClampTopLevel(req.Score)
	if err := s.saveOps(req.UserID, st); err != nil {
		writeErr(w, 500, "SetUserOps: "+err.Error())
		return
	}
	admin := actorOrHeader(r, req.Admin)
	aid, _ := s.cfg.Store.InsertGMAudit(admin, "top_level_set", req.UserID, map[string]any{
		"before": before, "score": st.CurTopLevel,
	})
	writeJSON(w, map[string]any{"ok": true, "auditId": aid, "userId": req.UserID, "curTopLevel": st.CurTopLevel})
}

// handleTopLevelAdd POST /api/top-level/add  {userId, delta}
func (s *Server) handleTopLevelAdd(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, 405, "POST only")
		return
	}
	var req topLevelReq
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if s.cfg.Store == nil || req.UserID <= 0 {
		writeErr(w, 400, "需要 userId 且 Store 就绪")
		return
	}
	if req.Delta == 0 {
		writeErr(w, 400, "delta 不能为 0")
		return
	}
	u, err := s.cfg.Store.FindByUserID(req.UserID)
	if err != nil || u == nil {
		writeErr(w, 404, "用户不存在")
		return
	}
	st := s.loadOps(req.UserID)
	before := st.CurTopLevel
	st.CurTopLevel = store.ClampTopLevel(st.CurTopLevel + req.Delta)
	if err := s.saveOps(req.UserID, st); err != nil {
		writeErr(w, 500, "SetUserOps: "+err.Error())
		return
	}
	admin := actorOrHeader(r, req.Admin)
	aid, _ := s.cfg.Store.InsertGMAudit(admin, "top_level_add", req.UserID, map[string]any{
		"before": before, "delta": req.Delta, "score": st.CurTopLevel,
	})
	writeJSON(w, map[string]any{"ok": true, "auditId": aid, "userId": req.UserID, "curTopLevel": st.CurTopLevel})
}
