package tableloader

import (
	"math/rand"
	"strconv"
	"strings"
)

// ItemMeta 可执行道具效果（items.xml 属性，战外 2326 / 战内 2406）。
type ItemMeta struct {
	ID             int
	HP             int
	PP             int
	IncreMonLv     int
	DecreMonLv     bool
	ExpGrant       int
	MaxHPUp        int
	MonAttrReset   bool
	MonNatureReset bool
	RandomDv       bool
	NatureSet      []int
	NaturePool     []int
	NewSeIdx       int
	EvRemove       int // 1HP 2Atk 3Def 4SA 5SD 6Spd 7全部；0=无
	// 特性道具
	NewSeReset        bool // 融合精灵特性重组
	NonFuseAddNewse   bool // 开启特性（无则分配）
	NonFuseResetNewse int  // 1=重随；>1=指定 Idx
	// 天赋 / 元神
	AddDv            int  // 稳定提升 DV
	BalanceDv        bool // 随机 ±1 DV（300790）
	YuanshenDegrade  bool // 融合还原
}

func (m ItemMeta) HasEffect() bool {
	return m.HP > 0 || m.PP > 0 || m.IncreMonLv > 0 || m.DecreMonLv ||
		m.ExpGrant > 0 || m.MaxHPUp > 0 || m.MonAttrReset || m.MonNatureReset ||
		m.RandomDv || len(m.NatureSet) > 0 || len(m.NaturePool) > 0 || m.NewSeIdx > 0 ||
		m.EvRemove > 0 || m.NewSeReset || m.NonFuseAddNewse || m.NonFuseResetNewse != 0 ||
		m.AddDv > 0 || m.BalanceDv || m.YuanshenDegrade
}

func (m ItemMeta) HasTraitEffect() bool {
	return m.NewSeReset || m.NonFuseAddNewse || m.NonFuseResetNewse != 0
}

// ItemMetaOf 查道具效果；无则 ok=false。
func (c *Catalog) ItemMetaOf(itemID int) (ItemMeta, bool) {
	if c == nil {
		return ItemMeta{}, false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	m, ok := c.ItemMeta[itemID]
	return m, ok
}

// ItemHealHP 体力恢复量（xml HP 优先，否则硬编码兜底）。
func (c *Catalog) ItemHealHP(itemID int) int {
	if m, ok := c.ItemMetaOf(itemID); ok && m.HP > 0 {
		return m.HP
	}
	return fallbackPotionHP(itemID)
}

// ItemRestorePP 活力恢复量。
func (c *Catalog) ItemRestorePP(itemID int) int {
	if m, ok := c.ItemMetaOf(itemID); ok && m.PP > 0 {
		return m.PP
	}
	return fallbackPotionPP(itemID)
}

func fallbackPotionHP(itemID int) int {
	switch itemID {
	case 300011:
		return 20
	case 300012:
		return 50
	case 300013:
		return 100
	case 300014:
		return 150
	case 300015, 300076, 300077:
		return 200
	case 300020:
		return 3000
	case 300154:
		return 200
	case 300155:
		return 150
	case 300156:
		return 100
	default:
		return 0
	}
}

func fallbackPotionPP(itemID int) int {
	switch itemID {
	case 300016:
		return 5
	case 300017, 300023, 300073:
		return 10
	case 300018, 300074:
		return 20
	case 300019:
		return 40
	default:
		return 0
	}
}

// PickNature 从 NatureSet / NaturePool 抽性格；无则 -1。
func (m ItemMeta) PickNature() int {
	if len(m.NatureSet) > 0 {
		return m.NatureSet[rand.Intn(len(m.NatureSet))]
	}
	if len(m.NaturePool) > 0 {
		return m.NaturePool[rand.Intn(len(m.NaturePool))]
	}
	return -1
}

func parseIntListAttr(s string) []int {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	parts := strings.Fields(s)
	out := make([]int, 0, len(parts))
	for _, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			continue
		}
		out = append(out, n)
	}
	return out
}

// parseYieldingEVAttr 解析 "0 1 0 0 0 0" / 逗号分隔为六维学习力产出。
func parseYieldingEVAttr(s string) [6]int {
	s = strings.TrimSpace(s)
	if s == "" {
		return [6]int{}
	}
	if strings.Contains(s, ",") {
		s = strings.ReplaceAll(s, ",", " ")
	}
	parts := parseIntListAttr(s)
	var out [6]int
	for i := 0; i < 6 && i < len(parts); i++ {
		out[i] = parts[i]
	}
	return out
}
