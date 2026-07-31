package gameserver

import (
	"math/rand"

	"niaohao/server/internal/tableloader"
)

// battleBuff 单侧持续 SideEffect（回合末递减 rounds）。
type battleBuff struct {
	// 13：被吸取（每回合失 1/8 最大体力给对方）
	DrainRounds byte
	// 21：反弹所受伤害的 1/ReflectDenom
	ReflectRounds byte
	ReflectDenom  byte
	// 42：电系招式伤害翻倍
	ElecDoubleRounds byte
	// 44：受特殊攻击伤害减半
	SpecHalfRounds byte
	// 46：接下来 ImmuneHits 次攻击伤害变为 0
	ImmuneHits byte
	// 47：免疫能力下降
	ImmuneDropRounds byte
	// 48：免疫异常
	ImmuneStatusRounds byte
	// 49：下次受击伤害减去 FlatReduceNext
	FlatReduceNext uint32
	// 50：受物理攻击伤害减半
	PhysHalfRounds byte
	// 53：自身攻击伤害 × DmgMul（默认 2）
	DmgMulRounds byte
	DmgMul       byte
	// 54：自身攻击伤害变为 1/OutDmgDenom
	OutDmgDenomRounds byte
	OutDmgDenom       byte
	// 57：每回合回复最大体力 1/HealDenom
	HealRounds byte
	HealDenom  byte
	// 58：必中要害（暴击）
	MustCritRounds byte
	// 60/76：每回合固定伤害
	DotRounds byte
	DotFlat   uint32
	// 65：指定属性技能威力倍率
	TypeBoostRounds byte
	TypeBoostType   byte
	TypeBoostMul    byte
	// 68：本回合致死留 1
	EndureRounds byte
	// 77：每回合固定回血
	FlatHealRounds byte
	FlatHealAmt    uint32
	// 78：物理攻击必 miss
	PhysMissRounds byte
	// 86：特殊攻击必 miss
	SpecMissRounds byte
	// 73：本回合先手后受伤双倍反击（一次性）
	CounterDouble bool
	// 81：直接攻击必中
	MustHitRounds byte
	// 41：火系伤害减半
	FireHalfRounds byte
	// 89：造成伤害吸血
	VampRounds byte
	VampDenom  byte
	// 84：受物理攻击概率麻痹对手
	OnHitParaRounds byte
	OnHitParaChance byte
	// 92：受物理攻击概率冻伤对手
	OnHitFreezeRounds byte
	OnHitFreezeChance byte
	// 45/51：能力与对方相同
	EqualFoeDefRounds byte
	EqualFoeAtkRounds byte
	// 91：对手状态变化同步到自身
	SyncChangesRounds byte
	// 83：强制先手（雄性）
	PriorityRounds byte
	// 98：对雄性伤害 ×Mul
	MaleDmgRounds byte
	MaleDmgMul    byte
	// 108：受物理攻击概率烧伤对手
	OnHitBurnRounds byte
	OnHitBurnChance byte
	// 123：受击时自身能力上升
	OnHurtBoostRounds byte
	OnHurtBoostStat   byte
	OnHurtBoostDelta  int8
	// 463：每回合所受伤害减少 FlatReduceAmt，持续 FlatReduceRounds
	FlatReduceRounds byte
	FlatReduceAmt    uint32
	// 104：每次直接攻击概率附带衰弱
	OnHitWeakRounds  byte
	OnHitWeakChance  byte
	// 109：造成伤害时概率冻伤
	OnHitFreezeAtkRounds byte
	OnHitFreezeAtkChance byte
	// 110：躲避攻击时概率自升
	OnDodgeBoostRounds byte
	OnDodgeBoostChance byte
	OnDodgeBoostStat   byte
	// 116：先手吸血 20%
	FirstVampRounds byte
	// 117：先手害怕
	FirstFearRounds byte
	// 125：单次受伤上限
	DmgCapRounds byte
	DmgCap       uint32
	// 126：每回合攻速上升
	GrowAtkSpdRounds byte
	GrowAtkSpdDelta  int8
	// 128：受伤转回血
	AbsorbRounds byte
	// 190：受伤清敌强化
	OnHurtClearBoostRounds byte
	// 439：自身弱化/异常时敌方每回合固伤
	CondDotRounds byte
	CondDotFlat   uint32
	// 441：连击提升暴击（累计点数，/16 制额外）
	CritStackRounds byte
	CritStackStep   byte
	CritStackMax    byte
	CritStackCur    byte
	// 448：每回合压低对手能力
	FoeStageDotRounds byte
	FoeStageDotDelta  [5]int8 // 仅用前 5 项（命中忽略）
	// 461/475：低血/低伤后必暴
	// （复用 MustCritRounds）
	// 478：对手属性技无效
	BlockAttrSkillRounds byte
	// 545：受伤超阈值则给对手异常
	OnHurtStatusRounds byte
	OnHurtStatusThr    uint32
	OnHurtStatusIdx    byte
	// 156：自身能力增强失效（正强化视为 0）
	BoostNullRounds byte
	// 433：每回合提升自身能力
	SelfStageGrowRounds byte
	SelfStageGrowDelta  [5]int8
	// 1211：伤害吸收盾
	DamageAbsorb uint32
	// 1850：下 n 次攻击伤害 +pct%
	OutDmgBoostHits byte
	OutDmgBoostPct  byte
	// 2236：下 n 次支援类回复翻倍
	SupportDoubleHits byte
	// 454：残血强制先手（复用 PriorityRounds，由结算写入）
}

func pickSelfBuff(st *BattleState, playerIsAtk bool) *battleBuff {
	if playerIsAtk {
		return &st.PlayerBuff
	}
	return &st.EnemyBuff
}

func pickFoeBuff(st *BattleState, playerIsAtk bool) *battleBuff {
	if playerIsAtk {
		return &st.EnemyBuff
	}
	return &st.PlayerBuff
}

func readRoundsArg(args []int, off *int, def int) int {
	if off == nil {
		return def
	}
	if *off < len(args) && args[*off] > 0 {
		v := args[*off]
		*off++
		return v
	}
	return def
}

func clampRounds(n int) byte {
	if n < 1 {
		n = 1
	}
	if n > 16 {
		n = 16
	}
	return byte(n)
}

// applyOneOngoingBuff 按单个 eid 写入持续效果并推进 argOff。
func applyOneOngoingBuff(st *BattleState, eid int, args []int, argOff int, playerIsAtk bool) int {
	if st == nil {
		return argOff
	}
	self := pickSelfBuff(st, playerIsAtk)
	foe := pickFoeBuff(st, playerIsAtk)
	switch eid {
	case 13:
		rounds := readRoundsArg(args, &argOff, 5)
		foeType := st.EnemyType
		if !playerIsAtk {
			foeType = st.PlayerType
		}
		if foeType != 1 {
			foe.DrainRounds = clampRounds(rounds)
		}
	case 21:
		lo := readRoundsArg(args, &argOff, 3)
		hi := readRoundsArg(args, &argOff, lo)
		denom := readRoundsArg(args, &argOff, 2)
		if hi < lo {
			hi = lo
		}
		r := lo
		if hi > lo {
			r = lo + rand.Intn(hi-lo+1)
		}
		self.ReflectRounds = clampRounds(r)
		if denom < 1 {
			denom = 2
		}
		self.ReflectDenom = byte(denom)
	case 42:
		lo := readRoundsArg(args, &argOff, 3)
		hi := readRoundsArg(args, &argOff, lo)
		r := lo
		if hi > lo {
			r = lo + rand.Intn(hi-lo+1)
		}
		self.ElecDoubleRounds = clampRounds(r)
	case 44:
		self.SpecHalfRounds = clampRounds(readRoundsArg(args, &argOff, 3))
	case 46:
		n := readRoundsArg(args, &argOff, 1)
		if n < 1 {
			n = 1
		}
		if n > 8 {
			n = 8
		}
		self.ImmuneHits = byte(n)
	case 47:
		self.ImmuneDropRounds = clampRounds(readRoundsArg(args, &argOff, 3))
	case 48:
		self.ImmuneStatusRounds = clampRounds(readRoundsArg(args, &argOff, 3))
	case 49:
		flat := readRoundsArg(args, &argOff, 20)
		if flat < 0 {
			flat = 0
		}
		self.FlatReduceNext = uint32(flat)
	case 50:
		self.PhysHalfRounds = clampRounds(readRoundsArg(args, &argOff, 3))
	case 53:
		rounds := readRoundsArg(args, &argOff, 3)
		mul := readRoundsArg(args, &argOff, 2)
		if mul < 2 {
			mul = 2
		}
		self.DmgMulRounds = clampRounds(rounds)
		self.DmgMul = byte(mul)
	case 54:
		rounds := readRoundsArg(args, &argOff, 3)
		denom := readRoundsArg(args, &argOff, 2)
		if denom < 2 {
			denom = 2
		}
		foe.OutDmgDenomRounds = clampRounds(rounds)
		foe.OutDmgDenom = byte(denom)
	case 57:
		rounds := readRoundsArg(args, &argOff, 3)
		denom := readRoundsArg(args, &argOff, 8)
		if denom < 1 {
			denom = 8
		}
		self.HealRounds = clampRounds(rounds)
		self.HealDenom = byte(denom)
	case 58:
		self.MustCritRounds = clampRounds(readRoundsArg(args, &argOff, 3))
	case 60:
		rounds := readRoundsArg(args, &argOff, 3)
		flat := readRoundsArg(args, &argOff, 20)
		if flat < 1 {
			flat = 1
		}
		foe.DotRounds = clampRounds(rounds)
		foe.DotFlat = uint32(flat)
	case 65:
		rounds := readRoundsArg(args, &argOff, 3)
		typ := readRoundsArg(args, &argOff, 0)
		mul := readRoundsArg(args, &argOff, 2)
		if mul < 2 {
			mul = 2
		}
		self.TypeBoostRounds = clampRounds(rounds)
		self.TypeBoostType = byte(typ)
		self.TypeBoostMul = byte(mul)
	case 68:
		self.EndureRounds = clampRounds(readRoundsArg(args, &argOff, 1))
	case 76:
		chance := readRoundsArg(args, &argOff, 30)
		rounds := readRoundsArg(args, &argOff, 3)
		flat := readRoundsArg(args, &argOff, 20)
		if chance > 100 {
			chance = 100
		}
		if chance < 100 && rand.Intn(100) >= chance {
			break
		}
		if flat < 1 {
			flat = 1
		}
		foe.DotRounds = clampRounds(rounds)
		foe.DotFlat = uint32(flat)
	case 77:
		rounds := readRoundsArg(args, &argOff, 3)
		amt := readRoundsArg(args, &argOff, 20)
		if amt < 1 {
			amt = 1
		}
		self.FlatHealRounds = clampRounds(rounds)
		self.FlatHealAmt = uint32(amt)
	case 78:
		self.PhysMissRounds = clampRounds(readRoundsArg(args, &argOff, 1))
	case 86:
		self.SpecMissRounds = clampRounds(readRoundsArg(args, &argOff, 1))
	case 81:
		self.MustHitRounds = clampRounds(readRoundsArg(args, &argOff, 1))
	case 41:
		// Arg: lo hi（常用 3 3 / 5 5）→ 取高值回合
		lo := readRoundsArg(args, &argOff, 3)
		hi := readRoundsArg(args, &argOff, lo)
		r := lo
		if hi > lo {
			r = hi
		}
		self.FireHalfRounds = clampRounds(r)
	case 89:
		rounds := readRoundsArg(args, &argOff, 3)
		denom := readRoundsArg(args, &argOff, 8)
		if denom < 1 {
			denom = 8
		}
		self.VampRounds = clampRounds(rounds)
		self.VampDenom = byte(denom)
	case 90:
		// 与 53 同语义：n 回合伤害 ×m
		rounds := readRoundsArg(args, &argOff, 2)
		mul := readRoundsArg(args, &argOff, 2)
		if mul < 2 {
			mul = 2
		}
		self.DmgMulRounds = clampRounds(rounds)
		self.DmgMul = byte(mul)
	case 84:
		rounds := readRoundsArg(args, &argOff, 5)
		chance := readRoundsArg(args, &argOff, 50)
		if chance > 100 {
			chance = 100
		}
		self.OnHitParaRounds = clampRounds(rounds)
		self.OnHitParaChance = byte(chance)
	case 92:
		rounds := readRoundsArg(args, &argOff, 5)
		chance := readRoundsArg(args, &argOff, 50)
		if chance > 100 {
			chance = 100
		}
		self.OnHitFreezeRounds = clampRounds(rounds)
		self.OnHitFreezeChance = byte(chance)
	case 45:
		self.EqualFoeDefRounds = clampRounds(readRoundsArg(args, &argOff, 5))
	case 51:
		self.EqualFoeAtkRounds = clampRounds(readRoundsArg(args, &argOff, 5))
	case 91:
		self.SyncChangesRounds = clampRounds(readRoundsArg(args, &argOff, 5))
	case 98:
		rounds := readRoundsArg(args, &argOff, 3)
		mul := readRoundsArg(args, &argOff, 2)
		if mul < 1 {
			mul = 2
		}
		self.MaleDmgRounds = clampRounds(rounds)
		self.MaleDmgMul = byte(mul)
	case 108:
		rounds := readRoundsArg(args, &argOff, 5)
		chance := readRoundsArg(args, &argOff, 50)
		if chance > 100 {
			chance = 100
		}
		self.OnHitBurnRounds = clampRounds(rounds)
		self.OnHitBurnChance = byte(chance)
	case 123:
		rounds := readRoundsArg(args, &argOff, 3)
		stat := readRoundsArg(args, &argOff, stageAtk)
		delta := readRoundsArg(args, &argOff, 1)
		if delta < 1 {
			delta = 1
		}
		self.OnHurtBoostRounds = clampRounds(rounds)
		self.OnHurtBoostStat = byte(stat)
		self.OnHurtBoostDelta = int8(delta)
	case 463:
		rounds := readRoundsArg(args, &argOff, 3)
		amt := readRoundsArg(args, &argOff, 20)
		if amt < 1 {
			amt = 1
		}
		self.FlatReduceRounds = clampRounds(rounds)
		self.FlatReduceAmt = uint32(amt)
	case 104:
		rounds := readRoundsArg(args, &argOff, 3)
		chance := readRoundsArg(args, &argOff, 30)
		if chance > 100 {
			chance = 100
		}
		self.OnHitWeakRounds = clampRounds(rounds)
		self.OnHitWeakChance = byte(chance)
	case 106:
		self.SpecMissRounds = clampRounds(readRoundsArg(args, &argOff, 3))
	case 109:
		rounds := readRoundsArg(args, &argOff, 3)
		chance := readRoundsArg(args, &argOff, 30)
		if chance > 100 {
			chance = 100
		}
		self.OnHitFreezeAtkRounds = clampRounds(rounds)
		self.OnHitFreezeAtkChance = byte(chance)
	case 110:
		rounds := readRoundsArg(args, &argOff, 3)
		chance := readRoundsArg(args, &argOff, 30)
		stat := readRoundsArg(args, &argOff, stageAtk)
		if chance > 100 {
			chance = 100
		}
		self.OnDodgeBoostRounds = clampRounds(rounds)
		self.OnDodgeBoostChance = byte(chance)
		self.OnDodgeBoostStat = byte(stat)
	case 116:
		self.FirstVampRounds = clampRounds(3)
	case 117:
		self.FirstFearRounds = clampRounds(5)
	case 125:
		rounds := readRoundsArg(args, &argOff, 3)
		cap := readRoundsArg(args, &argOff, 100)
		if cap < 1 {
			cap = 1
		}
		self.DmgCapRounds = clampRounds(rounds)
		self.DmgCap = uint32(cap)
	case 126:
		rounds := readRoundsArg(args, &argOff, 3)
		delta := readRoundsArg(args, &argOff, 1)
		if delta < 1 {
			delta = 1
		}
		self.GrowAtkSpdRounds = clampRounds(rounds)
		self.GrowAtkSpdDelta = int8(delta)
	case 128:
		self.AbsorbRounds = clampRounds(readRoundsArg(args, &argOff, 3))
	case 190:
		self.OnHurtClearBoostRounds = clampRounds(readRoundsArg(args, &argOff, 3))
	case 439:
		rounds := readRoundsArg(args, &argOff, 3)
		flat := readRoundsArg(args, &argOff, 50)
		if flat < 1 {
			flat = 1
		}
		self.CondDotRounds = clampRounds(rounds)
		self.CondDotFlat = uint32(flat)
	case 441:
		step := readRoundsArg(args, &argOff, 5)
		maxS := readRoundsArg(args, &argOff, 30)
		if step < 1 {
			step = 1
		}
		if maxS < step {
			maxS = step
		}
		self.CritStackRounds = clampRounds(5)
		self.CritStackStep = byte(step)
		self.CritStackMax = byte(maxS)
	case 448:
		rounds := readRoundsArg(args, &argOff, 3)
		self.FoeStageDotRounds = clampRounds(rounds)
		for i := 0; i < 5; i++ {
			if argOff < len(args) {
				self.FoeStageDotDelta[i] = int8(args[argOff])
				argOff++
			}
		}
		if argOff < len(args) { // 命中等级占位
			argOff++
		}
	case 156:
		foe.BoostNullRounds = clampRounds(readRoundsArg(args, &argOff, 3))
	case 433:
		rounds := readRoundsArg(args, &argOff, 3)
		self.SelfStageGrowRounds = clampRounds(rounds)
		for i := 0; i < 5; i++ {
			if argOff < len(args) {
				self.SelfStageGrowDelta[i] = int8(args[argOff])
				argOff++
			}
		}
		if argOff < len(args) { // 命中等级占位
			argOff++
		}
	case 478:
		foe.BlockAttrSkillRounds = clampRounds(readRoundsArg(args, &argOff, 3))
	case 545:
		rounds := readRoundsArg(args, &argOff, 3)
		thr := readRoundsArg(args, &argOff, 100)
		idx := readRoundsArg(args, &argOff, 0)
		self.OnHurtStatusRounds = clampRounds(rounds)
		self.OnHurtStatusThr = uint32(thr)
		self.OnHurtStatusIdx = byte(idx)
	}
	return argOff
}

// applyOngoingBuffSideEffects 保留给测试；生产路径走 applyOneOngoingBuff。
func applyOngoingBuffSideEffects(st *BattleState, d *tableloader.SkillDef, ids []int, args []int, argOff int, playerIsAtk bool) int {
	_ = d
	for _, eid := range ids {
		switch eid {
		case 13, 21, 41, 42, 44, 45, 46, 47, 48, 49, 50, 51, 53, 54, 57, 58, 60, 65, 68, 76, 77, 78, 81, 84, 86, 89, 90, 91, 92, 98, 104, 106, 108, 109, 110, 116, 117, 123, 125, 126, 128, 190, 439, 441, 448, 463, 478, 545:
			argOff = applyOneOngoingBuff(st, eid, args, argOff, playerIsAtk)
		}
	}
	return argOff
}

// sideEffectPercentHPDamage SideEffect 28：削减对方当前体力 1/n。
func sideEffectPercentHPDamage(d *tableloader.SkillDef, foeHP uint32) uint32 {
	if d == nil || !skillHasSideEffect(d, 28) || foeHP == 0 {
		return 0
	}
	args := parseSideEffectArgs(d.SideEffectArg)
	n := 4
	if len(args) >= 1 && args[0] > 0 {
		n = args[0]
	}
	if n < 1 {
		n = 1
	}
	dmg := foeHP / uint32(n)
	if dmg < 1 {
		dmg = 1
	}
	return dmg
}

// sideEffectOHKO SideEffect 36：n% 秒杀。
func sideEffectOHKO(d *tableloader.SkillDef, foeHP uint32) uint32 {
	if d == nil || !skillHasSideEffect(d, 36) || foeHP == 0 {
		return 0
	}
	args := parseSideEffectArgs(d.SideEffectArg)
	chance := 30
	if len(args) >= 1 {
		chance = args[0]
	}
	if chance > 100 {
		chance = 100
	}
	if chance < 100 && rand.Intn(100) >= chance {
		return 0
	}
	return foeHP
}

// sideEffectPPDrain SideEffect 39：n% 减少对方所有技能 m 点 PP。
func sideEffectPPDrain(d *tableloader.SkillDef, skills [][2]uint32) {
	if d == nil || !skillHasSideEffect(d, 39) || len(skills) == 0 {
		return
	}
	args := parseSideEffectArgs(d.SideEffectArg)
	chance, cut := 30, 1
	if len(args) >= 1 {
		chance = args[0]
	}
	if len(args) >= 2 && args[1] > 0 {
		cut = args[1]
	}
	if chance > 100 {
		chance = 100
	}
	if chance < 100 && rand.Intn(100) >= chance {
		return
	}
	for i := range skills {
		if skills[i][0] == 0 {
			continue
		}
		if skills[i][1] > uint32(cut) {
			skills[i][1] -= uint32(cut)
		} else {
			skills[i][1] = 0
		}
	}
}

// sideEffectCutMaxHP SideEffect 38：对方最大体力下降 n（不低于当前体力下限 1）。
func sideEffectCutMaxHP(d *tableloader.SkillDef, maxHP, hp *uint32) {
	if d == nil || !skillHasSideEffect(d, 38) || maxHP == nil || hp == nil {
		return
	}
	args := parseSideEffectArgs(d.SideEffectArg)
	cut := 20
	if len(args) >= 1 && args[0] > 0 {
		cut = args[0]
	}
	if *maxHP > uint32(cut)+1 {
		*maxHP -= uint32(cut)
	} else {
		*maxHP = 1
	}
	if *hp > *maxHP {
		*hp = *maxHP
	}
}

// adjustSkillPowerSelfHP SideEffect 37：自身 HP < 1/n 时威力 ×mul。
func adjustSkillPowerSelfHP(d *tableloader.SkillDef, power int, selfHP, selfMax uint32) int {
	if d == nil || !skillHasSideEffect(d, 37) || power <= 0 || selfMax == 0 {
		return power
	}
	args := parseSideEffectArgs(d.SideEffectArg)
	denom, mul := 3, 2
	if len(args) >= 1 && args[0] > 0 {
		denom = args[0]
	}
	if len(args) >= 2 && args[1] > 0 {
		mul = args[1]
	}
	if denom < 1 {
		denom = 1
	}
	if selfHP*uint32(denom) >= selfMax {
		return power
	}
	return power * mul
}

// applyOutgoingDamageBuff 攻击方持续伤害修正（53/54/42）。
func applyOutgoingDamageBuff(atk *battleBuff, sk *tableloader.SkillDef, dmg uint32) uint32 {
	if dmg == 0 || atk == nil {
		return dmg
	}
	if atk.OutDmgDenomRounds > 0 && atk.OutDmgDenom > 1 {
		dmg = dmg / uint32(atk.OutDmgDenom)
		if dmg < 1 {
			dmg = 1
		}
	}
	if atk.DmgMulRounds > 0 && atk.DmgMul > 1 {
		dmg *= uint32(atk.DmgMul)
	}
	if atk.ElecDoubleRounds > 0 && sk != nil && sk.Type == 5 {
		dmg *= 2
	}
	if atk.TypeBoostRounds > 0 && atk.TypeBoostMul > 1 && sk != nil && sk.Type == int(atk.TypeBoostType) {
		dmg *= uint32(atk.TypeBoostMul)
	}
	if atk.OutDmgBoostHits > 0 && atk.OutDmgBoostPct > 0 {
		dmg = dmg * uint32(100+int(atk.OutDmgBoostPct)) / 100
		atk.OutDmgBoostHits--
	}
	return dmg
}

// applyIncomingDamageBuff 防守方持续伤害修正（44/46/49/50）；返回实际伤害与是否触发反弹。
func applyIncomingDamageBuff(def *battleBuff, sk *tableloader.SkillDef, dmg uint32) uint32 {
	if dmg == 0 || def == nil {
		return dmg
	}
	if def.ImmuneHits > 0 {
		def.ImmuneHits--
		return 0
	}
	if def.DamageAbsorb > 0 {
		if dmg <= def.DamageAbsorb {
			def.DamageAbsorb -= dmg
			return 0
		}
		dmg -= def.DamageAbsorb
		def.DamageAbsorb = 0
	}
	if def.FlatReduceNext > 0 {
		if dmg > def.FlatReduceNext {
			dmg -= def.FlatReduceNext
		} else {
			dmg = 0
		}
		def.FlatReduceNext = 0
	}
	if def.FlatReduceRounds > 0 && def.FlatReduceAmt > 0 {
		if dmg > def.FlatReduceAmt {
			dmg -= def.FlatReduceAmt
		} else {
			dmg = 0
		}
	}
	if def.DmgCapRounds > 0 && def.DmgCap > 0 && dmg > def.DmgCap {
		dmg = def.DmgCap
	}
	if def.AbsorbRounds > 0 && dmg > 0 {
		// 受伤转回血：在 applyAbsorbHeal 中处理，这里只标记
	}
	cat := 1
	if sk != nil {
		cat = sk.Category
		if cat == 0 {
			cat = 1
		}
	}
	if cat == 1 && def.PhysHalfRounds > 0 {
		dmg /= 2
	}
	if cat == 2 && def.SpecHalfRounds > 0 {
		dmg /= 2
	}
	// SideEffect 41：火系（Type=3）伤害减半
	if def.FireHalfRounds > 0 && sk != nil && sk.Type == 3 {
		dmg /= 2
	}
	return dmg
}

// applyReflectDamage SideEffect 21：受击后反弹。
func applyReflectDamage(def *battleBuff, lost uint32, atkHP *uint32) {
	if def == nil || lost == 0 || atkHP == nil || def.ReflectRounds == 0 || def.ReflectDenom < 1 {
		return
	}
	recoil := lost / uint32(def.ReflectDenom)
	if recoil < 1 {
		recoil = 1
	}
	_ = applyDamage(atkHP, recoil)
}

// applyEndureLeaveOne SideEffect 68：致死时留 1。
func applyEndureLeaveOne(def *battleBuff, hpBefore, dmg uint32) uint32 {
	if def == nil || def.EndureRounds == 0 || dmg == 0 || hpBefore <= 1 {
		return dmg
	}
	if dmg >= hpBefore {
		return hpBefore - 1
	}
	return dmg
}

// transferNegativeStages SideEffect 63。
func transferNegativeStages(self, foe *[5]int8) {
	if self == nil || foe == nil {
		return
	}
	for i := range self {
		if self[i] < 0 {
			foe[i] = int8(clampStage(int(foe[i]) + int(self[i])))
			self[i] = 0
		}
	}
}

// sideEffectRandomPower SideEffect 61/70/118：随机威力。
func sideEffectRandomPower(d *tableloader.SkillDef, base int) int {
	if d == nil {
		return base
	}
	if skillHasSideEffect(d, 61) {
		return 50 + rand.Intn(101) // 50~150
	}
	if skillHasSideEffect(d, 70) || skillHasSideEffect(d, 118) {
		return 150 + rand.Intn(71) // 150~220
	}
	return base
}

// sideEffectStatusTriple SideEffect 74/75：三选一异常。
func sideEffectStatusTriple(d *tableloader.SkillDef, st *BattleState, playerIsAtk bool) {
	if d == nil || st == nil {
		return
	}
	var pool []int
	if skillHasSideEffect(d, 74) {
		pool = []int{11, 12, 14} // 毒/烧/冻
	} else if skillHasSideEffect(d, 75) {
		pool = []int{10, 16, 15} // 麻/睡/怕
	} else {
		return
	}
	if rand.Intn(100) >= 30 {
		return
	}
	eid := pool[rand.Intn(len(pool))]
	if playerIsAtk {
		if statusImmuneFromBuff(&st.EnemyBuff) || !canApplyEnemyBattleStatus(st, eid) {
			return
		}
		setStatus(&st.EnemyStatus, eid)
	} else {
		if statusImmuneFromBuff(&st.PlayerBuff) {
			return
		}
		setStatus(&st.PlayerStatus, eid)
	}
}

// statusPowerBoost SideEffect 64：烧伤/冻伤/中毒时伤害×2。
func statusPowerBoost(d *tableloader.SkillDef, status *battleStatus, dmg uint32) uint32 {
	if d == nil || status == nil || dmg == 0 || !skillHasSideEffect(d, 64) {
		return dmg
	}
	if status.Burn || status.Freeze || status.Poison {
		return dmg * 2
	}
	return dmg
}

// physMissForced SideEffect 78：物理攻击对自身必 miss。
func physMissForced(def *battleBuff, sk *tableloader.SkillDef) bool {
	if def == nil || def.PhysMissRounds == 0 || sk == nil {
		return false
	}
	cat := sk.Category
	if cat == 0 {
		cat = 1
	}
	return cat == 1
}

func mustCritFromBuff(b *battleBuff) bool {
	return b != nil && b.MustCritRounds > 0
}

func statusImmuneFromBuff(b *battleBuff) bool {
	return b != nil && b.ImmuneStatusRounds > 0
}

func stageDropImmuneFromBuff(b *battleBuff) bool {
	return b != nil && b.ImmuneDropRounds > 0
}

// tickBattleBuffs 回合末：吸取/回血 + 递减回合。
func tickBattleBuffs(st *BattleState) {
	tickBattleBuffEffects(st)
	decrementBattleBuffRounds(st)
}

// tickBattleBuffEffects 仅结算吸取/回血/DoT 等血量变化，不递减回合（便于 2505 先带满额状态图标）。
func tickBattleBuffEffects(st *BattleState) {
	if st == nil {
		return
	}
	if st.EnemyBuff.DrainRounds > 0 && st.EnemyHP > 0 && st.EnemyMaxHP > 0 {
		d := st.EnemyMaxHP / 8
		if d < 1 {
			d = 1
		}
		lost := applyDamage(&st.EnemyHP, d)
		st.PlayerHP += lost
		if st.PlayerHP > st.PlayerMaxHP {
			st.PlayerHP = st.PlayerMaxHP
		}
	}
	if st.PlayerBuff.DrainRounds > 0 && st.PlayerHP > 0 && st.PlayerMaxHP > 0 {
		d := st.PlayerMaxHP / 8
		if d < 1 {
			d = 1
		}
		lost := applyDamage(&st.PlayerHP, d)
		st.EnemyHP += lost
		if st.EnemyHP > st.EnemyMaxHP {
			st.EnemyHP = st.EnemyMaxHP
		}
	}
	healSide := func(b *battleBuff, hp, max *uint32) {
		if b == nil || b.HealRounds == 0 || b.HealDenom < 1 || hp == nil || max == nil || *hp == 0 {
			return
		}
		h := *max / uint32(b.HealDenom)
		if h < 1 {
			h = 1
		}
		*hp += h
		if *hp > *max {
			*hp = *max
		}
	}
	healSide(&st.PlayerBuff, &st.PlayerHP, &st.PlayerMaxHP)
	healSide(&st.EnemyBuff, &st.EnemyHP, &st.EnemyMaxHP)

	flatDot := func(b *battleBuff, hp *uint32) {
		if b == nil || b.DotRounds == 0 || b.DotFlat == 0 || hp == nil || *hp == 0 {
			return
		}
		_ = applyDamage(hp, b.DotFlat)
	}
	flatDot(&st.EnemyBuff, &st.EnemyHP)
	flatDot(&st.PlayerBuff, &st.PlayerHP)
	flatHeal := func(b *battleBuff, hp, max *uint32) {
		if b == nil || b.FlatHealRounds == 0 || b.FlatHealAmt == 0 || hp == nil || max == nil || *hp == 0 {
			return
		}
		*hp += b.FlatHealAmt
		if *hp > *max {
			*hp = *max
		}
	}
	flatHeal(&st.PlayerBuff, &st.PlayerHP, &st.PlayerMaxHP)
	flatHeal(&st.EnemyBuff, &st.EnemyHP, &st.EnemyMaxHP)
}

func decrementBattleBuffRounds(st *BattleState) {
	if st == nil {
		return
	}
	dec := func(b *battleBuff) {
		if b == nil {
			return
		}
		dec1 := func(v *byte) {
			if *v > 0 {
				*v--
			}
		}
		dec1(&b.DrainRounds)
		dec1(&b.ReflectRounds)
		dec1(&b.ElecDoubleRounds)
		dec1(&b.SpecHalfRounds)
		dec1(&b.ImmuneDropRounds)
		dec1(&b.ImmuneStatusRounds)
		dec1(&b.PhysHalfRounds)
		dec1(&b.DmgMulRounds)
		dec1(&b.OutDmgDenomRounds)
		dec1(&b.HealRounds)
		dec1(&b.MustCritRounds)
		dec1(&b.DotRounds)
		dec1(&b.TypeBoostRounds)
		dec1(&b.EndureRounds)
		dec1(&b.FlatHealRounds)
		dec1(&b.PhysMissRounds)
		dec1(&b.SpecMissRounds)
		dec1(&b.MustHitRounds)
		dec1(&b.FireHalfRounds)
		dec1(&b.VampRounds)
		dec1(&b.OnHitParaRounds)
		dec1(&b.OnHitFreezeRounds)
		dec1(&b.EqualFoeDefRounds)
		dec1(&b.EqualFoeAtkRounds)
		dec1(&b.SyncChangesRounds)
		dec1(&b.PriorityRounds)
		dec1(&b.MaleDmgRounds)
		dec1(&b.OnHitBurnRounds)
		dec1(&b.OnHurtBoostRounds)
		dec1(&b.FlatReduceRounds)
		dec1(&b.OnHitWeakRounds)
		dec1(&b.OnHitFreezeAtkRounds)
		dec1(&b.OnDodgeBoostRounds)
		dec1(&b.FirstVampRounds)
		dec1(&b.FirstFearRounds)
		dec1(&b.DmgCapRounds)
		dec1(&b.GrowAtkSpdRounds)
		dec1(&b.AbsorbRounds)
		dec1(&b.OnHurtClearBoostRounds)
		dec1(&b.CondDotRounds)
		dec1(&b.CritStackRounds)
		dec1(&b.FoeStageDotRounds)
		dec1(&b.BlockAttrSkillRounds)
		dec1(&b.OnHurtStatusRounds)
		dec1(&b.BoostNullRounds)
		dec1(&b.SelfStageGrowRounds)
	}
	dec(&st.PlayerBuff)
	dec(&st.EnemyBuff)
	tickTypeOverride(st)
}
