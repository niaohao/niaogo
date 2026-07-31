package gameserver

import (
	"encoding/binary"
	"log"
	"math/rand"
	"sync"

	"niaohao/server/internal/store"
)

// 精灵大乱斗 CMD 2431：随机抽 6 只终形（ID≤2000、Lv100），玩家 3 vs AI 3；
// 临时精灵不落库；奖励按攻略：输赢皆 5w 积累经验日限 2；累计 30 胜发寒流枪 100245。
const (
	grandMeleeEnemyLevel    = 100
	grandMeleePetMaxID      = 2000
	grandMeleeMinBagPets   = 3
	grandMeleeTeamSize      = 3
	grandMeleeWinKey        = "grandMeleeWins"
	grandMeleeColdGunFlag   = "grandMeleeColdGun"
	grandMeleeColdGunWins   = 30
	grandMeleeColdGunItemID = 100245
	grandMeleeCatchBase     = uint32(0xF0000000)
)

type grandMeleeSession struct {
	Player []store.Pet
	Enemy  []store.Pet
}

type grandMeleeHub struct {
	mu sync.Mutex
	m  map[int64]*grandMeleeSession
}

func (h *grandMeleeHub) set(uid int64, player, enemy []store.Pet) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.m == nil {
		h.m = make(map[int64]*grandMeleeSession)
	}
	h.m[uid] = &grandMeleeSession{Player: player, Enemy: enemy}
}

func (h *grandMeleeHub) get(uid int64) *grandMeleeSession {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.m[uid]
}

func (h *grandMeleeHub) clear(uid int64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.m, uid)
}

func (s *Server) clearGrandMeleeSession(uid int64) {
	s.melee.clear(uid)
}

func (s *Server) grandMeleePlayerPets(uid int64) []store.Pet {
	if sess := s.melee.get(uid); sess != nil {
		return sess.Player
	}
	return nil
}

func (s *Server) grandMeleeEnemyPets(uid int64) []store.Pet {
	if sess := s.melee.get(uid); sess != nil {
		return sess.Enemy
	}
	return nil
}

func (s *Server) setGrandMeleePlayerHP(uid int64, catch uint32, hp uint32) {
	s.melee.mu.Lock()
	defer s.melee.mu.Unlock()
	sess := s.melee.m[uid]
	if sess == nil {
		return
	}
	for i := range sess.Player {
		if uint32(sess.Player[i].CatchTime) == catch {
			sess.Player[i].CurrentHP = int(hp)
			return
		}
	}
}

func (s *Server) handleGrandMeleeJoin(c *Client, uid uint32) {
	fail := func() {
		out := make([]byte, 4)
		binary.BigEndian.PutUint32(out, 1)
		s.send(c, 2431, uid, 0, out)
	}
	if st := s.battles.get(int64(uid)); st != nil && st.Active {
		fail()
		return
	}
	nBag := 0
	if s.cfg.Store != nil {
		pets, _ := s.cfg.Store.ListBagPets(int64(uid))
		nBag = len(pets)
	}
	if nBag < grandMeleeMinBagPets {
		s.sendAlert(int64(uid), "需要带上3只以上精灵才能参加精灵大乱斗")
		fail()
		return
	}
	all, ok := s.pickGrandMeleeTempPets(grandMeleeTeamSize * 2)
	if !ok {
		s.sendAlert(int64(uid), "大乱斗精灵池不足，请稍后再试")
		fail()
		return
	}
	rand.Shuffle(len(all), func(i, j int) { all[i], all[j] = all[j], all[i] })
	playerPets := append([]store.Pet(nil), all[0:grandMeleeTeamSize]...)
	enemyPets := append([]store.Pet(nil), all[grandMeleeTeamSize:]...)
	s.melee.set(int64(uid), playerPets, enemyPets)
	if !s.beginGrandMeleeFight(c, uid, playerPets, enemyPets) {
		s.clearGrandMeleeSession(int64(uid))
		fail()
		return
	}
	out := make([]byte, 4)
	binary.BigEndian.PutUint32(out, 0)
	s.send(c, 2431, uid, 0, out)
	log.Printf("[CMD] OK     2431 START_PET_WAR UID=%d player=%v enemy=%v -> 2503",
		uid, petIDsOf(playerPets), petIDsOf(enemyPets))
}

func petIDsOf(pets []store.Pet) []int {
	out := make([]int, len(pets))
	for i := range pets {
		out[i] = pets[i].PetID
	}
	return out
}

func (s *Server) pickGrandMeleeTempPets(count int) ([]store.Pet, bool) {
	pool := s.grandMeleeIDPool()
	if len(pool) < count {
		return nil, false
	}
	rand.Shuffle(len(pool), func(i, j int) { pool[i], pool[j] = pool[j], pool[i] })
	base := grandMeleeCatchBase | uint32(rand.Intn(1<<20))<<8 | uint32(rand.Intn(1<<8))
	pets := make([]store.Pet, 0, count)
	for i := 0; i < count; i++ {
		base++
		if base == 0 {
			base = grandMeleeCatchBase + 1
		}
		pets = append(pets, s.buildGrandMeleeTempPet(pool[i], base))
	}
	return pets, true
}

func (s *Server) grandMeleeIDPool() []int {
	if s.cfg.Catalog == nil {
		return nil
	}
	return s.cfg.Catalog.FinalFormPetIDs(grandMeleePetMaxID)
}

func (s *Server) buildGrandMeleeTempPet(petID int, catchTime uint32) store.Pet {
	name := ""
	if s.cfg.Catalog != nil {
		name = s.cfg.Catalog.PetNameOf(petID)
	}
	if name == "" {
		name = "大乱斗精灵"
	}
	skills := make([]int, 0, 4)
	for _, sk := range s.enemySkillsForPet(petID, grandMeleeEnemyLevel) {
		if sk[0] > 0 {
			skills = append(skills, int(sk[0]))
		}
	}
	if len(skills) == 0 {
		skills = []int{10001}
	}
	return store.Pet{
		CatchTime: int64(catchTime),
		PetID:     petID,
		Name:      name,
		Level:     grandMeleeEnemyLevel,
		DV:        31,
		Nature:    rand.Intn(25),
		Skills:    skills,
		InBag:     true,
	}
}

func (s *Server) beginGrandMeleeFight(c *Client, uid uint32, playerPets, enemyPets []store.Pet) bool {
	if len(playerPets) == 0 || len(enemyPets) == 0 {
		return false
	}
	lead := &playerPets[0]
	pid, lv, name, php, patk, pdef, psa, psd, pspd := petCombatStats(lead)
	ehp, eatk, edef, esa, esd, espd := enemyCombatStats(enemyPets[0].PetID, enemyPets[0].Level)
	if ehp <= 0 {
		ehp = 1
	}
	if php <= 0 {
		php = 1
	}
	enemyIDs := make([]int, len(enemyPets))
	for i := range enemyPets {
		enemyIDs[i] = enemyPets[i].PetID
	}
	enemyName := enemyPets[0].Name
	if enemyName == "" {
		enemyName = "大乱斗对手"
	}
	nick := s.nickOf(uid)
	mapID := c.MapID
	if mapID <= 0 {
		mapID = defaultMapID
	}
	st := &BattleState{
		Active:          true,
		MapID:           mapID,
		FightKind:       fightKindNormal,
		IsGrandMelee:    true,
		DailyExpKind:    dailyExpGrandMelee,
		EnemyCatchable:  false,
		EnemyID:         enemyPets[0].PetID,
		EnemyLevel:      enemyPets[0].Level,
		EnemyName:       enemyName,
		EnemyHP:         uint32(ehp),
		EnemyMaxHP:      uint32(ehp),
		EnemyAtk:        eatk,
		EnemyDef:        edef,
		EnemySpAtk:      esa,
		EnemySpDef:      esd,
		EnemySpd:        espd,
		EnemySkills:     s.skillsFromPet(&enemyPets[0]),
		EnemyType:       s.petTypeOf(enemyPets[0].PetID),
		EnemyDV:         enemyPets[0].DV,
		EnemyTeamIDs:    enemyIDs,
		EnemyTeamIndex:  0,
		PlayerPetID:     pid,
		PlayerLevel:     lv,
		PlayerName:      name,
		PlayerCatchTime: uint32(lead.CatchTime),
		PlayerHP:        uint32(php),
		PlayerMaxHP:     uint32(php),
		PlayerAtk:       patk,
		PlayerDef:       pdef,
		PlayerSpAtk:     psa,
		PlayerSpDef:     psd,
		PlayerSpd:       pspd,
		PlayerSkills:    s.skillsFromPet(lead),
		PlayerType:      s.petTypeOf(pid),
		PlayerDV:        lead.DV,
	}
	s.battles.set(int64(uid), st)

	petInfoForceEmptySkills = debugFightUIEmptySkills
	playerSimple := s.simplePetsForBattle(uid, st, playerPets)
	enemySimple := s.simplePetsFromList(enemyPets)
	out := buildNoteReadyToFightSides(uid, nick, st, playerSimple, enemySimple)
	petInfoForceEmptySkills = false

	s.send(c, 2503, uid, 0, out)
	s.syncBagPetsToClientTagged(c, uid, playerPets, st.PlayerCatchTime, "grand-melee-post-2503")
	log.Printf("[CMD] OK     2503 NOTE_READY_TO_FIGHT UID=%d grand-melee pets=%d+%d",
		uid, len(playerPets), len(enemyPets))
	return true
}

func (s *Server) simplePetsFromList(pets []store.Pet) [][]byte {
	out := make([][]byte, 0, len(pets))
	for i := range pets {
		p := &pets[i]
		pid, lv, _, php, _, _, _, _, _ := petCombatStats(p)
		sk := s.skillsFromPet(p)
		if php <= 0 {
			php = 1
		}
		out = append(out, buildSimplePetInfo(
			uint32(pid), uint32(lv), uint32(php), uint32(php),
			uint32(p.CatchTime), sk, uint32(pid), 0, 0,
		))
	}
	return out
}

func buildNoteReadyToFightSides(uid uint32, nick string, st *BattleState, playerPets, enemyPets [][]byte) []byte {
	if len(playerPets) == 0 {
		playerPets = [][]byte{buildSimplePetInfo(
			uint32(st.PlayerPetID), uint32(st.PlayerLevel),
			st.PlayerHP, st.PlayerMaxHP, st.PlayerCatchTime,
			st.PlayerSkills, 0, 0, 0,
		)}
	}
	if len(enemyPets) == 0 {
		enemyPets = [][]byte{buildSimplePetInfo(
			uint32(st.EnemyID), uint32(st.EnemyLevel),
			st.EnemyHP, st.EnemyMaxHP, 0,
			st.EnemySkills, 0, 0, 0,
		)}
	}
	var buf bytesBuffer
	buf.putU32(2)
	buf.write(buildFightUserInfo(uid, nick, 0))
	buf.putU32(uint32(len(playerPets)))
	for _, pet := range playerPets {
		buf.write(pet)
	}
	buf.putU32(0)
	buf.write(buildFightUserInfo(0, st.EnemyName, 0))
	buf.putU32(uint32(len(enemyPets)))
	for _, pet := range enemyPets {
		buf.write(pet)
	}
	buf.putU32(0)
	return buf.bytes()
}

// tryGrandMeleeEnemySwitch 击败当前 AI 精灵后换下一只；成功则推 2407。
func (s *Server) tryGrandMeleeEnemySwitch(c *Client, uid uint32, st *BattleState) bool {
	if st == nil || !st.IsGrandMelee || st.EnemyHP > 0 {
		return false
	}
	next := st.EnemyTeamIndex + 1
	if next < 0 || next >= len(st.EnemyTeamIDs) {
		return false
	}
	enemyPets := s.grandMeleeEnemyPets(int64(uid))
	if next >= len(enemyPets) {
		return false
	}
	ep := &enemyPets[next]
	ehp, eatk, edef, esa, esd, espd := enemyCombatStats(ep.PetID, ep.Level)
	if ehp <= 0 {
		ehp = 1
	}
	name := ep.Name
	if name == "" {
		name = "大乱斗对手"
	}
	st.EnemyTeamIndex = next
	st.EnemyID = ep.PetID
	st.EnemyLevel = ep.Level
	st.EnemyName = name
	st.EnemyHP = uint32(ehp)
	st.EnemyMaxHP = uint32(ehp)
	st.EnemyAtk = eatk
	st.EnemyDef = edef
	st.EnemySpAtk = esa
	st.EnemySpDef = esd
	st.EnemySpd = espd
	st.EnemySkills = s.skillsFromPet(ep)
	st.EnemyType = s.petTypeOf(ep.PetID)
	st.EnemyDV = ep.DV
	st.EnemyStages = [5]int8{}
	st.EnemyStatus = battleStatus{}
	st.EnemyBuff = battleBuff{}
	st.EnemyConsecSkillID = 0
	st.EnemyConsecSkillCount = 0
	s.battles.set(int64(uid), st)

	out := buildChangePetInfo(0, ep.PetID, name, uint32(ep.Level), st.EnemyHP, st.EnemyMaxHP, 0)
	if c != nil {
		s.send(c, 2407, uid, 0, out)
	}
	log.Printf("[grand-melee] UID=%d enemy switch idx=%d pet=%d hp=%d", uid, next, ep.PetID, st.EnemyHP)
	return true
}

// battleBagSource 大乱斗用临时精灵，否则背包。
func (s *Server) battleBagSource(uid uint32, st *BattleState) []store.Pet {
	if st != nil && st.IsGrandMelee {
		return s.grandMeleePlayerPets(int64(uid))
	}
	if s.cfg.Store == nil {
		return nil
	}
	bag, _ := s.cfg.Store.ListBagPets(int64(uid))
	return bag
}

// grantGrandMeleeWinProgress 大乱斗胜利：累计胜场，满 30 自动发寒流枪（终身一次）。
func (s *Server) grantGrandMeleeWinProgress(c *Client, uid uint32, st *BattleState) {
	if st == nil || (!st.IsGrandMelee && st.DailyExpKind != dailyExpGrandMelee) || s.cfg.Store == nil {
		return
	}
	n := s.bumpLifetime(int64(uid), grandMeleeWinKey)
	if n < grandMeleeColdGunWins {
		return
	}
	if !s.tryMarkLifetime(int64(uid), grandMeleeColdGunFlag) {
		return
	}
	_ = s.cfg.Store.AddItem(int64(uid), grandMeleeColdGunItemID, 1)
	s.sendAlert(int64(uid), "大乱斗累计30胜，获得寒流枪！装备后可捕捉小荧蜂")
	log.Printf("[grand-melee] cold gun UID=%d wins=%d", uid, n)
}
