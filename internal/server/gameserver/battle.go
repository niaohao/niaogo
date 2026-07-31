package gameserver

import (
	"bytes"
	"encoding/binary"
	"math/rand"
	"sync"

	"niaohao/server/internal/packet"
	"niaohao/server/internal/store"
	"niaohao/server/internal/tableloader"
)

// 未配置 BOSS / 野怪槽为空时的回退敌人（pets.xml ID=58 塔奇拉顿，地面系）。
const (
	fallbackEnemyPetID = 58
	fallbackEnemyName  = "塔奇拉顿"
)

// 战斗结束 reason（对齐客户端 PetFightController / 参考 battle.REASON_*）
const (
	fightReasonNormal   = 0
	fightReasonExit     = 1 // 对方退出/断线
	fightReasonOvertime = 2 // 对方操作超时
	fightReasonError    = 4
	fightReasonEscape   = 5
)

const (
	fightKindNormal = 0
	fightKindBrave  = 1
	fightKindFresh  = 2
)

const (
	dailyExpNone       = 0
	dailyExpPetKing    = 1
	dailyExpGrandMelee = 2
)

// BattleState PvE/PvP 对战状态（2411→2503→2404→2505… / 2401→2501→2403→2503…）。
type BattleState struct {
	Active    bool
	MapID     int
	Round     uint32
	FightKind int

	EnemyID        int
	EnemyLevel     int
	EnemyName      string
	EnemyHP        uint32
	EnemyMaxHP     uint32
	EnemyCatchTime uint32 // PvP：对方出战 catchTime（2504 须非 0）
	EnemyAtk       int
	EnemyDef       int
	EnemySpAtk     int
	EnemySpDef     int
	EnemySpd       int
	EnemySkills    [][2]uint32
	EnemyCatchable bool
	EnemyType      int

	PlayerPetID     int
	PlayerLevel     int
	PlayerName      string
	PlayerCatchTime uint32
	PlayerHP        uint32
	PlayerMaxHP     uint32
	PlayerAtk       int
	PlayerDef       int
	PlayerSpAtk     int
	PlayerSpDef     int
	PlayerSpd       int
	PlayerSkills    [][2]uint32 // id, pp
	PlayerType      int
	PlayerCritBonus int // 能量珠 Eid=30 + 特性会心：额外致命分母（/16）
	PlayerTrait     int // 特性 NewSeIdx 1006-1045
	PlayerDV        int // 个体（113/1901）
	EnemyDV         int
	PlayerStages    [5]int8 // 攻防特攻特防速度
	EnemyStages     [5]int8
	PlayerStatus    battleStatus
	EnemyStatus     battleStatus

	// SideEffect 9：连续同一技能威力递增
	PlayerConsecSkillID    uint32
	PlayerConsecSkillCount uint32
	EnemyConsecSkillID     uint32
	EnemyConsecSkillCount  uint32
	// SideEffect 32：若干回合内暴击率 +1/16
	PlayerCritBonusRounds byte
	EnemyCritBonusRounds  byte

	// 持续类 SideEffect（己方/敌方各一份）
	PlayerBuff battleBuff
	EnemyBuff  battleBuff

	// SideEffect 34/73：本回合累计受伤
	PlayerLastTaken uint32
	EnemyLastTaken  uint32
	// SideEffect 52：下 1 个技能失效
	PlayerSkillFail bool
	EnemySkillFail  bool
	// SideEffect 62：延迟秒杀倒计时
	PlayerDoomRounds byte
	EnemyDoomRounds  byte
	// SideEffect 17：蓄力
	PlayerChargeSkill uint32
	EnemyChargeSkill  uint32
	PlayerChargeReady bool
	EnemyChargeReady  bool
	// SideEffect 59/67/71：下场精灵遗留
	PlayerNextStageBoost     [5]int8
	EnemyNextStageBoost      [5]int8
	PlayerNextMustCritRounds byte
	EnemyNextMustCritRounds  byte
	PlayerNextEnterCutDenom  byte
	EnemyNextEnterCutDenom   byte
	// SideEffect 202：下一只己方出场攻防+1
	PlayerNextEnterAtkDefBoost bool
	// SideEffect 445：战后发赛尔豆
	RewardCoins445 int
	// SideEffect 1635：延迟回满
	PlayerDelayedFullHealRounds byte
	EnemyDelayedFullHealRounds  byte
	// SideEffect 795：同技叠伤使用次数
	PlayerEffect795Uses byte
	EnemyEffect795Uses  byte
	// SideEffect 55/56：属性交换/复制
	PlayerTypeSaved          byte
	EnemyTypeSaved           byte
	PlayerTypeOverrideRounds byte
	EnemyTypeOverrideRounds  byte

	// BossRegion：2411 param2（谱尼封印 1..8 / 真身 8 等）
	BossRegion uint32
	// 谱尼多命 / 元素交替 / 真身破虚无
	PuniTotalLives         int
	PuniCurrentLife        int
	PuniElementLastType    byte // 上一发元素技 Type（12/13）
	PuniTrueFormSuppressed bool // 时空感应破虚无

	// 野生遭遇：尼尔/尼奥/洛森逃跑与学习力判定
	IsWildMonster      bool
	HasSeenEscapeBlock bool // 尼奥需见过尼尔系；尼尔需尼奥/艾菲德斯站场过

	// DailyExpKind：匹配日常经验（王战/大乱斗），输赢皆可领、有日限
	DailyExpKind int

	// 精灵大乱斗：临时 3v3，不写背包
	IsGrandMelee   bool
	EnemyTeamIDs   []int // AI 三只种族 ID
	EnemyTeamIndex int   // 当前敌方索引

	// 特训结算用
	ForceSinglePet    bool            // 雷伊/盖亚特训：倒地不换宠
	IsGaiyaAppear     bool            // 盖亚的出现（2421）：按日规则发精元
	LastPlayerSkillID uint32          // 本场玩家最后一次有效出手技能
	LastHitWasCrit    bool            // 最后一击是否暴击
	PlayerUsedSkills  map[uint32]bool // 本场用过的技能

	// PvP（对齐参考服行动槽）：OpponentUID!=0 为玩家对战。
	OpponentUID int64
	PvPMode     uint32 // 1=单挑 2=多精灵

	PvPSubmittedType      uint8  // 0无 1技能 2药 3换宠
	PvPSubmittedSkillID   uint32
	PvPSubmittedItemID    uint32
	PvPSubmittedCatchTime uint32
	PvPDeferSwitch        bool // 本回合主动换宠，结算前再推对方 2407

	PvPStartedAt int64  // unix，开战(2503)时间，加载超时用
	PvPLoadPct   uint32 // 本端 2441 进度
	PvPReady     bool   // 已发 2404，离开加载阶段
	PvPWaitGen   uint32 // 等对方行动的世代号，超时 watchdog 防误判

	// 本场已倒地 catchTime（多精灵换宠/全灭判定；会话级，不写 DB）
	FaintedCatchTimes map[uint32]bool
}

func (st *BattleState) markPetFainted(catch uint32) {
	if st == nil || catch == 0 {
		return
	}
	if st.FaintedCatchTimes == nil {
		st.FaintedCatchTimes = make(map[uint32]bool)
	}
	st.FaintedCatchTimes[catch] = true
}

func (st *BattleState) isPetFainted(catch uint32) bool {
	return st != nil && catch != 0 && st.FaintedCatchTimes != nil && st.FaintedCatchTimes[catch]
}

// allowsPetSwitch 倒地后是否允许换宠（单挑/特训不可）。
func (st *BattleState) allowsPetSwitch() bool {
	if st == nil || st.ForceSinglePet {
		return false
	}
	if st.isPvP() && st.PvPMode == pvpModeSingle {
		return false
	}
	return true
}

type battleHub struct {
	mu sync.Mutex
	m  map[int64]*BattleState
}

func (h *battleHub) set(uid int64, b *BattleState) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.m == nil {
		h.m = make(map[int64]*BattleState)
	}
	h.m[uid] = b
}

func (h *battleHub) get(uid int64) *BattleState {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.m[uid]
}

func (h *battleHub) clear(uid int64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.m, uid)
}

func resolveChallengeBoss(mapID int, param2 uint32) (petID, level int, name string) {
	if pid, lv, nm, ok := resolveLeiyiTrainBoss(param2); ok {
		return pid, lv, nm
	}
	if pid, lv, nm, ok := resolveTrainRoomEnemy(mapID, param2); ok {
		if defaultSkillCatalog != nil {
			if n := defaultSkillCatalog.PetNameOf(pid); n != "" {
				nm = n
			}
		}
		return pid, lv, nm
	}
	if pid, lv, nm, ok := resolveMantisBoss(mapID, param2); ok {
		return pid, lv, nm
	}
	if pid, lv, nm, _, ok := resolveLanlanBoss(mapID, param2); ok {
		return pid, lv, nm
	}
	if alias, ok := bossMapAlias[mapID]; ok {
		mapID = alias
	}
	if byParam, ok := mapBossByParam[mapID]; ok {
		if e, ok := byParam[param2]; ok {
			return e.PetID, e.Level, e.Name
		}
	}
	// 未配置 BOSS 时的回退（原 13 比比鼠 → 58 塔奇拉顿）
	return fallbackEnemyPetID, 5, fallbackEnemyName
}

func stubCombatStats(level int) (hp, atk, def, sa, sd, spd int) {
	if level <= 0 {
		level = 5
	}
	hp = 40 + level*8
	atk = 35 + level*2
	def = 30 + level*2
	sa = 35 + level*2
	sd = 30 + level*2
	spd = 30 + level
	return
}

// enemyCombatStats 按种族表算敌人六维（野怪/BOSS 共用）；无种族时回退 stub。
func enemyCombatStats(petID, level int) (hp, atk, def, sa, sd, spd int) {
	if level <= 0 {
		level = 1
	}
	if petBaseFromCatalog(petID) == nil {
		return stubCombatStats(level)
	}
	dv := rand.Intn(32)
	nature := rand.Intn(25)
	return petSixStats(petID, level, dv, nature, [6]int{})
}

func petCombatStats(p *store.Pet) (petID, level int, name string, hp, atk, def, sa, sd, spd int) {
	petID = 1
	level = 5
	if p != nil {
		if p.PetID > 0 {
			petID = p.PetID
		}
		if p.Level > 0 {
			level = p.Level
		}
	}
	defBase := resolvePetBaseStats(petID)
	name = defBase.Name
	if name == "" {
		if d, ok := starterPets[petID]; ok {
			name = d.Name
		} else {
			name = "未知精灵"
		}
	}
	if p != nil && p.Name != "" {
		name = p.Name
	}
	hp, atk, def, sa, sd, spd = petSixStatsFromPet(p)
	return
}

func petBaseFromCatalog(petID int) *tableloader.PetBaseDef {
	if defaultSkillCatalog == nil {
		return nil
	}
	return defaultSkillCatalog.PetBase(petID)
}

func resolvePetBaseStats(petID int) starterDef {
	if d := petBaseFromCatalog(petID); d != nil && d.HP > 0 {
		return starterDef{
			Name: d.Name, HP: d.HP, Atk: d.Atk, Def: d.Def,
			SpAtk: d.SpAtk, SpDef: d.SpDef, Spd: d.Spd,
		}
	}
	if def, ok := starterPets[petID]; ok {
		return def
	}
	return starterDef{HP: 50, Atk: 50, Def: 50, SpAtk: 50, SpDef: 50, Spd: 50}
}

func (s *Server) skillDef(id int) *tableloader.SkillDef {
	if s.cfg.Catalog != nil {
		return s.cfg.Catalog.Skill(id)
	}
	return nil
}

func (s *Server) skillMaxPP(id int) uint32 {
	if d := s.skillDef(id); d != nil && d.MaxPP > 0 {
		return uint32(d.MaxPP)
	}
	return 20
}

// calcSkillDamage 对齐赛尔/参考 CalculateBaseDamage：
// floor((level*0.4+2)*power*atk/def/50+2) * 随机(217~255)/255
func calcSkillDamage(level, power, category, atk, def, spAtk, spDef int) uint32 {
	if power <= 0 || category == 4 {
		return 0
	}
	attack, defense := atk, def
	if category == 2 {
		attack, defense = spAtk, spDef
	}
	if defense < 1 {
		defense = 1
	}
	base := int(((float64(level)*0.4+2.0)*float64(power)*float64(attack)/float64(defense)/50.0)+2.0)
	if base < 1 {
		base = 1
	}
	// 217–255 / 255 ≈ 85%–100%
	factor := 217 + rand.Intn(39)
	dmg := base * factor / 255
	if dmg < 1 {
		dmg = 1
	}
	return uint32(dmg)
}

func (s *Server) damageWithSkill(skillID uint32, level, atk, def, sa, sd int, atkPetType, defPetType int) uint32 {
	return s.damageWithSkillAdj(skillID, level, atk, def, sa, sd, atkPetType, defPetType, skillPowerAdj{})
}

func (s *Server) damageWithSkillAdj(skillID uint32, level, atk, def, sa, sd int, atkPetType, defPetType int, adj skillPowerAdj) uint32 {
	power, cat, skType := 40, 1, 8
	if d := s.skillDef(int(skillID)); d != nil {
		power = d.Power
		cat = d.Category
		skType = d.Type
		if cat == 0 {
			cat = 1
		}
		power = sideEffectRandomPower(d, power)
		power = adjustSkillPower(d, power, adj)
	}
	dmg := calcSkillDamage(level, power, cat, atk, def, sa, sd)
	if dmg == 0 {
		return 0
	}
	mod := typeMultiplier(skType, defPetType) * stabBonus(skType, atkPetType)
	out := float64(dmg) * mod
	if out < 1 && mod > 0 {
		out = 1
	}
	return uint32(out)
}

func (s *Server) petTypeOf(petID int) int {
	if s.cfg.Catalog != nil {
		if t := s.cfg.Catalog.PetTypeID(petID); t > 0 {
			return t
		}
	}
	if def, ok := starterPets[petID]; ok {
		_ = def
	}
	// 新手三系兜底
	switch petID {
	case 1, 2, 3:
		return 1
	case 4, 5, 6:
		return 2
	case 7, 8, 9:
		return 3
	}
	return 8
}

func buildFightUserInfo(uid uint32, nick string, topScore uint32) []byte {
	var buf bytes.Buffer
	packet.WriteU32(&buf, uid)
	packet.WriteFixedString(&buf, nick, 16)
	packet.WriteU32(&buf, topScore)
	return buf.Bytes()
}

// buildSimplePetInfo 2503 用 PetInfo(param2=false)，本客户端 80 字节（至 buffID）。
func buildSimplePetInfo(petID, level, hp, maxHP, catchTime uint32, skills [][2]uint32, skinID, generation, buffID uint32) []byte {
	b := make([]byte, 80)
	binary.BigEndian.PutUint32(b[0:4], petID)
	binary.BigEndian.PutUint32(b[4:8], level)
	binary.BigEndian.PutUint32(b[8:12], hp)
	binary.BigEndian.PutUint32(b[12:16], maxHP)
	valid := uint32(0)
	if !debugFightNoSkills && !petInfoForceEmptySkills {
		for _, sk := range skills {
			if sk[0] != 0 {
				valid++
			}
		}
	}
	binary.BigEndian.PutUint32(b[16:20], valid)
	off := 20
	for i := 0; i < 4; i++ {
		var sid, pp uint32
		if !debugFightNoSkills && !petInfoForceEmptySkills && i < len(skills) {
			sid, pp = skills[i][0], skills[i][1]
		}
		binary.BigEndian.PutUint32(b[off:off+4], sid)
		binary.BigEndian.PutUint32(b[off+4:off+8], pp)
		off += 8
	}
	binary.BigEndian.PutUint32(b[52:56], catchTime)
	binary.BigEndian.PutUint32(b[68:72], skinID)
	binary.BigEndian.PutUint32(b[72:76], generation)
	binary.BigEndian.PutUint32(b[76:80], buffID)
	return b
}

func buildFightPetInfo(uid uint32, petID int, name string, catchTime, hp, maxHP, level, catchable uint32, battleLv [6]int8) []byte {
	if maxHP == 0 {
		maxHP = 1
	}
	if hp > maxHP {
		hp = maxHP
	}
	var buf bytes.Buffer
	packet.WriteU32(&buf, uid)
	packet.WriteU32(&buf, uint32(petID))
	packet.WriteFixedString(&buf, name, 16)
	packet.WriteU32(&buf, catchTime)
	packet.WriteU32(&buf, hp)
	packet.WriteU32(&buf, maxHP)
	packet.WriteU32(&buf, level)
	packet.WriteU32(&buf, catchable)
	for i := 0; i < 6; i++ {
		_ = buf.WriteByte(byte(battleLv[i]))
	}
	return buf.Bytes()
}

// buildAttackValue 对齐本客户端 AttackValue.as（无 typeEffectID）；status/battleLv 全 0。
func buildAttackValue(userID, skillID, atkTimes, lostHP uint32, gainHP, remainHP int32, maxHP, state, isCrit, petType uint32, skills [][2]uint32) []byte {
	return buildAttackValueWithFX(userID, skillID, atkTimes, lostHP, gainHP, remainHP, maxHP, state, isCrit, petType, [20]byte{}, [6]int8{}, skills)
}

// buildAttackValueFromState 回填本方/敌方异常与能力等级，供客户端战报图标。
// petType 为 0 时从 BattleState 取属性，避免 2505 把客户端属性图标清空。
func buildAttackValueFromState(userID, skillID, atkTimes, lostHP uint32, gainHP, remainHP int32, maxHP, state, isCrit, petType uint32, st *BattleState, playerSide bool, skills [][2]uint32) []byte {
	var status [20]byte
	var lv [6]int8
	if st != nil {
		status = encodeFightStatusForSide(st, playerSide)
		if playerSide {
			lv = encodeBattleLv(st.PlayerStages)
			if petType == 0 {
				petType = uint32(st.PlayerType)
			}
		} else {
			lv = encodeBattleLv(st.EnemyStages)
			if petType == 0 {
				petType = uint32(st.EnemyType)
			}
		}
	}
	return buildAttackValueWithFX(userID, skillID, atkTimes, lostHP, gainHP, remainHP, maxHP, state, isCrit, petType, status, lv, skills)
}

// buildAttackValueWithFX skills 非空时写入 skillList；空 skillList → 78 字节。
func buildAttackValueWithFX(userID, skillID, atkTimes, lostHP uint32, gainHP, remainHP int32, maxHP, state, isCrit, petType uint32, status [20]byte, battleLv [6]int8, skills [][2]uint32) []byte {
	if maxHP == 0 {
		maxHP = 1
	}
	if remainHP < 0 {
		remainHP = 0
	}
	if int32(maxHP) < remainHP {
		remainHP = int32(maxHP)
	}
	n := 0
	for _, sk := range skills {
		if sk[0] != 0 {
			n++
		}
	}
	b := make([]byte, 78+n*8)
	off := 0
	put := func(v uint32) {
		binary.BigEndian.PutUint32(b[off:off+4], v)
		off += 4
	}
	put(userID)
	put(skillID)
	put(atkTimes)
	put(lostHP)
	put(uint32(gainHP))
	put(uint32(remainHP))
	put(maxHP)
	put(state)
	put(uint32(n))
	for _, sk := range skills {
		if sk[0] == 0 {
			continue
		}
		put(sk[0])
		put(sk[1])
	}
	put(isCrit)
	copy(b[off:off+20], status[:])
	off += 20
	for i := 0; i < 6; i++ {
		b[off+i] = byte(battleLv[i])
	}
	off += 6
	put(0) // maxShield
	put(0) // curShield
	put(petType)
	return b
}

func buildFightOverInfo(reason, winnerID uint32) []byte {
	return buildFightOverInfoTimes(reason, winnerID, 0, 0, 0, 0, 0)
}

// buildFightOverInfoTimes 对齐 FightOverInfo：reason+winner+two+three+autoFt+energy+learn。
func buildFightOverInfoTimes(reason, winnerID, two, three, autoFt, energy, learn uint32) []byte {
	b := make([]byte, 28)
	binary.BigEndian.PutUint32(b[0:4], reason)
	binary.BigEndian.PutUint32(b[4:8], winnerID)
	binary.BigEndian.PutUint32(b[8:12], two)
	binary.BigEndian.PutUint32(b[12:16], three)
	binary.BigEndian.PutUint32(b[16:20], autoFt)
	binary.BigEndian.PutUint32(b[20:24], energy)
	binary.BigEndian.PutUint32(b[24:28], learn)
	return b
}

// buildChangePetInfo 对齐 ChangePetInfo.as：40 字节。
func buildChangePetInfo(uid uint32, petID int, name string, level, hp, maxHP, catchTime uint32) []byte {
	b := make([]byte, 40)
	binary.BigEndian.PutUint32(b[0:4], uid)
	binary.BigEndian.PutUint32(b[4:8], uint32(petID))
	nb := []byte(name)
	if len(nb) > 16 {
		nb = nb[:16]
	}
	copy(b[8:24], nb)
	binary.BigEndian.PutUint32(b[24:28], level)
	binary.BigEndian.PutUint32(b[28:32], hp)
	binary.BigEndian.PutUint32(b[32:36], maxHP)
	binary.BigEndian.PutUint32(b[36:40], catchTime)
	return b
}

// buildUsePetItemInfo 对齐 UsePetItemInfo.as：16 字节。
func buildUsePetItemInfo(uid, itemID, userHP uint32, changeHP int32) []byte {
	b := make([]byte, 16)
	binary.BigEndian.PutUint32(b[0:4], uid)
	binary.BigEndian.PutUint32(b[4:8], itemID)
	binary.BigEndian.PutUint32(b[8:12], userHP)
	binary.BigEndian.PutUint32(b[12:16], uint32(changeHP))
	return b
}

// buildCatchPetInfo 对齐 CatchPetInfo.as：8 字节；catchTime=0 表示失败。
func buildCatchPetInfo(catchTime, petID uint32) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint32(b[0:4], catchTime)
	binary.BigEndian.PutUint32(b[4:8], petID)
	return b
}

// buildNoteUpdateProp 对齐 PetUpdatePropInfo + UpdatePropInfo（本客户端无异色扩展）。
func buildNoteUpdateProp(catchTime uint32, petID, level, exp, curLvExp, nextLvExp, maxHP, atk, def, sa, sd, spd int, ev [6]int) []byte {
	var buf bytes.Buffer
	w := func(v uint32) { packet.WriteU32(&buf, v) }
	w(0) // addition
	w(1) // count
	w(catchTime)
	w(uint32(petID))
	w(uint32(level))
	w(uint32(exp))
	w(uint32(curLvExp))
	w(uint32(nextLvExp))
	w(uint32(maxHP))
	w(uint32(atk))
	w(uint32(def))
	w(uint32(sa))
	w(uint32(sd))
	w(uint32(spd))
	for i := 0; i < 6; i++ {
		w(uint32(ev[i]))
	}
	return buf.Bytes()
}

func pickBattlePet(bag []store.Pet) *store.Pet {
	if len(bag) == 0 {
		return nil
	}
	best := &bag[0]
	for i := 1; i < len(bag); i++ {
		if bag[i].BagPos < best.BagPos {
			best = &bag[i]
		}
	}
	return best
}

func (s *Server) skillsFromPet(p *store.Pet) [][2]uint32 {
	if debugFightNoSkills {
		return nil
	}
	ensurePetSkills(p)
	out := make([][2]uint32, 0, 4)
	if p != nil {
		for _, sid := range p.Skills {
			if sid <= 0 || !s.skillIDKnown(sid) {
				continue
			}
			// 进战与背包一致保留 Category=4（属性技）；fightOmitCategory4 仅作紧急回退开关
			if fightOmitCategory4 && !skillSafeForClientUI(sid) {
				continue
			}
			out = append(out, [2]uint32{uint32(sid), s.skillMaxPP(sid)})
			if len(out) >= 4 {
				break
			}
		}
	}
	if len(out) == 0 {
		out = append(out, [2]uint32{10001, s.skillMaxPP(10001)})
	}
	return out
}

// skillSafeForClientUI：SkillBtnView/SelectPetPanel 用 getTypeEN→gotoAndStop。
// Category=4 → "prop"，本端会断线；仅放行 Category 1–3 且 Type 1–20。
// 无技能表时：20000–29999 按属性技惯例剔除（与本客户端 Move ID 段一致）。
func skillSafeForClientUI(skillID int) bool {
	d := skillDefLookup(skillID)
	if d == nil {
		if skillID >= 20000 && skillID < 30000 {
			return false
		}
		return skillID > 0
	}
	if d.Category == 4 {
		return false
	}
	if d.Category < 1 || d.Category > 3 {
		return false
	}
	if d.Type < 1 || d.Type > 20 {
		return false
	}
	return true
}

func skillDefLookup(skillID int) *tableloader.SkillDef {
	if skillID <= 0 {
		return nil
	}
	if defaultSkillCatalog != nil {
		if d := defaultSkillCatalog.Skill(skillID); d != nil {
			return d
		}
	}
	return nil
}

func (s *Server) skillSafeForFightUI(skillID int) bool {
	if s != nil {
		if d := s.skillDefOf(skillID); d != nil {
			if d.Category == 4 || d.Category < 1 || d.Category > 3 {
				return false
			}
			if d.Type < 1 || d.Type > 20 {
				return false
			}
			return true
		}
	}
	return skillSafeForClientUI(skillID)
}

func (s *Server) skillDefOf(skillID int) *tableloader.SkillDef {
	if skillID <= 0 {
		return nil
	}
	if s != nil && s.cfg.Catalog != nil {
		if d := s.cfg.Catalog.Skill(skillID); d != nil {
			return d
		}
	}
	if defaultSkillCatalog != nil {
		return defaultSkillCatalog.Skill(skillID)
	}
	return nil
}

// skillIDKnown 技能表有条目才下发（缺表项时客户端 SkillXMLInfo.getTypeEN 会 NPE）。
func (s *Server) skillIDKnown(skillID int) bool {
	if skillID <= 0 {
		return false
	}
	if s.skillDefOf(skillID) != nil {
		return true
	}
	switch skillID {
	case 10001, 10004, 10006, 20001, 20002, 20004:
		return true
	}
	return false
}

func (s *Server) skillCatalog() *tableloader.Catalog {
	if s != nil && s.cfg.Catalog != nil {
		return s.cfg.Catalog
	}
	return defaultSkillCatalog
}

// enemySkillsForPet 敌方技能：取当前等级已学会的攻击技（最近 4 个）；无则撞击。
// fightOmitCategory4=true 时额外过滤 Category=4（紧急回退）。
func (s *Server) enemySkillsForPet(petID, level int) [][2]uint32 {
	raw := [][2]uint32{}
	if def, ok := starterPets[petID]; ok && len(def.Skills) > 0 {
		for _, sid := range def.Skills {
			raw = append(raw, [2]uint32{uint32(sid), s.skillMaxPP(sid)})
			if len(raw) >= 4 {
				break
			}
		}
	} else {
		ids := s.enemyDefaultSkillIDs(petID, level)
		for _, sid := range ids {
			raw = append(raw, [2]uint32{uint32(sid), s.skillMaxPP(sid)})
		}
	}
	out := make([][2]uint32, 0, 4)
	for _, sk := range raw {
		if !s.skillIDKnown(int(sk[0])) {
			continue
		}
		if fightOmitCategory4 && !skillSafeForClientUI(int(sk[0])) {
			continue
		}
		out = append(out, sk)
		if len(out) >= 4 {
			break
		}
	}
	if len(out) == 0 {
		out = [][2]uint32{{10001, s.skillMaxPP(10001)}}
	}
	return out
}

// enemyDefaultSkillIDs 当前等级可学技能中优先取物攻/特攻最近 4 个（跳过变化技），避免首位长期卡在撞击。
func (s *Server) enemyDefaultSkillIDs(petID, level int) []int {
	if level <= 0 {
		level = 5
	}
	cat := s.skillCatalog()
	if cat != nil {
		moves := cat.MovesUpToLevel(petID, level)
		if len(moves) > 0 {
			atk := make([]int, 0, len(moves))
			for _, m := range moves {
				if m.ID <= 0 || !s.skillIDKnown(m.ID) {
					// 缺表项（如英卡洛斯高阶 26026/31836）不可入选：否则取最近 4 个后被滤空 → 只会撞击
					continue
				}
				d := cat.Skill(m.ID)
				if d != nil && d.Category == 4 {
					continue
				}
				if d != nil && d.Category != 1 && d.Category != 2 && d.Power <= 0 {
					continue
				}
				atk = append(atk, m.ID)
			}
			if len(atk) == 0 {
				// 仅有变化技时退回全部可学且有表项的技
				for _, m := range moves {
					if m.ID > 0 && s.skillIDKnown(m.ID) {
						atk = append(atk, m.ID)
					}
				}
			}
			if len(atk) > 0 {
				start := 0
				if len(atk) > 4 {
					start = len(atk) - 4
				}
				return append([]int(nil), atk[start:]...)
			}
		}
	}
	switch petID {
	case fallbackEnemyPetID: // 塔奇拉顿 LearnableMoves 兜底
		all := []struct{ lv, id int }{
			{1, 10127}, {5, 20004}, {10, 10128}, {15, 20068},
		}
		out := make([]int, 0, 4)
		for _, m := range all {
			if m.lv <= level {
				out = append(out, m.id)
			}
			if len(out) >= 4 {
				break
			}
		}
		if len(out) > 0 {
			return out
		}
	}
	return []int{10001}
}

// pickEnemyBattleSkill PvE 敌方选招：在仍有 PP 的技能中随机；优先物攻/特攻，避免只会放撞击。
func (s *Server) pickEnemyBattleSkill(st *BattleState) uint32 {
	if st == nil || len(st.EnemySkills) == 0 {
		return 10001
	}
	infinite := enemyHasInfinitePP(st)
	var atk, other []uint32
	for _, sk := range st.EnemySkills {
		sid := sk[0]
		if sid == 0 {
			continue
		}
		if !infinite && sk[1] == 0 {
			continue
		}
		d := s.skillDef(int(sid))
		if d == nil {
			atk = append(atk, sid)
			continue
		}
		switch d.Category {
		case 1, 2:
			atk = append(atk, sid)
		case 4:
			other = append(other, sid)
		default:
			if d.Power > 0 {
				atk = append(atk, sid)
			} else {
				other = append(other, sid)
			}
		}
	}
	pool := atk
	if len(pool) == 0 {
		pool = other
	}
	if len(pool) == 0 {
		return firstSkillWithPP(st.EnemySkills)
	}
	return pool[rand.Intn(len(pool))]
}

func ensurePetSkills(p *store.Pet) {
	if p == nil || debugFightNoSkills {
		return
	}
	has := false
	for _, sid := range p.Skills {
		if sid > 0 {
			has = true
			break
		}
	}
	if !has {
		if def, ok := starterPets[p.PetID]; ok && len(def.Skills) > 0 {
			p.Skills = append([]int(nil), def.Skills...)
		} else {
			p.Skills = []int{10001}
		}
	}
	fillPetSkillsUpToFourPkg(p)
}

// fillPetSkillsUpToFourPkg 无 Server 时用 defaultSkillCatalog。
func fillPetSkillsUpToFourPkg(p *store.Pet) bool {
	s := &Server{}
	if defaultSkillCatalog != nil {
		s.cfg.Catalog = defaultSkillCatalog
	}
	return s.fillPetSkillsUpToFour(p)
}

func decSkillPP(skills [][2]uint32, skillID uint32) {
	for i := range skills {
		if skills[i][0] == skillID && skills[i][1] > 0 {
			skills[i][1]--
			return
		}
	}
}
