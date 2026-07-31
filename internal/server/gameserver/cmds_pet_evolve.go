package gameserver

import (
	"encoding/binary"
	"log"
	"math/rand"
	"time"

	"niaohao/server/internal/store"
	"niaohao/server/internal/tableloader"
)

// handlePetEvolve CMD 2314 PET_EVOLVTION：实验室进化仓。
// 请求 catchTime(4)+evolveIndex(4)；失败 u32(0)；成功 u32(1)+PetInfo，并推 2508。
func (s *Server) handlePetEvolve(c *Client, uid uint32, body []byte) {
	fail := func() {
		out := make([]byte, 4)
		s.send(c, 2314, uid, 0, out)
	}
	if s.cfg.Store == nil || len(body) < 8 {
		fail()
		return
	}
	catchTime := int64(binary.BigEndian.Uint32(body[0:4]))
	evolveIndex := int(binary.BigEndian.Uint32(body[4:8]))
	p, err := s.cfg.Store.GetPetByCatchTime(int64(uid), catchTime)
	if err != nil || p == nil || !p.InBag {
		fail()
		return
	}
	if p.FormLocked != 0 {
		fail()
		log.Printf("[CMD] OK     2314 UID=%d form locked catch=%d", uid, catchTime)
		return
	}
	def := s.petBase(p.PetID)
	if def == nil {
		fail()
		return
	}
	// 进化仓：EvolveBabin=1 或 EvolvFlag>0 或 EvolvesTo>0
	if def.EvolveBabin != 1 && def.EvolvFlag == 0 && def.EvolvesTo == 0 {
		fail()
		return
	}

	targetID := 0
	needItem, needCnt := 0, 0

	if def.EvolvFlag > 0 && def.EvolvesTo == 0 {
		if evolveIndex <= 0 {
			fail()
			return
		}
		branches, ok := tableloader.EvolveBranches(def.EvolvFlag)
		if !ok || evolveIndex > len(branches) {
			fail()
			return
		}
		br := branches[evolveIndex-1]
		targetID = br.MonTo
		needItem, needCnt = br.EvolvItem, br.EvolvItemCount
	} else if def.EvolvesTo > 0 {
		targetID = def.EvolvesTo
		needItem, needCnt = def.EvolvItem, def.EvolvItemCount
		if needCnt <= 0 && needItem > 0 {
			needCnt = 1
		}
	} else {
		fail()
		return
	}

	tgt := s.petBase(targetID)
	if tgt == nil {
		fail()
		return
	}
	needLv := tgt.EvolvingLv
	if needLv <= 0 {
		needLv = def.EvolvingLv
	}
	if needLv > 0 && p.Level < needLv {
		fail()
		return
	}
	if needItem > 0 && needCnt > 0 {
		have, _ := s.cfg.Store.GetItemCount(int64(uid), needItem)
		if have < needCnt {
			fail()
			return
		}
		if err := s.cfg.Store.ConsumeItem(int64(uid), needItem, needCnt); err != nil {
			fail()
			return
		}
	}

	oldID := p.PetID
	p.PetID = targetID
	p.Exp = 0
	if n := s.cfg.Catalog.PetNameOf(targetID); n != "" {
		p.Name = n
	}
	s.fillPetSkillsUpToFour(p)
	if err := s.cfg.Store.UpsertPet(p); err != nil {
		fail()
		return
	}

	lv := p.Level
	if lv <= 0 {
		lv = 1
	}
	_, _, _, hp, atk, defv, sa, sd, spd := petCombatStats(p)
	prop := buildNoteUpdateProp(uint32(p.CatchTime), p.PetID, lv, p.Exp, p.Exp, petNextLevelExp(p.PetID, lv),
		hp, atk, defv, sa, sd, spd, p.EV)
	s.send(c, 2508, uid, 0, prop)

	petBody := buildPetInfo(p)
	out := make([]byte, 4+len(petBody))
	binary.BigEndian.PutUint32(out[0:4], 1)
	copy(out[4:], petBody)
	s.send(c, 2314, uid, 0, out)
	log.Printf("[CMD] OK     2314 PET_EVOLVTION UID=%d catch=%d %d->%d", uid, catchTime, oldID, targetID)
}

func (s *Server) petBase(id int) *tableloader.PetBaseDef {
	if s.cfg.Catalog == nil {
		return nil
	}
	return s.cfg.Catalog.PetBase(id)
}

// —— 收集计划 2311 / 2313 ——

var petCollectRewards = map[uint32]int{
	1: 4,   // 默认伊优；choice 覆盖
	2: 71,  // 派派
	3: 275, // 尹赫
}

var petCollectNoviceChoice = map[uint32]int{
	1: 1, // 布布种子
	2: 7, // 小火猴
	3: 4, // 伊优
}

// handleISCollect CMD 2313：请求 collectId(4)；应答 id(4)+collected(4)。
func (s *Server) handleISCollect(c *Client, uid uint32, body []byte) {
	var id uint32
	if len(body) >= 4 {
		id = binary.BigEndian.Uint32(body[0:4])
	}
	out := make([]byte, 8)
	binary.BigEndian.PutUint32(out[0:4], id)
	collected := uint32(0)
	if s.cfg.Store != nil {
		ok, _ := s.cfg.Store.IsPetCollectClaimed(int64(uid), int(id))
		if ok {
			collected = 1
		}
	}
	binary.BigEndian.PutUint32(out[4:8], collected)
	s.send(c, 2313, uid, 0, out)
}

// handlePetCollect CMD 2311：branch(4)+planId(4)[+choice(4)]；应答 petID(4)+catchTime(4)。
func (s *Server) handlePetCollect(c *Client, uid uint32, body []byte) {
	branch := uint32(1)
	if len(body) >= 4 {
		branch = binary.BigEndian.Uint32(body[0:4])
	}
	out := make([]byte, 8)
	if s.cfg.Store == nil {
		s.send(c, 2311, uid, 0, out)
		return
	}
	if branch >= 1 && branch <= 3 {
		if ok, _ := s.cfg.Store.IsPetCollectClaimed(int64(uid), int(branch)); ok {
			s.send(c, 2311, uid, 0, out)
			return
		}
	}
	reward := petCollectRewards[branch]
	if reward == 0 {
		reward = 4
	}
	if branch == 1 && len(body) >= 12 {
		choice := binary.BigEndian.Uint32(body[8:12])
		if id, ok := petCollectNoviceChoice[choice]; ok {
			reward = id
		}
	}
	catchTime, err := s.grantNewPet(int64(uid), reward, 1)
	if err != nil {
		s.send(c, 2311, uid, 0, out)
		return
	}
	_ = s.cfg.Store.MarkPetCollectClaimed(int64(uid), int(branch))
	binary.BigEndian.PutUint32(out[0:4], uint32(reward))
	binary.BigEndian.PutUint32(out[4:8], uint32(catchTime))
	s.send(c, 2311, uid, 0, out)
	log.Printf("[CMD] OK     2311 PET_COLLECT UID=%d branch=%d pet=%d", uid, branch, reward)
}

func (s *Server) grantNewPet(uid int64, petID, level int) (catchTime int64, err error) {
	return s.grantNewPetDV(uid, petID, level, -1)
}

func (s *Server) grantNewPetDV(uid int64, petID, level, dv int) (catchTime int64, err error) {
	return s.grantNewPetDVNature(uid, petID, level, dv, -1)
}

func (s *Server) grantNewPetDVNature(uid int64, petID, level, dv, nature int) (catchTime int64, err error) {
	return s.grantNewPetFull(uid, petID, level, dv, nature, -1)
}

func (s *Server) grantNewPetFull(uid int64, petID, level, dv, nature, trait int) (catchTime int64, err error) {
	if level <= 0 {
		level = 1
	}
	name := "精灵"
	if s.cfg.Catalog != nil {
		if n := s.cfg.Catalog.PetNameOf(petID); n != "" {
			name = n
		}
	}
	catchTime = time.Now().Unix()
	for i := 0; i < 5; i++ {
		if existing, _ := s.cfg.Store.GetPetByCatchTime(uid, catchTime); existing == nil {
			break
		}
		catchTime++
	}
	if dv < 0 {
		dv = rand.Intn(32)
	}
	if nature < 0 || nature > 24 {
		nature = rand.Intn(25)
	}
	// 野生/孵化/任务等默认无特性；仅融合等显式传入合法 Idx 才带特性（开启靠道具 NonFuseAddNewse）
	if !IsValidPetTrait(trait) {
		trait = 0
	}
	p := &store.Pet{
		UserID: uid, CatchTime: catchTime, PetID: petID, Name: name,
		Level: level, DV: dv, Nature: nature, Trait: trait,
		InBag: true, BagPos: 99, Skills: []int{10001},
	}
	n, _ := s.cfg.Store.CountBagPets(uid)
	if n >= store.MaxBagPets {
		p.InBag = false
		p.BagPos = -1
	}
	s.fillPetSkillsUpToFour(p)
	if err = s.cfg.Store.UpsertPet(p); err != nil {
		return 0, err
	}
	if p.Trait > 0 {
		_ = s.cfg.Store.SetPetTrait(uid, catchTime, p.Trait)
	}
	return catchTime, nil
}
