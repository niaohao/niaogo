package gm

import (
	"encoding/json"
	"math/rand"
	"net/http"
)

// petUpdateReq 改精灵：等级/经验/个体/性格/特性/技能/学习力（背包或仓库，按 catchTime 定位）。
type petUpdateReq struct {
	UserID    int64           `json:"userId"`
	UID       int64           `json:"uid"` // 兼容旧前端
	CatchTime int64           `json:"catchTime"`
	Level     *int            `json:"level"`
	Exp       *int            `json:"exp"`
	DV        *int            `json:"dv"`
	Nature    *int            `json:"nature"`
	Name      *string         `json:"name"`
	Trait     *int            `json:"trait"`  // 0=清除；-1=随机 1006-1045；1006-1045=指定
	Skills    []int           `json:"skills"` // 最长 4 槽；仅当请求含 skills 字段时写入
	EV        json.RawMessage `json:"ev"`     // [6] 或 {hp,atk,def,sa,sd,sp}
	// 扁平字段（对齐参考服 /gm/pet/ev）
	EvHP  *int `json:"ev_hp"`
	EvAtk *int `json:"ev_attack"`
	EvDef *int `json:"ev_defence"`
	EvSA  *int `json:"ev_sa"`
	EvSD  *int `json:"ev_sd"`
	EvSP  *int `json:"ev_sp"`
	Admin string `json:"admin"`
}

func (s *Server) handlePetUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, 405, "POST only")
		return
	}
	var raw map[string]json.RawMessage
	if err := decodeJSON(r, &raw); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	b, _ := json.Marshal(raw)
	var req petUpdateReq
	if err := json.Unmarshal(b, &req); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	_, hasSkills := raw["skills"]
	uid := req.UserID
	if uid <= 0 {
		uid = req.UID
	}
	if s.cfg.Store == nil || uid <= 0 || req.CatchTime <= 0 {
		writeErr(w, 400, "需要 userId/catchTime")
		return
	}
	u, err := s.cfg.Store.FindByUserID(uid)
	if err != nil || u == nil {
		writeErr(w, 404, "用户不存在")
		return
	}
	p, err := s.cfg.Store.GetPetByCatchTime(uid, req.CatchTime)
	if err != nil || p == nil {
		writeErr(w, 404, "未找到该精灵")
		return
	}

	changed := map[string]any{}
	coreDirty := false

	if req.Name != nil {
		name := *req.Name
		if name != p.Name {
			p.Name = name
			coreDirty = true
			changed["name"] = name
		}
	}
	if req.Level != nil {
		lv := *req.Level
		if lv < 1 {
			lv = 1
		}
		if lv > 100 {
			lv = 100
		}
		if lv != p.Level {
			p.Level = lv
			coreDirty = true
			changed["level"] = lv
		}
	}
	if req.Exp != nil {
		exp := *req.Exp
		if exp < 0 {
			exp = 0
		}
		if exp != p.Exp {
			p.Exp = exp
			coreDirty = true
			changed["exp"] = exp
		}
	}
	if req.DV != nil {
		dv := *req.DV
		if dv < 0 {
			dv = 0
		}
		if dv > 31 {
			dv = 31
		}
		if dv != p.DV {
			p.DV = dv
			coreDirty = true
			changed["dv"] = dv
		}
	}
	if req.Nature != nil {
		nat := *req.Nature
		if nat < 0 {
			nat = 0
		}
		if nat != p.Nature {
			p.Nature = nat
			coreDirty = true
			changed["nature"] = nat
		}
	}
	if hasSkills {
		skills := normalizePetSkills(req.Skills)
		if !intSliceEq(p.Skills, skills) {
			p.Skills = skills
			coreDirty = true
			changed["skills"] = skills
		}
	}

	traitDirty := false
	traitVal := 0
	if req.Trait != nil {
		t := *req.Trait
		if t == -1 {
			t = 1006 + rand.Intn(40) // 1006-1045
		}
		if t != 0 && (t < 1006 || t > 1045) {
			writeErr(w, 400, "特性须为 0(无)/-1(随机)/1006-1045")
			return
		}
		if t != p.Trait {
			traitVal = t
			traitDirty = true
			changed["trait"] = t
		}
	}

	ev, ok, errMsg := parsePetEV(req, p.EV)
	if errMsg != "" {
		writeErr(w, 400, errMsg)
		return
	}
	evDirty := false
	if ok {
		if ev != p.EV {
			p.EV = ev
			evDirty = true
			changed["ev"] = ev[:]
		}
	}

	if !coreDirty && !evDirty && !traitDirty {
		writeErr(w, 400, "无有效修改字段（level/exp/dv/nature/name/trait/skills/ev）")
		return
	}
	if coreDirty {
		if err := s.cfg.Store.UpsertPet(p); err != nil {
			writeErr(w, 500, "UpsertPet: "+err.Error())
			return
		}
	}
	if traitDirty {
		if err := s.cfg.Store.SetPetTrait(uid, req.CatchTime, traitVal); err != nil {
			writeErr(w, 500, "SetPetTrait: "+err.Error())
			return
		}
		p.Trait = traitVal
	}
	if evDirty {
		if err := s.cfg.Store.SetPetEV(uid, req.CatchTime, p.EV); err != nil {
			writeErr(w, 500, "SetPetEV: "+err.Error())
			return
		}
	}

	if s.cfg.Notify != nil {
		s.cfg.Notify.PushPetRefresh(uid, req.CatchTime)
	}

	admin := actorOrHeader(r, req.Admin)
	label := s.petLabel(p.PetID)
	aid, _ := s.cfg.Store.InsertGMAudit(admin, "pet_update", uid, map[string]any{
		"catchTime": req.CatchTime, "petId": p.PetID, "label": label,
		"inBag": p.InBag, "changed": changed,
	})
	writeJSON(w, map[string]any{
		"ok": true, "auditId": aid, "label": label, "catchTime": req.CatchTime,
		"name": p.Name, "level": p.Level, "exp": p.Exp, "dv": p.DV, "nature": p.Nature,
		"trait": p.Trait, "skills": p.Skills,
		"ev": map[string]int{
			"hp": p.EV[0], "atk": p.EV[1], "def": p.EV[2],
			"sa": p.EV[3], "sd": p.EV[4], "sp": p.EV[5],
		},
		"evSum": p.EV[0] + p.EV[1] + p.EV[2] + p.EV[3] + p.EV[4] + p.EV[5],
	})
}

func normalizePetSkills(in []int) []int {
	out := make([]int, 0, 4)
	for i := 0; i < len(in) && len(out) < 4; i++ {
		if in[i] > 0 {
			out = append(out, in[i])
		}
	}
	return out
}

func intSliceEq(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// parsePetEV 解析六维学习力；未传返回 ok=false；非法返回 errMsg。
func parsePetEV(req petUpdateReq, cur [6]int) (ev [6]int, ok bool, errMsg string) {
	ev = cur
	if len(req.EV) > 0 && string(req.EV) != "null" {
		var arr []int
		if err := json.Unmarshal(req.EV, &arr); err == nil && len(arr) == 6 {
			for i := 0; i < 6; i++ {
				ev[i] = arr[i]
			}
			ok = true
		} else {
			var obj struct {
				HP  int `json:"hp"`
				Atk int `json:"atk"`
				Def int `json:"def"`
				SA  int `json:"sa"`
				SD  int `json:"sd"`
				SP  int `json:"sp"`
			}
			if err := json.Unmarshal(req.EV, &obj); err != nil {
				return cur, false, "ev 须为长度6数组或 {hp,atk,def,sa,sd,sp}"
			}
			ev = [6]int{obj.HP, obj.Atk, obj.Def, obj.SA, obj.SD, obj.SP}
			ok = true
		}
	}
	// 扁平字段覆盖（可与 ev 对象混用）
	flat := []*int{req.EvHP, req.EvAtk, req.EvDef, req.EvSA, req.EvSD, req.EvSP}
	for i, ptr := range flat {
		if ptr != nil {
			ev[i] = *ptr
			ok = true
		}
	}
	if !ok {
		return cur, false, ""
	}
	sum := 0
	for i := 0; i < 6; i++ {
		if ev[i] < 0 || ev[i] > 255 {
			return cur, false, "学习力单项需在 0-255 之间"
		}
		sum += ev[i]
	}
	if sum > 510 {
		return cur, false, "学习力总和不能超过 510"
	}
	return ev, true, ""
}
