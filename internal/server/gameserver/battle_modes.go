package gameserver

import "sync"

// darkDoorInfo 暗黑武斗场：大门序号 + 子门（对照客户端 DarkDoorChoicePanel 槽位）。
type darkDoorInfo struct {
	DoorIndex uint32
	SubIndex  uint32
}

// modeHub 勇者之塔 / 试炼塔 / 暗黑道场会话态（不改 users 表）。
type modeHub struct {
	mu        sync.Mutex
	brave     map[int64]int // uid -> 当前选中层
	fresh     map[int64]int // uid -> 试炼塔层
	darkDoor  map[int64]darkDoorInfo
	braveBoss map[int64][]uint32 // uid -> 本层 Boss 列表
}

func (h *modeHub) setBrave(uid int64, level int, bosses []uint32) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.brave == nil {
		h.brave = make(map[int64]int)
		h.braveBoss = make(map[int64][]uint32)
	}
	h.brave[uid] = level
	h.braveBoss[uid] = append([]uint32(nil), bosses...)
}

func (h *modeHub) getBrave(uid int64) (level int, bosses []uint32) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.brave[uid], append([]uint32(nil), h.braveBoss[uid]...)
}

func (h *modeHub) clearBrave(uid int64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.brave, uid)
	delete(h.braveBoss, uid)
}

func (h *modeHub) setFresh(uid int64, level int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.fresh == nil {
		h.fresh = make(map[int64]int)
	}
	h.fresh[uid] = level
}

func (h *modeHub) getFresh(uid int64) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.fresh[uid]
}

func (h *modeHub) clearFresh(uid int64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.fresh, uid)
}

func (h *modeHub) setDarkDoor(uid int64, doorIndex, subIndex uint32) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.darkDoor == nil {
		h.darkDoor = make(map[int64]darkDoorInfo)
	}
	h.darkDoor[uid] = darkDoorInfo{DoorIndex: doorIndex, SubIndex: subIndex}
}

func (h *modeHub) getDarkDoor(uid int64) darkDoorInfo {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.darkDoor[uid]
}

func (h *modeHub) clearDarkDoor(uid int64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.darkDoor, uid)
}

const (
	braveTowerMaxLevel   = 100
	freshTowerMaxLevel   = 30
	darkPortalSlotStride = 6
	darkPortalKeyItemID  = 400053 // 暗黑之钥（无超能进第一门）
	darkPortalFallbackLv = 80
)

// 试炼之塔 1–30 层 Boss（对齐参考服常见低阶 ID）。
var freshTowerBossIDs = [freshTowerMaxLevel]uint32{
	10, 11, 12, 13, 14, 15, 16, 17, 18, 19,
	20, 21, 22, 23, 24, 25, 26, 27, 28, 29,
	30, 31, 32, 33, 34, 35, 36, 37, 38, 39,
}

// 本客户端 DarkPortalModel.enterMap：门 0–10 → 本地图 503–513（非参考服 10047+）。
var darkPortalDoorMapIDs = []int{503, 504, 505, 506, 507, 508, 509, 510, 511, 512, 513}

// 大门主 Boss（无子门表时兜底）。
var darkPortalDoorBoss = []uint32{171, 174, 177, 195, 222, 356, 438, 656, 779, 1182, 1403}

// braveTowerBosses 勇者之塔：每层 1 只；petID 在常见低阶区间轮换。
func braveTowerBosses(level int) []uint32 {
	if level < 1 {
		level = 1
	}
	id := uint32(10 + (level-1)%50)
	return []uint32{id}
}

func freshTowerBoss(level int) uint32 {
	if level < 1 {
		level = 1
	}
	if level > freshTowerMaxLevel {
		level = freshTowerMaxLevel
	}
	return freshTowerBossIDs[level-1]
}

// parseDarkPortalCurDoor 客户端 OPEN_DARKPORTAL 传的是槽位 ID：
// curDoor 0–5→第1门，6–11→第2门…；subIndex=curDoor%6。
func parseDarkPortalCurDoor(curDoor uint32) (doorIndex, subIndex uint32) {
	return curDoor / darkPortalSlotStride, curDoor % darkPortalSlotStride
}

func darkPortalMapID(doorIndex uint32) int {
	if int(doorIndex) >= len(darkPortalDoorMapIDs) {
		return darkPortalDoorMapIDs[0]
	}
	return darkPortalDoorMapIDs[doorIndex]
}

func darkPortalBoss(doorIndex uint32) uint32 {
	if int(doorIndex) >= len(darkPortalDoorBoss) {
		return darkPortalDoorBoss[0]
	}
	return darkPortalDoorBoss[doorIndex]
}

// darkPortalBossEntry 按大门+子门取 Boss；子门无配置时回退该门主 Boss。
func darkPortalBossEntry(doorIndex, subIndex uint32) (petID, level int) {
	mapID := darkPortalMapID(doorIndex)
	if byParam, ok := mapBossByParam[mapID]; ok {
		if e, ok := byParam[subIndex]; ok && e.PetID > 0 {
			lv := e.Level
			if lv <= 0 {
				lv = darkPortalFallbackLv
			}
			return e.PetID, lv
		}
		if e, ok := byParam[0]; ok && e.PetID > 0 {
			lv := e.Level
			if lv <= 0 {
				lv = darkPortalFallbackLv
			}
			return e.PetID, lv
		}
	}
	return int(darkPortalBoss(doorIndex)), darkPortalFallbackLv
}

// darkPortalRequiredSuperLevel 大门序号 0..N → 需超能等级 doorIndex+1。
func darkPortalRequiredSuperLevel(doorIndex uint32) int {
	need := int(doorIndex) + 1
	if need < 1 {
		need = 1
	}
	return need
}

func (s *Server) superNonoLevel(uid int64) int {
	if !s.hasActiveSuperNono(uid) {
		return 0
	}
	n, err := s.cfg.Store.GetOrInitNono(uid)
	if err != nil || n == nil {
		return 0
	}
	return n.SuperLevel
}

// darkPortalAccessOK 第2门起需超能等级；第1门可用超能或持有暗黑之钥。
func (s *Server) darkPortalAccessOK(uid int64, doorIndex uint32) (ok bool, needLv, haveLv int) {
	needLv = darkPortalRequiredSuperLevel(doorIndex)
	haveLv = s.superNonoLevel(uid)
	if doorIndex == 0 {
		if haveLv >= 1 {
			return true, needLv, haveLv
		}
		if s.cfg.Store != nil {
			n, _ := s.cfg.Store.GetItemCount(uid, darkPortalKeyItemID)
			if n > 0 {
				return true, needLv, haveLv
			}
		}
		return false, needLv, haveLv
	}
	return haveLv >= needLv, needLv, haveLv
}

// buildLevelBossBody：level(4)+count(4)+ids… 对齐 ChoiceLevelRequestInfo / SuccessFightRequestInfo。
func buildLevelBossBody(level uint32, bosses []uint32) []byte {
	if len(bosses) == 0 {
		bosses = []uint32{15}
	}
	out := make([]byte, 8+len(bosses)*4)
	putU32BE(out[0:4], level)
	putU32BE(out[4:8], uint32(len(bosses)))
	for i, id := range bosses {
		putU32BE(out[8+i*4:12+i*4], id)
	}
	return out
}

func putU32BE(b []byte, v uint32) {
	b[0] = byte(v >> 24)
	b[1] = byte(v >> 16)
	b[2] = byte(v >> 8)
	b[3] = byte(v)
}
