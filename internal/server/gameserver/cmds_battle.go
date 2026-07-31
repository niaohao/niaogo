package gameserver

import (
	"encoding/binary"
	"fmt"
	"log"
	"math/rand"

	"niaohao/server/internal/packet"
	"niaohao/server/internal/store"
)

// handleChallengeBoss CMD 2411：挑战 BOSS → ACK 2411 + 推送 2503 NOTE_READY_TO_FIGHT。
func (s *Server) handleChallengeBoss(c *Client, uid uint32, body []byte) {
	param2 := uint32(0)
	if len(body) >= 4 {
		param2 = binary.BigEndian.Uint32(body[0:4])
	}
	mapID := c.MapID
	if mapID <= 0 {
		mapID = defaultMapID
	}
	if !s.gatePuniChallenge(c, uid, mapID, param2) {
		return
	}
	enemyID, enemyLv, enemyName := resolveChallengeBoss(mapID, param2)
	s.markPuniDailyChallenge(uid, mapID, param2)
	s.send(c, 2411, uid, 0, nil) // MapProcess_514 等监听 CHALLENGE_BOSS 成功
	s.beginFightVsEnemy(c, uid, enemyID, enemyLv, false, fightKindNormal)
	if st := s.battles.get(int64(uid)); st != nil {
		st.BossRegion = param2
		st.PlayerUsedSkills = make(map[uint32]bool)
		if isLeiyiTrainBattle(st) || (mapID == 423 && (param2 == 4 || param2 == 5)) || (mapID == gaiyaTrainMapID && param2 == gaiyaTrainBossRegion) {
			st.ForceSinglePet = true
		}
		applyBossOpenBattleRules(st)
		s.initPuniBattleOnOpen(uid, st)
		s.battles.set(int64(uid), st)
	}
	log.Printf("[CMD] OK     2411 CHALLENGE_BOSS UID=%d map=%d param2=%d enemy=%d(%s) lv=%d -> 2503",
		uid, mapID, param2, enemyID, enemyName, enemyLv)
}

// beginFightVsEnemy 按精灵 ID 开战（塔/道场/师徒试炼共用）。
func (s *Server) beginFightVsEnemy(c *Client, uid uint32, enemyID, enemyLv int, catchable bool, fightKind int) {
	if enemyID <= 0 {
		enemyID = fallbackEnemyPetID
	}
	if enemyLv <= 0 {
		enemyLv = 5
	}
	enemyName := fallbackEnemyName
	if s.cfg.Catalog != nil {
		if n := s.cfg.Catalog.PetNameOf(enemyID); n != "" {
			enemyName = n
		}
	}
	ehp, eatk, edef, esa, esd, espd := enemyCombatStats(enemyID, enemyLv)

	var bag []store.Pet
	if s.cfg.Store != nil {
		bag, _ = s.cfg.Store.ListBagPets(int64(uid))
	}
	p := pickBattlePet(bag)
	petID, lv, name, php, patk, pdef, psa, psd, pspd := petCombatStats(p)
	var critBonus int
	patk, pdef, psa, psd, pspd, critBonus = s.applyEnergyBallBonus(p, patk, pdef, psa, psd, pspd)
	playerTrait := s.applyBattlePetTrait(int64(uid), p, &critBonus)
	skills := s.skillsFromPet(p)
	catch := uint32(0)
	playerDV := 0
	if p != nil {
		catch = uint32(p.CatchTime)
		playerDV = p.DV
	}
	if catch == 0 {
		catch = uint32(task86CatchTm(petID))
	}
	nick := ""
	if s.cfg.Store != nil {
		if u, err := s.cfg.Store.FindByUserID(int64(uid)); err == nil && u != nil {
			nick = u.Nickname
		}
	}
	if nick == "" {
		nick = "Seer"
	}
	mapID := c.MapID
	if mapID <= 0 {
		mapID = defaultMapID
	}
	st := &BattleState{
		Active:          true,
		MapID:           mapID,
		FightKind:       fightKind,
		EnemyID:         enemyID,
		EnemyLevel:      enemyLv,
		EnemyName:       enemyName,
		EnemyHP:         uint32(ehp),
		EnemyMaxHP:      uint32(ehp),
		EnemyAtk:        eatk,
		EnemyDef:        edef,
		EnemySpAtk:      esa,
		EnemySpDef:      esd,
		EnemySpd:        espd,
		EnemySkills:     s.enemySkillsForPet(enemyID, enemyLv),
		EnemyCatchable:  catchable,
		EnemyType:       s.petTypeOf(enemyID),
		PlayerPetID:     petID,
		PlayerLevel:     lv,
		PlayerName:      name,
		PlayerCatchTime: catch,
		PlayerHP:        s.recalledPetHP(int64(uid), catch, uint32(php)),
		PlayerMaxHP:     uint32(php),
		PlayerAtk:       patk,
		PlayerDef:       pdef,
		PlayerSpAtk:     psa,
		PlayerSpDef:     psd,
		PlayerSpd:       pspd,
		PlayerSkills:    skills,
		PlayerType:      s.petTypeOf(petID),
		PlayerCritBonus: critBonus,
		PlayerTrait:     playerTrait,
		PlayerDV:        playerDV,
		EnemyDV:         15,
	}
	if st.PlayerHP == 0 {
		st.PlayerHP = st.PlayerMaxHP
	}
	s.beginPvEFight(c, uid, nick, st, bag)
}

// beginPvEFight 写入状态并推 2503，再推 2301。
// PetFightEntry.setup(2503) 里 new PetFightController() 才会 addCmdListener(GET_PET_INFO)；
// 故 2301 必须紧跟在 2503 之后（同一 TCP 流上客户端先处理 2503 再处理 2301）。
// createSkillBtns（MULTI/ifne）读 PetManager.getPetInfo(catchTime).skillArray。
func (s *Server) beginPvEFight(c *Client, uid uint32, nick string, st *BattleState, bag []store.Pet) {
	s.consumeEnergyBallOnEnter(uid, st.PlayerCatchTime)
	s.battles.set(int64(uid), st)

	petInfoForceEmptySkills = debugFightUIEmptySkills
	playerPets := s.simplePetsForBattle(uid, st, bag)
	out := buildNoteReadyToFight(uid, nick, st, playerPets)
	petInfoForceEmptySkills = false

	s.send(c, 2503, uid, 0, out)
	leadCatch := uint32(0)
	if len(playerPets) > 0 && len(playerPets[0]) >= 56 {
		leadCatch = binary.BigEndian.Uint32(playerPets[0][52:56])
	}
	log.Printf("[CMD] OK     2503 NOTE_READY_TO_FIGHT UID=%d len=%d pets=%d leadCatch=%d fightCatch=%d catchable=%v emptyUI=%v",
		uid, len(out), len(playerPets), leadCatch, st.PlayerCatchTime, st.EnemyCatchable, debugFightUIEmptySkills)
	s.logNoteReadyDump(uid, out, playerPets, st)
	// 紧跟 2503：对战模块已监听，写入 PetManager
	s.syncBagPetsToClientTagged(c, uid, bag, st.PlayerCatchTime, "post-2503")
}

func (s *Server) logNoteReadyDump(uid uint32, body []byte, playerPets [][]byte, st *BattleState) {
	if st == nil {
		return
	}
	for i, pet := range playerPets {
		if len(pet) < 80 {
			log.Printf("[fight-dump] UID=%d simplePet[%d] len=%d SHORT", uid, i, len(pet))
			continue
		}
		pid := binary.BigEndian.Uint32(pet[0:4])
		lv := binary.BigEndian.Uint32(pet[4:8])
		hp := binary.BigEndian.Uint32(pet[8:12])
		maxHP := binary.BigEndian.Uint32(pet[12:16])
		skillNum := binary.BigEndian.Uint32(pet[16:20])
		var sk []uint32
		for j := 0; j < 4; j++ {
			sid := binary.BigEndian.Uint32(pet[20+j*8 : 24+j*8])
			if sid > 0 {
				sk = append(sk, sid)
			}
		}
		ct := binary.BigEndian.Uint32(pet[52:56])
		skin := binary.BigEndian.Uint32(pet[68:72])
		buff := binary.BigEndian.Uint32(pet[76:80])
		log.Printf("[fight-dump] UID=%d simplePet[%d] id=%d lv=%d hp=%d/%d skillNum=%d skills=%v catch=%d skin=%d buff=%d",
			uid, i, pid, lv, hp, maxHP, skillNum, sk, ct, skin, buff)
	}
	log.Printf("[fight-dump] UID=%d battle leadPet=%d leadCatch=%d enemy=%d(%s) combatSkills=%v emptyUI=%v",
		uid, st.PlayerPetID, st.PlayerCatchTime, st.EnemyID, st.EnemyName, skillIDsForLog(st.PlayerSkills), debugFightUIEmptySkills)
}

func (s *Server) syncBagPetsToClientTagged(c *Client, uid uint32, bag []store.Pet, leadCatch uint32, tagPrefix string) {
	if len(bag) == 0 {
		petInfoForceEmptySkills = debugFightUIEmptySkills
		if petBody := s.buildActivePetInfo(uid, s.battles.get(int64(uid))); len(petBody) > 0 {
			s.send(c, 2301, uid, 0, petBody)
			log.Printf("[CMD] OK     2301 GET_PET_INFO UID=%d catch=%d (%s) body=%d emptyUI=%v",
				uid, leadCatch, tagPrefix, len(petBody), debugFightUIEmptySkills)
		}
		petInfoForceEmptySkills = false
		return
	}
	ordered := make([]store.Pet, 0, len(bag))
	for i := range bag {
		if uint32(bag[i].CatchTime) == leadCatch {
			ordered = append(ordered, bag[i])
			break
		}
	}
	for i := range bag {
		if uint32(bag[i].CatchTime) == leadCatch {
			continue
		}
		ordered = append(ordered, bag[i])
	}
	if len(ordered) == 0 {
		ordered = bag
	}
	for i := range ordered {
		p := ordered[i]
		if !debugFightNoSkills && !debugFightUIEmptySkills {
			s.fillPetSkillsUpToFour(&p)
		}
		uiSkills := s.skillsFromPet(&p)
		sk := make([]int, 0, len(uiSkills))
		for _, pair := range uiSkills {
			sk = append(sk, int(pair[0]))
		}
		if debugFightUIEmptySkills {
			sk = nil
		}
		p.Skills = sk
		petInfoForceEmptySkills = debugFightUIEmptySkills
		body := buildPetInfo(&p)
		petInfoForceEmptySkills = false
		s.send(c, 2301, uid, 0, body)
		tag := tagPrefix
		if uint32(p.CatchTime) == leadCatch {
			tag = tagPrefix + " lead"
		}
		skillNum := uint32(0)
		bodyCatch := uint32(0)
		if len(body) >= 136 {
			skillNum = binary.BigEndian.Uint32(body[96:100])
			bodyCatch = binary.BigEndian.Uint32(body[132:136])
		}
		log.Printf("[CMD] OK     2301 GET_PET_INFO UID=%d catch=%d bodyCatch=%d (%s) body=%d skillNum=%d skills=%v emptyUI=%v",
			uid, p.CatchTime, bodyCatch, tag, len(body), skillNum, p.Skills, debugFightUIEmptySkills)
		if bodyCatch != 0 && bodyCatch != uint32(p.CatchTime) {
			log.Printf("[fight-dump] UID=%d 2301 catch mismatch store=%d body@132=%d", uid, p.CatchTime, bodyCatch)
		}
	}
}

// consumeEnergyBallOnEnter 出场扣 1 次能量珠剩余次数。
func (s *Server) consumeEnergyBallOnEnter(uid uint32, catch uint32) {
	if s.cfg.Store == nil || catch == 0 {
		return
	}
	p, err := s.cfg.Store.GetPetByCatchTime(int64(uid), int64(catch))
	if err != nil || p == nil || p.EnergyBallItemID <= 0 || p.EnergyBallLeftCount <= 0 {
		return
	}
	left := p.EnergyBallLeftCount - 1
	if left <= 0 {
		_ = s.cfg.Store.SetPetEnergyBall(int64(uid), int64(catch), 0, 0, 0)
		log.Printf("[battle] energyBall exhausted UID=%d catch=%d item=%d", uid, catch, p.EnergyBallItemID)
		return
	}
	_ = s.cfg.Store.SetPetEnergyBall(int64(uid), int64(catch), p.EnergyBallItemID, left, p.EnergyBallEffectID)
	log.Printf("[battle] energyBall -1 UID=%d catch=%d left=%d", uid, catch, left)
}

// handleFightNpcMonster CMD 2408：点野怪开战（槽位为 2004 压缩下标）。
func (s *Server) handleFightNpcMonster(c *Client, uid uint32, body []byte) {
	slotIdx := 0
	if len(body) >= 4 {
		slotIdx = int(binary.BigEndian.Uint32(body[0:4]))
	}
	mapID := c.MapID
	if mapID <= 0 {
		mapID = defaultMapID
	}
	slots := s.getOgreSlots(int64(uid), mapID)
	compact := compactOgreSlots(slots)
	if len(compact) == 0 {
		log.Printf("[CMD] OK     2408 FIGHT_NPC_MONSTER UID=%d map=%d slot=%d (no wild)", uid, mapID, slotIdx)
		return
	}
	enemyID, enemyLv := compact[0].PetID, compact[0].Level
	enemyName := fallbackEnemyName
	catchable := true
	if slotIdx < 0 || slotIdx >= len(compact) {
		slotIdx = 0
	}
	enemyID = compact[slotIdx].PetID
	enemyLv = compact[slotIdx].Level
	catchable = compact[slotIdx].CanCatch
	if enemyLv <= 0 {
		enemyLv = 5
	}
	if s.cfg.Catalog != nil {
		enemyName = s.cfg.Catalog.PetNameOf(enemyID)
	}
	if enemyName == "" {
		enemyName = fmt.Sprintf("野怪%d", enemyID)
	}
	if nid, nlv, ok := s.tryReplaceWildSpecial(enemyID, enemyLv); ok {
		enemyID, enemyLv = nid, nlv
		catchable = true
		enemyName = fallbackEnemyName
		if s.cfg.Catalog != nil {
			if n := s.cfg.Catalog.PetNameOf(enemyID); n != "" {
				enemyName = n
			}
		}
	}
	ehp, eatk, edef, esa, esd, espd := enemyCombatStats(enemyID, enemyLv)
	if ehp < 1 {
		ehp = 1
	}

	var bag []store.Pet
	if s.cfg.Store != nil {
		bag, _ = s.cfg.Store.ListBagPets(int64(uid))
	}
	p := pickBattlePet(bag)
	petID, lv, name, php, patk, pdef, psa, psd, pspd := petCombatStats(p)
	var critBonus int
	patk, pdef, psa, psd, pspd, critBonus = s.applyEnergyBallBonus(p, patk, pdef, psa, psd, pspd)
	playerTrait := s.applyBattlePetTrait(int64(uid), p, &critBonus)
	skills := s.skillsFromPet(p)
	catch := uint32(0)
	playerDV := 0
	if p != nil {
		catch = uint32(p.CatchTime)
		playerDV = p.DV
	}
	if catch == 0 {
		catch = uint32(task86CatchTm(petID))
	}
	nick := "Seer"
	if s.cfg.Store != nil {
		if u, err := s.cfg.Store.FindByUserID(int64(uid)); err == nil && u != nil && u.Nickname != "" {
			nick = u.Nickname
		}
	}

	enemySkills := s.enemySkillsForPet(enemyID, enemyLv)
	st := &BattleState{
		Active: true, MapID: mapID, IsWildMonster: true,
		EnemyID: enemyID, EnemyLevel: enemyLv, EnemyName: enemyName,
		EnemyHP: uint32(ehp), EnemyMaxHP: uint32(ehp),
		EnemyAtk: eatk, EnemyDef: edef, EnemySpAtk: esa, EnemySpDef: esd, EnemySpd: espd,
		EnemySkills: enemySkills, EnemyCatchable: catchable, EnemyType: s.petTypeOf(enemyID),
		PlayerPetID: petID, PlayerLevel: lv, PlayerName: name, PlayerCatchTime: catch,
		PlayerHP: s.recalledPetHP(int64(uid), catch, uint32(php)), PlayerMaxHP: uint32(php),
		PlayerAtk: patk, PlayerDef: pdef, PlayerSpAtk: psa, PlayerSpDef: psd, PlayerSpd: pspd,
		PlayerSkills: skills, PlayerType: s.petTypeOf(petID), PlayerCritBonus: critBonus,
		PlayerTrait: playerTrait, PlayerDV: playerDV, EnemyDV: 15,
	}
	if st.PlayerHP == 0 {
		st.PlayerHP = st.PlayerMaxHP
	}
	// 开战移除该压缩槽对应的一只怪
	s.removeCompactOgre(int64(uid), mapID, slotIdx)
	s.beginPvEFight(c, uid, nick, st, bag)
	log.Printf("[CMD] OK     2408 FIGHT_NPC_MONSTER UID=%d map=%d slot=%d enemy=%d(%s) lv=%d",
		uid, mapID, slotIdx, enemyID, enemyName, enemyLv)
}

func (s *Server) removeCompactOgre(uid int64, mapID, compactIdx int) {
	slots := s.ogres.get(uid, mapID)
	if slots == nil {
		return
	}
	seen := 0
	for i := range slots {
		if slots[i].PetID == 0 {
			continue
		}
		if seen == compactIdx {
			slots[i] = OgreSlot{}
			s.ogres.set(uid, mapID, slots)
			return
		}
		seen++
	}
}

// simplePetsForBattle：2503 必须带上背包全部出战宠（首发在前）。
// 本客户端 SelectPetPanel.initPanel 用 PetManager.catchTimes 遍历，
// 再 PetFightEntry._petInfoMap.getValue(catchTime)；缺宠会 NPE 黑屏。
// （单宠 len=228 只够 NoteReady 解析，出场建换宠面板仍会炸。）
func (s *Server) simplePetsForBattle(_ uint32, st *BattleState, bag []store.Pet) [][]byte {
	if st == nil {
		return nil
	}
	type item struct {
		catch uint32
		body  []byte
	}
	var list []item
	seen := map[uint32]bool{}
	add := func(petID, level, hp, maxHP, catchTime, skinID uint32, skills [][2]uint32) {
		if catchTime == 0 || seen[catchTime] {
			return
		}
		seen[catchTime] = true
		if skinID == 0 {
			skinID = petID
		}
		list = append(list, item{catch: catchTime, body: buildSimplePetInfo(
			petID, level, hp, maxHP, catchTime, skills, skinID, 0, 0,
		)})
	}
	leadSkin := uint32(st.PlayerPetID)
	for i := range bag {
		if uint32(bag[i].CatchTime) == st.PlayerCatchTime {
			if sid := petSkinID(&bag[i]); sid != 0 {
				leadSkin = sid
			}
			break
		}
	}
	add(uint32(st.PlayerPetID), uint32(st.PlayerLevel), st.PlayerHP, st.PlayerMaxHP, st.PlayerCatchTime, leadSkin, st.PlayerSkills)
	for i := range bag {
		p := &bag[i]
		if uint32(p.CatchTime) == st.PlayerCatchTime {
			continue
		}
		pid, lv, _, php, _, _, _, _, _ := petCombatStats(p)
		sk := s.skillsFromPet(p)
		skin := petSkinID(p)
		if skin == 0 {
			skin = uint32(pid)
		}
		add(uint32(pid), uint32(lv), uint32(php), uint32(php), uint32(p.CatchTime), skin, sk)
	}
	out := make([][]byte, len(list))
	for i := range list {
		out[i] = list[i].body
	}
	return out
}

func buildNoteReadyToFight(uid uint32, nick string, st *BattleState, playerPets [][]byte) []byte {
	if len(playerPets) == 0 {
		playerPets = [][]byte{buildSimplePetInfo(
			uint32(st.PlayerPetID), uint32(st.PlayerLevel),
			st.PlayerHP, st.PlayerMaxHP, st.PlayerCatchTime,
			st.PlayerSkills, 0, 0, 0,
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
	buf.putU32(1)
	buf.write(buildSimplePetInfo(
		uint32(st.EnemyID), uint32(st.EnemyLevel),
		st.EnemyHP, st.EnemyMaxHP, 0,
		st.EnemySkills, 0, 0, 0,
	))
	buf.putU32(0)
	return buf.bytes()
}

// handleReadyToFight CMD 2404。
// 本前端 createSkillBtns（MULTI）读 PetManager.getPetInfo(catchTime).skillArray；
// PetFightController 在 2503 后已监听 2301。空技能隔离已证明：无 2605 → getPetInfo null。
// 故 2404 时先推全背包 2301 写入 PetManager，再推 2504 开场（避免 2504 后才补包来不及/被丢）。
func (s *Server) handleReadyToFight(c *Client, uid uint32) {
	st := s.battles.get(int64(uid))
	if st == nil || !st.Active {
		log.Printf("[CMD] OK     2404 READY_TO_FIGHT UID=%d (no battle) -> 2506 error", uid)
		s.send(c, 2506, uid, 0, buildFightOverInfo(4, 0))
		return
	}
	// 2404 应答 body=userID：本端/对方 FightLoadingView.ok()
	ack2404 := make([]byte, 4)
	binary.BigEndian.PutUint32(ack2404, uid)
	s.send(c, 2404, uid, 0, ack2404)

	if st.isPvP() {
		st.PvPReady = true
		s.battles.set(int64(uid), st)
		// 转发给对手，加载界面显示对方就绪
		if oc := s.clientOf(st.OpponentUID); oc != nil {
			s.send(oc, 2404, uid, 0, ack2404)
		}
	} else {
		s.consumeAutoFightOnEnter(int64(uid), st)
	}

	var bag []store.Pet
	if s.cfg.Store != nil {
		bag, _ = s.cfg.Store.ListBagPets(int64(uid))
	}
	if st.IsGrandMelee {
		if mp := s.grandMeleePlayerPets(int64(uid)); len(mp) > 0 {
			bag = mp
		}
	}
	s.syncBagPetsToClientTagged(c, uid, bag, st.PlayerCatchTime, "pre-2504")

	oppUID := uint32(0)
	oppCatch := uint32(0)
	enemyID, enemyLv := st.EnemyID, st.EnemyLevel
	enemyName := st.EnemyName
	enemyHP, enemyMax := st.EnemyHP, st.EnemyMaxHP
	enemyStages := st.EnemyStages
	if st.isPvP() {
		oppUID = uint32(st.OpponentUID)
		// 以对手当前 BattleState 为准刷新敌方段，避免记忆血/截断导致 111/100、模型错绑
		if opp := s.battles.get(st.OpponentUID); opp != nil && opp.Active {
			oppCatch = opp.PlayerCatchTime
			enemyID, enemyLv = opp.PlayerPetID, opp.PlayerLevel
			enemyName = opp.PlayerName
			enemyHP, enemyMax = opp.PlayerHP, opp.PlayerMaxHP
			enemyStages = opp.PlayerStages
			st.EnemyID, st.EnemyLevel, st.EnemyName = enemyID, enemyLv, enemyName
			st.EnemyHP, st.EnemyMaxHP, st.EnemyCatchTime = enemyHP, enemyMax, oppCatch
			st.EnemyStages = enemyStages
			s.battles.set(int64(uid), st)
		} else {
			oppCatch = st.EnemyCatchTime
		}
	}
	catchable := uint32(0)
	if st.EnemyCatchable {
		catchable = 1
	}
	player := buildFightPetInfo(uid, st.PlayerPetID, st.PlayerName, st.PlayerCatchTime,
		st.PlayerHP, st.PlayerMaxHP, uint32(st.PlayerLevel), 0, encodeBattleLv(st.PlayerStages))
	enemy := buildFightPetInfo(oppUID, enemyID, enemyName, oppCatch,
		enemyHP, enemyMax, uint32(enemyLv), catchable, encodeBattleLv(enemyStages))
	body := make([]byte, 4+len(player)+len(enemy))
	binary.BigEndian.PutUint32(body[0:4], 0) // isCanAuto
	copy(body[4:], player)
	copy(body[4+len(player):], enemy)

	log.Printf("[CMD] OK     2404 READY_TO_FIGHT UID=%d -> 2504 player=%d catch=%d enemy=%d enemyCatch=%d hp=%d/%d pvp=%v body=%d",
		uid, st.PlayerPetID, st.PlayerCatchTime, enemyID, oppCatch, enemyHP, enemyMax, st.isPvP(), len(body))
	s.logFightStartDump(uid, body)
	s.send(c, 2504, uid, 0, body)
	log.Printf("[fight-dump] UID=%d pre-2504 2301 done; expect ITEM_LIST/2605 after openning", uid)
}

func (s *Server) logFightStartDump(uid uint32, body []byte) {
	if len(body) < 4+50 {
		log.Printf("[fight-dump] UID=%d 2504 body too short len=%d", uid, len(body))
		return
	}
	isCanAuto := binary.BigEndian.Uint32(body[0:4])
	dump := func(tag string, off int) {
		if off+50 > len(body) {
			return
		}
		p := body[off : off+50]
		userID := binary.BigEndian.Uint32(p[0:4])
		petID := binary.BigEndian.Uint32(p[4:8])
		name := packet.ReadFixedString(p[8:24])
		ct := binary.BigEndian.Uint32(p[24:28])
		hp := binary.BigEndian.Uint32(p[28:32])
		maxHP := binary.BigEndian.Uint32(p[32:36])
		lv := binary.BigEndian.Uint32(p[36:40])
		catchable := binary.BigEndian.Uint32(p[40:44])
		log.Printf("[fight-dump] UID=%d 2504.%s userID=%d petID=%d name=%q catch=%d hp=%d/%d lv=%d catchable=%d",
			uid, tag, userID, petID, name, ct, hp, maxHP, lv, catchable)
	}
	log.Printf("[fight-dump] UID=%d 2504 isCanAuto=%d hex=%x", uid, isCanAuto, body)
	dump("player", 4)
	dump("enemy", 4+50)
}

func skillIDsForLog(skills [][2]uint32) []uint32 {
	out := make([]uint32, 0, len(skills))
	for _, sk := range skills {
		if sk[0] > 0 {
			out = append(out, sk[0])
		}
	}
	return out
}

// buildActivePetInfo 构造出战精灵完整 PetInfo；库中无技能时用种族默认技能兜底。
func (s *Server) buildActivePetInfo(uid uint32, st *BattleState) []byte {
	if st == nil {
		return nil
	}
	var p *store.Pet
	if s.cfg.Store != nil && st.PlayerCatchTime > 0 {
		p, _ = s.cfg.Store.GetPetByCatchTime(int64(uid), int64(st.PlayerCatchTime))
	}
	if p == nil {
		// 内存态兜底，保证技能面板至少有技能
		skills := make([]int, 0, len(st.PlayerSkills))
		for _, sk := range st.PlayerSkills {
			if sk[0] > 0 {
				skills = append(skills, int(sk[0]))
			}
		}
		p = &store.Pet{
			UserID:    int64(uid),
			CatchTime: int64(st.PlayerCatchTime),
			PetID:     st.PlayerPetID,
			Name:      st.PlayerName,
			Level:     st.PlayerLevel,
			DV:        20,
			InBag:     true,
			Skills:    skills,
		}
	}
	// 开战同步：只用已确定的出战技能，禁止 fill 补槽改写包体技能列表。
	skills := make([]int, 0, len(st.PlayerSkills))
	for _, sk := range st.PlayerSkills {
		if sk[0] > 0 {
			skills = append(skills, int(sk[0]))
		}
	}
	if len(skills) > 0 {
		p.Skills = skills
	} else {
		ensurePetSkills(p)
	}
	return buildPetInfo(p)
}

// handleUseSkill CMD 2405：多回合结算（PvE 立即双方；PvP 等双方出手）。
func (s *Server) handleUseSkill(c *Client, uid uint32, body []byte) {
	s.send(c, 2405, uid, 0, nil)

	st := s.battles.get(int64(uid))
	if st == nil || !st.Active {
		log.Printf("[CMD] OK     2405 USE_SKILL UID=%d (no battle)", uid)
		return
	}
	skillID := uint32(0)
	if len(body) >= 4 {
		skillID = binary.BigEndian.Uint32(body[0:4])
	}
	if skillID == 0 && len(st.PlayerSkills) > 0 {
		skillID = st.PlayerSkills[0][0]
	}
	if !hasSkillPP(st.PlayerSkills, skillID) {
		skillID = firstSkillWithPP(st.PlayerSkills)
	}

	if st.isPvP() {
		s.handlePvPUseSkill(c, uid, st, skillID)
		return
	}
	s.resolvePvESkillTurn(c, uid, st, skillID)
}

func (s *Server) handlePvPUseSkill(c *Client, uid uint32, st *BattleState, skillID uint32) {
	switch s.pvpSubmit(uid, st, pvpActSkill, skillID, 0, 0) {
	case pvpWait, pvpDone:
		return
	case pvpContinue:
		opp := s.battles.get(st.OpponentUID)
		if opp == nil {
			return
		}
		// 以当前提交者视角取双方最新状态
		st = s.battles.get(int64(uid))
		s.resolvePvPSkillRound(uid, st, uint32(st.OpponentUID), opp)
	}
}

func (s *Server) finishPvP(uidA uint32, a *BattleState, uidB uint32, b *BattleState, winner uint32) {
	s.finishPvPWithReason(uidA, a, uidB, b, winner, fightReasonNormal)
}

func (s *Server) finishPvPWithReason(uidA uint32, a *BattleState, uidB uint32, b *BattleState, winner, reason uint32) {
	ca, cb := s.clientOf(int64(uidA)), s.clientOf(int64(uidB))
	if ca != nil {
		s.finishFight(ca, uidA, a, reason, winner)
	} else {
		s.battles.clear(int64(uidA))
	}
	// finishFight clears A; restore B briefly for its finish
	if b != nil {
		b.Active = true
		s.battles.set(int64(uidB), b)
	}
	if cb != nil {
		s.finishFight(cb, uidB, b, reason, winner)
	} else {
		s.battles.clear(int64(uidB))
	}
}

func (s *Server) resolvePvESkillTurn(c *Client, uid uint32, st *BattleState, skillID uint32) {
	applyPuniRoundStart(st)

	playerFromCharge, enemyFromCharge := false, false
	if charged := takeChargeSkill(st, true); charged != 0 {
		skillID = charged
		playerFromCharge = true
	}
	decSkillPP(st.PlayerSkills, skillID)

	enemySkill := s.pickEnemyBattleSkill(st)
	if chargedE := takeChargeSkill(st, false); chargedE != 0 {
		enemySkill = chargedE
		enemyFromCharge = true
	}
	if !enemyHasInfinitePP(st) {
		decSkillPP(st.EnemySkills, enemySkill)
	}

	playerCtrlAtStart := snapshotControlStatus(st.PlayerStatus)
	enemyCtrlAtStart := snapshotControlStatus(st.EnemyStatus)
	playerSkip := consumeSkipStatus(&st.PlayerStatus)
	enemySkip := consumeSkipStatus(&st.EnemyStatus)

	// 先手：速度比较；Boss 先制加成（雷伊/盖亚等 +6）简化为敌方先手；83 强制先手
	playerFirst := st.stagedSpd(true) >= st.stagedSpd(false)
	if forceFirst, forceSecond := priorityFromBuff(&st.PlayerBuff, &st.EnemyBuff); forceFirst {
		playerFirst = true
	} else if forceSecond {
		playerFirst = false
	}
	if bossPriorityBonusBattle(st) > 0 {
		playerFirst = false
	}

	advanceConsecutiveSkill(&st.PlayerConsecSkillID, &st.PlayerConsecSkillCount, skillID)
	advanceConsecutiveSkill(&st.EnemyConsecSkillID, &st.EnemyConsecSkillCount, enemySkill)

	pDef := s.skillDef(int(skillID))
	eDef := s.skillDef(int(enemySkill))

	// SideEffect 17：蓄力回合不出手；释放回合不再重新蓄力
	if !playerSkip && !playerFromCharge && beginCharge(st, skillID, pDef, true) {
		playerSkip = true
	}
	if !enemySkip && !enemyFromCharge && beginCharge(st, enemySkill, eDef, false) {
		enemySkip = true
	}
	// SideEffect 52：技能失效
	if !playerSkip && consumeSkillFail(st, true) {
		playerSkip = true
	}
	if !enemySkip && consumeSkillFail(st, false) {
		enemySkip = true
	}
	// SideEffect 478：对手属性技无效
	if !playerSkip && attrSkillBlocked(st, true, pDef) {
		playerSkip = true
	}
	if !enemySkip && attrSkillBlocked(st, false, eDef) {
		enemySkip = true
	}

	playerHit := !playerSkip && (mustHitFromBuff(&st.PlayerBuff) || s.checkSkillHitTrait(skillID, 0, 0, st.PlayerTrait, 0))
	enemyHit := !enemySkip && (mustHitFromBuff(&st.EnemyBuff) || s.checkSkillHitTrait(enemySkill, 0, 0, 0, st.PlayerTrait))

	// SideEffect 7：对方体力不高于自身则无法命中
	if playerHit {
		if _, ok := sameLifeDamage(pDef, st.PlayerHP, st.EnemyHP); ok && st.EnemyHP <= st.PlayerHP {
			playerHit = false
		}
	}
	if enemyHit {
		if _, ok := sameLifeDamage(eDef, st.EnemyHP, st.PlayerHP); ok && st.PlayerHP <= st.EnemyHP {
			enemyHit = false
		}
	}
	// SideEffect 78/86：物理/特殊攻击对有护盾方必 miss
	if playerHit && (physMissForced(&st.EnemyBuff, pDef) || specMissForced(&st.EnemyBuff, pDef)) {
		playerHit = false
	}
	if enemyHit && (physMissForced(&st.PlayerBuff, eDef) || specMissForced(&st.PlayerBuff, eDef)) {
		enemyHit = false
	}
	if !playerHit && !playerSkip {
		applyOnDodgeBoost(st, false)
	}
	if !enemyHit && !enemySkip {
		applyOnDodgeBoost(st, true)
	}

	pHits, eHits := 1, 1
	if playerHit {
		pHits = sideEffectHitCount(pDef)
	}
	if enemyHit {
		eHits = sideEffectHitCount(eDef)
	}

	playerDmg := uint32(0)
	enemyDmg := uint32(0)
	if playerHit {
		if dmg80, loss, ok := sacrificeHalfEqualDamage(pDef, st.PlayerHP, st.PlayerMaxHP); ok {
			playerDmg = dmg80
			_ = applyDamage(&st.PlayerHP, loss)
		} else if dmg112, loss, ok := sacrificeAllForFlat(pDef, st.PlayerHP); ok {
			playerDmg = dmg112
			_ = applyDamage(&st.PlayerHP, loss)
		} else if dmg7, ok := sameLifeDamage(pDef, st.PlayerHP, st.EnemyHP); ok {
			playerDmg = dmg7
		} else {
			foeDef, foeSpDef := defStatsForSkill(st, pDef, false)
			playerDmg = s.damageWithSkillAdj(skillID, st.PlayerLevel,
				st.stagedAtk(true), foeDef, st.stagedSpAtk(true), foeSpDef,
				st.PlayerType, st.EnemyType, skillPowerAdj{
					FoeHP: st.EnemyHP, FoeMaxHP: st.EnemyMaxHP,
					SelfHP: st.PlayerHP, SelfMaxHP: st.PlayerMaxHP,
					GoingFirst: playerFirst, ConsecCount: st.PlayerConsecSkillCount,
					FoeStages: &st.EnemyStages, SelfDV: st.PlayerDV,
				})
			playerDmg *= uint32(pHits)
			// 28/29/93 粉伤不并入 lostHP，见 apply 段
			if ohko := sideEffectOHKO(pDef, st.EnemyHP); ohko > 0 {
				playerDmg = ohko
			}
			var instant bool
			playerDmg, instant = applyTraitOutgoingDamage(st.PlayerTrait, pDef, playerDmg)
			if instant && st.EnemyHP > 0 {
				playerDmg = st.EnemyHP
			}
			if mul := bossDamageTakenMultiplier(st.EnemyID); mul > 1 && playerDmg > 0 {
				playerDmg *= uint32(mul)
			}
			playerDmg = applyOutgoingDamageBuff(&st.PlayerBuff, pDef, playerDmg)
			playerDmg = applyIncomingDamageBuff(&st.EnemyBuff, pDef, playerDmg)
			playerDmg = statusPowerBoost(pDef, &st.PlayerStatus, playerDmg)
			playerDmg = foeStatusDamageMul(pDef, &st.EnemyStatus, playerDmg)
			playerDmg = lowHPDamageScale(pDef, st.PlayerHP, st.PlayerMaxHP, playerDmg)
			playerDmg = sideEffectChanceMulDamage(pDef, playerDmg)
			foeGender := 0
			if s.cfg.Catalog != nil {
				foeGender = s.cfg.Catalog.PetGender(st.EnemyID)
			}
			playerDmg = applyHighDamageSideEffects(pDef, playerDmg, &st.EnemyStatus, st.PlayerType, foeGender, playerFirst)
			playerDmg = maleDamageMulFromBuff(&st.PlayerBuff, foeGender, playerDmg)
			playerDmg = applyMoreDamageSideEffects(pDef, playerDmg, st.EnemyHP, &st.EnemyStatus, &st.EnemyStages, playerFirst)
			skType := 0
			if pDef != nil {
				skType = pDef.Type
			}
			playerDmg = applyFreq2DamageSideEffects(pDef, playerDmg, st.PlayerHP, st.PlayerMaxHP, skType, st.EnemyType)
			playerDmg = applyFreq3DamageSideEffects(pDef, playerDmg, st.PlayerHP, st.EnemyHP,
				st.PlayerLevel, st.stagedSpd(true), st.PlayerType, st.EnemyType, st.PlayerConsecSkillCount)
			playerDmg = applyFreq4DamageSideEffects(pDef, playerDmg, st.PlayerHP, &st.PlayerStages, st.EnemyDef, st.PlayerConsecSkillCount, &st.EnemyStatus)
			playerDmg = applyFreq5DamageSideEffects(pDef, playerDmg, st.EnemyHP, &st.PlayerEffect795Uses)
			playerDmg = applyLeaveOneHP(pDef, st.EnemyHP, playerDmg)
			playerDmg = applyEndureLeaveOne(&st.EnemyBuff, st.EnemyHP, playerDmg)
			if playerFromCharge && playerDmg > 0 {
				playerDmg *= 2
			}
		}
		if cd := sideEffectCounterDamage(pDef, st.PlayerLastTaken); cd > 0 {
			playerDmg = cd
		}
	}
	if enemyHit {
		if dmg80, loss, ok := sacrificeHalfEqualDamage(eDef, st.EnemyHP, st.EnemyMaxHP); ok {
			enemyDmg = dmg80
			_ = applyDamage(&st.EnemyHP, loss)
		} else if dmg112, loss, ok := sacrificeAllForFlat(eDef, st.EnemyHP); ok {
			enemyDmg = dmg112
			_ = applyDamage(&st.EnemyHP, loss)
		} else if dmg7, ok := sameLifeDamage(eDef, st.EnemyHP, st.PlayerHP); ok {
			enemyDmg = dmg7
		} else {
			foeDef, foeSpDef := defStatsForSkill(st, eDef, true)
			enemyDmg = s.damageWithSkillAdj(enemySkill, st.EnemyLevel,
				st.stagedAtk(false), foeDef, st.stagedSpAtk(false), foeSpDef,
				st.EnemyType, st.PlayerType, skillPowerAdj{
					FoeHP: st.PlayerHP, FoeMaxHP: st.PlayerMaxHP,
					SelfHP: st.EnemyHP, SelfMaxHP: st.EnemyMaxHP,
					GoingFirst: !playerFirst, ConsecCount: st.EnemyConsecSkillCount,
					FoeStages: &st.PlayerStages, SelfDV: st.EnemyDV,
				})
			enemyDmg *= uint32(eHits)
			// 28/29/93 粉伤不并入 lostHP
			if ohko := sideEffectOHKO(eDef, st.PlayerHP); ohko > 0 {
				enemyDmg = ohko
			}
			enemyDmg = applyOutgoingDamageBuff(&st.EnemyBuff, eDef, enemyDmg)
			enemyDmg = applyIncomingDamageBuff(&st.PlayerBuff, eDef, enemyDmg)
			enemyDmg = statusPowerBoost(eDef, &st.EnemyStatus, enemyDmg)
			enemyDmg = foeStatusDamageMul(eDef, &st.PlayerStatus, enemyDmg)
			enemyDmg = lowHPDamageScale(eDef, st.EnemyHP, st.EnemyMaxHP, enemyDmg)
			enemyDmg = sideEffectChanceMulDamage(eDef, enemyDmg)
			foeGender := 0
			if s.cfg.Catalog != nil {
				foeGender = s.cfg.Catalog.PetGender(st.PlayerPetID)
			}
			enemyDmg = applyHighDamageSideEffects(eDef, enemyDmg, &st.PlayerStatus, st.EnemyType, foeGender, !playerFirst)
			enemyDmg = maleDamageMulFromBuff(&st.EnemyBuff, foeGender, enemyDmg)
			enemyDmg = applyMoreDamageSideEffects(eDef, enemyDmg, st.PlayerHP, &st.PlayerStatus, &st.PlayerStages, !playerFirst)
			skType := 0
			if eDef != nil {
				skType = eDef.Type
			}
			enemyDmg = applyFreq2DamageSideEffects(eDef, enemyDmg, st.EnemyHP, st.EnemyMaxHP, skType, st.PlayerType)
			enemyDmg = applyFreq3DamageSideEffects(eDef, enemyDmg, st.EnemyHP, st.PlayerHP,
				st.EnemyLevel, st.stagedSpd(false), st.EnemyType, st.PlayerType, st.EnemyConsecSkillCount)
			enemyDmg = applyFreq4DamageSideEffects(eDef, enemyDmg, st.EnemyHP, &st.EnemyStages, st.PlayerDef, st.EnemyConsecSkillCount, &st.PlayerStatus)
			enemyDmg = applyFreq5DamageSideEffects(eDef, enemyDmg, st.PlayerHP, &st.EnemyEffect795Uses)
			enemyDmg = applyLeaveOneHP(eDef, st.PlayerHP, enemyDmg)
			enemyDmg = applyEndureLeaveOne(&st.PlayerBuff, st.PlayerHP, enemyDmg)
			if enemyFromCharge && enemyDmg > 0 {
				enemyDmg *= 2
			}
		}
		if cd := sideEffectCounterDamage(eDef, st.EnemyLastTaken); cd > 0 {
			enemyDmg = cd
		}
	}
	pCritExtra := critExtraWithRounds(st.PlayerCritBonus, st.PlayerCritBonusRounds) + sleepCritExtra(pDef, st.EnemyStatus.Sleep) + critExtraFromStack(&st.PlayerBuff)
	eCritExtra := critExtraWithRounds(0, st.EnemyCritBonusRounds) + sleepCritExtra(eDef, st.PlayerStatus.Sleep) + critExtraFromStack(&st.EnemyBuff)
	pCrit := playerHit && (mustCritFromBuff(&st.PlayerBuff) || mustCritFromSideEffect193(pDef, &st.EnemyStatus) || mustCritFromAnyStatus(pDef, &st.EnemyStatus) || rollPlayerCrit(pCritExtra))
	eCrit := enemyHit && (mustCritFromBuff(&st.EnemyBuff) || mustCritFromSideEffect193(eDef, &st.PlayerStatus) || mustCritFromAnyStatus(eDef, &st.PlayerStatus) || rollPlayerCrit(eCritExtra))
	if pCrit && !skillHasSideEffect(pDef, 34) {
		playerDmg = playerDmg * 3 / 2
		if playerDmg < 1 {
			playerDmg = 1
		}
	}
	if eCrit && !skillHasSideEffect(eDef, 34) {
		enemyDmg = enemyDmg * 3 / 2
		if enemyDmg < 1 {
			enemyDmg = 1
		}
	}
	if enemyHit && enemyDmg > 0 {
		enemyDmg = applyTraitIncomingDamage(st.PlayerTrait, st.PlayerHP, enemyDmg)
	}
	enemyDmg = applyBossHalfHPOneShot(st, enemyDmg, enemyHit)
	enemyDmg = applyPuniSuperLowHPOneShot(st, enemyDmg, enemyHit)

	// 谱尼封印：虚无 miss / 元素门控 / 能量反噬 / 永恒减半
	if playerHit {
		playerDmg, playerHit = applyPuniOnPlayerSkillHit(st, skillID, pDef, playerDmg, playerHit)
		playerDmg = applyPuniEternalHalf(st, playerDmg)
	}

	var (
		pLost, eLost         uint32
		pAtkTimes, eAtkTimes uint32
		pSkillOut, eSkillOut uint32
	)
	pSkillOut, eSkillOut = skillID, enemySkill
	pAtkTimes, eAtkTimes = uint32(pHits), uint32(eHits)
	if playerSkip || !playerHit {
		pAtkTimes, playerDmg = 0, 0
		if playerSkip {
			pSkillOut = 0
		} else if skillHasSideEffect(pDef, 72) {
			st.PlayerHP = 0
		}
	}
	if enemySkip || !enemyHit {
		eAtkTimes, enemyDmg = 0, 0
		if enemySkip {
			eSkillOut = 0
		} else if skillHasSideEffect(eDef, 72) {
			st.EnemyHP = 0
		}
	}

	if playerFirst {
		if pAtkTimes > 0 {
			foeHPBefore := st.EnemyHP
			pink := sideEffectPinkDamage(pDef, foeHPBefore)
			pLost = playerDmg
			actual := applyDamage(&st.EnemyHP, playerDmg)
			applyPinkDamage(&st.EnemyHP, pink)
			noteLastDamageTaken(st, false, actual)
			applyReflectDamage(&st.EnemyBuff, actual, &st.PlayerHP)
			tryCounterDoubleReflect(&st.EnemyBuff, actual, &st.PlayerHP)
			tryOnHitStatus(&st.EnemyBuff, pDef, &st.PlayerStatus)
			applyOnHurtStageBoost(&st.EnemyBuff, &st.EnemyStages)
			applyOnHurtDefenderBuffs(st, false, actual)
			applyVampOnDamage(&st.PlayerBuff, actual, &st.PlayerHP, &st.PlayerMaxHP)
			if heal := traitDrainHeal(st.PlayerTrait, actual); heal > 0 {
				st.PlayerHP += heal
				if st.PlayerHP > st.PlayerMaxHP {
					st.PlayerHP = st.PlayerMaxHP
				}
			}
			s.applySkillSideEffects(st, skillID, actual, true, true)
			tryInvalidateSkill(st, pDef, true, true)
			armDoom(st, pDef, true)
			applyFirstStrikeReflect(st, pDef, true, true)
			applyOnKOEffects(st, pDef, true, foeHPBefore)
			applySacrificeEffects(st, pDef, true)
		}
		// 先手挂上控场后：后手本回合无法出手（不清除，避免图标只闪 1 回合）
		if !enemySkip && newlyControlledAfterOpponent(st.EnemyStatus, enemyCtrlAtStart) {
			enemySkip = true
			eAtkTimes, eLost, eSkillOut = 0, 0, 0
		}
		if st.EnemyHP == 0 || st.PlayerHP == 0 {
			eAtkTimes, eLost, eSkillOut = 0, 0, 0
		} else if eAtkTimes > 0 {
			foeHPBefore := st.PlayerHP
			// 后手反击：用本回合刚记下的受伤重算
			if skillHasSideEffect(eDef, 34) {
				if cd := sideEffectCounterDamage(eDef, st.EnemyLastTaken); cd > 0 {
					enemyDmg = cd
				}
			}
			pink := sideEffectPinkDamage(eDef, foeHPBefore)
			eLost = enemyDmg
			actual := applyDamage(&st.PlayerHP, enemyDmg)
			applyPinkDamage(&st.PlayerHP, pink)
			noteLastDamageTaken(st, true, actual)
			applyReflectDamage(&st.PlayerBuff, actual, &st.EnemyHP)
			tryCounterDoubleReflect(&st.PlayerBuff, actual, &st.EnemyHP)
			tryOnHitStatus(&st.PlayerBuff, eDef, &st.EnemyStatus)
			applyOnHurtStageBoost(&st.PlayerBuff, &st.PlayerStages)
			applyOnHurtDefenderBuffs(st, true, actual)
			applyVampOnDamage(&st.EnemyBuff, actual, &st.EnemyHP, &st.EnemyMaxHP)
			s.applySkillSideEffects(st, enemySkill, actual, false, false)
			tryInvalidateSkill(st, eDef, false, false)
			armDoom(st, eDef, false)
			applyOnKOEffects(st, eDef, false, foeHPBefore)
			applySacrificeEffects(st, eDef, false)
			applyPvEPlayerTraitOnHit(st, enemySkill, actual, eDef)
		}
	} else {
		if eAtkTimes > 0 {
			foeHPBefore := st.PlayerHP
			pink := sideEffectPinkDamage(eDef, foeHPBefore)
			eLost = enemyDmg
			actual := applyDamage(&st.PlayerHP, enemyDmg)
			applyPinkDamage(&st.PlayerHP, pink)
			noteLastDamageTaken(st, true, actual)
			applyReflectDamage(&st.PlayerBuff, actual, &st.EnemyHP)
			tryCounterDoubleReflect(&st.PlayerBuff, actual, &st.EnemyHP)
			tryOnHitStatus(&st.PlayerBuff, eDef, &st.EnemyStatus)
			applyOnHurtStageBoost(&st.PlayerBuff, &st.PlayerStages)
			applyOnHurtDefenderBuffs(st, true, actual)
			applyVampOnDamage(&st.EnemyBuff, actual, &st.EnemyHP, &st.EnemyMaxHP)
			s.applySkillSideEffects(st, enemySkill, actual, false, true)
			tryInvalidateSkill(st, eDef, false, true)
			armDoom(st, eDef, false)
			applyFirstStrikeReflect(st, eDef, false, true)
			applyOnKOEffects(st, eDef, false, foeHPBefore)
			applySacrificeEffects(st, eDef, false)
			applyPvEPlayerTraitOnHit(st, enemySkill, actual, eDef)
		}
		if !playerSkip && newlyControlledAfterOpponent(st.PlayerStatus, playerCtrlAtStart) {
			playerSkip = true
			pAtkTimes, pLost, pSkillOut = 0, 0, 0
		}
		if st.PlayerHP == 0 || st.EnemyHP == 0 {
			pAtkTimes, pLost, pSkillOut = 0, 0, 0
		} else if pAtkTimes > 0 {
			foeHPBefore := st.EnemyHP
			if skillHasSideEffect(pDef, 34) {
				if cd := sideEffectCounterDamage(pDef, st.PlayerLastTaken); cd > 0 {
					playerDmg = cd
				}
			}
			pink := sideEffectPinkDamage(pDef, foeHPBefore)
			pLost = playerDmg
			actual := applyDamage(&st.EnemyHP, playerDmg)
			applyPinkDamage(&st.EnemyHP, pink)
			noteLastDamageTaken(st, false, actual)
			applyReflectDamage(&st.EnemyBuff, actual, &st.PlayerHP)
			tryCounterDoubleReflect(&st.EnemyBuff, actual, &st.PlayerHP)
			tryOnHitStatus(&st.EnemyBuff, pDef, &st.PlayerStatus)
			applyOnHurtStageBoost(&st.EnemyBuff, &st.EnemyStages)
			applyOnHurtDefenderBuffs(st, false, actual)
			applyVampOnDamage(&st.PlayerBuff, actual, &st.PlayerHP, &st.PlayerMaxHP)
			if heal := traitDrainHeal(st.PlayerTrait, actual); heal > 0 {
				st.PlayerHP += heal
				if st.PlayerHP > st.PlayerMaxHP {
					st.PlayerHP = st.PlayerMaxHP
				}
			}
			s.applySkillSideEffects(st, skillID, actual, true, false)
			tryInvalidateSkill(st, pDef, true, false)
			armDoom(st, pDef, true)
			applyOnKOEffects(st, pDef, true, foeHPBefore)
			applySacrificeEffects(st, pDef, true)
		}
	}
	st.PlayerHP = tryTraitRecoverOnLowHP(st.PlayerTrait, st.PlayerHP, st.PlayerMaxHP)
	tickStatusDamage(st)
	tickBattleBuffEffects(st)
	tickCritBonusRounds(st)
	tickDoom(st)

	pCritFlag, eCritFlag := uint32(0), uint32(0)
	if pCrit && pAtkTimes > 0 {
		pCritFlag = 1
	}
	if eCrit && eAtkTimes > 0 {
		eCritFlag = 1
	}

	if pAtkTimes > 0 && pSkillOut > 0 {
		if st.PlayerUsedSkills == nil {
			st.PlayerUsedSkills = make(map[uint32]bool)
		}
		st.PlayerUsedSkills[pSkillOut] = true
		st.LastPlayerSkillID = pSkillOut
		st.LastHitWasCrit = pCrit
	}

	playerAv := buildAttackValueFromState(uid, pSkillOut, pAtkTimes, pLost, 0, int32(st.PlayerHP), st.PlayerMaxHP, 0, pCritFlag, 0, st, true, st.PlayerSkills)
	enemyAv := buildAttackValueFromState(0, eSkillOut, eAtkTimes, eLost, 0, int32(st.EnemyHP), st.EnemyMaxHP, 0, eCritFlag, 0, st, false, nil)
	decrementBattleBuffRounds(st)
	tickDelayedFullHeal(st)
	tickGrowAtkSpd(st)
	tickFoeStageDot(st)
	tickSelfStageGrow(st)
	tickCondDot439(st)
	st.PlayerEffect795Uses = 0
	st.EnemyEffect795Uses = 0

	var out []byte
	if playerFirst {
		out = append(playerAv, enemyAv...)
	} else {
		out = append(enemyAv, playerAv...)
	}
	st.Round++
	log.Printf("[CMD] OK     2405 USE_SKILL UID=%d skill=%d dmg=%d/%d hp=%d/%d enemy=%d/%d first=%v round=%d crit=%v/%v hit=%v/%v trait=%d",
		uid, skillID, playerDmg, enemyDmg, st.PlayerHP, st.PlayerMaxHP, st.EnemyHP, st.EnemyMaxHP, playerFirst, st.Round, pCrit, eCrit, playerHit, enemyHit, st.PlayerTrait)
	s.send(c, 2505, uid, 0, out)

	if st.EnemyHP == 0 {
		if tryPuniLifeSwitch(st) {
			s.battles.set(int64(uid), st)
			s.sendEnemyLifeSwitch2407(c, uid, st)
			log.Printf("[CMD] OK     2405 USE_SKILL UID=%d puni lifeSwitch cur=%d/%d hp=%d -> 2407",
				uid, st.PuniCurrentLife, st.PuniTotalLives, st.EnemyHP)
			return
		}
		if s.tryGrandMeleeEnemySwitch(c, uid, st) {
			return
		}
		s.finishFight(c, uid, st, fightReasonNormal, uid)
		return
	}
	if st.PlayerHP == 0 {
		st.markPetFainted(st.PlayerCatchTime)
		if !st.ForceSinglePet && s.hasOtherLivingPet(uid, st) {
			s.battles.set(int64(uid), st)
			log.Printf("[CMD] OK     2405 USE_SKILL UID=%d faint -> wait CHANGE_PET", uid)
			return
		}
		s.finishFight(c, uid, st, fightReasonNormal, 0)
		return
	}
	if s.checkWildSpecialEscape(c, uid, st) {
		return
	}
	s.battles.set(int64(uid), st)
}

// finishFight 参考服：胜方先 2508 再 2506；PvE 结束前不发 2301（避免卡住）。
func (s *Server) finishFight(c *Client, uid uint32, st *BattleState, reason, winner uint32) {
	if st == nil {
		return
	}
	// 会话内记住出战 HP（败北 0 血也记住，下次开战回满）；大乱斗临时宠不记
	if !st.IsGrandMelee && st.PlayerCatchTime > 0 {
		if st.PlayerHP == 0 {
			s.forgetPetHP(int64(uid), st.PlayerCatchTime)
		} else {
			s.rememberPetHP(int64(uid), st.PlayerCatchTime, st.PlayerHP)
		}
	}
	if winner == uid {
		if !st.isPvP() {
			if st.RewardCoins445 > 0 && s.cfg.Store != nil {
				_ = s.cfg.Store.AddCoins(int64(uid), st.RewardCoins445)
			}
			// 大乱斗临时精灵不给种族经验/学习力
			var skillNote []byte
			if !st.IsGrandMelee {
				skillNote = s.grantBattleWinReward(uid, st)
				s.grantWildBattleDrops(c, uid, st)
				s.grantPuniFragmentReward(c, uid, st)
				s.grantSPTFirstKillReward(c, uid, st)
				s.onAchieveBattleWin(uid, st)
				s.applyTowerWinProgress(int64(uid), st.FightKind)
				s.grantBraveTowerDailyReward(c, uid, st)
				s.grantMantisWeeklyReward(c, uid, st)
				s.grantLanlanHonorReward(c, uid, st)
				s.applyLeiyiEnergyTrainOnWin(uid, st)
				s.noteLeiyiSkillTrainOnWin(c, uid, st)
				s.unlockGaiyaEffectOnWin(c, uid, st)
				s.onGaiyaAppearWin(c, uid, st)
				s.markDailyBossChallengeLimit(uid, st)
			}
			s.grantGrandMeleeWinProgress(c, uid, st)
			if len(skillNote) > 0 {
				s.send(c, 2507, uid, 0, skillNote)
				log.Printf("[CMD] OK     2507 NOTE_UPDATE_SKILL UID=%d catch=%d body=%d", uid, st.PlayerCatchTime, len(skillNote))
			}
			if !st.IsGrandMelee {
				if prop := s.buildActivePetProp(uid, st); len(prop) > 0 {
					s.send(c, 2508, uid, 0, prop)
					log.Printf("[CMD] OK     2508 NOTE_UPDATE_PROP UID=%d catch=%d", uid, st.PlayerCatchTime)
				}
			}
		} else if s.cfg.Store != nil {
			_ = s.cfg.Store.AddCoins(int64(uid), 20)
		}
	}
	// PvE 结束（胜负皆可）：清野怪等定时刷新；大乱斗/塔类不刷图
	if !st.isPvP() && st.FightKind == fightKindNormal && !st.IsGrandMelee {
		s.refreshMapOgresAfterFight(c, uid, st.MapID)
	}
	if st.isPvP() {
		s.grantPvPHonor(uid, st, winner == uid)
		s.grantRankDailyReward(uid, st, winner == uid)
	}
	// 王战/大乱斗日常经验：输赢皆可（finish 前保留 st）
	s.grantMatchDailyExp(c, uid, st)
	wasGaiyaAppear := st.IsGaiyaAppear
	gaiyaMap := st.MapID
	wasMelee := st.IsGrandMelee
	s.battles.clear(int64(uid))
	if wasMelee {
		s.clearGrandMeleeSession(int64(uid))
	}
	bt := s.boostTimesOf(int64(uid))
	s.send(c, 2506, uid, 0, buildFightOverInfoTimes(reason, winner,
		uint32(max0(bt.TwoTimes)), uint32(max0(bt.ThreeTimes)),
		uint32(max0(bt.AutoFightTimes)), uint32(max0(bt.EnergyTimes)),
		uint32(max0(bt.LearnTimes))))
	log.Printf("[CMD] OK     2506 FIGHT_OVER UID=%d reason=%d winner=%d learn=%d", uid, reason, winner, bt.LearnTimes)
	// 败北时也重推 2022，否则回图后盖亚消失需重进
	if wasGaiyaAppear && winner != uid {
		s.pushGaiyaAppearNote(c, uid, gaiyaMap)
	}
}

func (s *Server) grantBattleWinReward(uid uint32, st *BattleState) (skillNote []byte) {
	if s.cfg.Store == nil || st == nil || st.PlayerCatchTime == 0 {
		return nil
	}
	p, err := s.cfg.Store.GetPetByCatchTime(int64(uid), int64(st.PlayerCatchTime))
	if err != nil || p == nil {
		return nil
	}
	oldLevel := p.Level
	gain := battleYieldingExp(s, st)
	bt := s.boostTimesOf(int64(uid))
	mult := battleExpMultiplier(bt.TwoTimes, bt.ThreeTimes)
	if mult > 1 {
		gain *= mult
		s.consumeBattleExpBoost(int64(uid), mult)
	}
	_ = applyPetExpGain(p, gain)
	skillNote = s.afterPetLevelChange(p, oldLevel)
	s.applyBattleYieldingEV(int64(uid), p, st)
	_ = s.cfg.Store.UpsertPet(p)
	_ = s.cfg.Store.AddCoins(int64(uid), 10+st.EnemyLevel)
	s.contributeTeacherExpPond(int64(uid), gain)
	return skillNote
}

// battleYieldingExp 优先用 pets.xml YieldingExp；无表则回退 15+敌方等级×2。
func battleYieldingExp(s *Server, st *BattleState) int {
	if st == nil {
		return 15
	}
	if s != nil && s.cfg.Catalog != nil {
		if y := s.cfg.Catalog.YieldingExpOf(st.EnemyID); y > 0 {
			return y
		}
	}
	return 15 + st.EnemyLevel*2
}

// grantPuniFragmentReward 谱尼封印/真身首次击败发裂片；集齐 8 片自动合成精元。
func (s *Server) grantPuniFragmentReward(c *Client, uid uint32, st *BattleState) {
	if s.cfg.Store == nil || st == nil || c == nil {
		return
	}
	if !isPuniSealBoss(st.MapID, st.EnemyID, st.BossRegion) {
		return
	}
	fragID := getPuniFragmentItemID(st.BossRegion)
	if fragID <= 0 {
		return
	}
	sealKey := puniSealDefeatBase + int(st.BossRegion)
	ok, err := s.cfg.Store.HasDefeatedSPT(int64(uid), sealKey)
	if err != nil || ok {
		return
	}
	if err := s.cfg.Store.MarkDefeatedSPT(int64(uid), sealKey); err != nil {
		log.Printf("[puni] mark defeated UID=%d key=%d err=%v", uid, sealKey, err)
		return
	}
	if err := s.cfg.Store.AddItem(int64(uid), fragID, 1); err != nil {
		log.Printf("[puni] add fragment UID=%d item=%d err=%v", uid, fragID, err)
		return
	}
	s.send(c, 8004, uid, 0, buildBossMonster8004Body(0, 0, 0, uint32(fragID), 1))
	log.Printf("[CMD] OK     8004 GET_BOSS_MONSTER UID=%d puni region=%d frag=%d", uid, st.BossRegion, fragID)

	for region := uint32(1); region <= 8; region++ {
		id := getPuniFragmentItemID(region)
		n, _ := s.cfg.Store.GetItemCount(int64(uid), id)
		if n < 1 {
			return
		}
	}
	essenceN, _ := s.cfg.Store.GetItemCount(int64(uid), puniEssenceItemID)
	if essenceN > 0 {
		return
	}
	for region := uint32(1); region <= 8; region++ {
		id := getPuniFragmentItemID(region)
		if err := s.cfg.Store.ConsumeItem(int64(uid), id, 1); err != nil {
			log.Printf("[puni] consume frag UID=%d item=%d err=%v", uid, id, err)
			return
		}
	}
	if err := s.cfg.Store.AddItem(int64(uid), puniEssenceItemID, 1); err != nil {
		log.Printf("[puni] add essence UID=%d err=%v", uid, err)
		return
	}
	s.send(c, 8004, uid, 0, buildBossMonster8004Body(0, 0, 0, uint32(puniEssenceItemID), 1))
	log.Printf("[CMD] OK     8004 GET_BOSS_MONSTER UID=%d puni essence=%d", uid, puniEssenceItemID)
}

func (s *Server) buildActivePetProp(uid uint32, st *BattleState) []byte {
	if st == nil {
		return nil
	}
	level, exp := st.PlayerLevel, 0
	var ev [6]int
	if s.cfg.Store != nil && st.PlayerCatchTime > 0 {
		if p, _ := s.cfg.Store.GetPetByCatchTime(int64(uid), int64(st.PlayerCatchTime)); p != nil {
			level, exp = p.Level, p.Exp
			ev = p.EV
		}
	}
	return buildNoteUpdateProp(st.PlayerCatchTime, st.PlayerPetID, level, exp, exp, petNextLevelExp(int(st.PlayerPetID), level),
		int(st.PlayerMaxHP), st.PlayerAtk, st.PlayerDef, st.PlayerSpAtk, st.PlayerSpDef, st.PlayerSpd, ev)
}

// handleEscapeFight CMD 2410 逃跑：先 ACK 2410，再 2506 reason=5。
func (s *Server) handleEscapeFight(c *Client, uid uint32) {
	s.send(c, 2410, uid, 0, nil)
	st := s.battles.get(int64(uid))
	if st != nil && st.PlayerCatchTime > 0 {
		s.rememberPetHP(int64(uid), st.PlayerCatchTime, st.PlayerHP)
	}
	if st != nil && st.isPvP() {
		oppUID := st.OpponentUID
		opp := s.battles.get(oppUID)
		s.battles.clear(int64(uid))
		s.battles.clear(oppUID)
		s.send(c, 2506, uid, 0, buildFightOverInfo(fightReasonEscape, uint32(oppUID)))
		if oc := s.clientOf(oppUID); oc != nil {
			if opp != nil && opp.PlayerCatchTime > 0 {
				s.rememberPetHP(oppUID, opp.PlayerCatchTime, opp.PlayerHP)
			}
			s.send(oc, 2506, uint32(oppUID), 0, buildFightOverInfo(fightReasonNormal, uint32(oppUID)))
		}
		log.Printf("[CMD] OK     2410 ESCAPE_FIGHT PvP UID=%d opp=%d", uid, oppUID)
		return
	}
	mapID := 0
	if st != nil {
		mapID = st.MapID
	}
	s.battles.clear(int64(uid))
	log.Printf("[CMD] OK     2410 ESCAPE_FIGHT UID=%d -> 2506", uid)
	s.send(c, 2506, uid, 0, buildFightOverInfo(fightReasonEscape, 0))
	if mapID > 0 {
		s.refreshMapOgresAfterFight(c, uid, mapID)
	}
}

func hasSkillPP(skills [][2]uint32, skillID uint32) bool {
	for _, sk := range skills {
		if sk[0] == skillID {
			return sk[1] > 0
		}
	}
	return false
}

func firstSkillWithPP(skills [][2]uint32) uint32 {
	for _, sk := range skills {
		if sk[0] != 0 && sk[1] > 0 {
			return sk[0]
		}
	}
	if len(skills) > 0 {
		return skills[0][0]
	}
	return 10001
}

func rollCrit() bool {
	return rand.Intn(100) < 6
}

// handleFightLoadPercent CMD 2441：须回 userID+percent，空包会让客户端 NPE。
// PvP：本端 ACK + 转发给对手（对齐参考 LOAD_PERCENT）。
func (s *Server) handleFightLoadPercent(c *Client, uid uint32, body []byte) {
	pct := uint32(0)
	if len(body) >= 4 {
		pct = binary.BigEndian.Uint32(body[0:4])
	}
	if pct > 100 {
		pct = 100
	}
	ack := make([]byte, 8)
	binary.BigEndian.PutUint32(ack[0:4], uid)
	binary.BigEndian.PutUint32(ack[4:8], pct)
	s.send(c, 2441, uid, 0, ack)

	st := s.battles.get(int64(uid))
	if st == nil || !st.Active || !st.isPvP() {
		return
	}
	st.PvPLoadPct = pct
	s.battles.set(int64(uid), st)
	if oc := s.clientOf(st.OpponentUID); oc != nil {
		s.send(oc, 2441, uid, 0, ack)
	}
}

type bytesBuffer struct {
	b []byte
}

func (w *bytesBuffer) putU32(v uint32) {
	var tmp [4]byte
	binary.BigEndian.PutUint32(tmp[:], v)
	w.b = append(w.b, tmp[:]...)
}

func (w *bytesBuffer) write(p []byte) {
	w.b = append(w.b, p...)
}

func (w *bytesBuffer) bytes() []byte {
	return w.b
}
