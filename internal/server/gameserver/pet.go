package gameserver

import (
	"bytes"
	"encoding/binary"
	"math"
	"strconv"

	"niaohao/server/internal/packet"
	"niaohao/server/internal/store"
	"niaohao/server/internal/tableloader"
)

// debugFightNoSkills 排查开关：为 true 时登录/2301/2503 技能槽全 0。
const debugFightNoSkills = false

// fightOmitCategory4：曾为防 SkillBtnView gotoAndStop("prop") 而在进战过滤 Category=4。
// 2026-07-25 空技能隔离已证明断线主因不是 prop；对战需显示属性技，故关闭过滤。
const fightOmitCategory4 = false

// debugFightUIEmptySkills：进战专用隔离（默认关）。为 true 时 2503/进战 2301 技能槽强制 0。
// 2026-07-25 实测：emptyUI 仍无 2605+conn- → 断在 getPetInfo(null)/getFighterMode，非 SkillBtnView。
const debugFightUIEmptySkills = false

// petInfoForceEmptySkills 由进战发包路径临时置 true，供 buildPetInfo/buildSimplePetInfo 读取。
var petInfoForceEmptySkills bool

// 新手三选一：请求 param 1/2/3 → 精灵 ID；也兼容直接传物种 ID。
var novicePetChoice = map[uint32]int{
	1: 1, // 布布种子
	2: 7, // 小火猴
	3: 4, // 伊优
}

type starterDef struct {
	Name   string
	HP     int
	Atk    int
	Def    int
	SpAtk  int
	SpDef  int
	Spd    int
	Skills []int
}

// 对齐 pets.xml 基础值；等级 5、DV=20 时用简化公式算六维。
var starterPets = map[int]starterDef{
	1: {Name: "布布种子", HP: 55, Atk: 69, Def: 65, SpAtk: 45, SpDef: 55, Spd: 31, Skills: []int{10001, 20001}},
	4: {Name: "伊优", HP: 53, Atk: 51, Def: 53, SpAtk: 61, SpDef: 56, Spd: 40, Skills: []int{10004, 20002}},
	7: {Name: "小火猴", HP: 44, Atk: 58, Def: 44, SpAtk: 58, SpDef: 44, Spd: 61, Skills: []int{10006, 20004}},
}

func task86CatchTm(petID int) int64 {
	return int64(0x69686700 + uint32(petID))
}

func resolveNovicePetID(param uint32) int {
	if id, ok := novicePetChoice[param]; ok {
		return id
	}
	if _, ok := starterPets[int(param)]; ok {
		return int(param)
	}
	return 1
}

func newStarterPet(uid int64, petID int) *store.Pet {
	def, ok := starterPets[petID]
	if !ok {
		petID = 1
		def = starterPets[1]
	}
	return &store.Pet{
		UserID:    uid,
		CatchTime: task86CatchTm(petID),
		PetID:     petID,
		Name:      def.Name,
		Level:     5,
		Exp:       0,
		DV:        20,
		Nature:    0,
		BagPos:   0,
		InBag:    true,
		Skills:    append([]int(nil), def.Skills...),
	}
}

// calcStat 无性格修正的非 HP 公式（兼容旧调用）。
func calcStat(base, dv, level, ev int) int {
	return calcStatWithNature(base, dv, level, ev, 1.0)
}

// calcStatWithNature 官方：floor(((2*base+DV+EV/4)*Lv/100+5)*性格修正)
func calcStatWithNature(base, dv, level, ev int, natureMod float64) int {
	if ev < 0 {
		ev = 0
	}
	if natureMod <= 0 {
		natureMod = 1
	}
	v := (float64(base*2+dv)+float64(ev)/4.0)*float64(level)/100.0 + 5.0
	if natureMod != 1.0 {
		v *= natureMod
	}
	stat := int(math.Floor(v))
	if stat < 1 {
		stat = 1
	}
	return stat
}

// calcHP HP 不受性格影响：floor((2*base+DV+EV/4)*Lv/100)+Lv+10
func calcHP(base, dv, level, ev int) int {
	if ev < 0 {
		ev = 0
	}
	hp := int(math.Floor((float64(base*2+dv)+float64(ev)/4.0)*float64(level)/100.0)) + level + 10
	if hp < 1 {
		hp = 1
	}
	return hp
}

// petSixStats 按种族/DV/EV/性格计算面板六维。
func petSixStats(petID, level, dv, nature int, ev [6]int) (hp, atk, def, sa, sd, spd int) {
	if level <= 0 {
		level = 1
	}
	if dv < 0 {
		dv = 0
	}
	if dv > 31 {
		dv = 31
	}
	clampPetEV(&ev)
	base := resolvePetBaseStats(petID)
	mods := tableloader.NatureModsOf(nature)
	hp = calcHP(base.HP, dv, level, ev[0])
	atk = calcStatWithNature(base.Atk, dv, level, ev[1], mods.Atk)
	def = calcStatWithNature(base.Def, dv, level, ev[2], mods.Def)
	sa = calcStatWithNature(base.SpAtk, dv, level, ev[3], mods.SpAtk)
	sd = calcStatWithNature(base.SpDef, dv, level, ev[4], mods.SpDef)
	spd = calcStatWithNature(base.Spd, dv, level, ev[5], mods.Spd)
	return
}

// petNaturalSixStats 公式面板（种族+DV+EV+性格+特训 Bonus），忽略 GM 覆盖。
func petNaturalSixStats(p *store.Pet) (hp, atk, def, sa, sd, spd int) {
	if p == nil {
		return petSixStats(1, 5, 20, 0, [6]int{})
	}
	petID, level, dv, nature := p.PetID, p.Level, p.DV, p.Nature
	if petID <= 0 {
		petID = 1
	}
	if level <= 0 {
		level = 5
	}
	hp, atk, def, sa, sd, spd = petSixStats(petID, level, dv, nature, p.EV)
	hp += p.Bonus[0]
	atk += p.Bonus[1]
	def += p.Bonus[2]
	sa += p.Bonus[3]
	sd += p.Bonus[4]
	spd += p.Bonus[5]
	if hp < 1 {
		hp = 1
	}
	return
}

func petSixStatsFromPet(p *store.Pet) (hp, atk, def, sa, sd, spd int) {
	if p != nil && p.HasGMStats {
		hp, atk, def, sa, sd, spd = p.GMStats[0], p.GMStats[1], p.GMStats[2], p.GMStats[3], p.GMStats[4], p.GMStats[5]
		if hp < 1 {
			hp = 1
		}
		for _, v := range []*int{&atk, &def, &sa, &sd, &spd} {
			if *v < 1 {
				*v = 1
			}
		}
		return
	}
	return petNaturalSixStats(p)
}

// PetPanelSnapshot GM 用：当前面板 + 公式正常值。
func PetPanelSnapshot(p *store.Pet) (current, natural [6]int, gmLocked bool) {
	nh, na, nd, nsa, nsd, nsp := petNaturalSixStats(p)
	natural = [6]int{nh, na, nd, nsa, nsd, nsp}
	ch, ca, cd, csa, csd, csp := petSixStatsFromPet(p)
	current = [6]int{ch, ca, cd, csa, csd, csp}
	if p != nil {
		gmLocked = p.HasGMStats
	}
	return
}

// evTotal 学习力总和。
func evTotal(ev [6]int) int {
	sum := 0
	for _, v := range ev {
		sum += v
	}
	return sum
}

// addEVWithCap 只加不减，单项≤255，总和≤510。
func addEVWithCap(current, yield [6]int) [6]int {
	if evTotal(current) >= 510 {
		return current
	}
	var ev [6]int
	for i := 0; i < 6; i++ {
		ev[i] = current[i] + yield[i]
		if ev[i] < 0 {
			ev[i] = 0
		}
		if ev[i] > 255 {
			ev[i] = 255
		}
	}
	if evTotal(ev) <= 510 {
		return ev
	}
	over := evTotal(ev) - 510
	order := []int{5, 4, 3, 2, 1, 0}
	for _, i := range order {
		if over <= 0 {
			break
		}
		added := ev[i] - current[i]
		if added <= 0 {
			continue
		}
		d := added
		if d > over {
			d = over
		}
		ev[i] -= d
		over -= d
	}
	return ev
}

// clampPetEV 单项≤255、总和≤510。
func clampPetEV(ev *[6]int) {
	if ev == nil {
		return
	}
	sum := 0
	for i := 0; i < 6; i++ {
		if ev[i] < 0 {
			ev[i] = 0
		}
		if ev[i] > 255 {
			ev[i] = 255
		}
		sum += ev[i]
	}
	for sum > 510 {
		cut := false
		for i := 0; i < 6; i++ {
			if ev[i] > 0 {
				ev[i]--
				sum--
				cut = true
				if sum <= 510 {
					break
				}
			}
		}
		if !cut {
			break
		}
	}
}

// skillPPForBuild 完整 PetInfo 里的当前 PP；优先读已加载 SkillXML，否则 20。
func skillPPForBuild(skillID int) uint32 {
	if skillID <= 0 {
		return 0
	}
	if defaultSkillCatalog != nil {
		if d := defaultSkillCatalog.Skill(skillID); d != nil && d.MaxPP > 0 {
			return uint32(d.MaxPP)
		}
	}
	return 20
}

// buildPetInfo 对齐本仓库反编译 PetInfo(param2=true) 布局（无参考服 bonus/刻印扩展）。
func buildPetInfo(p *store.Pet) []byte {
	var buf bytes.Buffer
	w32 := func(v uint32) { packet.WriteU32(&buf, v) }
	wStr := func(s string, n int) { packet.WriteFixedString(&buf, s, n) }

	petID := p.PetID
	if petID <= 0 {
		petID = 1
	}
	level := p.Level
	if level <= 0 {
		level = 5
	}
	dv := p.DV
	if dv < 0 {
		dv = 0
	}
	def, ok := starterPets[petID]
	if !ok {
		def = starterDef{Name: p.Name, HP: 50, Atk: 50, Def: 50, SpAtk: 50, SpDef: 50, Spd: 50, Skills: []int{10001}}
	}
	if base := resolvePetBaseStats(petID); base.HP > 0 {
		def.HP, def.Atk, def.Def = base.HP, base.Atk, base.Def
		def.SpAtk, def.SpDef, def.Spd = base.SpAtk, base.SpDef, base.Spd
		if base.Name != "" {
			def.Name = base.Name
		}
	}
	name := p.Name
	if name == "" {
		name = def.Name
	}
	hp, atk, defv, sa, sd, spd := petSixStatsFromPet(p)
	curHP := hp
	if p.CurrentHP > 0 && p.CurrentHP < hp {
		curHP = p.CurrentHP
	}

	skills := p.Skills
	if len(skills) == 0 {
		skills = def.Skills
	}
	skillIDs := [4]int{}
	valid := 0
	// debugFightNoSkills / 进战空技能隔离：技能槽全 0，且不回落 10001。
	if !debugFightNoSkills && !petInfoForceEmptySkills {
		for i := 0; i < 4 && i < len(skills); i++ {
			sid := skills[i]
			if sid <= 0 {
				continue
			}
			if defaultSkillCatalog != nil && defaultSkillCatalog.Skill(sid) == nil {
				continue
			}
			// 背包/唤醒仪/进战均保留 Category=4（属性技）
			skillIDs[valid] = sid
			valid++
		}
		if valid == 0 {
			skillIDs[0] = 10001
			valid = 1
		}
	}

	w32(uint32(petID))
	wStr(name, 16)
	w32(uint32(dv))
	w32(uint32(p.Nature))
	w32(uint32(level))
	w32(uint32(p.Exp)) // exp
	w32(uint32(p.Exp)) // lvExp
	w32(uint32(petNextLevelExp(petID, level)))
	w32(uint32(curHP))
	w32(uint32(hp))
	w32(uint32(atk))
	w32(uint32(defv))
	w32(uint32(sa))
	w32(uint32(sd))
	w32(uint32(spd))
	for i := 0; i < 6; i++ {
		w32(uint32(p.EV[i]))
	}
	w32(uint32(valid))
	for i := 0; i < 4; i++ {
		w32(uint32(skillIDs[i]))
		pp := uint32(0)
		if skillIDs[i] > 0 {
			pp = skillPPForBuild(skillIDs[i])
		}
		w32(pp)
	}
	w32(uint32(p.CatchTime))
	w32(301) // catchMap
	w32(0)
	w32(uint32(level))
	writePetEffectList(&buf, p)
	w32(petSkinID(p)) // skinID = 展示形态
	w32(0)            // generation
	w32(0)            // cost
	return buf.Bytes()
}

// writePetEffectList 对齐本客户端 PetEffectInfo（PetInfo.as param2=true）。
// 布局每项 24B：itemId(u32)+status(u8)+leftCount(u8)+effectID(u16)+p1(u8)+pad(u8)+p2(u8)+utf13。
// 本客户端 PetDataPanel 用 effectList[0] 显示特性（itemId 1006–1045），故特性须排在能量珠前。
// 禁止照搬参考服 status=4 异能 16 字节参数区；当前统一走定长 24B 布局。
func writePetEffectList(buf *bytes.Buffer, p *store.Pet) {
	w16 := func(v uint16) { packet.WriteU16(buf, v) }
	if p == nil {
		w16(0)
		return
	}
	type entry struct {
		write func()
	}
	var entries []entry
	// 1) 特性 status=1（仅已开启/融合写入的 Trait，不懒分配）
	if IsValidPetTrait(p.Trait) {
		trait := p.Trait
		eid, a0, a1 := petTraitEffectParams(trait)
		entries = append(entries, entry{write: func() {
			packet.WriteU32(buf, uint32(trait)) // itemId = NewSeIdx
			buf.WriteByte(1)
			buf.WriteByte(0)
			packet.WriteU16(buf, eid)
			buf.WriteByte(a0)
			buf.WriteByte(0)
			buf.WriteByte(a1)
			packet.WriteFixedString(buf, strconv.Itoa(trait), 13)
		}})
	}
	// 2) 能量珠 status=2（PetDataPanel/PetBagPanelNew 按 status==2 画图标）
	if p.EnergyBallItemID > 0 && p.EnergyBallLeftCount > 0 {
		left := p.EnergyBallLeftCount
		if left > 255 {
			left = 255
		}
		itemID, effID := p.EnergyBallItemID, p.EnergyBallEffectID
		entries = append(entries, entry{write: func() {
			packet.WriteU32(buf, uint32(itemID))
			buf.WriteByte(2)
			buf.WriteByte(byte(left))
			packet.WriteU16(buf, uint16(effID))
			buf.WriteByte(0)
			buf.WriteByte(0)
			buf.WriteByte(0)
			packet.WriteFixedString(buf, strconv.Itoa(itemID), 13)
		}})
	}
	w16(uint16(len(entries)))
	for _, e := range entries {
		e.write()
	}
}

// petTraitEffectParams 从表 PetEffectXML（Stat=1）取 Eid/Args，写 PetEffectInfo 参数三字节。
func petTraitEffectParams(trait int) (eid uint16, arg0, arg1 byte) {
	if defaultSkillCatalog == nil {
		return 0, 0, 0
	}
	d, ok := defaultSkillCatalog.PetEffectByIdx(trait)
	if !ok || d.Stat != 1 {
		return 0, 0, 0
	}
	if d.Eid > 0 && d.Eid <= 0xffff {
		eid = uint16(d.Eid)
	}
	if len(d.Args) > 0 && d.Args[0] >= 0 && d.Args[0] <= 255 {
		arg0 = byte(d.Args[0])
	}
	if len(d.Args) > 1 && d.Args[1] >= 0 && d.Args[1] <= 255 {
		arg1 = byte(d.Args[1])
	}
	return eid, arg0, arg1
}

func buildPetTakeOutInfo(firstPetTime uint32, flag uint32, petBody []byte) []byte {
	body := make([]byte, 12+len(petBody))
	binary.BigEndian.PutUint32(body[0:4], 0) // homeEnergy
	binary.BigEndian.PutUint32(body[4:8], firstPetTime)
	binary.BigEndian.PutUint32(body[8:12], flag)
	if len(petBody) > 0 {
		copy(body[12:], petBody)
	}
	return body
}
