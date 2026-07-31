package gameserver

import (
	"encoding/binary"
	"log"

	"niaohao/server/internal/cmdname"
	"niaohao/server/internal/store"
	"niaohao/server/internal/tableloader"
)

// handlePetStudySkill CMD 2307：学习/替换技能（升级面板 MultiSkillPanel 确认后）。
// 请求 20B：catchTime + 1 + 1 + dropSkillId + studySkillId；应答 ret(4) 0=成功。
func (s *Server) handlePetStudySkill(c *Client, uid uint32, body []byte) {
	fail := func(why string) {
		out := make([]byte, 4)
		binary.BigEndian.PutUint32(out, 1)
		s.send(c, 2307, uid, 0, out)
		log.Printf("[CMD] WARN  %s UID=%d %s", cmdname.Format(2307), uid, why)
	}
	if len(body) < 20 {
		fail("body<20")
		return
	}
	if s.cfg.Store == nil {
		fail("no store")
		return
	}
	catch := int64(binary.BigEndian.Uint32(body[0:4]))
	oldSid := int(binary.BigEndian.Uint32(body[12:16]))
	newSid := int(binary.BigEndian.Uint32(body[16:20]))
	if newSid <= 1 {
		fail("bad newSid")
		return
	}
	p, err := s.cfg.Store.GetPetByCatchTime(int64(uid), catch)
	if err != nil || p == nil {
		fail("pet miss")
		return
	}
	if s.cfg.Catalog != nil && !s.cfg.Catalog.CanLearnMove(p.PetID, newSid) {
		inBank := false
		for _, sid := range p.LearnedSkillBank {
			if sid == newSid {
				inBank = true
				break
			}
		}
		// 表未覆盖时仍允许（兜底）；LearnedSkillBank 特训技一律放行
		if !inBank {
			if base := s.cfg.Catalog.PetBase(p.PetID); base != nil && len(base.LearnableMoves) > 0 {
				fail("cannot learn")
				return
			}
		}
	}
	current := normalizeSkillSlots(p.Skills)
	pre := append([]int(nil), current...)
	filled := 0
	for _, sid := range current {
		if sid > 1 {
			filled++
		}
	}
	target := -1
	if oldSid > 1 {
		for i, sid := range current {
			if sid == oldSid {
				target = i
				break
			}
		}
	}
	if target < 0 {
		for i, sid := range current {
			if sid == 0 {
				target = i
				break
			}
		}
	}
	if target < 0 {
		target = 0
	}
	if filled >= 4 {
		if oldSid <= 1 {
			fail("full need drop")
			return
		}
		ok := false
		for _, sid := range current {
			if sid == oldSid {
				ok = true
				break
			}
		}
		if !ok {
			fail("drop not owned")
			return
		}
	}
	if current[target] == newSid {
		out := make([]byte, 4)
		s.send(c, 2307, uid, 0, out)
		log.Printf("[CMD] OK     %s UID=%d catch=%d already slot=%d sid=%d",
			cmdname.Format(2307), uid, catch, target, newSid)
		return
	}
	for i := range current {
		if i != target && current[i] == newSid {
			current[i] = current[target]
			break
		}
	}
	current[target] = newSid
	p.Skills = current
	s.mergeLearnedSkillBank(p, newSid)
	if err := s.cfg.Store.UpsertPet(p); err != nil {
		fail("save: " + err.Error())
		return
	}
	_ = s.cfg.Store.SetPetLearnedSkillBank(int64(uid), p.CatchTime, p.LearnedSkillBank)
	out := make([]byte, 4)
	s.send(c, 2307, uid, 0, out)
	log.Printf("[CMD] OK     %s UID=%d catch=%d pet=%d slot=%d %v -> %v new=%d",
		cmdname.Format(2307), uid, catch, p.PetID, target, pre, current, newSid)
	// 刷新背包 PetInfo
	s.send(c, 2301, uid, 0, buildPetInfo(p))
}

// handlePetSkillSwitch CMD 2312：技能唤醒仪替换（背包 SkillReplacePanel）。
// 常见请求：
//
//	A) catchTime(4)+count(4)+[slot(4)+skillId(4)]*count；count=1 且 body=20 时 body[16:20] 为新技能
//	B) catchTime(4)+skillId×4
//
// 应答 ret(4)：0 成功；成功后再推 2301。
func (s *Server) handlePetSkillSwitch(c *Client, uid uint32, body []byte) {
	fail := func(why string) {
		out := make([]byte, 4)
		binary.BigEndian.PutUint32(out, 1)
		s.send(c, 2312, uid, 0, out)
		log.Printf("[CMD] WARN  %s UID=%d %s body=%d", cmdname.Format(2312), uid, why, len(body))
	}
	ok := func(p *store.Pet, detail string) {
		out := make([]byte, 4)
		s.send(c, 2312, uid, 0, out)
		sk := []int(nil)
		if p != nil {
			sk = p.Skills
			s.send(c, 2301, uid, 0, buildPetInfo(p))
		}
		log.Printf("[CMD] OK     %s UID=%d %s skills=%v", cmdname.Format(2312), uid, detail, sk)
	}
	if s.cfg.Store == nil || len(body) < 4 {
		fail("bad req")
		return
	}
	catch := int64(binary.BigEndian.Uint32(body[0:4]))
	p, err := s.cfg.Store.GetPetByCatchTime(int64(uid), catch)
	if err != nil || p == nil {
		fail("pet miss")
		return
	}
	current := normalizeSkillSlots(p.Skills)
	skillOK := func(sid int) bool {
		if sid <= 1 {
			return false
		}
		if s.cfg.Catalog == nil {
			return true
		}
		if s.cfg.Catalog.Skill(sid) != nil {
			return true
		}
		// 表缺技能定义时仍允许种族可学技
		return s.cfg.Catalog.CanLearnMove(p.PetID, sid)
	}

	final := append([]int(nil), current...)
	changed := false

	// 格式 A：catchTime + count + pairs
	if len(body) >= 12 {
		count := int(binary.BigEndian.Uint32(body[4:8]))
		if count >= 1 && count <= 4 && len(body) >= 8+count*8 {
			for i := 0; i < count; i++ {
				slot := int(binary.BigEndian.Uint32(body[8+8*i : 12+8*i]))
				sid := int(binary.BigEndian.Uint32(body[12+8*i : 16+8*i]))
				target := slot
				// 20B 单槽：body[16:20]=新技能；body[12:16]=旧技能（用于纠正槽位）
				if count == 1 && len(body) >= 20 && i == 0 {
					newSid := int(binary.BigEndian.Uint32(body[16:20]))
					oldSid := sid
					if skillOK(newSid) {
						sid = newSid
					}
					if oldSid > 1 {
						for si, v := range current {
							if v == oldSid {
								if si != slot {
									target = si
								}
								break
							}
						}
					}
				}
				if target < 0 || target > 3 || !skillOK(sid) {
					continue
				}
				// 新技能已在其它槽：与目标槽交换，避免丢招
				for j := 0; j < 4; j++ {
					if j != target && final[j] == sid {
						final[j] = final[target]
						break
					}
				}
				if final[target] != sid {
					final[target] = sid
					changed = true
				}
			}
			if changed {
				pre := append([]int(nil), current...)
				p.Skills = final
				for _, sid := range pre {
					s.mergeLearnedSkillBank(p, sid)
				}
				for _, sid := range final {
					s.mergeLearnedSkillBank(p, sid)
				}
				if err := s.cfg.Store.UpsertPet(p); err != nil {
					fail("save: " + err.Error())
					return
				}
				_ = s.cfg.Store.SetPetLearnedSkillBank(int64(uid), p.CatchTime, p.LearnedSkillBank)
				ok(p, "fmtA")
				return
			}
			ok(p, "fmtA nochange")
			return
		}
	}

	// 格式 B：catchTime + 4×skillId
	if len(body) >= 20 {
		maybeCount := binary.BigEndian.Uint32(body[4:8])
		if maybeCount > 4 { // 不像格式 A 的 count
			seen := map[int]bool{}
			n := 0
			for i := 0; i < 4; i++ {
				sid := int(binary.BigEndian.Uint32(body[4+4*i : 8+4*i]))
				if !skillOK(sid) || seen[sid] {
					continue
				}
				seen[sid] = true
				final[n] = sid
				n++
			}
			for n < 4 {
				final[n] = 0
				n++
			}
			p.Skills = final
			for _, sid := range current {
				s.mergeLearnedSkillBank(p, sid)
			}
			for _, sid := range final {
				s.mergeLearnedSkillBank(p, sid)
			}
			if err := s.cfg.Store.UpsertPet(p); err != nil {
				fail("save: " + err.Error())
				return
			}
			_ = s.cfg.Store.SetPetLearnedSkillBank(int64(uid), p.CatchTime, p.LearnedSkillBank)
			ok(p, "fmtB")
			return
		}
	}

	ok(p, "noop")
}

func normalizeSkillSlots(skills []int) []int {
	out := make([]int, 4)
	n := 0
	seen := map[int]bool{}
	for _, sid := range skills {
		if sid <= 0 || seen[sid] || n >= 4 {
			continue
		}
		seen[sid] = true
		out[n] = sid
		n++
	}
	return out
}

// fillPetSkillsUpToFour 按种族表 LearningLv≤当前等级，空槽补满到最多 4 个（不替换已有）。
func (s *Server) fillPetSkillsUpToFour(p *store.Pet) bool {
	if p == nil {
		return false
	}
	current := normalizeSkillSlots(p.Skills)
	changed := false
	// 保留 Category=4（属性技）在存档；进战由 skillsFromPet 下发（fightOmitCategory4=false）
	filled := 0
	seen := map[int]bool{}
	for _, sid := range current {
		if sid > 0 {
			seen[sid] = true
			filled++
		}
	}
	var moves []tableloader.LearnableMove
	if s != nil && s.cfg.Catalog != nil {
		moves = s.cfg.Catalog.MovesUpToLevel(p.PetID, p.Level)
	} else if defaultSkillCatalog != nil {
		moves = defaultSkillCatalog.MovesUpToLevel(p.PetID, p.Level)
	}
	for _, m := range moves {
		if filled >= 4 {
			break
		}
		if m.ID <= 0 || seen[m.ID] || !s.skillIDKnown(m.ID) {
			continue
		}
		for i := 0; i < 4; i++ {
			if current[i] == 0 {
				current[i] = m.ID
				seen[m.ID] = true
				filled++
				changed = true
				break
			}
		}
	}
	if filled == 0 {
		current = normalizeSkillSlots(nil)
		current[0] = 10001
		if def, ok := starterPets[p.PetID]; ok && len(def.Skills) > 0 {
			n := 0
			for i := 0; i < len(def.Skills) && n < 4; i++ {
				sid := def.Skills[i]
				current[n] = sid
				n++
			}
			if n == 0 {
				current[0] = 10001
			}
		}
		p.Skills = normalizeSkillSlots(current)
		return true
	}
	if changed || !skillsEqual4(p.Skills, current) {
		p.Skills = normalizeSkillSlots(current)
		return true
	}
	return false
}

func skillsEqual4(a, b []int) bool {
	aa, bb := normalizeSkillSlots(a), normalizeSkillSlots(b)
	for i := 0; i < 4; i++ {
		if aa[i] != bb[i] {
			return false
		}
	}
	return true
}

// applyLevelUpSkills 升级后：空槽自动补学本区间新技能；满槽待替换的进 2507 unactive。
// 返回 2507 包体（无需推送时 nil）。不在此补「旧等级已可学但未携带」的技能（由 fillPetSkillsUpToFour 负责）。
func (s *Server) applyLevelUpSkills(p *store.Pet, oldLevel int) []byte {
	if p == nil || s.cfg.Catalog == nil || p.Level <= oldLevel {
		return nil
	}
	newIDs := s.cfg.Catalog.SkillsLearnedBetween(p.PetID, oldLevel, p.Level)
	if len(newIDs) == 0 {
		return nil
	}
	current := normalizeSkillSlots(p.Skills)
	owned := map[int]bool{}
	for _, sid := range current {
		if sid > 0 {
			owned[sid] = true
		}
	}
	var autoLearn, needReplace []int
	for _, sid := range newIDs {
		if sid <= 0 || owned[sid] {
			continue
		}
		empty := -1
		for i, v := range current {
			if v == 0 {
				empty = i
				break
			}
		}
		if empty >= 0 {
			current[empty] = sid
			owned[sid] = true
			autoLearn = append(autoLearn, sid)
		} else {
			needReplace = append(needReplace, sid)
		}
	}
	p.Skills = current
	if len(autoLearn) == 0 && len(needReplace) == 0 {
		return nil
	}
	return buildNoteUpdateSkill(uint32(p.CatchTime), autoLearn, needReplace)
}

// buildNoteUpdateSkill CMD 2507。
// 客户端 UpdateSkillManager：activeSkills → SingleSkillPanel（已自动学会提示）；
// unactiveSkills → MultiSkillPanel（需 2307 替换）。
func buildNoteUpdateSkill(catchTime uint32, activeIDs, unactiveIDs []int) []byte {
	n := 16 + 4*len(activeIDs) + 4*len(unactiveIDs)
	body := make([]byte, 0, n)
	put := func(v uint32) {
		var tmp [4]byte
		binary.BigEndian.PutUint32(tmp[:], v)
		body = append(body, tmp[:]...)
	}
	put(1) // UpdateSkillInfo count
	put(catchTime)
	put(uint32(len(activeIDs)))
	put(uint32(len(unactiveIDs)))
	for _, id := range activeIDs {
		put(uint32(id))
	}
	for _, id := range unactiveIDs {
		put(uint32(id))
	}
	return body
}

// handleGetPetSkill CMD 2336：技能唤醒仪/背包「可学增量」。
// 请求 catchTime(4)；应答 count(4)+skillId(4)*count。
// 客户端会把 PetXML 等级可学列表与本包拼接，故此处只回「表外增量」（LearnedSkillBank）。
// 切勿空 body：PetManager.onGetSuccessHandler 会对 null data 直接 NPE。
func (s *Server) handleGetPetSkill(c *Client, uid uint32, body []byte) {
	catch := uint32(0)
	if len(body) >= 4 {
		catch = binary.BigEndian.Uint32(body[0:4])
	}
	out := make([]byte, 4)
	if s.cfg.Store != nil && catch > 0 {
		if p, err := s.cfg.Store.GetPetByCatchTime(int64(uid), int64(catch)); err == nil && p != nil {
			equipped := make(map[int]bool, 4)
			for _, sid := range p.Skills {
				if sid > 1 {
					equipped[sid] = true
				}
			}
			ids := make([]int, 0, len(p.LearnedSkillBank))
			for _, sid := range p.LearnedSkillBank {
				if sid <= 1 || equipped[sid] {
					continue
				}
				ids = append(ids, sid)
			}
			out = make([]byte, 4+4*len(ids))
			binary.BigEndian.PutUint32(out[0:4], uint32(len(ids)))
			for i, sid := range ids {
				binary.BigEndian.PutUint32(out[4+4*i:8+4*i], uint32(sid))
			}
			log.Printf("[CMD] OK     %s UID=%d catch=%d skills=%d", cmdname.Format(2336), uid, catch, len(ids))
			s.send(c, 2336, uid, 0, out)
			return
		}
	}
	s.send(c, 2336, uid, 0, out)
	log.Printf("[CMD] OK     %s UID=%d catch=%d skills=0", cmdname.Format(2336), uid, catch)
}
