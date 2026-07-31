package gameserver

import (
	"bytes"
	"encoding/binary"
	"log"
	"math/rand"

	"niaohao/server/internal/cmdname"
	"niaohao/server/internal/packet"
	"niaohao/server/internal/store"
	"niaohao/server/internal/tableloader"
)

// handleUsePetItemOutOfFight CMD 2326：战外用药。请求 catchTime+itemID。
// 应答 UsePetItemOutOfFightInfo（本客户端无 shiny）。
func (s *Server) handleUsePetItemOutOfFight(c *Client, uid uint32, body []byte) {
	catch, itemID := uint32(0), uint32(0)
	if len(body) >= 8 {
		catch = binary.BigEndian.Uint32(body[0:4])
		itemID = binary.BigEndian.Uint32(body[4:8])
	}
	if s.cfg.Store == nil || catch == 0 || itemID == 0 {
		s.send(c, 2326, uid, 0, nil)
		log.Printf("[CMD] OK     %s UID=%d (empty)", cmdname.Format(2326), uid)
		return
	}
	p, err := s.cfg.Store.GetPetByCatchTime(int64(uid), int64(catch))
	if err != nil || p == nil {
		s.send(c, 2326, uid, 0, nil)
		log.Printf("[CMD] OK     %s UID=%d catch=%d miss", cmdname.Format(2326), uid, catch)
		return
	}

	meta, hasMeta := tableloader.ItemMeta{}, false
	if s.cfg.Catalog != nil {
		meta, hasMeta = s.cfg.Catalog.ItemMetaOf(int(itemID))
	}
	heal := uint32(0)
	ppRest := uint32(0)
	if s.cfg.Catalog != nil {
		heal = uint32(s.cfg.Catalog.ItemHealHP(int(itemID)))
		ppRest = uint32(s.cfg.Catalog.ItemRestorePP(int(itemID)))
	} else {
		heal = potionHealHP(itemID)
		ppRest = potionRestorePP(itemID)
	}
	// 能量珠走 2610，此处不处理 NewSeIdx 纯珠
	usable := heal > 0 || ppRest > 0 || (hasMeta && meta.HasEffect() && meta.NewSeIdx == 0)
	if hasMeta && meta.NewSeIdx > 0 && heal == 0 && ppRest == 0 && meta.IncreMonLv == 0 &&
		!meta.DecreMonLv && meta.ExpGrant == 0 && !meta.MonAttrReset && !meta.MonNatureReset &&
		!meta.RandomDv && len(meta.NatureSet) == 0 && len(meta.NaturePool) == 0 && meta.EvRemove == 0 &&
		!meta.HasTraitEffect() && !meta.YuanshenDegrade && !meta.BalanceDv && meta.AddDv == 0 {
		usable = false
	}
	if hasMeta && (meta.HasTraitEffect() || meta.YuanshenDegrade || meta.BalanceDv || meta.AddDv > 0) {
		usable = true
	}
	// 硬编码兜底：表未解析时仍放行关键功能药
	switch itemID {
	case 300036, 300105, 300790, 300878:
		usable = true
	}
	if !usable {
		s.send(c, 2326, uid, 0, nil)
		log.Printf("[CMD] OK     %s UID=%d item=%d unsupported", cmdname.Format(2326), uid, itemID)
		return
	}

	// 预检：失败不扣道具
	if (itemID == 300036 || itemID == 300105 || meta.YuanshenDegrade) && !s.isFusionPetID(p.PetID) {
		s.send(c, 2326, uid, 0, nil)
		log.Printf("[CMD] OK     %s UID=%d item=%d not fusion", cmdname.Format(2326), uid, itemID)
		return
	}
	if (itemID == 300790 || meta.BalanceDv || itemID == 300878 || meta.AddDv > 0) && p.DV >= 31 {
		s.send(c, 2326, uid, 0, nil)
		log.Printf("[CMD] OK     %s UID=%d item=%d dv full", cmdname.Format(2326), uid, itemID)
		return
	}

	if err := s.cfg.Store.ConsumeItem(int64(uid), int(itemID), 1); err != nil {
		s.send(c, 2326, uid, 0, nil)
		log.Printf("[CMD] OK     %s UID=%d item=%d no stock: %v", cmdname.Format(2326), uid, itemID, err)
		return
	}

	changed := false
	evBefore := p.EV
	dvBefore := p.DV
	petIDBefore := p.PetID

	// 专用效果优先于通用 meta
	switch {
	case itemID == 300036 || itemID == 300105 || meta.YuanshenDegrade:
		if s.applyFusionPetDegrade(p) {
			changed = true
		}
	case itemID == 300790 || meta.BalanceDv:
		if p.DV <= 1 {
			p.DV = 2
		} else if rand.Intn(2) == 0 {
			p.DV--
		} else {
			p.DV++
			if p.DV > 31 {
				p.DV = 31
			}
		}
		changed = true
	case itemID == 300878 || meta.AddDv > 0:
		add := meta.AddDv
		if add <= 0 {
			add = 1
		}
		p.DV += add
		if p.DV > 31 {
			p.DV = 31
		}
		changed = true
	default:
		if hasMeta {
			changed = applyPetItemMeta(p, meta) || changed
			if applyPetTraitItem(p, meta, s.cfg.Catalog) {
				changed = true
			}
		}
	}
	if s.tryDirectEvolve(p) {
		changed = true
		s.fillPetSkillsUpToFour(p)
	}

	maxHP := uint32(petMaxHP(p))
	cur := s.recalledPetHP(int64(uid), catch, maxHP)
	if heal > 0 {
		cur += heal
		if cur > maxHP {
			cur = maxHP
		}
		if cur >= maxHP {
			s.forgetPetHP(int64(uid), catch)
			p.CurrentHP = 0
		} else {
			s.rememberPetHP(int64(uid), catch, cur)
			p.CurrentHP = int(cur)
		}
		changed = true
	} else if cur < maxHP {
		p.CurrentHP = int(cur)
	} else {
		p.CurrentHP = 0
	}

	if changed {
		_ = s.cfg.Store.UpsertPet(p)
		if p.EV != evBefore {
			_ = s.cfg.Store.SetPetEV(int64(uid), int64(catch), p.EV)
		}
		if p.CurrentHP > 0 {
			_ = s.cfg.Store.SetPetCurrentHP(int64(uid), int64(catch), p.CurrentHP)
		} else if heal > 0 {
			_ = s.cfg.Store.SetPetCurrentHP(int64(uid), int64(catch), 0)
		}
		if p.Trait > 0 {
			_ = s.cfg.Store.SetPetTrait(int64(uid), int64(catch), p.Trait)
		} else if petIDBefore != p.PetID {
			_ = s.cfg.Store.SetPetTrait(int64(uid), int64(catch), 0)
		}
		if itemID == 300036 || itemID == 300105 || meta.YuanshenDegrade {
			_ = s.cfg.Store.SetPetFormDisplay(int64(uid), int64(catch), 0, 0, 0)
			_ = s.cfg.Store.SetPetEnergyBall(int64(uid), int64(catch), 0, 0, 0)
		}
	}

	out := buildUsePetItemOutOfFightInfo(p)
	s.send(c, 2326, uid, 0, out)
	if info := buildPetInfo(p); len(info) > 0 {
		s.send(c, 2301, uid, 0, info)
	}
	log.Printf("[CMD] OK     %s UID=%d catch=%d item=%d heal=%d pp+=%d lv=%d nature=%d dv=%d->%d trait=%d hp=%d/%d",
		cmdname.Format(2326), uid, catch, itemID, heal, ppRest, p.Level, p.Nature, dvBefore, p.DV, p.Trait, cur, maxHP)
}

// applyPetItemMeta 应用非 HP/PP 类效果；有变更返回 true。
func applyPetItemMeta(p *store.Pet, meta tableloader.ItemMeta) bool {
	if p == nil {
		return false
	}
	changed := false
	if meta.IncreMonLv > 0 {
		p.Level += meta.IncreMonLv
		if p.Level > 100 {
			p.Level = 100
		}
		p.Exp = 0
		changed = true
	}
	if meta.DecreMonLv && p.Level > 1 {
		p.Level--
		changed = true
	}
	if meta.ExpGrant > 0 {
		if applyPetExpGain(p, meta.ExpGrant) > 0 {
			changed = true
		}
	}
	if meta.MonAttrReset || meta.RandomDv {
		p.DV = rand.Intn(32)
		changed = true
	}
	if meta.MonNatureReset {
		p.Nature = rand.Intn(25)
		changed = true
	}
	if n := meta.PickNature(); n >= 0 {
		p.Nature = n
		changed = true
	}
	if meta.EvRemove > 0 {
		switch meta.EvRemove {
		case 1, 2, 3, 4, 5, 6:
			p.EV[meta.EvRemove-1] = 0
		case 7:
			p.EV = [6]int{}
		}
		changed = true
	}
	return changed
}

// applyPetTraitItem 特性开启/重组；有变更返回 true。
func applyPetTraitItem(p *store.Pet, meta tableloader.ItemMeta, cat *tableloader.Catalog) bool {
	if p == nil || !meta.HasTraitEffect() {
		return false
	}
	isFuse := false
	if cat != nil {
		if d := cat.PetBase(p.PetID); d != nil {
			isFuse = d.IsFuseMon
		}
	}
	if meta.NewSeReset {
		if !isFuse {
			return false
		}
		RerollPetTrait(p, true)
		return true
	}
	if meta.NonFuseAddNewse {
		if IsValidPetTrait(p.Trait) {
			return false
		}
		AssignPetTraitIfNeeded(p)
		return IsValidPetTrait(p.Trait)
	}
	if meta.NonFuseResetNewse == 1 {
		RerollPetTrait(p, true)
		return true
	}
	if meta.NonFuseResetNewse > 1 {
		if !IsValidPetTrait(meta.NonFuseResetNewse) && meta.NonFuseResetNewse < 1006 {
			// 指定 Idx 可能超出 1045（如强袭 1065），仍写入以便客户端展示
		}
		p.Trait = meta.NonFuseResetNewse
		return true
	}
	return false
}

func petMaxHP(p *store.Pet) int {
	if p == nil {
		return 50
	}
	hpBase := 50
	if def, ok := starterPets[p.PetID]; ok {
		hpBase = def.HP
	}
	if base := resolvePetBaseStats(p.PetID); base.HP > 0 {
		hpBase = base.HP
	}
	lv, dv := p.Level, p.DV
	if lv <= 0 {
		lv = 5
	}
	if dv < 0 {
		dv = 0
	}
	maxHP := calcHP(hpBase, dv, lv, p.EV[0])
	return maxHP
}

// buildUsePetItemOutOfFightInfo 对齐反编译 UsePetItemOutOfFightInfo（nature 在 dv 前；EV 在属性前）。
func buildUsePetItemOutOfFightInfo(p *store.Pet) []byte {
	if p == nil {
		return nil
	}
	var buf bytes.Buffer
	w32 := func(v uint32) { packet.WriteU32(&buf, v) }
	wStr := func(s string, n int) { packet.WriteFixedString(&buf, s, n) }

	ensurePetSkills(p)
	def, ok := starterPets[p.PetID]
	if !ok {
		def = starterDef{Name: p.Name, HP: 50, Atk: 50, Def: 50, SpAtk: 50, SpDef: 50, Spd: 50, Skills: []int{10001}}
	}
	if base := resolvePetBaseStats(p.PetID); base.HP > 0 {
		def.HP, def.Atk, def.Def = base.HP, base.Atk, base.Def
		def.SpAtk, def.SpDef, def.Spd = base.SpAtk, base.SpDef, base.Spd
		if base.Name != "" {
			def.Name = base.Name
		}
	}
	lv, dv := p.Level, p.DV
	if lv <= 0 {
		lv = 5
	}
	if dv < 0 {
		dv = 0
	}
	name := p.Name
	if name == "" {
		name = def.Name
	}
	maxHP, atk, defv, sa, sd, spd := petSixStatsFromPet(p)
	curHP := maxHP
	if p.CurrentHP > 0 && p.CurrentHP < maxHP {
		curHP = p.CurrentHP
	}

	skills := p.Skills
	if len(skills) == 0 {
		skills = def.Skills
	}
	skillIDs := make([]int, 0, 4)
	for _, sid := range skills {
		if sid <= 0 {
			continue
		}
		// 战外面板同背包：保留属性技；进战另过滤
		skillIDs = append(skillIDs, sid)
		if len(skillIDs) >= 4 {
			break
		}
	}
	if len(skillIDs) == 0 {
		skillIDs = []int{10001}
	}

	w32(uint32(p.CatchTime))
	w32(uint32(p.PetID))
	wStr(name, 16)
	w32(uint32(p.Nature))
	w32(uint32(dv))
	w32(uint32(lv))
	w32(uint32(curHP))
	w32(uint32(maxHP))
	w32(uint32(p.Exp))
	for i := 0; i < 6; i++ {
		w32(uint32(p.EV[i]))
	}
	w32(uint32(atk))
	w32(uint32(sa))
	w32(uint32(defv))
	w32(uint32(sd))
	w32(uint32(spd))
	w32(uint32(len(skillIDs)))
	for _, sid := range skillIDs {
		w32(uint32(sid))
		w32(skillPPForBuild(sid))
	}
	return buf.Bytes()
}
