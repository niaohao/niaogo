package gameserver

import (
	"encoding/binary"
	"log"
	"sync"
	"time"

	"niaohao/server/internal/cmdname"
	"niaohao/server/internal/store"
)

const (
	pvpModeSingle = 1
	pvpModeMulti  = 2
	pvpInviteTTL  = 60 * time.Second
)

type pvpInvite struct {
	Inviter int64
	Target  int64
	Mode    uint32
	At      time.Time
}

type pvpInviteHub struct {
	mu sync.Mutex
	m  map[int64]*pvpInvite // key = inviter
}

func (h *pvpInviteHub) put(inv *pvpInvite) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.m == nil {
		h.m = make(map[int64]*pvpInvite)
	}
	h.m[inv.Inviter] = inv
}

func (h *pvpInviteHub) take(inviter int64) *pvpInvite {
	h.mu.Lock()
	defer h.mu.Unlock()
	inv := h.m[inviter]
	delete(h.m, inviter)
	return inv
}

func (h *pvpInviteHub) clearInviter(inviter int64) *pvpInvite {
	return h.take(inviter)
}

// clearTarget 移除指向 target 的挂起邀请，返回被清掉的邀请列表。
func (h *pvpInviteHub) clearTarget(target int64) []*pvpInvite {
	h.mu.Lock()
	defer h.mu.Unlock()
	var out []*pvpInvite
	for inviter, inv := range h.m {
		if inv != nil && inv.Target == target {
			out = append(out, inv)
			delete(h.m, inviter)
		}
	}
	return out
}

// handleInviteToFight CMD 2401：邀请对战。请求 target+mode；ACK result(4)；推 2501 给对方。
func (s *Server) handleInviteToFight(c *Client, uid uint32, body []byte) {
	target, mode := uint32(0), uint32(pvpModeMulti)
	if len(body) >= 4 {
		target = binary.BigEndian.Uint32(body[0:4])
	}
	if len(body) >= 8 {
		mode = binary.BigEndian.Uint32(body[4:8])
	}
	if mode != pvpModeSingle {
		mode = pvpModeMulti
	}
	ack := make([]byte, 4)
	s.send(c, 2401, uid, 0, ack)

	if target == 0 || target == uid {
		log.Printf("[CMD] OK     %s UID=%d bad target=%d", cmdname.Format(2401), uid, target)
		return
	}
	if st := s.battles.get(int64(uid)); st != nil && st.Active {
		s.sendAlert(int64(uid), "当前正在战斗中，无法发起邀请")
		return
	}
	tc := s.clientOf(int64(target))
	if tc == nil || !tc.LoggedIn {
		s.sendAlert(int64(uid), "对方不在线")
		return
	}
	if st := s.battles.get(int64(target)); st != nil && st.Active {
		s.sendAlert(int64(uid), "对方正在战斗中")
		return
	}

	s.pvpInvites.clearInviter(int64(uid))
	s.pvpInvites.put(&pvpInvite{
		Inviter: int64(uid), Target: int64(target), Mode: mode, At: time.Now(),
	})

	note := make([]byte, 24)
	binary.BigEndian.PutUint32(note[0:4], uid)
	putFixedNick(note, 4, s.nickOf(uid))
	binary.BigEndian.PutUint32(note[20:24], mode)
	s.send(tc, 2501, uid, 0, note)
	log.Printf("[CMD] OK     %s UID=%d -> %d mode=%d +2501", cmdname.Format(2401), uid, target, mode)
}

// handleInviteFightCancel CMD 2402：取消邀请；若仍在 PvP 加载(未 2404)则双方中止。
func (s *Server) handleInviteFightCancel(c *Client, uid uint32) {
	inv := s.pvpInvites.clearInviter(int64(uid))
	s.send(c, 2402, uid, 0, make([]byte, 4))
	if st := s.battles.get(int64(uid)); st != nil && st.Active && st.isPvP() && !st.PvPReady {
		s.abortPvPLoading(int64(uid), st.OpponentUID, "cancel_during_load")
		log.Printf("[CMD] OK     %s UID=%d abort PvP loading", cmdname.Format(2402), uid)
		return
	}
	if inv != nil {
		log.Printf("[CMD] OK     %s UID=%d cleared target=%d", cmdname.Format(2402), uid, inv.Target)
	} else {
		log.Printf("[CMD] OK     %s UID=%d", cmdname.Format(2402), uid)
	}
}

// handleHandleFightInvite CMD 2403：接受/拒绝。请求 inviter+result+mode。
// 推 2502 给邀请方；接受则双方 2503。
func (s *Server) handleHandleFightInvite(c *Client, uid uint32, body []byte) {
	inviterUID, result, mode := uint32(0), uint32(0), uint32(pvpModeMulti)
	if len(body) >= 4 {
		inviterUID = binary.BigEndian.Uint32(body[0:4])
	}
	if len(body) >= 8 {
		result = binary.BigEndian.Uint32(body[4:8])
	}
	if len(body) >= 12 {
		mode = binary.BigEndian.Uint32(body[8:12])
	}

	inv := s.pvpInvites.take(int64(inviterUID))
	valid := inv != nil && inv.Target == int64(uid) && time.Since(inv.At) <= pvpInviteTTL
	if valid && inv.Mode != 0 {
		mode = inv.Mode
	}

	ack := make([]byte, 4)
	if result == 1 && !valid {
		binary.BigEndian.PutUint32(ack, 1)
		s.send(c, 2403, uid, 0, ack)
		s.sendAlert(int64(uid), "邀请已失效或对方已取消")
		log.Printf("[CMD] OK     %s UID=%d accept fail invalid invite from=%d", cmdname.Format(2403), uid, inviterUID)
		return
	}
	s.send(c, 2403, uid, 0, ack)

	nick := s.nickOf(uid)
	note := make([]byte, 24)
	binary.BigEndian.PutUint32(note[0:4], uid)
	putFixedNick(note, 4, nick)
	binary.BigEndian.PutUint32(note[20:24], result)
	if ic := s.clientOf(int64(inviterUID)); ic != nil {
		s.send(ic, 2502, uid, 0, note)
	}

	if result != 1 {
		log.Printf("[CMD] OK     %s UID=%d reject inviter=%d", cmdname.Format(2403), uid, inviterUID)
		return
	}
	if !s.startPvPMatch(int64(inviterUID), int64(uid), mode) {
		s.sendAlert(int64(uid), "无法开始对战")
		if ic := s.clientOf(int64(inviterUID)); ic != nil {
			fail := make([]byte, 24)
			binary.BigEndian.PutUint32(fail[0:4], uid)
			putFixedNick(fail, 4, nick)
			binary.BigEndian.PutUint32(fail[20:24], 0)
			s.send(ic, 2502, uid, 0, fail)
		}
		return
	}
	log.Printf("[CMD] OK     %s UID=%d accept inviter=%d mode=%d -> 2503", cmdname.Format(2403), uid, inviterUID, mode)
}

func (s *Server) startPvPMatch(uid1, uid2 int64, mode uint32) bool {
	c1, c2 := s.clientOf(uid1), s.clientOf(uid2)
	if c1 == nil || c2 == nil {
		return false
	}
	bag1, bag2 := []store.Pet{}, []store.Pet{}
	if s.cfg.Store != nil {
		bag1, _ = s.cfg.Store.ListBagPets(uid1)
		bag2, _ = s.cfg.Store.ListBagPets(uid2)
	}
	p1, p2 := pickBattlePet(bag1), pickBattlePet(bag2)
	if p1 == nil || p2 == nil {
		return false
	}
	if mode != pvpModeSingle {
		mode = pvpModeMulti
	}

	st1 := s.buildPvPSideState(c1, uid1, uid2, p1, p2, mode)
	st2 := s.buildPvPSideState(c2, uid2, uid1, p2, p1, mode)
	// 镜像敌方技能/六维来自对方出战宠
	s.fillPvPEnemyFromPet(st1, p2)
	s.fillPvPEnemyFromPet(st2, p1)

	now := time.Now().Unix()
	st1.PvPStartedAt, st2.PvPStartedAt = now, now

	s.battles.clear(uid1)
	s.battles.clear(uid2)
	s.battles.set(uid1, st1)
	s.battles.set(uid2, st2)

	body1 := s.buildNoteReadyToFightPvP(uint32(uid1), s.nickOf(uint32(uid1)), st1, bag1, uint32(uid2), s.nickOf(uint32(uid2)), st2, bag2, mode)
	body2 := s.buildNoteReadyToFightPvP(uint32(uid2), s.nickOf(uint32(uid2)), st2, bag2, uint32(uid1), s.nickOf(uint32(uid1)), st1, bag1, mode)
	s.send(c1, 2503, uint32(uid1), 0, body1)
	s.send(c2, 2503, uint32(uid2), 0, body2)
	// 与 PvE 一致：2503 后立刻 2301，写入 PetManager（SelectPetPanel/技能栏靠 catchTime 取 skillArray）
	s.syncBagPetsToClientTagged(c1, uint32(uid1), bag1, st1.PlayerCatchTime, "pvp-post-2503")
	s.syncBagPetsToClientTagged(c2, uint32(uid2), bag2, st2.PlayerCatchTime, "pvp-post-2503")
	s.schedulePvPLoadingWatchdog(uid1, uid2)
	log.Printf("[CMD] OK     2503 NOTE_READY_TO_FIGHT PvP %d vs %d mode=%d pets=%d/%d",
		uid1, uid2, mode, len(bag1), len(bag2))
	return true
}

func (s *Server) buildPvPSideState(c *Client, selfUID, oppUID int64, selfPet, _ *store.Pet, mode uint32) *BattleState {
	petID, lv, name, php, patk, pdef, psa, psd, pspd := petCombatStats(selfPet)
	patk, pdef, psa, psd, pspd, crit := s.applyEnergyBallBonus(selfPet, patk, pdef, psa, psd, pspd)
	trait := s.applyBattlePetTrait(selfUID, selfPet, &crit)
	catch := uint32(selfPet.CatchTime)
	mapID := defaultMapID
	if c != nil && c.MapID > 0 {
		mapID = c.MapID
	}
	st := &BattleState{
		Active: true, MapID: mapID, OpponentUID: oppUID, PvPMode: mode,
		PlayerPetID: petID, PlayerLevel: lv, PlayerName: name, PlayerCatchTime: catch,
		PlayerHP: s.recalledPetHP(selfUID, catch, uint32(php)), PlayerMaxHP: uint32(php),
		PlayerAtk: patk, PlayerDef: pdef, PlayerSpAtk: psa, PlayerSpDef: psd, PlayerSpd: pspd,
		PlayerSkills: s.skillsFromPet(selfPet), PlayerType: s.petTypeOf(petID), PlayerCritBonus: crit,
		PlayerTrait: trait, PlayerDV: 0,
	}
	if selfPet != nil {
		st.PlayerDV = selfPet.DV
	}
	if mode == pvpModeSingle {
		st.DailyExpKind = dailyExpPetKing
	}
	if st.PlayerMaxHP == 0 {
		st.PlayerMaxHP = 1
	}
	if st.PlayerHP == 0 || st.PlayerHP > st.PlayerMaxHP {
		st.PlayerHP = st.PlayerMaxHP
	}
	s.consumeEnergyBallOnEnter(uint32(selfUID), catch)
	return st
}

func (s *Server) fillPvPEnemyFromPet(st *BattleState, opp *store.Pet) {
	if st == nil || opp == nil {
		return
	}
	pid, lv, name, hp, atk, def, sa, sd, spd := petCombatStats(opp)
	atk, def, sa, sd, spd, _ = s.applyEnergyBallBonus(opp, atk, def, sa, sd, spd)
	st.EnemyID, st.EnemyLevel, st.EnemyName = pid, lv, name
	st.EnemyCatchTime = uint32(opp.CatchTime)
	st.EnemyMaxHP = uint32(hp)
	if st.EnemyMaxHP == 0 {
		st.EnemyMaxHP = 1
	}
	st.EnemyHP = s.recalledPetHP(st.OpponentUID, st.EnemyCatchTime, st.EnemyMaxHP)
	if st.EnemyHP == 0 || st.EnemyHP > st.EnemyMaxHP {
		st.EnemyHP = st.EnemyMaxHP
	}
	st.EnemyAtk, st.EnemyDef, st.EnemySpAtk, st.EnemySpDef, st.EnemySpd = atk, def, sa, sd, spd
	st.EnemySkills = s.skillsFromPet(opp)
	st.EnemyType = s.petTypeOf(pid)
	st.EnemyCatchable = false
	st.EnemyDV = 0
	if opp != nil {
		st.EnemyDV = opp.DV
	}
}

// buildNoteReadyToFightPvP：两侧均为玩家；self 在前。
// 本客户端 SelectPetPanel 始终遍历 PetManager.catchTimes 再查 _petInfoMap，
// 故 2503 必须带全背包（单挑也不可截断，否则 #1009 黑屏）。
func (s *Server) buildNoteReadyToFightPvP(
	selfUID uint32, selfNick string, selfSt *BattleState, selfBag []store.Pet,
	oppUID uint32, oppNick string, oppSt *BattleState, oppBag []store.Pet,
	_ uint32,
) []byte {
	selfPets := s.simplePetsForBattle(selfUID, selfSt, selfBag)
	oppPets := s.simplePetsForBattle(oppUID, oppSt, oppBag)
	var buf bytesBuffer
	buf.putU32(2)
	buf.write(buildFightUserInfo(selfUID, selfNick, 0))
	buf.putU32(uint32(len(selfPets)))
	for _, p := range selfPets {
		buf.write(p)
	}
	buf.putU32(0) // clothes
	buf.write(buildFightUserInfo(oppUID, oppNick, 0))
	buf.putU32(uint32(len(oppPets)))
	for _, p := range oppPets {
		buf.write(p)
	}
	buf.putU32(0)
	return buf.bytes()
}

func (s *Server) clientOf(uid int64) *Client {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.byUID[uid]
}

func (s *Server) sendAlert(uid int64, msg string) {
	if c := s.clientOf(uid); c != nil {
		s.pushAlert(c, uint32(uid), msg)
	}
}

func (st *BattleState) isPvP() bool {
	return st != nil && st.OpponentUID != 0
}
