package gameserver

import (
	"encoding/binary"
	"log"
	"sync"

	"niaohao/server/internal/cmdname"
)

// MAP_BOSS 2021：BossCmdListener → BossController.add(id,region,hp,pos)
// 仅 OgreXMLInfo 有 boss 配置且带防护罩的 SPT（本客户端目前 map12 蘑菇怪）需进图推送。
// ATTACK_BOSS 2412：射击破罩，包体 region(4)；应答当前罩血(4)；并推 2021 更新。

const (
	mapBossPosRemove     = 200
	mapBossShieldDefault = 100 // 满罩；每次约扣 25%
)

// shieldBossByMap：进图需推 2021 且 2412 破罩的 (mapID → region → petID)。
var shieldBossByMap = map[int]map[uint32]int{
	12: {0: 47}, // 克洛斯密林 · 蘑菇怪
}

type bossShieldHub struct {
	mu sync.Mutex
	// uid -> "map_region" -> currentHP（进图重置为满）
	m map[int64]map[string]int
}

func shieldKey(mapID int, region uint32) string {
	return itoaU32(uint32(mapID)) + "_" + itoaU32(region)
}

func (h *bossShieldHub) reset(uid int64, mapID int, region uint32, maxHP int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.m == nil {
		h.m = make(map[int64]map[string]int)
	}
	if h.m[uid] == nil {
		h.m[uid] = make(map[string]int)
	}
	if maxHP <= 0 {
		maxHP = mapBossShieldDefault
	}
	h.m[uid][shieldKey(mapID, region)] = maxHP
}

func (h *bossShieldHub) get(uid int64, mapID int, region uint32, maxHP int) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.m == nil || h.m[uid] == nil {
		return maxHP
	}
	if hp, ok := h.m[uid][shieldKey(mapID, region)]; ok {
		return hp
	}
	return maxHP
}

func (h *bossShieldHub) set(uid int64, mapID int, region uint32, hp int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.m == nil {
		h.m = make(map[int64]map[string]int)
	}
	if h.m[uid] == nil {
		h.m[uid] = make(map[string]int)
	}
	h.m[uid][shieldKey(mapID, region)] = hp
}

func buildMapBossEntry(petID int, region uint32, hp, pos uint32) []byte {
	b := make([]byte, 16)
	binary.BigEndian.PutUint32(b[0:4], uint32(petID))
	binary.BigEndian.PutUint32(b[4:8], region)
	binary.BigEndian.PutUint32(b[8:12], hp)
	binary.BigEndian.PutUint32(b[12:16], pos)
	return b
}

func buildMapBossListBody(entries [][]byte) []byte {
	out := make([]byte, 4)
	binary.BigEndian.PutUint32(out, uint32(len(entries)))
	for _, e := range entries {
		out = append(out, e...)
	}
	return out
}

// pushMapBossOnEnter 进图推 MAP_BOSS（防护罩满血 ×2，第二次触发 BossModel 走动）。
func (s *Server) pushMapBossOnEnter(c *Client, mapID int) {
	if c == nil || mapID <= 0 {
		return
	}
	byRegion, ok := shieldBossByMap[mapID]
	if !ok {
		return
	}
	uid := c.UserID
	for region, petID := range byRegion {
		maxHP := mapBossShieldDefault
		s.bossShield.reset(uid, mapID, region, maxHP)
		entry := buildMapBossEntry(petID, region, uint32(maxHP), 0)
		body := buildMapBossListBody([][]byte{entry})
		s.send(c, 2021, uint32(uid), 0, body)
		s.send(c, 2021, uint32(uid), 0, body)
		log.Printf("[CMD] OK     %s UID=%d map=%d pet=%d hp=%d (x2)", cmdname.Format(2021), uid, mapID, petID, maxHP)
	}
}

// handleAttackBoss CMD 2412：破防护罩；非罩 Boss 空回 0。
func (s *Server) handleAttackBoss(c *Client, uid uint32, body []byte) {
	region := uint32(0)
	if len(body) >= 4 {
		region = binary.BigEndian.Uint32(body[0:4])
	}
	mapID := 0
	if c != nil {
		mapID = c.MapID
	}
	out := make([]byte, 4)
	byRegion, ok := shieldBossByMap[mapID]
	if !ok {
		s.send(c, 2412, uid, 0, out)
		return
	}
	petID, ok := byRegion[region]
	if !ok || petID <= 0 {
		s.send(c, 2412, uid, 0, out)
		return
	}
	maxHP := mapBossShieldDefault
	cur := s.bossShield.get(int64(uid), mapID, region, maxHP)
	if cur <= 0 {
		cur = maxHP
	}
	dmg := maxHP / 4
	if dmg < 1 {
		dmg = 1
	}
	newHP := cur - dmg
	if newHP < 0 {
		newHP = 0
	}
	s.bossShield.set(int64(uid), mapID, region, newHP)
	binary.BigEndian.PutUint32(out, uint32(newHP))
	s.send(c, 2412, uid, 0, out)

	if newHP == 0 {
		// 先移除再重加 hp=0，客户端才去掉防护罩可开战
		rm := buildMapBossListBody([][]byte{buildMapBossEntry(petID, region, 0, mapBossPosRemove)})
		s.send(c, 2021, uid, 0, rm)
	}
	upd := buildMapBossListBody([][]byte{buildMapBossEntry(petID, region, uint32(newHP), 0)})
	s.send(c, 2021, uid, 0, upd)
	log.Printf("[CMD] OK     %s UID=%d map=%d region=%d hp %d->%d", cmdname.Format(2412), uid, mapID, region, cur, newHP)
}
