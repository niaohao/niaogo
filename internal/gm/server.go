package gm

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"

	"niaohao/server/internal/defaults"
	"niaohao/server/internal/store"
	"niaohao/server/internal/tableloader"
)

// OnlinePlayer GM 在线列表项（与 gameserver 字段对齐）。
type OnlinePlayer struct {
	UserID   int64  `json:"userId"`
	MapID    int    `json:"mapId"`
	Remote   string `json:"remote"`
	InBattle bool   `json:"inBattle"`
}

// Notifier 在线推送 / 踢人（由 gameserver 经 NotifyFuncs 适配）；可为 nil。
type Notifier interface {
	PushItemGain(uid int64, itemID, count int)
	PushPetGain(uid int64, petID int, catchTime int64)
	PushPetRefresh(uid, catchTime int64) // 改宠后推 2508+2301
	PushCurrencyBalance(uid int64)
	ListOnlinePlayers() []OnlinePlayer
	KickUser(uid int64) bool
}

type Config struct {
	Catalog    *tableloader.Catalog
	Store      *store.MySQL
	Notify     Notifier
	StaticDir  string
	ConfigDir  string // gm_auth.json
	// PanelCalc 当前面板 + 公式正常值（由 gameserver.PetPanelSnapshot 注入）
	PanelCalc func(p *store.Pet) (current, natural [6]int, gmLocked bool)
}

type Server struct {
	cfg  Config
	auth *authState
	Mux  *http.ServeMux
}

func New(cfg Config) *Server {
	s := &Server{cfg: cfg, auth: loadAuth(cfg.ConfigDir), Mux: http.NewServeMux()}
	s.Mux.HandleFunc("/api/health", s.handleHealth)
	s.Mux.HandleFunc("/api/login", s.handleLogin)
	s.Mux.HandleFunc("/api/logout", s.handleLogout)
	s.Mux.HandleFunc("/api/me", s.handleMe)

	authed := func(h http.HandlerFunc) http.HandlerFunc { return s.requireAuth(h) }
	s.Mux.HandleFunc("/api/tables/stats", authed(s.handleTableStats))
	s.Mux.HandleFunc("/api/item/label", authed(s.handleItemLabel))
	s.Mux.HandleFunc("/api/pet/label", authed(s.handlePetLabel))
	s.Mux.HandleFunc("/api/catalog/items", authed(s.handleSearchItems))
	s.Mux.HandleFunc("/api/catalog/pets", authed(s.handleSearchPets))
	s.Mux.HandleFunc("/api/users", authed(s.handleSearchUsers))
	s.Mux.HandleFunc("/api/user/detail", authed(s.handleUserDetail))
	s.Mux.HandleFunc("/api/online", authed(s.handleOnline))
	s.Mux.HandleFunc("/api/kick", authed(s.handleKick))
	s.Mux.HandleFunc("/api/grant/currency", authed(s.handleGrantCurrency))
	s.Mux.HandleFunc("/api/grant/item", authed(s.handleGrantItem))
	s.Mux.HandleFunc("/api/grant/pet", authed(s.handleGrantPet))
	s.Mux.HandleFunc("/api/grant/pets-all", authed(s.handleGrantPetsAll))
	s.Mux.HandleFunc("/api/grant/kit", authed(s.handleGrantKit))
	s.Mux.HandleFunc("/api/top-level", authed(s.handleTopLevelGet))
	s.Mux.HandleFunc("/api/top-level/set", authed(s.handleTopLevelSet))
	s.Mux.HandleFunc("/api/top-level/add", authed(s.handleTopLevelAdd))
	s.Mux.HandleFunc("/api/pet/update", authed(s.handlePetUpdate))
	s.Mux.HandleFunc("/api/pet/stats", authed(s.handlePetStats))
	s.Mux.HandleFunc("/api/pet/stats/restore", authed(s.handlePetStatsRestore))
	s.Mux.HandleFunc("/api/audit", authed(s.handleAuditList))
	fs := http.FileServer(http.Dir(cfg.StaticDir))
	s.Mux.Handle("/", fs)
	return s
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	backend := ""
	if s.cfg.Store != nil {
		backend = s.cfg.Store.Backend()
	}
	writeJSON(w, map[string]any{
		"ok": true, "service": "nieo-gm", "port": defaults.GMHTTP,
		"store": s.cfg.Store != nil, "backend": backend,
	})
}

func (s *Server) handleTableStats(w http.ResponseWriter, _ *http.Request) {
	items, pets, loaded := 0, 0, false
	if s.cfg.Catalog != nil {
		items, pets, loaded = s.cfg.Catalog.Stats()
	}
	writeJSON(w, map[string]any{"loaded": loaded, "item_count": items, "pet_count": pets})
}

func (s *Server) handleItemLabel(w http.ResponseWriter, r *http.Request) {
	id := queryInt(r, "id")
	label := "未知(" + strconv.Itoa(id) + ")"
	if s.cfg.Catalog != nil {
		label = s.cfg.Catalog.ItemLabel(id)
	}
	writeJSON(w, map[string]any{"id": id, "label": label})
}

func (s *Server) handlePetLabel(w http.ResponseWriter, r *http.Request) {
	id := queryInt(r, "id")
	label := "未知(" + strconv.Itoa(id) + ")"
	if s.cfg.Catalog != nil {
		label = s.cfg.Catalog.PetLabel(id)
	}
	writeJSON(w, map[string]any{"id": id, "label": label})
}

func (s *Server) handleSearchItems(w http.ResponseWriter, r *http.Request) {
	limit := queryInt(r, "limit")
	if limit <= 0 || limit > 200 {
		limit = 80
	}
	hits := []tableloader.SearchHit{}
	if s.cfg.Catalog != nil {
		hits = s.cfg.Catalog.SearchItems(r.URL.Query().Get("q"), limit)
	}
	writeJSON(w, map[string]any{"ok": true, "hits": hits, "list": hits})
}

func (s *Server) handleSearchPets(w http.ResponseWriter, r *http.Request) {
	limit := queryInt(r, "limit")
	if limit <= 0 || limit > 200 {
		limit = 80
	}
	hits := []tableloader.SearchHit{}
	if s.cfg.Catalog != nil {
		hits = s.cfg.Catalog.SearchPets(r.URL.Query().Get("q"), limit)
	}
	writeJSON(w, map[string]any{"ok": true, "hits": hits, "list": hits})
}

func (s *Server) handleSearchUsers(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Store == nil {
		writeErr(w, 500, "store 未就绪")
		return
	}
	list, err := s.cfg.Store.SearchUsers(r.URL.Query().Get("q"), 50)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, map[string]any{"ok": true, "users": list})
}

func (s *Server) handleAuditList(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Store == nil {
		writeErr(w, 500, "store 未就绪")
		return
	}
	list, err := s.cfg.Store.ListGMAudit(50)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, map[string]any{"ok": true, "rows": list})
}

type currencyReq struct {
	UserID  int64  `json:"userId"`
	Coins   int    `json:"coins"`
	Gold    int    `json:"gold"`
	ExpPool int    `json:"expPool"` // 积累经验（虚拟道具 ID=3）→ users.exp_pool
	Admin   string `json:"admin"`
}

func (s *Server) handleGrantCurrency(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, 405, "POST only")
		return
	}
	var req currencyReq
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if s.cfg.Store == nil || req.UserID <= 0 {
		writeErr(w, 400, "需要 userId 且 Store 就绪")
		return
	}
	if req.Coins == 0 && req.Gold == 0 && req.ExpPool == 0 {
		writeErr(w, 400, "coins/gold/expPool 增量不能都为 0")
		return
	}
	u, err := s.cfg.Store.FindByUserID(req.UserID)
	if err != nil || u == nil {
		writeErr(w, 404, "用户不存在")
		return
	}
	admin := actorOrHeader(r, req.Admin)
	if req.Coins != 0 {
		if err := s.cfg.Store.AddCoins(req.UserID, req.Coins); err != nil {
			writeErr(w, 500, "AddCoins: "+err.Error())
			return
		}
		if s.cfg.Notify != nil && req.Coins > 0 {
			s.cfg.Notify.PushItemGain(req.UserID, 1, req.Coins)
		}
	}
	goldBal := u.Gold
	if req.Gold != 0 {
		goldBal, err = s.cfg.Store.AddGoldWithLedger(req.UserID, req.Gold, "gm_grant", admin)
		if err != nil {
			writeErr(w, 500, "AddGold: "+err.Error())
			return
		}
		if s.cfg.Notify != nil && req.Gold > 0 {
			s.cfg.Notify.PushItemGain(req.UserID, 5, req.Gold)
		}
	}
	expBal := 0
	if req.ExpPool != 0 {
		expBal, err = s.cfg.Store.AddExpPool(req.UserID, req.ExpPool)
		if err != nil {
			writeErr(w, 500, "AddExpPool: "+err.Error())
			return
		}
		if s.cfg.Notify != nil && req.ExpPool > 0 {
			s.cfg.Notify.PushItemGain(req.UserID, 3, req.ExpPool)
		}
	} else {
		expBal, _ = s.cfg.Store.GetExpPool(req.UserID)
	}
	if s.cfg.Notify != nil {
		s.cfg.Notify.PushCurrencyBalance(req.UserID)
	}
	aid, _ := s.cfg.Store.InsertGMAudit(admin, "grant_currency", req.UserID, map[string]any{
		"coins": req.Coins, "gold": req.Gold, "expPool": req.ExpPool,
		"goldBalance": goldBal, "expPoolBalance": expBal,
	})
	u2, _ := s.cfg.Store.FindByUserID(req.UserID)
	coins := 0
	if u2 != nil {
		coins = u2.Coins
	}
	writeJSON(w, map[string]any{"ok": true, "auditId": aid, "coins": coins, "gold": goldBal, "expPool": expBal})
}

type itemReq struct {
	UserID int64  `json:"userId"`
	ItemID int    `json:"itemId"`
	Count  int    `json:"count"`
	Admin  string `json:"admin"`
}

func (s *Server) handleGrantItem(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, 405, "POST only")
		return
	}
	var req itemReq
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if s.cfg.Store == nil || req.UserID <= 0 || req.ItemID <= 0 {
		writeErr(w, 400, "需要 userId/itemId")
		return
	}
	if req.Count == 0 {
		req.Count = 1
	}
	u, err := s.cfg.Store.FindByUserID(req.UserID)
	if err != nil || u == nil {
		writeErr(w, 404, "用户不存在")
		return
	}
	admin := actorOrHeader(r, req.Admin)
	// 虚拟货币（非背包）：1=赛尔豆、3=积累经验→exp_pool、5=金豆
	switch req.ItemID {
	case 1:
		if err := s.cfg.Store.AddCoins(req.UserID, req.Count); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		if s.cfg.Notify != nil {
			if req.Count > 0 {
				s.cfg.Notify.PushItemGain(req.UserID, 1, req.Count)
			}
			s.cfg.Notify.PushCurrencyBalance(req.UserID)
		}
		label := s.itemLabel(1)
		aid, _ := s.cfg.Store.InsertGMAudit(admin, "grant_item", req.UserID, map[string]any{
			"itemId": 1, "label": label, "count": req.Count,
		})
		u2, _ := s.cfg.Store.FindByUserID(req.UserID)
		coins := 0
		if u2 != nil {
			coins = u2.Coins
		}
		writeJSON(w, map[string]any{"ok": true, "auditId": aid, "label": label, "coins": coins})
		return
	case 3:
		bal, err := s.cfg.Store.AddExpPool(req.UserID, req.Count)
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		if s.cfg.Notify != nil && req.Count > 0 {
			s.cfg.Notify.PushItemGain(req.UserID, 3, req.Count)
		}
		label := s.itemLabel(3)
		aid, _ := s.cfg.Store.InsertGMAudit(admin, "grant_item", req.UserID, map[string]any{
			"itemId": 3, "label": label, "count": req.Count, "expPoolBalance": bal,
		})
		writeJSON(w, map[string]any{"ok": true, "auditId": aid, "label": label, "expPool": bal})
		return
	case 5:
		bal, err := s.cfg.Store.AddGoldWithLedger(req.UserID, req.Count, "gm_grant_item", admin)
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		if s.cfg.Notify != nil {
			if req.Count > 0 {
				s.cfg.Notify.PushItemGain(req.UserID, 5, req.Count)
			}
			s.cfg.Notify.PushCurrencyBalance(req.UserID)
		}
		label := s.itemLabel(5)
		aid, _ := s.cfg.Store.InsertGMAudit(admin, "grant_item", req.UserID, map[string]any{
			"itemId": 5, "label": label, "count": req.Count, "goldBalance": bal,
		})
		writeJSON(w, map[string]any{"ok": true, "auditId": aid, "label": label, "gold": bal})
		return
	}
	if err := s.cfg.Store.AddItem(req.UserID, req.ItemID, req.Count); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if s.cfg.Notify != nil && req.Count > 0 {
		s.cfg.Notify.PushItemGain(req.UserID, req.ItemID, req.Count)
	}
	label := s.itemLabel(req.ItemID)
	aid, _ := s.cfg.Store.InsertGMAudit(admin, "grant_item", req.UserID, map[string]any{
		"itemId": req.ItemID, "label": label, "count": req.Count,
	})
	writeJSON(w, map[string]any{"ok": true, "auditId": aid, "label": label, "count": req.Count})
}

type petReq struct {
	UserID int64  `json:"userId"`
	PetID  int    `json:"petId"`
	Level  int    `json:"level"`
	DV     int    `json:"dv"`
	Nature int    `json:"nature"`
	Admin  string `json:"admin"`
}

func (s *Server) handleGrantPet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, 405, "POST only")
		return
	}
	var req petReq
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if s.cfg.Store == nil || req.UserID <= 0 || req.PetID <= 0 {
		writeErr(w, 400, "需要 userId/petId")
		return
	}
	u, err := s.cfg.Store.FindByUserID(req.UserID)
	if err != nil || u == nil {
		writeErr(w, 404, "用户不存在")
		return
	}
	name := "精灵"
	if s.cfg.Catalog != nil {
		if n := s.cfg.Catalog.PetNameOf(req.PetID); n != "" {
			name = n
		}
	}
	if req.Level <= 0 {
		req.Level = 1
	}
	if req.DV < 0 {
		req.DV = 0
	}
	skills := []int{10001}
	if s.cfg.Catalog != nil {
		skills = s.cfg.Catalog.DefaultSkillsAtLevel(req.PetID, req.Level)
	}
	catch, err := s.cfg.Store.GrantPet(req.UserID, req.PetID, name, req.Level, req.DV, req.Nature, skills)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if s.cfg.Notify != nil {
		s.cfg.Notify.PushPetGain(req.UserID, req.PetID, catch)
	}
	admin := actorOrHeader(r, req.Admin)
	label := s.petLabel(req.PetID)
	aid, _ := s.cfg.Store.InsertGMAudit(admin, "grant_pet", req.UserID, map[string]any{
		"petId": req.PetID, "label": label, "level": req.Level, "dv": req.DV, "catchTime": catch,
	})
	writeJSON(w, map[string]any{"ok": true, "auditId": aid, "label": label, "catchTime": catch, "level": req.Level})
}

func (s *Server) handleGrantKit(w http.ResponseWriter, r *http.Request) {
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
	if s.cfg.Store == nil || req.UserID <= 0 {
		writeErr(w, 400, "需要 userId")
		return
	}
	u, err := s.cfg.Store.FindByUserID(req.UserID)
	if err != nil || u == nil {
		writeErr(w, 404, "用户不存在")
		return
	}
	admin := actorOrHeader(r, req.Admin)
	summary := make([]string, 0, 8)
	_ = s.cfg.Store.AddCoins(req.UserID, 50000)
	summary = append(summary, "赛尔豆+50000")
	if _, err := s.cfg.Store.AddGoldWithLedger(req.UserID, 100, "gm_kit", admin); err == nil {
		summary = append(summary, "金豆+100")
	}
	for _, pair := range [][2]int{{300001, 20}, {300011, 20}, {300016, 10}} {
		_ = s.cfg.Store.AddItem(req.UserID, pair[0], pair[1])
		summary = append(summary, s.itemLabel(pair[0])+"×"+strconv.Itoa(pair[1]))
		if s.cfg.Notify != nil {
			s.cfg.Notify.PushItemGain(req.UserID, pair[0], pair[1])
		}
	}
	if s.cfg.Notify != nil {
		s.cfg.Notify.PushItemGain(req.UserID, 1, 50000)
		s.cfg.Notify.PushItemGain(req.UserID, 5, 100)
		s.cfg.Notify.PushCurrencyBalance(req.UserID)
	}
	aid, _ := s.cfg.Store.InsertGMAudit(admin, "grant_kit", req.UserID, map[string]any{"summary": summary})
	writeJSON(w, map[string]any{"ok": true, "auditId": aid, "summary": summary})
}

func (s *Server) itemLabel(id int) string {
	if s.cfg.Catalog != nil {
		return s.cfg.Catalog.ItemLabel(id)
	}
	return "物品(" + strconv.Itoa(id) + ")"
}

func (s *Server) petLabel(id int) string {
	if s.cfg.Catalog != nil {
		return s.cfg.Catalog.PetLabel(id)
	}
	return "精灵(" + strconv.Itoa(id) + ")"
}

func actor(a string) string {
	a = strings.TrimSpace(a)
	if a == "" {
		return "gm"
	}
	return a
}

func actorOrHeader(r *http.Request, fallback string) string {
	if u := r.Header.Get("X-GM-User"); u != "" {
		return u
	}
	return actor(fallback)
}

func decodeJSON(r *http.Request, dst any) error {
	defer r.Body.Close()
	b, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		return err
	}
	return json.Unmarshal(b, dst)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	w.WriteHeader(code)
	writeJSON(w, map[string]any{"ok": false, "error": msg})
}

func queryInt(r *http.Request, key string) int {
	n, _ := strconv.Atoi(r.URL.Query().Get(key))
	return n
}
