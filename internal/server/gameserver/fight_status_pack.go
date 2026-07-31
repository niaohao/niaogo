package gameserver

// 客户端 PetFightMsgManager.STATUS_ARRAY 下标：
// 0麻痹 1中毒 2烧伤 3吸取对方体力 4被对方吸取体力 5冻伤 6害怕 7疲惫 8睡眠
// … 17免疫能力下降 18免疫异常状态；值为剩余回合（展示用）。

const (
	statusIdxPara         = 0
	statusIdxPoison       = 1
	statusIdxBurn         = 2
	statusIdxDrain        = 3 // 吸取对方的体力（己方在吸）
	statusIdxDrained      = 4 // 被对方吸取体力
	statusIdxFreeze       = 5
	statusIdxFear         = 6
	statusIdxTired        = 7
	statusIdxSleep        = 8
	statusIdxImmuneDrop   = 17
	statusIdxImmuneStatus = 18
)

func encodeFightStatus(st battleStatus) (out [20]byte) {
	if st.Para {
		out[statusIdxPara] = 2
	}
	if st.Poison {
		out[statusIdxPoison] = 2
	}
	if st.Burn {
		out[statusIdxBurn] = 2
	}
	if st.Freeze {
		out[statusIdxFreeze] = 1
	}
	if st.Fear {
		out[statusIdxFear] = 1
	}
	if st.Tired {
		out[statusIdxTired] = 1
	}
	if st.Sleep {
		out[statusIdxSleep] = 1
	}
	return out
}

// encodeFightStatusForSide 含寄生种子与免疫类持续效果（对照参考服 status[3]/[4]/[17]/[18]）。
func encodeFightStatusForSide(st *BattleState, playerSide bool) (out [20]byte) {
	if st == nil {
		return out
	}
	var bs battleStatus
	var selfBuff, foeBuff battleBuff
	if playerSide {
		bs = st.PlayerStatus
		selfBuff, foeBuff = st.PlayerBuff, st.EnemyBuff
	} else {
		bs = st.EnemyStatus
		selfBuff, foeBuff = st.EnemyBuff, st.PlayerBuff
	}
	out = encodeFightStatus(bs)
	// 寄生：参考服 player.Drain + enemy.Drained；本服 DrainRounds 挂在「被吸方」Buff 上
	if foeBuff.DrainRounds > 0 {
		out[statusIdxDrain] = foeBuff.DrainRounds
	}
	if selfBuff.DrainRounds > 0 {
		out[statusIdxDrained] = selfBuff.DrainRounds
	}
	if selfBuff.ImmuneDropRounds > 0 {
		out[statusIdxImmuneDrop] = selfBuff.ImmuneDropRounds
	}
	if selfBuff.ImmuneStatusRounds > 0 {
		out[statusIdxImmuneStatus] = selfBuff.ImmuneStatusRounds
	}
	return out
}

// encodeBattleLv 攻防特攻特防速度 + 命中(本服暂无，恒 0)。
func encodeBattleLv(stages [5]int8) (out [6]int8) {
	copy(out[:5], stages[:])
	return out
}

// controlStatusSnapshot 回合初控场快照（用于后手「本回合新挂上」仍跳过且不提前清除）。
type controlStatusSnapshot struct {
	Para   bool
	Freeze bool
	Fear   bool
	Tired  bool
	Sleep  bool
}

func snapshotControlStatus(st battleStatus) controlStatusSnapshot {
	return controlStatusSnapshot{
		Para: st.Para, Freeze: st.Freeze, Fear: st.Fear, Tired: st.Tired, Sleep: st.Sleep,
	}
}

// newlyControlledAfterOpponent 对手先手后新挂上的控场：本行动槽无法出手，但不清除（图标仍显示）。
func newlyControlledAfterOpponent(cur battleStatus, atStart controlStatusSnapshot) bool {
	return (cur.Sleep && !atStart.Sleep) ||
		(cur.Freeze && !atStart.Freeze) ||
		(cur.Fear && !atStart.Fear) ||
		(cur.Tired && !atStart.Tired) ||
		(cur.Para && !atStart.Para)
}
