package gameserver

import (
	"log"

	"niaohao/server/internal/store"
)

// 对齐参考服 pvp_round_engine：每方每回合一次行动，凑齐后按矩阵结算。
const (
	pvpActNone   uint8 = 0
	pvpActSkill  uint8 = 1
	pvpActItem   uint8 = 2
	pvpActSwitch uint8 = 3
)

type pvpSubmitResult int

const (
	pvpWait     pvpSubmitResult = iota // 等对方
	pvpContinue                        // 双方齐全且含技能 → 继续技能结算
	pvpDone                            // 已 ZERO 结算或拒绝
)

func (st *BattleState) pvpClearAction() {
	if st == nil {
		return
	}
	st.PvPSubmittedType = pvpActNone
	st.PvPSubmittedSkillID = 0
	st.PvPSubmittedItemID = 0
	st.PvPSubmittedCatchTime = 0
	st.PvPDeferSwitch = false
}

func (st *BattleState) pvpHasAction() bool {
	return st != nil && st.PvPSubmittedType != pvpActNone
}

func pvpClearBoth(a, b *BattleState) {
	a.pvpClearAction()
	b.pvpClearAction()
}

// pvpSubmit 写入本方行动；双方齐全则按矩阵调度。调用方已持有各自 BattleState 指针。
func (s *Server) pvpSubmit(uid uint32, st *BattleState, actType uint8, skillID, itemID, catchTime uint32) pvpSubmitResult {
	if st == nil || !st.isPvP() || actType == pvpActNone {
		return pvpDone
	}
	if st.pvpHasAction() && !(actType == pvpActSkill && st.PvPSubmittedType == pvpActSkill) {
		return pvpDone
	}
	st.PvPSubmittedType = actType
	st.PvPSubmittedSkillID = skillID
	st.PvPSubmittedItemID = itemID
	st.PvPSubmittedCatchTime = catchTime
	if actType == pvpActSwitch {
		st.PvPDeferSwitch = true
	}
	s.battles.set(int64(uid), st)

	opp := s.battles.get(st.OpponentUID)
	if opp == nil || !opp.Active || !opp.pvpHasAction() {
		st.PvPWaitGen++
		gen := st.PvPWaitGen
		s.battles.set(int64(uid), st)
		log.Printf("[PvP] UID=%d act=%d wait opp=%d gen=%d", uid, actType, st.OpponentUID, gen)
		s.schedulePvPActionWatchdog(int64(uid), gen)
		return pvpWait
	}

	my, ot := st.PvPSubmittedType, opp.PvPSubmittedType
	// 双技能 / 技能+非技能 → 由技能结算管线处理
	if my == pvpActSkill || ot == pvpActSkill {
		log.Printf("[PvP] UID=%d vs %d resolve skill-led my=%d opp=%d", uid, st.OpponentUID, my, ot)
		return pvpContinue
	}
	// 纯非技能 → 零伤 2505
	s.pvpSendZeroDamage2505(uid, st, uint32(st.OpponentUID), opp)
	return pvpDone
}

func (s *Server) pvpSendZeroDamage2505(uidA uint32, a *BattleState, uidB uint32, b *BattleState) {
	a.EnemyHP, a.EnemyMaxHP = b.PlayerHP, b.PlayerMaxHP
	b.EnemyHP, b.EnemyMaxHP = a.PlayerHP, a.PlayerMaxHP
	aSwitch, bSwitch := a.PvPDeferSwitch, b.PvPDeferSwitch
	a.Round++
	b.Round = a.Round
	pvpClearBoth(a, b)
	s.battles.set(int64(uidA), a)
	s.battles.set(int64(uidB), b)

	s.pvpPushDeferredSwitch(uidA, a, uidB, b, aSwitch, bSwitch)

	avA := buildAttackValueFromState(uidA, 0, 0, 0, 0, int32(a.PlayerHP), a.PlayerMaxHP, 0, 0, 0, a, true, a.PlayerSkills)
	avB := buildAttackValueFromState(uidB, 0, 0, 0, 0, int32(b.PlayerHP), b.PlayerMaxHP, 0, 0, 0, b, true, b.PlayerSkills)
	outA := append(append([]byte{}, avA...), avB...)
	outB := append(append([]byte{}, avB...), avA...)
	if ca := s.clientOf(int64(uidA)); ca != nil {
		s.send(ca, 2505, uidA, 0, outA)
	}
	if cb := s.clientOf(int64(uidB)); cb != nil {
		s.send(cb, 2505, uidB, 0, outB)
	}
	log.Printf("[CMD] OK     2505 PvP ZERO round=%d %d vs %d", a.Round, uidA, uidB)
}

func (s *Server) pvpPushDeferredSwitch(uidA uint32, a *BattleState, uidB uint32, b *BattleState, aSw, bSw bool) {
	if aSw {
		if cb := s.clientOf(int64(uidB)); cb != nil {
			s.send(cb, 2407, uidB, 0, buildChangePetInfo(uidA, a.PlayerPetID, a.PlayerName,
				uint32(a.PlayerLevel), a.PlayerHP, a.PlayerMaxHP, a.PlayerCatchTime))
		}
	}
	if bSw {
		if ca := s.clientOf(int64(uidA)); ca != nil {
			s.send(ca, 2407, uidA, 0, buildChangePetInfo(uidB, b.PlayerPetID, b.PlayerName,
				uint32(b.PlayerLevel), b.PlayerHP, b.PlayerMaxHP, b.PlayerCatchTime))
		}
	}
}

// resolvePvPSkillRound 双方行动齐全且至少一方是技能：结算技能伤害（非技能方本回合不出招）。
func (s *Server) resolvePvPSkillRound(uidA uint32, a *BattleState, uidB uint32, b *BattleState) {
	syncPvPStagesMirror(a, b)

	skillA, skillB := uint32(0), uint32(0)
	aAtk, bAtk := uint32(0), uint32(0)
	aFromCharge, bFromCharge := false, false
	if a.PvPSubmittedType == pvpActSkill {
		skillA = a.PvPSubmittedSkillID
		if charged := takeChargeSkill(a, true); charged != 0 {
			skillA = charged
			aFromCharge = true
		}
		aAtk = 1
		decSkillPP(a.PlayerSkills, skillA)
	}
	if b.PvPSubmittedType == pvpActSkill {
		skillB = b.PvPSubmittedSkillID
		if charged := takeChargeSkill(b, true); charged != 0 {
			skillB = charged
			bFromCharge = true
		}
		bAtk = 1
		decSkillPP(b.PlayerSkills, skillB)
	}

	aCtrlAtStart := snapshotControlStatus(a.PlayerStatus)
	bCtrlAtStart := snapshotControlStatus(b.PlayerStatus)
	aSkip := consumeSkipStatus(&a.PlayerStatus)
	bSkip := consumeSkipStatus(&b.PlayerStatus)
	aDef, bDef := s.skillDef(int(skillA)), s.skillDef(int(skillB))
	if aAtk > 0 && !aSkip && !aFromCharge && beginCharge(a, skillA, aDef, true) {
		aSkip = true
	}
	if bAtk > 0 && !bSkip && !bFromCharge && beginCharge(b, skillB, bDef, true) {
		bSkip = true
	}
	if aAtk > 0 && !aSkip && consumeSkillFail(a, true) {
		aSkip = true
	}
	if bAtk > 0 && !bSkip && consumeSkillFail(b, true) {
		bSkip = true
	}
	if aAtk > 0 && !aSkip && attrSkillBlocked(a, true, aDef) {
		aSkip = true
	}
	if bAtk > 0 && !bSkip && attrSkillBlocked(b, true, bDef) {
		bSkip = true
	}
	aHit := aAtk > 0 && !aSkip && (mustHitFromBuff(&a.PlayerBuff) || s.checkSkillHitTrait(skillA, 0, 0, a.PlayerTrait, b.PlayerTrait))
	bHit := bAtk > 0 && !bSkip && (mustHitFromBuff(&b.PlayerBuff) || s.checkSkillHitTrait(skillB, 0, 0, b.PlayerTrait, a.PlayerTrait))

	aDmg, bDmg := uint32(0), uint32(0)
	aFirst := a.stagedSpd(true) >= b.stagedSpd(true)
	if forceFirst, forceSecond := priorityFromBuff(&a.PlayerBuff, &b.PlayerBuff); forceFirst {
		aFirst = true
	} else if forceSecond {
		aFirst = false
	}
	if aHit && bHit {
		// keep speed order / 83 强制先手
	} else if aHit && !bHit {
		aFirst = true
	} else if bHit && !aHit {
		aFirst = false
	}
	advanceConsecutiveSkill(&a.PlayerConsecSkillID, &a.PlayerConsecSkillCount, skillA)
	advanceConsecutiveSkill(&b.PlayerConsecSkillID, &b.PlayerConsecSkillCount, skillB)
	if aHit {
		if _, ok := sameLifeDamage(aDef, a.PlayerHP, b.PlayerHP); ok && b.PlayerHP <= a.PlayerHP {
			aHit = false
		}
	}
	if bHit {
		if _, ok := sameLifeDamage(bDef, b.PlayerHP, a.PlayerHP); ok && a.PlayerHP <= b.PlayerHP {
			bHit = false
		}
	}
	if aHit && (physMissForced(&b.PlayerBuff, aDef) || specMissForced(&b.PlayerBuff, aDef)) {
		aHit = false
	}
	if bHit && (physMissForced(&a.PlayerBuff, bDef) || specMissForced(&a.PlayerBuff, bDef)) {
		bHit = false
	}
	if !aHit && aAtk > 0 && !aSkip {
		// a miss → b dodge boost；双方各用自己状态机
		applyOnDodgeBoost(b, true)
	}
	if !bHit && bAtk > 0 && !bSkip {
		applyOnDodgeBoost(a, true)
	}
	aHits, bHits := 1, 1
	if aHit {
		aHits = sideEffectHitCount(aDef)
		if dmg80, loss, ok := sacrificeHalfEqualDamage(aDef, a.PlayerHP, a.PlayerMaxHP); ok {
			aDmg = dmg80
			_ = applyDamage(&a.PlayerHP, loss)
		} else if dmg112, loss, ok := sacrificeAllForFlat(aDef, a.PlayerHP); ok {
			aDmg = dmg112
			_ = applyDamage(&a.PlayerHP, loss)
		} else if dmg7, ok := sameLifeDamage(aDef, a.PlayerHP, b.PlayerHP); ok {
			aDmg = dmg7
		} else {
			bDefStat, bSpDefStat := b.stagedDef(true), b.stagedSpDef(true)
			if skillHasSideEffect(aDef, 195) {
				bDefStat, bSpDefStat = b.stagedDefIgnoreBoost(true), b.stagedSpDefIgnoreBoost(true)
			}
			aDmg = s.damageWithSkillAdj(skillA, a.PlayerLevel,
				a.stagedAtk(true), bDefStat, a.stagedSpAtk(true), bSpDefStat,
				a.PlayerType, b.PlayerType, skillPowerAdj{
					FoeHP: b.PlayerHP, FoeMaxHP: b.PlayerMaxHP,
					SelfHP: a.PlayerHP, SelfMaxHP: a.PlayerMaxHP,
					GoingFirst: aFirst, ConsecCount: a.PlayerConsecSkillCount,
					FoeStages: &b.PlayerStages, SelfDV: a.PlayerDV,
				})
			aDmg *= uint32(aHits)
			// 28/29/93 粉伤不并入 lostHP
			if ohko := sideEffectOHKO(aDef, b.PlayerHP); ohko > 0 {
				aDmg = ohko
			}
			var instant bool
			aDmg, instant = applyTraitOutgoingDamage(a.PlayerTrait, aDef, aDmg)
			if instant && b.PlayerHP > 0 {
				aDmg = b.PlayerHP
			}
			aDmg = applyOutgoingDamageBuff(&a.PlayerBuff, aDef, aDmg)
			aDmg = applyIncomingDamageBuff(&b.PlayerBuff, aDef, aDmg)
			aDmg = statusPowerBoost(aDef, &a.PlayerStatus, aDmg)
			aDmg = foeStatusDamageMul(aDef, &b.PlayerStatus, aDmg)
			aDmg = lowHPDamageScale(aDef, a.PlayerHP, a.PlayerMaxHP, aDmg)
			aDmg = sideEffectChanceMulDamage(aDef, aDmg)
			foeGender := 0
			if s.cfg.Catalog != nil {
				foeGender = s.cfg.Catalog.PetGender(b.PlayerPetID)
			}
			aDmg = applyHighDamageSideEffects(aDef, aDmg, &b.PlayerStatus, a.PlayerType, foeGender, aFirst)
			aDmg = maleDamageMulFromBuff(&a.PlayerBuff, foeGender, aDmg)
			aDmg = applyMoreDamageSideEffects(aDef, aDmg, b.PlayerHP, &b.PlayerStatus, &b.PlayerStages, aFirst)
			skType := 0
			if aDef != nil {
				skType = aDef.Type
			}
			aDmg = applyFreq2DamageSideEffects(aDef, aDmg, a.PlayerHP, a.PlayerMaxHP, skType, b.PlayerType)
			aDmg = applyFreq3DamageSideEffects(aDef, aDmg, a.PlayerHP, b.PlayerHP,
				a.PlayerLevel, a.stagedSpd(true), a.PlayerType, b.PlayerType, a.PlayerConsecSkillCount)
			aDmg = applyFreq4DamageSideEffects(aDef, aDmg, a.PlayerHP, &a.PlayerStages, b.PlayerDef, a.PlayerConsecSkillCount, &b.PlayerStatus)
			aDmg = applyFreq5DamageSideEffects(aDef, aDmg, b.PlayerHP, &a.PlayerEffect795Uses)
			aDmg = applyLeaveOneHP(aDef, b.PlayerHP, aDmg)
			aDmg = applyEndureLeaveOne(&b.PlayerBuff, b.PlayerHP, aDmg)
			if aFromCharge && aDmg > 0 {
				aDmg *= 2
			}
		}
		if cd := sideEffectCounterDamage(aDef, a.PlayerLastTaken); cd > 0 {
			aDmg = cd
		}
	}
	if bHit {
		bHits = sideEffectHitCount(bDef)
		if dmg80, loss, ok := sacrificeHalfEqualDamage(bDef, b.PlayerHP, b.PlayerMaxHP); ok {
			bDmg = dmg80
			_ = applyDamage(&b.PlayerHP, loss)
		} else if dmg112, loss, ok := sacrificeAllForFlat(bDef, b.PlayerHP); ok {
			bDmg = dmg112
			_ = applyDamage(&b.PlayerHP, loss)
		} else if dmg7, ok := sameLifeDamage(bDef, b.PlayerHP, a.PlayerHP); ok {
			bDmg = dmg7
		} else {
			aDefStat, aSpDefStat := a.stagedDef(true), a.stagedSpDef(true)
			if skillHasSideEffect(bDef, 195) {
				aDefStat, aSpDefStat = a.stagedDefIgnoreBoost(true), a.stagedSpDefIgnoreBoost(true)
			}
			bDmg = s.damageWithSkillAdj(skillB, b.PlayerLevel,
				b.stagedAtk(true), aDefStat, b.stagedSpAtk(true), aSpDefStat,
				b.PlayerType, a.PlayerType, skillPowerAdj{
					FoeHP: a.PlayerHP, FoeMaxHP: a.PlayerMaxHP,
					SelfHP: b.PlayerHP, SelfMaxHP: b.PlayerMaxHP,
					GoingFirst: !aFirst, ConsecCount: b.PlayerConsecSkillCount,
					FoeStages: &a.PlayerStages, SelfDV: b.PlayerDV,
				})
			bDmg *= uint32(bHits)
			// 28/29/93 粉伤不并入 lostHP
			if ohko := sideEffectOHKO(bDef, a.PlayerHP); ohko > 0 {
				bDmg = ohko
			}
			var instant bool
			bDmg, instant = applyTraitOutgoingDamage(b.PlayerTrait, bDef, bDmg)
			if instant && a.PlayerHP > 0 {
				bDmg = a.PlayerHP
			}
			bDmg = applyOutgoingDamageBuff(&b.PlayerBuff, bDef, bDmg)
			bDmg = applyIncomingDamageBuff(&a.PlayerBuff, bDef, bDmg)
			bDmg = statusPowerBoost(bDef, &b.PlayerStatus, bDmg)
			bDmg = foeStatusDamageMul(bDef, &a.PlayerStatus, bDmg)
			bDmg = lowHPDamageScale(bDef, b.PlayerHP, b.PlayerMaxHP, bDmg)
			bDmg = sideEffectChanceMulDamage(bDef, bDmg)
			foeGender := 0
			if s.cfg.Catalog != nil {
				foeGender = s.cfg.Catalog.PetGender(a.PlayerPetID)
			}
			bDmg = applyHighDamageSideEffects(bDef, bDmg, &a.PlayerStatus, b.PlayerType, foeGender, !aFirst)
			bDmg = maleDamageMulFromBuff(&b.PlayerBuff, foeGender, bDmg)
			bDmg = applyMoreDamageSideEffects(bDef, bDmg, a.PlayerHP, &a.PlayerStatus, &a.PlayerStages, !aFirst)
			skType := 0
			if bDef != nil {
				skType = bDef.Type
			}
			bDmg = applyFreq2DamageSideEffects(bDef, bDmg, b.PlayerHP, b.PlayerMaxHP, skType, a.PlayerType)
			bDmg = applyFreq3DamageSideEffects(bDef, bDmg, b.PlayerHP, a.PlayerHP,
				b.PlayerLevel, b.stagedSpd(true), b.PlayerType, a.PlayerType, b.PlayerConsecSkillCount)
			bDmg = applyFreq4DamageSideEffects(bDef, bDmg, b.PlayerHP, &b.PlayerStages, a.PlayerDef, b.PlayerConsecSkillCount, &a.PlayerStatus)
			bDmg = applyFreq5DamageSideEffects(bDef, bDmg, a.PlayerHP, &b.PlayerEffect795Uses)
			bDmg = applyLeaveOneHP(bDef, a.PlayerHP, bDmg)
			bDmg = applyEndureLeaveOne(&a.PlayerBuff, a.PlayerHP, bDmg)
			if bFromCharge && bDmg > 0 {
				bDmg *= 2
			}
		}
		if cd := sideEffectCounterDamage(bDef, b.PlayerLastTaken); cd > 0 {
			bDmg = cd
		}
	}
	aCritExtra := critExtraWithRounds(a.PlayerCritBonus, a.PlayerCritBonusRounds) + sleepCritExtra(aDef, b.PlayerStatus.Sleep) + critExtraFromStack(&a.PlayerBuff)
	bCritExtra := critExtraWithRounds(b.PlayerCritBonus, b.PlayerCritBonusRounds) + sleepCritExtra(bDef, a.PlayerStatus.Sleep) + critExtraFromStack(&b.PlayerBuff)
	aCrit := aHit && !skillHasSideEffect(aDef, 34) && (mustCritFromBuff(&a.PlayerBuff) || mustCritFromSideEffect193(aDef, &b.PlayerStatus) || mustCritFromAnyStatus(aDef, &b.PlayerStatus) || rollPlayerCrit(aCritExtra))
	bCrit := bHit && !skillHasSideEffect(bDef, 34) && (mustCritFromBuff(&b.PlayerBuff) || mustCritFromSideEffect193(bDef, &a.PlayerStatus) || mustCritFromAnyStatus(bDef, &a.PlayerStatus) || rollPlayerCrit(bCritExtra))
	if aCrit {
		aDmg = aDmg * 3 / 2
		if aDmg < 1 {
			aDmg = 1
		}
	}
	if bCrit {
		bDmg = bDmg * 3 / 2
		if bDmg < 1 {
			bDmg = 1
		}
	}
	if aHit && aDmg > 0 {
		aDmg = applyTraitIncomingDamage(b.PlayerTrait, b.PlayerHP, aDmg)
	}
	if bHit && bDmg > 0 {
		bDmg = applyTraitIncomingDamage(a.PlayerTrait, a.PlayerHP, bDmg)
	}
	if aSkip || !aHit {
		aAtk, aDmg = 0, 0
		if aSkip {
			skillA = 0
		} else if skillHasSideEffect(aDef, 72) {
			a.PlayerHP = 0
		}
	} else {
		aAtk = uint32(aHits)
	}
	if bSkip || !bHit {
		bAtk, bDmg = 0, 0
		if bSkip {
			skillB = 0
		} else if skillHasSideEffect(bDef, 72) {
			b.PlayerHP = 0
		}
	} else {
		bAtk = uint32(bHits)
	}

	// 仅一方出招时，该方视为先手（覆盖速度判定）
	if aAtk > 0 && bAtk == 0 {
		aFirst = true
	} else if bAtk > 0 && aAtk == 0 {
		aFirst = false
	}

	hpA, hpB := a.PlayerHP, b.PlayerHP
	var aLost, bLost uint32
	doA := func(wentFirst bool) {
		if aAtk == 0 || hpB == 0 || hpA == 0 {
			return
		}
		if !wentFirst && skillHasSideEffect(aDef, 34) {
			if cd := sideEffectCounterDamage(aDef, a.PlayerLastTaken); cd > 0 {
				aDmg = cd
			}
		}
		foeHPBefore := hpB
		pink := sideEffectPinkDamage(aDef, foeHPBefore)
		aLost = aDmg
		actual := applyDamage(&hpB, aDmg)
		applyPinkDamage(&hpB, pink)
		noteLastDamageTaken(b, true, actual) // b 是防守方玩家
		a.EnemyLastTaken = actual
		applyReflectDamage(&b.PlayerBuff, actual, &hpA)
		tryCounterDoubleReflect(&b.PlayerBuff, actual, &hpA)
		tryOnHitStatus(&b.PlayerBuff, aDef, &a.PlayerStatus)
		applyOnHurtStageBoost(&b.PlayerBuff, &b.PlayerStages)
		applyOnHurtDefenderBuffs(b, true, actual)
		applyVampOnDamage(&a.PlayerBuff, actual, &hpA, &a.PlayerMaxHP)
		if heal := traitDrainHeal(a.PlayerTrait, actual); heal > 0 {
			hpA += heal
			if hpA > a.PlayerMaxHP {
				hpA = a.PlayerMaxHP
			}
		}
		a.PlayerHP, a.EnemyHP = hpA, hpB
		b.PlayerHP, b.EnemyHP = hpB, hpA
		s.applySkillSideEffects(a, skillA, actual, true, wentFirst)
		tryInvalidateSkill(a, aDef, true, wentFirst)
		if a.EnemySkillFail {
			b.PlayerSkillFail = true
			a.EnemySkillFail = false
		}
		armDoom(a, aDef, true)
		applyFirstStrikeReflect(a, aDef, true, wentFirst)
		applyOnKOEffects(a, aDef, true, foeHPBefore)
		if a.EnemyNextEnterCutDenom > 0 {
			b.PlayerNextEnterCutDenom = a.EnemyNextEnterCutDenom
			a.EnemyNextEnterCutDenom = 0
		}
		applySacrificeEffects(a, aDef, true)
		hpA, hpB = a.PlayerHP, a.EnemyHP
		b.PlayerHP, b.EnemyHP = hpB, hpA
		pushPvPStagesAfterSideEffect(a, b)
		applyTraitReactiveOnHit(b.PlayerTrait, s.skillDef(int(skillA)), actual, &b.PlayerStages, &a.PlayerStages, &a.PlayerStatus)
		syncPvPStagesMirror(a, b)
		hpA, hpB = a.PlayerHP, b.PlayerHP
	}
	doB := func(wentFirst bool) {
		if bAtk == 0 || hpA == 0 || hpB == 0 {
			return
		}
		if !wentFirst && skillHasSideEffect(bDef, 34) {
			if cd := sideEffectCounterDamage(bDef, b.PlayerLastTaken); cd > 0 {
				bDmg = cd
			}
		}
		foeHPBefore := hpA
		pink := sideEffectPinkDamage(bDef, foeHPBefore)
		bLost = bDmg
		actual := applyDamage(&hpA, bDmg)
		applyPinkDamage(&hpA, pink)
		noteLastDamageTaken(a, true, actual)
		b.EnemyLastTaken = actual
		applyReflectDamage(&a.PlayerBuff, actual, &hpB)
		tryCounterDoubleReflect(&a.PlayerBuff, actual, &hpB)
		tryOnHitStatus(&a.PlayerBuff, bDef, &b.PlayerStatus)
		applyOnHurtStageBoost(&a.PlayerBuff, &a.PlayerStages)
		applyOnHurtDefenderBuffs(a, true, actual)
		applyVampOnDamage(&b.PlayerBuff, actual, &hpB, &b.PlayerMaxHP)
		if heal := traitDrainHeal(b.PlayerTrait, actual); heal > 0 {
			hpB += heal
			if hpB > b.PlayerMaxHP {
				hpB = b.PlayerMaxHP
			}
		}
		b.PlayerHP, b.EnemyHP = hpB, hpA
		a.PlayerHP, a.EnemyHP = hpA, hpB
		s.applySkillSideEffects(b, skillB, actual, true, wentFirst)
		tryInvalidateSkill(b, bDef, true, wentFirst)
		if b.EnemySkillFail {
			a.PlayerSkillFail = true
			b.EnemySkillFail = false
		}
		armDoom(b, bDef, true)
		applyFirstStrikeReflect(b, bDef, true, wentFirst)
		applyOnKOEffects(b, bDef, true, foeHPBefore)
		if b.EnemyNextEnterCutDenom > 0 {
			a.PlayerNextEnterCutDenom = b.EnemyNextEnterCutDenom
			b.EnemyNextEnterCutDenom = 0
		}
		applySacrificeEffects(b, bDef, true)
		hpA, hpB = a.PlayerHP, b.PlayerHP
		pushPvPStagesAfterSideEffect(b, a)
		applyTraitReactiveOnHit(a.PlayerTrait, s.skillDef(int(skillB)), actual, &a.PlayerStages, &b.PlayerStages, &b.PlayerStatus)
		syncPvPStagesMirror(a, b)
		hpA, hpB = a.PlayerHP, b.PlayerHP
	}
	if aFirst {
		doA(true)
		if !bSkip && newlyControlledAfterOpponent(b.PlayerStatus, bCtrlAtStart) {
			bSkip = true
			bAtk, bLost, skillB = 0, 0, 0
		}
		if hpB == 0 || hpA == 0 {
			bAtk, bLost, skillB = 0, 0, 0
		} else if bAtk > 0 {
			doB(false)
		}
	} else {
		doB(true)
		if !aSkip && newlyControlledAfterOpponent(a.PlayerStatus, aCtrlAtStart) {
			aSkip = true
			aAtk, aLost, skillA = 0, 0, 0
		}
		if hpA == 0 || hpB == 0 {
			aAtk, aLost, skillA = 0, 0, 0
		} else if aAtk > 0 {
			doA(false)
		}
	}
	a.PlayerHP, b.PlayerHP = hpA, hpB
	a.EnemyHP, b.EnemyHP = b.PlayerHP, a.PlayerHP
	a.EnemyMaxHP, b.EnemyMaxHP = b.PlayerMaxHP, a.PlayerMaxHP
	syncPvPStagesMirror(a, b)
	tickPlayerStatusDamage(a)
	tickPlayerStatusDamage(b)
	tickBattleBuffEffects(a)
	tickTypeOverride(b)
	b.PlayerHP = a.EnemyHP
	b.PlayerMaxHP = a.EnemyMaxHP
	b.PlayerBuff = a.EnemyBuff
	tickCritBonusRounds(a)
	tickCritBonusRounds(b)
	// 延迟秒杀：各自 PlayerDoomRounds 对敌方生效
	tickDoomPvP(a, b)
	hpA, hpB = a.PlayerHP, b.PlayerHP
	syncPvPStagesMirror(a, b)
	a.EnemyHP, b.EnemyHP = hpB, hpA
	a.EnemyMaxHP, b.EnemyMaxHP = b.PlayerMaxHP, a.PlayerMaxHP
	a.EnemySkills = append([][2]uint32(nil), b.PlayerSkills...)
	b.EnemySkills = append([][2]uint32(nil), a.PlayerSkills...)

	aCritF, bCritF := uint32(0), uint32(0)
	if aCrit && aAtk > 0 {
		aCritF = 1
	}
	if bCrit && bAtk > 0 {
		bCritF = 1
	}
	avA := buildAttackValueFromState(uidA, skillA, aAtk, aLost, 0, int32(a.PlayerHP), a.PlayerMaxHP, 0, aCritF, 0, a, true, a.PlayerSkills)
	avB := buildAttackValueFromState(uidB, skillB, bAtk, bLost, 0, int32(b.PlayerHP), b.PlayerMaxHP, 0, bCritF, 0, b, true, b.PlayerSkills)
	decrementBattleBuffRounds(a)
	tickDelayedFullHeal(a)
	tickGrowAtkSpd(a)
	tickFoeStageDot(a)
	tickSelfStageGrow(a)
	tickCondDot439(a)
	a.PlayerEffect795Uses = 0
	b.PlayerEffect795Uses = 0
	b.PlayerBuff = a.EnemyBuff
	b.EnemyBuff = a.PlayerBuff
	b.PlayerDelayedFullHealRounds = a.EnemyDelayedFullHealRounds
	b.EnemyDelayedFullHealRounds = a.PlayerDelayedFullHealRounds
	b.PlayerHP, b.PlayerMaxHP = a.EnemyHP, a.EnemyMaxHP
	a.EnemyHP, a.EnemyMaxHP = b.PlayerHP, b.PlayerMaxHP

	var outA, outB []byte
	if aFirst {
		outA = append(append([]byte{}, avA...), avB...)
		outB = append(append([]byte{}, avB...), avA...)
	} else {
		outA = append(append([]byte{}, avB...), avA...)
		outB = append(append([]byte{}, avA...), avB...)
	}

	aSw, bSw := a.PvPDeferSwitch, b.PvPDeferSwitch
	a.Round++
	b.Round = a.Round
	pvpClearBoth(a, b)
	s.battles.set(int64(uidA), a)
	s.battles.set(int64(uidB), b)

	s.pvpPushDeferredSwitch(uidA, a, uidB, b, aSw, bSw)

	if ca := s.clientOf(int64(uidA)); ca != nil {
		s.send(ca, 2505, uidA, 0, outA)
	}
	if cb := s.clientOf(int64(uidB)); cb != nil {
		s.send(cb, 2505, uidB, 0, outB)
	}
	log.Printf("[CMD] OK     2505 PvP SKILL round=%d %d(%d) vs %d(%d)", a.Round, uidA, a.PlayerHP, uidB, b.PlayerHP)

	if hpB == 0 {
		b.markPetFainted(b.PlayerCatchTime)
		if s.hasOtherLivingPet(uidB, b) {
			s.battles.set(int64(uidA), a)
			s.battles.set(int64(uidB), b)
			log.Printf("[PvP] UID=%d faint wait CHANGE_PET", uidB)
			s.schedulePvPFaintSwitchWatchdog(int64(uidB), b.PlayerCatchTime)
			return
		}
		s.finishPvP(uidA, a, uidB, b, uidA)
		return
	}
	if hpA == 0 {
		a.markPetFainted(a.PlayerCatchTime)
		if s.hasOtherLivingPet(uidA, a) {
			s.battles.set(int64(uidA), a)
			s.battles.set(int64(uidB), b)
			log.Printf("[PvP] UID=%d faint wait CHANGE_PET", uidA)
			s.schedulePvPFaintSwitchWatchdog(int64(uidA), a.PlayerCatchTime)
			return
		}
		s.finishPvP(uidA, a, uidB, b, uidB)
	}
}

// syncOppEnemyFromSelf 将 self 出战镜像到对方 Enemy*。
func (s *Server) syncOppEnemyFromSelf(selfUID int64, self *BattleState, pet *store.Pet) {
	if self == nil || !self.isPvP() {
		return
	}
	opp := s.battles.get(self.OpponentUID)
	if opp == nil {
		return
	}
	if pet != nil {
		s.fillPvPEnemyFromPet(opp, pet)
	}
	opp.EnemyHP = self.PlayerHP
	opp.EnemyMaxHP = self.PlayerMaxHP
	if opp.EnemyHP > opp.EnemyMaxHP {
		opp.EnemyHP = opp.EnemyMaxHP
	}
	opp.EnemyCatchTime = self.PlayerCatchTime
	opp.EnemySkills = append([][2]uint32(nil), self.PlayerSkills...)
	opp.EnemyType = self.PlayerType
	opp.EnemyStages = self.PlayerStages
	opp.EnemyStatus = self.PlayerStatus
	opp.EnemyBuff = self.PlayerBuff
	s.battles.set(self.OpponentUID, opp)
}
