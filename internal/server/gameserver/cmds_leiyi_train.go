package gameserver

import (
	"encoding/binary"
	"log"
	"time"

	"niaohao/server/internal/cmdname"
	"niaohao/server/internal/store"
)

const (
	leiyiPetID              = 70
	leiyiTrainBossBase      = 10000
	leiyiTrainPhantomPetID  = 5005 // 倒影雷伊
	leiyiSkillAuroraBlade   = 10823
	leiyiSkillVitalOrb      = 10824
	leiyiSkillThunderFlash  = 10825
	leiyiSkillLightningAura = 20363
	leiyiSkillThunderAwake  = 20364
	skillBaiGuangRen        = 10170
	skillLeiYuTian          = 20086
	skillJiDianQianNiao     = 10175
)

// handleLeiyiTrainGetStatus CMD 2393：6×(today/current/total) u32 = 72B。
func (s *Server) handleLeiyiTrainGetStatus(c *Client, uid uint32, _ []byte) {
	st := store.LeiyiTrainProgress{}
	if s.cfg.Store != nil {
		st, _ = s.cfg.Store.GetLeiyiTrain(int64(uid))
	}
	st, changed := store.NormalizeLeiyiTrain(st, time.Now())
	if changed && s.cfg.Store != nil {
		_ = s.cfg.Store.SetLeiyiTrain(int64(uid), st)
	}
	body := make([]byte, 6*3*4)
	off := 0
	for i := 0; i < 6; i++ {
		binary.BigEndian.PutUint32(body[off:off+4], uint32(st.Today[i]))
		off += 4
		binary.BigEndian.PutUint32(body[off:off+4], uint32(st.Current[i]))
		off += 4
		binary.BigEndian.PutUint32(body[off:off+4], uint32(st.Total[i]))
		off += 4
	}
	s.send(c, 2393, uid, 0, body)
	log.Printf("[CMD] OK     %s UID=%d today=%v current=%v total=%v",
		cmdname.Format(2393), uid, st.Today, st.Current, st.Total)
}

func isLeiyiEnergyTrain(region uint32) bool {
	return region >= leiyiTrainBossBase && region <= leiyiTrainBossBase+6
}

func isLeiyiSkillTrainBoss(mapID int, region uint32, enemyID int) bool {
	if region != 1 {
		return false
	}
	switch {
	case mapID == 17 && enemyID == 42:
		return true
	case mapID == 27 && enemyID == 69:
		return true
	case mapID == 49 && enemyID == 113:
		return true
	case mapID == 40 && enemyID == 50:
		return true
	}
	return false
}

func isLeiyiTrainBattle(st *BattleState) bool {
	if st == nil {
		return false
	}
	if isLeiyiEnergyTrain(st.BossRegion) {
		return true
	}
	return isLeiyiSkillTrainBoss(st.MapID, st.BossRegion, st.EnemyID)
}

// resolveLeiyiTrainBoss 2411 param2=10000..10006；体能 0..5 → 雷伊幻影 Lv50；10006 → 倒影 5005 Lv100。
func resolveLeiyiTrainBoss(param2 uint32) (petID, level int, name string, ok bool) {
	if param2 < leiyiTrainBossBase || param2 > leiyiTrainBossBase+6 {
		return 0, 0, "", false
	}
	if param2 == leiyiTrainBossBase+6 {
		return leiyiTrainPhantomPetID, 100, "雷伊的幻影", true
	}
	return leiyiPetID, 50, "雷伊幻影", true
}

// applyLeiyiEnergyTrainOnWin 体能特训胜利：进度 + 面板 Bonus。
func (s *Server) applyLeiyiEnergyTrainOnWin(uid uint32, st *BattleState) {
	if s.cfg.Store == nil || st == nil || !isLeiyiEnergyTrain(st.BossRegion) || st.BossRegion == leiyiTrainBossBase+6 {
		return
	}
	idx := int(st.BossRegion - leiyiTrainBossBase)
	if idx < 0 || idx >= 6 {
		return
	}
	p, err := s.cfg.Store.GetPetByCatchTime(int64(uid), int64(st.PlayerCatchTime))
	if err != nil || p == nil || p.PetID != leiyiPetID {
		bag, _ := s.cfg.Store.ListBagPets(int64(uid))
		p = nil
		for i := range bag {
			if bag[i].PetID == leiyiPetID {
				p = &bag[i]
				break
			}
		}
	}
	if p == nil || p.PetID != leiyiPetID {
		log.Printf("[LeiyiTrain] skip energy: no雷伊 UID=%d idx=%d", uid, idx)
		return
	}

	lt, _ := s.cfg.Store.GetLeiyiTrain(int64(uid))
	lt, _ = store.NormalizeLeiyiTrain(lt, time.Now())
	if lt.Current[idx] >= lt.Total[idx] {
		return
	}
	step := 2
	if idx == 0 {
		step = 5
	}
	remain := lt.Total[idx] - lt.Current[idx]
	if remain <= 0 {
		return
	}
	gain := step
	if gain > remain {
		gain = remain
	}
	lt.Today[idx] += gain
	lt.Current[idx] += gain
	_ = s.cfg.Store.SetLeiyiTrain(int64(uid), lt)

	// 特训项：体力/防御/特防/攻击/特攻/速度 → Bonus[HP/Atk/Def/SA/SD/Spd]
	slotOf := [6]int{0, 2, 4, 1, 3, 5}
	slot := slotOf[idx]
	cap := lt.Total[idx]
	add := gain
	if room := cap - p.Bonus[slot]; add > room {
		add = room
	}
	if add < 0 {
		add = 0
	}
	if add > 0 {
		p.Bonus[slot] += add
		_ = s.cfg.Store.SetPetTrainBonus(int64(uid), p.CatchTime, p.Bonus)
	}
	log.Printf("[LeiyiTrain] energy idx=%d slot=%d +%d bonus+%d catch=%d current=%v",
		idx, slot, gain, add, p.CatchTime, lt.Current)
}

// noteLeiyiSkillTrainOnWin 技能特训胜利条件满足时推 2510，并写入技能银行。
func (s *Server) noteLeiyiSkillTrainOnWin(c *Client, uid uint32, st *BattleState) {
	if s.cfg.Store == nil || st == nil || c == nil {
		return
	}
	p, err := s.cfg.Store.GetPetByCatchTime(int64(uid), int64(st.PlayerCatchTime))
	if err != nil || p == nil || p.PetID != leiyiPetID {
		return
	}
	used := func(sid uint32) bool {
		return st.PlayerUsedSkills != nil && st.PlayerUsedSkills[sid]
	}
	reward := 0
	switch {
	case st.MapID == 17 && st.BossRegion == 1 && st.EnemyID == 42 && used(skillBaiGuangRen):
		reward = leiyiSkillAuroraBlade
	case st.MapID == 27 && st.BossRegion == 1 && st.EnemyID == 69 && used(skillLeiYuTian):
		reward = leiyiSkillLightningAura
	case st.MapID == 49 && st.BossRegion == 1 && st.EnemyID == 113 && used(skillJiDianQianNiao):
		reward = leiyiSkillVitalOrb
	case st.MapID == 40 && st.BossRegion == 1 && st.EnemyID == 50 && st.Round >= 10 && st.EnemyStatus.Para:
		reward = leiyiSkillThunderAwake
	case st.MapID == 32 && st.BossRegion == leiyiTrainBossBase+6 && st.EnemyID == leiyiTrainPhantomPetID:
		reward = leiyiSkillThunderFlash
	}
	if reward <= 0 {
		return
	}
	s.mergeLearnedSkillBank(p, reward)
	_ = s.cfg.Store.SetPetLearnedSkillBank(int64(uid), p.CatchTime, p.LearnedSkillBank)
	s.grantLeiyiSkillBankToAll(int64(uid), reward)
	s.pushLearnSpecialSkill2510(c, uid, uint32(p.CatchTime), uint32(reward))
}

func (s *Server) pushLearnSpecialSkill2510(c *Client, uid uint32, catchTime, skillID uint32) {
	body := make([]byte, 16)
	binary.BigEndian.PutUint32(body[0:4], 1)
	binary.BigEndian.PutUint32(body[4:8], catchTime)
	binary.BigEndian.PutUint32(body[8:12], 0) // active=0 → unactive 列表
	binary.BigEndian.PutUint32(body[12:16], skillID)
	s.send(c, 2510, uid, 0, body)
	log.Printf("[CMD] OK     %s UID=%d catch=%d skill=%d", cmdname.Format(2510), uid, catchTime, skillID)
}

func (s *Server) mergeLearnedSkillBank(p *store.Pet, skillID int) {
	if p == nil || skillID <= 1 {
		return
	}
	for _, id := range p.LearnedSkillBank {
		if id == skillID {
			return
		}
	}
	p.LearnedSkillBank = append(p.LearnedSkillBank, skillID)
}

func (s *Server) grantLeiyiSkillBankToAll(uid int64, skillID int) {
	if s.cfg.Store == nil || skillID <= 1 {
		return
	}
	apply := func(list []store.Pet) {
		for i := range list {
			if list[i].PetID != leiyiPetID {
				continue
			}
			p := list[i]
			s.mergeLearnedSkillBank(&p, skillID)
			_ = s.cfg.Store.SetPetLearnedSkillBank(uid, p.CatchTime, p.LearnedSkillBank)
		}
	}
	if bag, err := s.cfg.Store.ListBagPets(uid); err == nil {
		apply(bag)
	}
	if stor, err := s.cfg.Store.ListStoragePets(uid); err == nil {
		apply(stor)
	}
}

func leiyiTrainRewardSkillForTaskStep(taskID, param uint32) int {
	switch taskID {
	case 121:
		return leiyiSkillAuroraBlade
	case 122:
		switch param {
		case 0:
			return leiyiSkillLightningAura
		case 1:
			return leiyiSkillVitalOrb
		case 2:
			return leiyiSkillThunderAwake
		case 3:
			return leiyiSkillThunderFlash
		}
	}
	return 0
}

// onLeiyiTrainTaskComplete 2202 推进 121/122：写 buf、技能银行；122 非末步保持 accepted。
func (s *Server) onLeiyiTrainTaskComplete(uid int64, taskID, param uint32) (keepAccepted bool) {
	if s.cfg.Store == nil {
		return false
	}
	sid := leiyiTrainRewardSkillForTaskStep(taskID, param)
	if sid > 0 {
		s.grantLeiyiSkillBankToAll(uid, sid)
	}
	if taskID == 121 {
		// 完成后自动接 122
		s.setTaskStatus(uid, 122, taskStatusAccepted)
		return false
	}
	if taskID != 122 {
		return false
	}
	t, _ := s.cfg.Store.GetTask(uid, 122)
	buf := make([]byte, 20)
	if t != nil && len(t.Buf) > 0 {
		copy(buf, t.Buf)
	}
	if param < 20 {
		buf[param] = 1
	}
	_ = s.cfg.Store.UpsertTaskBuf(uid, 122, buf)
	if param < 3 {
		s.setTaskStatus(uid, 122, taskStatusAccepted)
		return true
	}
	return false
}
