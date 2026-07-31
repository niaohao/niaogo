package gameserver

import (
	"encoding/binary"
	"encoding/json"
	"log"
	"math/rand"
	"os"
	"strconv"
	"sync"
	"time"
)

// 野生精灵定时刷新（对照参考服 OgreEnterMapDelay / OgreFightEndDelay / OgreRefreshInterval）
const (
	ogreEnterMapDelay   = 5 * time.Second  // 进图后多久允许首次刷新
	ogreFightEndDelay   = 5 * time.Second  // 战毕后多久允许刷新
	ogreRefreshInterval = 15 * time.Second // 两次刷新间隔
	ogreTickInterval    = 1 * time.Second
)

// OgreSlot 本客户端 2004 仅 petID（无 shiny 字段），每图最多展示 4 只（压缩到槽 0–3）。
type OgreSlot struct {
	PetID    int
	Level    int
	CanCatch bool
}

// ogreRefreshState 单玩家刷新时间状态。
type ogreRefreshState struct {
	EnterMapTime     time.Time
	LastFightEndTime time.Time // zero = 无战毕限制
	LastRefreshTime  time.Time // zero = 尚未刷新过（等进图延迟）
	MapID            int
	RefreshEpoch     uint64
}

type ogreHub struct {
	mu     sync.Mutex
	m      map[int64]map[int][]OgreSlot // uid -> mapID -> slots
	state  map[int64]*ogreRefreshState
	stopCh chan struct{}
}

func (h *ogreHub) get(uid int64, mapID int) []OgreSlot {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.m == nil {
		return nil
	}
	return h.m[uid][mapID]
}

func (h *ogreHub) set(uid int64, mapID int, slots []OgreSlot) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.m == nil {
		h.m = make(map[int64]map[int][]OgreSlot)
	}
	if h.m[uid] == nil {
		h.m[uid] = make(map[int][]OgreSlot)
	}
	h.m[uid][mapID] = slots
}

func (h *ogreHub) clear(uid int64, mapID int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.m != nil && h.m[uid] != nil {
		delete(h.m[uid], mapID)
	}
}

func (h *ogreHub) setEnterMap(uid int64, mapID int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.state == nil {
		h.state = make(map[int64]*ogreRefreshState)
	}
	st := h.state[uid]
	if st == nil {
		st = &ogreRefreshState{}
		h.state[uid] = st
	}
	st.MapID = mapID
	st.RefreshEpoch++
	st.EnterMapTime = time.Now()
	st.LastRefreshTime = time.Time{}
	st.LastFightEndTime = time.Time{}
}

func (h *ogreHub) setFightEnd(uid int64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.state == nil {
		h.state = make(map[int64]*ogreRefreshState)
	}
	st := h.state[uid]
	if st == nil {
		st = &ogreRefreshState{}
		h.state[uid] = st
	}
	st.LastFightEndTime = time.Now()
}

func (h *ogreHub) setLastRefresh(uid int64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.state == nil {
		h.state = make(map[int64]*ogreRefreshState)
	}
	st := h.state[uid]
	if st == nil {
		st = &ogreRefreshState{}
		h.state[uid] = st
	}
	st.LastRefreshTime = time.Now()
}

func (h *ogreHub) snapshotState(uid int64) (st ogreRefreshState, ok bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	p := h.state[uid]
	if p == nil {
		return st, false
	}
	return *p, true
}

type wildEntry struct {
	PetID, LvMin, LvMax int
	CanCatch            bool
}

type wildPool struct {
	Common []wildEntry
	Rare   []wildEntry
}

// rareEmptySlots 稀有槽内空数 N；P(稀有)=(1/3)×1/(1+N)。默认 20≈1.6%。
var rareEmptySlots = 20

// mapWildPool 由 map_wild_config.json 覆盖加载（参考 mapogres 全图池）。
var mapWildPool = map[int]wildPool{}

func mapHasOgreConfig(mapID int) bool {
	p, ok := mapWildPool[mapID]
	return ok && (len(p.Common) > 0 || len(p.Rare) > 0)
}

func loadMapWildConfig(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var raw struct {
		ShinyRate      int `json:"shinyRate"`
		RareEmptySlots int `json:"rareEmptySlots"`
		Maps           map[string]struct {
			Common []struct {
				PetID    int   `json:"petId"`
				LevelMin int   `json:"levelMin"`
				LevelMax int   `json:"levelMax"`
				CanCatch *bool `json:"canCatch"`
			} `json:"common"`
			Rare []struct {
				PetID    int   `json:"petId"`
				LevelMin int   `json:"levelMin"`
				LevelMax int   `json:"levelMax"`
				CanCatch *bool `json:"canCatch"`
			} `json:"rare"`
		} `json:"maps"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if raw.RareEmptySlots >= 0 {
		rareEmptySlots = raw.RareEmptySlots
	}
	toEntry := func(list []struct {
		PetID    int   `json:"petId"`
		LevelMin int   `json:"levelMin"`
		LevelMax int   `json:"levelMax"`
		CanCatch *bool `json:"canCatch"`
	}) []wildEntry {
		out := make([]wildEntry, 0, len(list))
		for _, e := range list {
			if e.PetID <= 0 {
				continue
			}
			cc := true
			if e.CanCatch != nil {
				cc = *e.CanCatch
			} else {
				cc = wildPetCatchableDefault(e.PetID)
			}
			lvMin, lvMax := e.LevelMin, e.LevelMax
			if lvMin <= 0 {
				lvMin = 5
			}
			if lvMax < lvMin {
				lvMax = lvMin
			}
			out = append(out, wildEntry{e.PetID, lvMin, lvMax, cc})
		}
		return out
	}
	next := make(map[int]wildPool, len(raw.Maps))
	for idStr, m := range raw.Maps {
		mapID, err := strconv.Atoi(idStr)
		if err != nil || mapID <= 0 {
			continue
		}
		pool := wildPool{
			Common: toEntry(m.Common),
			Rare:   toEntry(m.Rare),
		}
		if len(pool.Common) == 0 && len(pool.Rare) == 0 {
			continue
		}
		next[mapID] = pool
	}
	if len(next) == 0 {
		return nil
	}
	mapWildPool = next
	log.Printf("[ogre] map_wild_config loaded maps=%d rareEmptySlots=%d from %s", len(mapWildPool), rareEmptySlots, path)
	return nil
}

// wildPetCatchableDefault 参考：进化形态（EvolvesFrom>0）默认不可捕。
func wildPetCatchableDefault(petID int) bool {
	if d := petBaseFromCatalog(petID); d != nil && d.EvolvesFrom > 0 {
		return false
	}
	return true
}

func pickWildEntry(list []wildEntry) OgreSlot {
	if len(list) == 0 {
		return OgreSlot{}
	}
	e := list[rand.Intn(len(list))]
	lv := e.LvMin
	if e.LvMax > e.LvMin {
		lv = e.LvMin + rand.Intn(e.LvMax-e.LvMin+1)
	}
	if lv < 1 {
		lv = 5
	}
	cc := e.CanCatch
	if d := petBaseFromCatalog(e.PetID); d != nil && d.EvolvesFrom > 0 {
		cc = false
	}
	return OgreSlot{PetID: e.PetID, Level: lv, CanCatch: cc}
}

// generateOgreSlots 对齐参考两阶段：9 槽抽 3（含 1 稀有槽），稀有槽再 1/(1+N) 出稀有；结果放入 9 格中最多 4 格展示位。
func generateOgreSlots(mapID int) []OgreSlot {
	pool, ok := mapWildPool[mapID]
	if !ok || (len(pool.Common) == 0 && len(pool.Rare) == 0) {
		return make([]OgreSlot, 9)
	}
	slots := make([]OgreSlot, 9)

	idxs := []int{0, 1, 2, 3, 4, 5, 6, 7, 8}
	rand.Shuffle(len(idxs), func(i, j int) { idxs[i], idxs[j] = idxs[j], idxs[i] })

	n := rareEmptySlots
	if n < 0 {
		n = 0
	}
	denom := 1 + n
	drawn := make([]OgreSlot, 0, 3)
	rareOnly := len(pool.Common) == 0 && len(pool.Rare) > 0
	for i := 0; i < 3; i++ {
		isRareSlot := idxs[i] == 0
		if isRareSlot && len(pool.Rare) > 0 {
			// 仅稀有池：稀有槽必出，避免 1/(1+20) 饿死整图
			if rareOnly || rand.Intn(denom) == 0 {
				drawn = append(drawn, pickWildEntry(pool.Rare))
			} else {
				drawn = append(drawn, OgreSlot{})
			}
			continue
		}
		if len(pool.Common) > 0 {
			drawn = append(drawn, pickWildEntry(pool.Common))
		} else if len(pool.Rare) > 0 {
			drawn = append(drawn, pickWildEntry(pool.Rare))
		} else {
			drawn = append(drawn, OgreSlot{})
		}
	}

	place := []int{0, 1, 2, 3}
	rand.Shuffle(len(place), func(i, j int) { place[i], place[j] = place[j], place[i] })
	for i, s := range drawn {
		if i >= len(place) {
			break
		}
		if s.PetID > 0 {
			slots[place[i]] = s
		}
	}
	return slots
}

// buildMapOgreList 本客户端：9 × u32 petID = 36 字节（无 shiny）。
func buildMapOgreList(slots []OgreSlot) []byte {
	body := make([]byte, 36)
	compact := make([]OgreSlot, 0, 4)
	for i := 0; i < len(slots) && i < 9; i++ {
		if slots[i].PetID > 0 {
			compact = append(compact, slots[i])
			if len(compact) >= 4 {
				break
			}
		}
	}
	for i := 0; i < len(compact) && i < 9; i++ {
		binary.BigEndian.PutUint32(body[i*4:(i+1)*4], uint32(compact[i].PetID))
	}
	return body
}

func emptyMapOgreList() []byte {
	return make([]byte, 36)
}

func compactOgreSlots(slots []OgreSlot) []OgreSlot {
	out := make([]OgreSlot, 0, 4)
	for i := 0; i < len(slots) && i < 9; i++ {
		if slots[i].PetID > 0 {
			out = append(out, slots[i])
			if len(out) >= 4 {
				break
			}
		}
	}
	return out
}

// getOgreSlots 只读当前槽位，不自动生成（进图空窗期 / 战后清空期由定时器填充）。
func (s *Server) getOgreSlots(uid int64, mapID int) []OgreSlot {
	return s.ogres.get(uid, mapID)
}

func (s *Server) pushMapOgreList(c *Client) {
	if c == nil || !c.LoggedIn {
		return
	}
	slots := s.getOgreSlots(c.UserID, c.MapID)
	body := buildMapOgreList(slots)
	s.send(c, 2004, uint32(c.UserID), 0, body)
	n := 0
	for _, sl := range compactOgreSlots(slots) {
		if sl.PetID > 0 {
			n++
		}
	}
	log.Printf("[ogre] uid=%d map=%d wild=%d", c.UserID, c.MapID, n)
}

// onEnterMapOgres 进图：登记时间、清槽、先推空 2004；真实精灵由定时器 ≥5s 后首次推送。
func (s *Server) onEnterMapOgres(c *Client, mapID int) {
	if c == nil || c.UserID == 0 || mapID <= 0 {
		return
	}
	s.ogres.clear(c.UserID, mapID)
	s.ogres.setEnterMap(c.UserID, mapID)
	s.send(c, 2004, uint32(c.UserID), 0, emptyMapOgreList())
	log.Printf("[ogre] enter empty 2004 uid=%d map=%d (first spawn after %v)", c.UserID, mapID, ogreEnterMapDelay)
}

// refreshMapOgresAfterFight 战后：记战毕时间、清槽、推空 2004；≥5s 后由定时器刷新（并受 15s 间隔限制）。
func (s *Server) refreshMapOgresAfterFight(c *Client, uid uint32, mapID int) {
	if mapID <= 0 {
		return
	}
	s.ogres.setFightEnd(int64(uid))
	s.ogres.clear(int64(uid), mapID)
	if c == nil || !c.LoggedIn {
		return
	}
	s.send(c, 2004, uid, 0, emptyMapOgreList())
	log.Printf("[ogre] fight-end empty 2004 uid=%d map=%d (respawn after %v)", uid, mapID, ogreFightEndDelay)
}

func (s *Server) startOgreRefreshLoop() {
	if s.ogres.stopCh == nil {
		s.ogres.stopCh = make(chan struct{})
	}
	go s.runOgreRefreshLoop()
}

func (s *Server) runOgreRefreshLoop() {
	ticker := time.NewTicker(ogreTickInterval)
	defer ticker.Stop()
	for {
		select {
		case <-s.ogres.stopCh:
			return
		case <-ticker.C:
			s.tickOgreRefresh()
		}
	}
}

func (s *Server) ogreRefreshStillValid(uid int64, c *Client, epoch uint64, mapID int) bool {
	if c == nil || !c.LoggedIn || c.MapID != mapID || mapID <= 0 {
		return false
	}
	st, ok := s.ogres.snapshotState(uid)
	if !ok || st.RefreshEpoch != epoch {
		return false
	}
	if st.MapID > 0 && st.MapID != mapID {
		return false
	}
	return true
}

// tickOgreRefresh 每秒检查：进图≥5s、非战斗、战毕≥5s、距上次刷新≥15s → 空 2004 再推新一批。
func (s *Server) tickOgreRefresh() {
	s.mu.Lock()
	clients := make([]*Client, 0, len(s.byUID))
	for _, c := range s.byUID {
		if c != nil && c.LoggedIn && c.UserID > 0 {
			clients = append(clients, c)
		}
	}
	s.mu.Unlock()

	now := time.Now()
	for _, c := range clients {
		uid := c.UserID
		mapID := c.MapID
		if mapID <= 0 || !mapHasOgreConfig(mapID) {
			continue
		}
		if st := s.battles.get(uid); st != nil && st.Active {
			continue
		}

		state, ok := s.ogres.snapshotState(uid)
		if !ok {
			s.ogres.setEnterMap(uid, mapID)
			continue
		}
		if state.MapID > 0 && state.MapID != mapID {
			continue
		}
		if !state.LastFightEndTime.IsZero() && now.Sub(state.LastFightEndTime) < ogreFightEndDelay {
			continue
		}
		if now.Sub(state.EnterMapTime) < ogreEnterMapDelay {
			continue
		}
		if !state.LastRefreshTime.IsZero() && now.Sub(state.LastRefreshTime) < ogreRefreshInterval {
			continue
		}

		epoch := state.RefreshEpoch
		refreshMapID := mapID
		if state.MapID > 0 {
			refreshMapID = state.MapID
		}
		if !s.ogreRefreshStillValid(uid, c, epoch, refreshMapID) {
			continue
		}

		// 先空包再实包，客户端看到整批消失再出现
		s.send(c, 2004, uint32(uid), 0, emptyMapOgreList())
		if !s.ogreRefreshStillValid(uid, c, epoch, refreshMapID) {
			continue
		}
		newSlots := generateOgreSlots(refreshMapID)
		s.ogres.set(uid, refreshMapID, newSlots)
		if !s.ogreRefreshStillValid(uid, c, epoch, refreshMapID) {
			continue
		}
		s.send(c, 2004, uint32(uid), 0, buildMapOgreList(newSlots))
		s.ogres.setLastRefresh(uid)
		n := len(compactOgreSlots(newSlots))
		log.Printf("[ogre] refresh uid=%d map=%d wild=%d", uid, refreshMapID, n)
	}
}
