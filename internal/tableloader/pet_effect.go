package tableloader

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// PetEffectDef 对齐 PetEffectXMLInfo.xml 的 NewSeIdx（能量珠等消耗型特效）。
type PetEffectDef struct {
	Idx   int
	Stat  int
	Times int
	Eid   int
	Args  []int
}

// ItemNewSeIdx 道具 → NewSeIdx（items.xml NewSeIdx 属性）。
func (c *Catalog) ItemNewSeIdx(itemID int) int {
	if c == nil {
		return 0
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.itemNewSe[itemID]
}

// PetEffectByIdx 按 NewSeIdx 查特效定义。
func (c *Catalog) PetEffectByIdx(idx int) (PetEffectDef, bool) {
	if c == nil || idx <= 0 {
		return PetEffectDef{}, false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	d, ok := c.petEffect[idx]
	return d, ok
}

// EnergyBallForItem 道具对应能量珠：返回 NewSeIdx 与 Times；非能量珠 ok=false。
func (c *Catalog) EnergyBallForItem(itemID int) (idx int, times int, ok bool) {
	if c == nil || itemID <= 0 {
		return 0, 0, false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	idx = c.itemNewSe[itemID]
	if idx <= 0 {
		return 0, 0, false
	}
	d, found := c.petEffect[idx]
	if !found || d.Stat != 2 || d.Times <= 0 {
		// 表缺 Times 时仍认 NewSeIdx，默认 10 次
		if found && d.Stat == 2 {
			t := d.Times
			if t <= 0 {
				t = 10
			}
			return idx, t, true
		}
		if !found {
			return idx, 10, true
		}
		return 0, 0, false
	}
	return idx, d.Times, true
}

func (c *Catalog) loadPetEffects(path string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	tagRe := regexp.MustCompile(`<NewSeIdx\b[^>]*/>`)
	attrRe := regexp.MustCompile(`(\w+)\s*=\s*"([^"]*)"`)
	for _, tag := range tagRe.FindAllString(string(b), -1) {
		attrs := map[string]string{}
		for _, m := range attrRe.FindAllStringSubmatch(tag, -1) {
			attrs[m[1]] = m[2]
		}
		idx, _ := strconv.Atoi(strings.TrimSpace(attrs["Idx"]))
		if idx <= 0 {
			continue
		}
		stat, _ := strconv.Atoi(strings.TrimSpace(attrs["Stat"]))
		times, _ := strconv.Atoi(strings.TrimSpace(attrs["Times"]))
		eid, _ := strconv.Atoi(strings.TrimSpace(attrs["Eid"]))
		var args []int
		for _, p := range strings.Fields(attrs["Args"]) {
			if n, e := strconv.Atoi(p); e == nil {
				args = append(args, n)
			}
		}
		c.petEffect[idx] = PetEffectDef{Idx: idx, Stat: stat, Times: times, Eid: eid, Args: args}
		if itemID, e := strconv.Atoi(strings.TrimSpace(attrs["ItemId"])); e == nil && itemID > 0 {
			if _, has := c.itemNewSe[itemID]; !has {
				c.itemNewSe[itemID] = idx
			}
		}
	}
	return nil
}

func petEffectXMLPath(xmlDir string) string {
	return filepath.Join(xmlDir, "PetEffectXMLInfo.xml")
}
