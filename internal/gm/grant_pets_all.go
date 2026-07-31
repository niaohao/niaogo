package gm

import (
	"net/http"

	"niaohao/server/internal/store"
)

// handleGrantPetsAll 一键发放「前端 PetXMLInfo 有名 ∩ pets.xml 有数值」的精灵。
// POST JSON: { userId, level?, dv?, nature?, perfect?, confirm:true }
// perfect=true：DV=31 + 学习力六维均分 510；可与 level 同用。
func (s *Server) handleGrantPetsAll(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, 405, "POST only")
		return
	}
	var req struct {
		UserID  int64  `json:"userId"`
		Level   int    `json:"level"`
		DV      int    `json:"dv"`
		Nature  int    `json:"nature"`
		Perfect bool   `json:"perfect"`
		Confirm bool   `json:"confirm"`
		Admin   string `json:"admin"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if !req.Confirm {
		writeErr(w, 400, "需要 confirm=true")
		return
	}
	if s.cfg.Store == nil || req.UserID <= 0 {
		writeErr(w, 400, "需要 userId")
		return
	}
	if s.cfg.Catalog == nil {
		writeErr(w, 503, "精灵表未加载")
		return
	}
	u, err := s.cfg.Store.FindByUserID(req.UserID)
	if err != nil || u == nil {
		writeErr(w, 404, "用户不存在")
		return
	}

	level := req.Level
	if level <= 0 {
		level = 1
	}
	if level > 100 {
		level = 100
	}
	dv := req.DV
	if dv < 0 {
		dv = 0
	}
	if dv > 31 {
		dv = 31
	}
	nature := req.Nature
	if nature < 0 {
		nature = 0
	}
	var ev [6]int
	if req.Perfect {
		dv = 31
		ev = store.PerfectBalancedEV()
	}

	ids := s.cfg.Catalog.GrantablePetIDs()
	if len(ids) == 0 {
		writeErr(w, 503, "可发放精灵为空（需前端 PetXMLInfo ∩ pets.xml）")
		return
	}

	batch := make([]store.Pet, 0, len(ids))
	for _, id := range ids {
		name := s.cfg.Catalog.FrontendPetNameOf(id)
		if name == "" {
			name = s.cfg.Catalog.PetNameOf(id)
		}
		if name == "" {
			continue // 前端无名不发
		}
		batch = append(batch, store.Pet{
			PetID:  id,
			Name:   name,
			Level:  level,
			DV:     dv,
			Nature: nature,
			Skills: s.cfg.Catalog.DefaultSkillsAtLevel(id, level),
			EV:     ev,
		})
	}
	if len(batch) == 0 {
		writeErr(w, 503, "过滤后无可发放精灵")
		return
	}

	granted, firstCatch, err := s.cfg.Store.GrantPetsBatch(req.UserID, batch)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}

	admin := actorOrHeader(r, req.Admin)
	aid, _ := s.cfg.Store.InsertGMAudit(admin, "grant_pets_all", req.UserID, map[string]any{
		"total": len(ids), "granted": granted, "batch": len(batch), "level": level, "dv": dv,
		"nature": nature, "perfect": req.Perfect, "firstCatch": firstCatch,
		"source": "PetXMLInfo∩pets.xml",
	})
	writeJSON(w, map[string]any{
		"ok": true, "auditId": aid, "total": len(ids), "granted": granted,
		"level": level, "dv": dv, "nature": nature, "perfect": req.Perfect,
		"firstCatchTime": firstCatch,
		"hint":           "仅发放前端表有名字的精灵；背包满后其余进仓库；在线建议重登",
	})
}
