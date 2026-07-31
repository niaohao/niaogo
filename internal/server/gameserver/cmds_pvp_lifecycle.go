package gameserver

import (
	"encoding/binary"
	"log"
	"time"
)

const (
	pvpLoadingWatchdogTimeout = 60 * time.Second
	pvpActionWatchdogTimeout  = 45 * time.Second
)

// schedulePvPLoadingWatchdog 卡在加载/首回合前（Round 仍为 0）超时则 abort。
// 对齐参考服：不以单方 PvPReady 提前放行（一方就绪另一方卡死时仍须超时清场）。
func (s *Server) schedulePvPLoadingWatchdog(uid1, uid2 int64) {
	go func() {
		time.Sleep(pvpLoadingWatchdogTimeout)
		st := s.battles.get(uid1)
		if st == nil || !st.Active || !st.isPvP() || st.OpponentUID != uid2 {
			return
		}
		if st.Round > 0 {
			return
		}
		s.abortPvPLoading(uid1, uid2, "loading_timeout")
	}()
}

// schedulePvPActionWatchdog 一方已提交行动、对方迟迟不出手 → 超时判胜（reason=overtime）。
func (s *Server) schedulePvPActionWatchdog(actedUID int64, gen uint32) {
	go func() {
		time.Sleep(pvpActionWatchdogTimeout)
		st := s.battles.get(actedUID)
		if st == nil || !st.Active || !st.isPvP() {
			return
		}
		if st.PvPWaitGen != gen || !st.pvpHasAction() {
			return
		}
		opp := s.battles.get(st.OpponentUID)
		if opp == nil || !opp.Active || opp.OpponentUID != actedUID {
			return
		}
		if opp.pvpHasAction() {
			return
		}
		log.Printf("[PvP] action timeout acted=%d opp=%d gen=%d -> overtime win", actedUID, st.OpponentUID, gen)
		s.finishPvPWithReason(uint32(actedUID), st, uint32(st.OpponentUID), opp, uint32(actedUID), fightReasonOvertime)
	}()
}

// schedulePvPFaintSwitchWatchdog 倒地后等换宠超时 → 判负。
func (s *Server) schedulePvPFaintSwitchWatchdog(faintedUID int64, catchTime uint32) {
	go func() {
		time.Sleep(pvpActionWatchdogTimeout)
		st := s.battles.get(faintedUID)
		if st == nil || !st.Active || !st.isPvP() {
			return
		}
		if st.PlayerHP > 0 || st.PlayerCatchTime != catchTime {
			return
		}
		oppUID := st.OpponentUID
		opp := s.battles.get(oppUID)
		if opp == nil || !opp.Active || opp.OpponentUID != faintedUID {
			return
		}
		log.Printf("[PvP] faint-switch timeout UID=%d catch=%d -> opp wins", faintedUID, catchTime)
		s.finishPvPWithReason(uint32(faintedUID), st, uint32(oppUID), opp, uint32(oppUID), fightReasonOvertime)
	}()
}

// abortPvPLoading 加载中止：双方 2506 reason=error，清 BattleState。
func (s *Server) abortPvPLoading(uid1, uid2 int64, reason string) {
	s.battles.clear(uid1)
	s.battles.clear(uid2)
	over := buildFightOverInfo(fightReasonError, 0)
	for _, uid := range []int64{uid1, uid2} {
		if uid <= 0 {
			continue
		}
		if c := s.clientOf(uid); c != nil {
			s.send(c, 2506, uint32(uid), 0, over)
		}
	}
	log.Printf("[PvP] abort loading reason=%s UID1=%d UID2=%d", reason, uid1, uid2)
}

// onUserDisconnect 断线：清邀请；PvP 则对手判胜。
func (s *Server) onUserDisconnect(uid int64) {
	if uid <= 0 {
		return
	}
	s.cleanupPvPInviteOnDisconnect(uid)
	s.settlePvPOnDisconnect(uid)
	s.battles.clear(uid)
}

func (s *Server) cleanupPvPInviteOnDisconnect(uid int64) {
	s.pvpInvites.clearInviter(uid)
	for _, inv := range s.pvpInvites.clearTarget(uid) {
		if inv == nil {
			continue
		}
		if c := s.clientOf(inv.Inviter); c != nil {
			note := make([]byte, 24)
			binary.BigEndian.PutUint32(note[0:4], uint32(uid))
			putFixedNick(note, 4, s.nickOf(uint32(uid)))
			binary.BigEndian.PutUint32(note[20:24], 0)
			s.send(c, 2502, uint32(uid), 0, note)
		}
	}
}

// settlePvPOnDisconnect 断线方认输，在线方 2506 reason=exit 判胜。
func (s *Server) settlePvPOnDisconnect(disconnectedUID int64) {
	st := s.battles.get(disconnectedUID)
	if st == nil || !st.Active || !st.isPvP() {
		return
	}
	oppUID := st.OpponentUID
	opp := s.battles.get(oppUID)
	if opp == nil || !opp.Active || opp.OpponentUID != disconnectedUID {
		s.battles.clear(disconnectedUID)
		return
	}
	if st.PlayerCatchTime > 0 {
		s.rememberPetHP(disconnectedUID, st.PlayerCatchTime, st.PlayerHP)
	}
	if opp.PlayerCatchTime > 0 {
		s.rememberPetHP(oppUID, opp.PlayerCatchTime, opp.PlayerHP)
	}
	s.battles.clear(disconnectedUID)
	s.battles.clear(oppUID)

	if oc := s.clientOf(oppUID); oc != nil {
		s.send(oc, 2506, uint32(oppUID), 0, buildFightOverInfo(fightReasonExit, uint32(oppUID)))
		if s.cfg.Store != nil {
			_ = s.cfg.Store.AddCoins(oppUID, 20)
		}
	}
	log.Printf("[PvP] disconnect settle: leave=%d winner=%d", disconnectedUID, oppUID)
}

// syncPvPStagesMirror 结算前对齐双方能力等级/异常/持续 buff/属性：A.Enemy* ↔ B.Player*。
func syncPvPStagesMirror(a, b *BattleState) {
	if a == nil || b == nil {
		return
	}
	a.EnemyStages = b.PlayerStages
	b.EnemyStages = a.PlayerStages
	a.EnemyStatus = b.PlayerStatus
	b.EnemyStatus = a.PlayerStatus
	a.EnemyBuff = b.PlayerBuff
	b.EnemyBuff = a.PlayerBuff
	a.EnemyType = b.PlayerType
	b.EnemyType = a.PlayerType
}

// pushPvPStagesAfterSideEffect 技能副作用后把本方对敌方的 stages/status/buff/属性 推到对方 Player*，
// 并把本方 Player* 镜像到对方 Enemy*（含属性系别）。
func pushPvPStagesAfterSideEffect(attacker, defender *BattleState) {
	if attacker == nil || defender == nil {
		return
	}
	defender.PlayerStages = attacker.EnemyStages
	defender.PlayerStatus = attacker.EnemyStatus
	defender.PlayerBuff = attacker.EnemyBuff
	defender.PlayerType = attacker.EnemyType
	defender.EnemyStages = attacker.PlayerStages
	defender.EnemyStatus = attacker.PlayerStatus
	defender.EnemyBuff = attacker.PlayerBuff
	defender.EnemyType = attacker.PlayerType
	attacker.EnemyStages = defender.PlayerStages
	attacker.EnemyStatus = defender.PlayerStatus
	attacker.EnemyBuff = defender.PlayerBuff
	attacker.EnemyType = defender.PlayerType
}
