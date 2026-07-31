package gm

import (
	"net/http"
	"strconv"

	"niaohao/server/internal/store"
)

const gmStatMax = 999999

func sixMap(v [6]int) map[string]int {
	return map[string]int{
		"hp": v[0], "atk": v[1], "def": v[2],
		"sa": v[3], "sd": v[4], "sp": v[5],
	}
}

func (s *Server) panelOf(p *store.Pet) (current, natural [6]int, locked bool) {
	if s.cfg.PanelCalc != nil {
		return s.cfg.PanelCalc(p)
	}
	return [6]int{}, [6]int{}, p != nil && p.HasGMStats
}
func (s *Server) handlePetStats(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handlePetStatsGet(w, r)
	case http.MethodPost:
		s.handlePetStatsSet(w, r)
	default:
		writeErr(w, 405, "GET or POST")
	}
}

func (s *Server) handlePetStatsGet(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Store == nil {
		writeErr(w, 500, "store 未就绪")
		return
	}
	uid, _ := strconv.ParseInt(r.URL.Query().Get("uid"), 10, 64)
	if uid <= 0 {
		uid, _ = strconv.ParseInt(r.URL.Query().Get("userId"), 10, 64)
	}
	catch, _ := strconv.ParseInt(r.URL.Query().Get("catchTime"), 10, 64)
	if uid <= 0 || catch <= 0 {
		writeErr(w, 400, "需要 uid/catchTime")
		return
	}
	p, err := s.cfg.Store.GetPetByCatchTime(uid, catch)
	if err != nil || p == nil {
		writeErr(w, 404, "未找到该精灵")
		return
	}
	cur, nat, locked := s.panelOf(p)
	where := "仓库"
	if p.InBag {
		where = "背包"
	}
	userView := map[string]any{"userId": uid}
	online, liveMap := false, 0
	if u, err := s.cfg.Store.FindByUserID(uid); err == nil && u != nil {
		userView["nickname"] = u.Nickname
		userView["email"] = u.Email
		userView["coins"] = u.Coins
		userView["gold"] = u.Gold
		userView["mapId"] = u.MapID
	}
	if s.cfg.Notify != nil {
		for _, op := range s.cfg.Notify.ListOnlinePlayers() {
			if op.UserID == uid {
				online = true
				liveMap = op.MapID
				break
			}
		}
	}
	writeJSON(w, map[string]any{
		"ok": true,
		"user": userView,
		"online": online, "liveMapId": liveMap,
		"pet": map[string]any{
			"userId": uid, "catchTime": p.CatchTime, "petId": p.PetID,
			"label": s.petLabel(p.PetID), "name": p.Name,
			"level": p.Level, "exp": p.Exp, "dv": p.DV, "nature": p.Nature,
			"inBag": p.InBag, "where": where,
			"ev": sixMap(p.EV), "bonus": sixMap(p.Bonus),
		},
		"gmLocked": locked,
		"current":  sixMap(cur),
		"natural":  sixMap(nat),
	})
}

func (s *Server) handlePetStatsSet(w http.ResponseWriter, r *http.Request) {
	var req struct {
		UserID    int64 `json:"userId"`
		UID       int64 `json:"uid"`
		CatchTime int64 `json:"catchTime"`
		HP        int   `json:"hp"`
		Atk       int   `json:"atk"`
		Def       int   `json:"def"`
		SA        int   `json:"sa"`
		SD        int   `json:"sd"`
		SP        int   `json:"sp"`
		// 也支持数组
		Stats []int  `json:"stats"`
		Admin string `json:"admin"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	uid := req.UserID
	if uid <= 0 {
		uid = req.UID
	}
	if s.cfg.Store == nil || uid <= 0 || req.CatchTime <= 0 {
		writeErr(w, 400, "需要 userId/catchTime")
		return
	}
	stats := [6]int{req.HP, req.Atk, req.Def, req.SA, req.SD, req.SP}
	if len(req.Stats) == 6 {
		for i := 0; i < 6; i++ {
			stats[i] = req.Stats[i]
		}
	}
	for i := 0; i < 6; i++ {
		if stats[i] < 1 {
			writeErr(w, 400, "能力值各项须 ≥1")
			return
		}
		if stats[i] > gmStatMax {
			writeErr(w, 400, "能力值单项不能超过 "+strconv.Itoa(gmStatMax))
			return
		}
	}
	p, err := s.cfg.Store.GetPetByCatchTime(uid, req.CatchTime)
	if err != nil || p == nil {
		writeErr(w, 404, "未找到该精灵")
		return
	}
	if err := s.cfg.Store.SetPetGMStats(uid, req.CatchTime, stats); err != nil {
		writeErr(w, 500, "SetPetGMStats: "+err.Error())
		return
	}
	p.GMStats = stats
	p.HasGMStats = true
	if s.cfg.Notify != nil {
		s.cfg.Notify.PushPetRefresh(uid, req.CatchTime)
	}
	_, nat, _ := s.panelOf(p)
	admin := actorOrHeader(r, req.Admin)
	label := s.petLabel(p.PetID)
	aid, _ := s.cfg.Store.InsertGMAudit(admin, "pet_stats_set", uid, map[string]any{
		"catchTime": req.CatchTime, "petId": p.PetID, "label": label,
		"stats": sixMap(stats), "natural": sixMap(nat),
	})
	writeJSON(w, map[string]any{
		"ok": true, "auditId": aid, "label": label, "gmLocked": true,
		"current": sixMap(stats), "natural": sixMap(nat),
	})
}

func (s *Server) handlePetStatsRestore(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, 405, "POST only")
		return
	}
	var req struct {
		UserID    int64  `json:"userId"`
		UID       int64  `json:"uid"`
		CatchTime int64  `json:"catchTime"`
		Admin     string `json:"admin"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	uid := req.UserID
	if uid <= 0 {
		uid = req.UID
	}
	if s.cfg.Store == nil || uid <= 0 || req.CatchTime <= 0 {
		writeErr(w, 400, "需要 userId/catchTime")
		return
	}
	p, err := s.cfg.Store.GetPetByCatchTime(uid, req.CatchTime)
	if err != nil || p == nil {
		writeErr(w, 404, "未找到该精灵")
		return
	}
	if err := s.cfg.Store.ClearPetGMStats(uid, req.CatchTime); err != nil {
		writeErr(w, 500, "ClearPetGMStats: "+err.Error())
		return
	}
	p.HasGMStats = false
	p.GMStats = [6]int{}
	if s.cfg.Notify != nil {
		s.cfg.Notify.PushPetRefresh(uid, req.CatchTime)
	}
	cur, nat, _ := s.panelOf(p)
	admin := actorOrHeader(r, req.Admin)
	label := s.petLabel(p.PetID)
	aid, _ := s.cfg.Store.InsertGMAudit(admin, "pet_stats_restore", uid, map[string]any{
		"catchTime": req.CatchTime, "petId": p.PetID, "label": label,
		"natural": sixMap(nat),
	})
	writeJSON(w, map[string]any{
		"ok": true, "auditId": aid, "label": label, "gmLocked": false,
		"current": sixMap(cur), "natural": sixMap(nat),
	})
}
