package gm

import (
	"net/http"
	"strconv"
	"time"

	"niaohao/server/internal/store"
)

func (s *Server) handleUserDetail(w http.ResponseWriter, r *http.Request) {
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
	bag, _ := s.cfg.Store.ListBagPets(uid)
	storage, _ := s.cfg.Store.ListStoragePets(uid)
	rowei, _ := s.cfg.Store.ListRoweiPets(uid)
	items, _ := s.cfg.Store.ListAllItems(uid)
	prog, _ := s.cfg.Store.GetProgress(uid)
	nono, _ := s.cfg.Store.GetNono(uid)
	tasks, _ := s.cfg.Store.ListTaskStatuses(uid)
	_, mails, _ := s.cfg.Store.ListMails(uid, 1)
	clothes, _ := s.cfg.Store.ListWornClothes(uid)

	online := false
	mapID := u.MapID
	inBattle := false
	if s.cfg.Notify != nil {
		for _, p := range s.cfg.Notify.ListOnlinePlayers() {
			if p.UserID == uid {
				online = true
				mapID = p.MapID
				inBattle = p.InBattle
				break
			}
		}
	}

	petView := func(list []store.Pet) []map[string]any {
		out := make([]map[string]any, 0, len(list))
		for _, p := range list {
			label := s.petLabel(p.PetID)
			out = append(out, map[string]any{
				"catchTime": p.CatchTime, "petId": p.PetID, "label": label,
				"name": p.Name, "level": p.Level, "exp": p.Exp, "dv": p.DV,
				"nature": p.Nature, "trait": p.Trait, "inBag": p.InBag, "bagPos": p.BagPos,
				"skills": p.Skills,
				"ev": []int{p.EV[0], p.EV[1], p.EV[2], p.EV[3], p.EV[4], p.EV[5]},
				"evSum": p.EV[0] + p.EV[1] + p.EV[2] + p.EV[3] + p.EV[4] + p.EV[5],
				"hasGMStats": p.HasGMStats,
			})
			if p.HasGMStats {
				out[len(out)-1]["gmStats"] = []int{p.GMStats[0], p.GMStats[1], p.GMStats[2], p.GMStats[3], p.GMStats[4], p.GMStats[5]}
			}
			if s.cfg.PanelCalc != nil {
				cur, nat, _ := s.cfg.PanelCalc(&p)
				out[len(out)-1]["stats"] = []int{cur[0], cur[1], cur[2], cur[3], cur[4], cur[5]}
				out[len(out)-1]["naturalStats"] = []int{nat[0], nat[1], nat[2], nat[3], nat[4], nat[5]}
			}
		}
		return out
	}
	itemView := make([]map[string]any, 0, len(items))
	for _, it := range items {
		itemView = append(itemView, map[string]any{
			"itemId": it.ItemID, "label": s.itemLabel(it.ItemID),
			"count": it.Count, "expire": it.ExpireTime,
		})
	}
	mailView := make([]map[string]any, 0, len(mails))
	for _, m := range mails {
		mailView = append(mailView, map[string]any{
			"id": m.ID, "template": m.Template, "from": m.FromNick,
			"read": m.Read, "claimed": m.Claimed, "time": m.MailTime,
			"hasReward": m.Reward.HasReward(),
		})
	}
	taskView := make([]map[string]any, 0, len(tasks))
	for id, st := range tasks {
		taskView = append(taskView, map[string]any{"taskId": id, "status": st})
	}
	clothView := make([]map[string]any, 0, len(clothes))
	for _, c := range clothes {
		clothView = append(clothView, map[string]any{
			"slot": c.SlotIdx, "itemId": c.ItemID, "label": s.itemLabel(c.ItemID), "level": c.Level,
		})
	}

	var nonoView any
	if nono != nil {
		nonoView = map[string]any{
			"hasNono": nono.HasNono, "nick": nono.Nick, "superNono": nono.SuperNono,
			"superLevel": nono.SuperLevel, "superStage": nono.SuperStage,
			"superMonths": nono.SuperMonths, "following": nono.IsFollowing(),
			"power": nono.Power, "vipEndTime": nono.VipEndTime, "color": nono.Color,
		}
	}

	expPool, _ := s.cfg.Store.GetExpPool(uid)
	ops, _ := s.cfg.Store.GetUserOps(uid)
	ops = store.NormalizeUserOps(ops, time.Now())
	writeJSON(w, map[string]any{
		"ok": true,
		"user": map[string]any{
			"userId": u.UserID, "nickname": u.Nickname, "email": u.Email,
			"coins": u.Coins, "gold": u.Gold, "expPool": expPool, "energy": u.Energy,
			"mapId": u.MapID, "posX": u.PosX, "posY": u.PosY,
			"loginCnt": u.LoginCnt, "lastOnline": u.LastOnline,
			"honor": ops.Honor, "curTopLevel": ops.CurTopLevel,
		},
		"online": online, "liveMapId": mapID, "inBattle": inBattle,
		"pets": map[string]any{
			"bag": petView(bag), "storage": petView(storage), "rowei": petView(rowei),
		},
		"items": itemView,
		"progress": map[string]any{
			"braveCur": prog.BraveCur, "braveMax": prog.BraveMax,
			"freshCur": prog.FreshCur, "freshMax": prog.FreshMax,
		},
		"nono": nonoView, "tasks": taskView, "mails": mailView, "clothes": clothView,
	})
}

func (s *Server) handleOnline(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Notify == nil {
		writeJSON(w, map[string]any{"ok": true, "players": []any{}})
		return
	}
	list := s.cfg.Notify.ListOnlinePlayers()
	// 补昵称
	out := make([]map[string]any, 0, len(list))
	for _, p := range list {
		nick := ""
		if s.cfg.Store != nil {
			if u, _ := s.cfg.Store.FindByUserID(p.UserID); u != nil {
				nick = u.Nickname
			}
		}
		out = append(out, map[string]any{
			"userId": p.UserID, "nickname": nick, "mapId": p.MapID,
			"remote": p.Remote, "inBattle": p.InBattle,
		})
	}
	writeJSON(w, map[string]any{"ok": true, "players": out, "count": len(out)})
}

func (s *Server) handleKick(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, 405, "POST only")
		return
	}
	var req struct {
		UserID int64  `json:"userId"`
		Admin  string `json:"admin"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if req.UserID <= 0 {
		writeErr(w, 400, "需要 userId")
		return
	}
	ok := false
	if s.cfg.Notify != nil {
		ok = s.cfg.Notify.KickUser(req.UserID)
	}
	admin := actor(req.Admin)
	if u := r.Header.Get("X-GM-User"); u != "" {
		admin = u
	}
	aid := int64(0)
	if s.cfg.Store != nil {
		aid, _ = s.cfg.Store.InsertGMAudit(admin, "kick", req.UserID, map[string]any{"ok": ok})
	}
	writeJSON(w, map[string]any{"ok": true, "kicked": ok, "auditId": aid})
}
