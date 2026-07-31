package gameserver

import (
	"encoding/binary"
	"log"
	"time"

	"niaohao/server/internal/cmdname"
)

// 盖亚的出现（SpecialPetActive / SPECIAL_PET_NOTE 2022）
// 按周几轮换地图：15 火山星 / 54 露西欧星 / 105 双子阿尔法星
// 点击 → FIGHT_SPECIAL_PET 2421 → 1v1 盖亚；按当日规则胜才发精元 400126（2202 taskID=99），否则 8010。

const (
	gaiyaAppearPetID       = 261
	gaiyaAppearLevel       = 70
	gaiyaAppearEssenceItem = 400126
	gaiyaAppearTaskID      = 99
	cmdSpecialPetNote      = 2022
	cmdSprintGiftNotice    = 8010
	gaiyaAppearLifetimeKey    = "gaiya_appear_essence"
)

// getGaiyaMapIDForToday 与前端 SpecialPetActive.ruleHashMap + 坐标表一致。
func getGaiyaMapIDForToday() int {
	return gaiyaMapForWeekday(int(time.Now().In(chinaTZ()).Weekday()))
}

func gaiyaMapForWeekday(weekday int) int {
	switch weekday {
	case 0, 2, 4: // 日/二/四：露西欧 — 致命一击
		return 54
	case 1, 5: // 一/五：火山星 — 2 回合内
		return 15
	case 3, 6: // 三/六：双子阿尔法 — 10 回合后再胜
		return 105
	default:
		return 54
	}
}

func chinaTZ() *time.Location {
	if loc, err := time.LoadLocation("Asia/Shanghai"); err == nil {
		return loc
	}
	return time.FixedZone("CST", 8*3600)
}

func (s *Server) pushGaiyaAppearNote(c *Client, uid uint32, mapID int) {
	if c == nil || mapID != getGaiyaMapIDForToday() {
		return
	}
	body := make([]byte, 8)
	binary.BigEndian.PutUint32(body[0:4], 1) // show
	binary.BigEndian.PutUint32(body[4:8], gaiyaAppearPetID)
	s.send(c, cmdSpecialPetNote, uid, 0, body)
	log.Printf("[CMD] OK     %s UID=%d map=%d show gaiya", cmdname.Format(cmdSpecialPetNote), uid, mapID)
}

// handleFightSpecialPet CMD 2421：盖亚的出现对战（无 body）；强制 1v1。
func (s *Server) handleFightSpecialPet(c *Client, uid uint32) {
	mapID := c.MapID
	if mapID != getGaiyaMapIDForToday() {
		log.Printf("[CMD] WARN  %s UID=%d map=%d not today gaiya map=%d",
			cmdname.Format(2421), uid, mapID, getGaiyaMapIDForToday())
		// 仍开战，避免点接受后无响应；胜负结算会判地图不符 → 8010
	}
	s.beginFightVsEnemy(c, uid, gaiyaAppearPetID, gaiyaAppearLevel, false, fightKindNormal)
	if st := s.battles.get(int64(uid)); st != nil {
		st.ForceSinglePet = true
		st.IsGaiyaAppear = true
		st.PlayerUsedSkills = make(map[uint32]bool)
		applyBossOpenBattleRules(st)
		s.battles.set(int64(uid), st)
	}
	log.Printf("[CMD] OK     %s UID=%d map=%d enemy=%d -> 2503",
		cmdname.Format(2421), uid, mapID, gaiyaAppearPetID)
}

func gaiyaAppearConditionOK(st *BattleState) bool {
	if st == nil {
		return false
	}
	need := getGaiyaMapIDForToday()
	if st.MapID != need {
		return false
	}
	switch need {
	case 15:
		return st.Round > 0 && st.Round <= 2
	case 54:
		return st.LastHitWasCrit
	case 105:
		return st.Round > 10
	default:
		return false
	}
}

func buildGaiyaAppearCompleteTaskBody() []byte {
	// NoviceFinishInfo: taskID+petID+captureTm+itemCount+[itemID+cnt]
	buf := make([]byte, 24)
	binary.BigEndian.PutUint32(buf[0:4], gaiyaAppearTaskID)
	binary.BigEndian.PutUint32(buf[4:8], 0)
	binary.BigEndian.PutUint32(buf[8:12], 0)
	binary.BigEndian.PutUint32(buf[12:16], 1)
	binary.BigEndian.PutUint32(buf[16:20], gaiyaAppearEssenceItem)
	binary.BigEndian.PutUint32(buf[20:24], 1)
	return buf
}

// onGaiyaAppearWin 盖亚出现战胜利：规则达成且未领过 → 发精元+2202；否则 8010。
func (s *Server) onGaiyaAppearWin(c *Client, uid uint32, st *BattleState) {
	if st == nil || !st.IsGaiyaAppear || st.EnemyID != gaiyaAppearPetID {
		return
	}
	if !gaiyaAppearConditionOK(st) {
		s.send(c, cmdSprintGiftNotice, uid, 0, nil)
		log.Printf("[盖亚出现] 规则未达成 UID=%d map=%d round=%d crit=%v -> 8010",
			uid, st.MapID, st.Round, st.LastHitWasCrit)
		s.pushGaiyaAppearNote(c, uid, st.MapID)
		return
	}
	if s.hasGaiyaAppearEssence(int64(uid)) {
		log.Printf("[盖亚出现] 已领精元 UID=%d 跳过", uid)
		s.pushGaiyaAppearNote(c, uid, st.MapID)
		return
	}
	if s.cfg.Store != nil {
		if err := s.cfg.Store.AddItem(int64(uid), gaiyaAppearEssenceItem, 1); err != nil {
			log.Printf("[盖亚出现] AddItem UID=%d err=%v", uid, err)
			s.send(c, cmdSprintGiftNotice, uid, 0, nil)
			return
		}
		_ = s.cfg.Store.MarkDefeatedSPT(int64(uid), gaiyaAppearPetID)
	}
	s.setLifetime(int64(uid), gaiyaAppearLifetimeKey, 1)
	s.send(c, 2202, uid, 0, buildGaiyaAppearCompleteTaskBody())
	log.Printf("[CMD] OK     %s UID=%d gaiya appear essence item=%d",
		cmdname.Format(2202), uid, gaiyaAppearEssenceItem)
	s.pushGaiyaAppearNote(c, uid, st.MapID)
}

func (s *Server) hasGaiyaAppearEssence(uid int64) bool {
	if s.lifetimeCount(uid, gaiyaAppearLifetimeKey) > 0 {
		return true
	}
	if s.cfg.Store == nil {
		return false
	}
	if ok, _ := s.cfg.Store.HasDefeatedSPT(uid, gaiyaAppearPetID); ok {
		return true
	}
	if n, _ := s.cfg.Store.GetItemCount(uid, gaiyaAppearEssenceItem); n >= 1 {
		return true
	}
	return false
}
