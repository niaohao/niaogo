package tableloader

import "sort"

// LearnableMove 精灵按等级可学技能。
type LearnableMove struct {
	ID    int
	Level int // LearningLv
}

// PetBaseDef 种族基础值 + 可学技能（来自 pets.xml Monster）。
type PetBaseDef struct {
	ID             int
	Name           string
	Type           int
	GrowthType     int
	Gender         int // 1 雄 2 雌
	HP             int
	Atk            int
	Def            int
	SpAtk          int
	SpDef          int
	Spd            int
	EvolvesFrom    int
	EvolvesTo      int
	EvolvingLv     int
	EvolvFlag      int
	EvolveBabin    int
	EvolvItem      int
	EvolvItemCount int
	IsFuseMon      bool
	IsRareMon      bool  // pets.xml IsRareMon：稀有野生，不触发尼尔/尼奥替换
	FreeForbidden  bool  // pets.xml FreeForbidden：不可回收（放生仍可进仓，但不发豆）
	PetClass       int
	CatchRate      int    // 0–255；0 表示表未写或不可捕默认偏低
	YieldingExp    int    // 击败后产出经验（pets.xml YieldingExp）
	YieldingEV     [6]int // HP Atk Def SpAtk SpDef Spd（击败后给学习力）
	LearnableMoves []LearnableMove
}

// CatchRateOf 野生捕捉率（/255）；无种族默认 45；表写 0 则按 0（极难捉）。
func (c *Catalog) CatchRateOf(petID int) int {
	d := c.PetBase(petID)
	if d == nil {
		return 45
	}
	if d.CatchRate < 0 {
		return 45
	}
	if d.CatchRate > 255 {
		return 255
	}
	return d.CatchRate
}

// YieldingEVOf 取种族击败产出学习力；无则全 0。
func (c *Catalog) YieldingEVOf(petID int) [6]int {
	d := c.PetBase(petID)
	if d == nil {
		return [6]int{}
	}
	return d.YieldingEV
}

// YieldingExpOf 取种族击败产出经验；无表或 ≤0 返回 0（调用方回退公式）。
func (c *Catalog) YieldingExpOf(petID int) int {
	d := c.PetBase(petID)
	if d == nil || d.YieldingExp <= 0 {
		return 0
	}
	return d.YieldingExp
}

// PetBase 取种族定义；不存在返回 nil。
func (c *Catalog) PetBase(id int) *PetBaseDef {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.PetBaseMap == nil {
		return nil
	}
	d, ok := c.PetBaseMap[id]
	if !ok {
		return nil
	}
	cp := d
	return &cp
}

// FinalFormPetIDs 返回 EvolvesTo=0 且 ID∈(0,maxID] 的终形种族（排除融合/稀有）。
func (c *Catalog) FinalFormPetIDs(maxID int) []int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.PetBaseMap == nil {
		return nil
	}
	out := make([]int, 0, 64)
	for id, d := range c.PetBaseMap {
		if id <= 0 || (maxID > 0 && id > maxID) {
			continue
		}
		if d.EvolvesTo != 0 || d.IsFuseMon || d.IsRareMon {
			continue
		}
		out = append(out, id)
	}
	return out
}

// CanLearnMove 该精灵种族表是否包含该技能（不限等级；升级替换由前端/2307 确认）。
func (c *Catalog) CanLearnMove(petID, moveID int) bool {
	d := c.PetBase(petID)
	if d == nil || moveID <= 0 {
		return false
	}
	for _, m := range d.LearnableMoves {
		if m.ID == moveID {
			return true
		}
	}
	return false
}

// SkillsLearnedAtLevel LearningLv == level 的技能。
func (c *Catalog) SkillsLearnedAtLevel(petID, level int) []int {
	d := c.PetBase(petID)
	if d == nil {
		return nil
	}
	var out []int
	for _, m := range d.LearnableMoves {
		if m.Level == level && m.ID > 0 {
			out = append(out, m.ID)
		}
	}
	return out
}

// SkillsLearnedBetween 等级 (fromLv, toLv] 区间新学会的技能（去重保序）。
func (c *Catalog) SkillsLearnedBetween(petID, fromLv, toLv int) []int {
	if toLv <= fromLv {
		return nil
	}
	seen := make(map[int]bool)
	var out []int
	for lv := fromLv + 1; lv <= toLv; lv++ {
		for _, sid := range c.SkillsLearnedAtLevel(petID, lv) {
			if sid <= 0 || seen[sid] {
				continue
			}
			seen[sid] = true
			out = append(out, sid)
		}
	}
	return out
}

// MovesUpToLevel LearningLv <= level，按学习等级升序。
func (c *Catalog) MovesUpToLevel(petID, level int) []LearnableMove {
	d := c.PetBase(petID)
	if d == nil {
		return nil
	}
	var out []LearnableMove
	for _, m := range d.LearnableMoves {
		if m.ID > 0 && m.Level <= level {
			out = append(out, m)
		}
	}
	return out
}

// EvolutionBaseForm 沿 EvolvesFrom 追溯到最初形态。
func (c *Catalog) EvolutionBaseForm(petID int) int {
	seen := map[int]bool{}
	cur := petID
	for cur > 0 && !seen[cur] {
		seen[cur] = true
		d := c.PetBase(cur)
		if d == nil || d.EvolvesFrom <= 0 {
			return cur
		}
		cur = d.EvolvesFrom
	}
	return petID
}

// AllPetIDs 返回 pets.xml 实装的全部种族 ID（升序）。
func (c *Catalog) AllPetIDs() []int {
	if c == nil {
		return nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	ids := make([]int, 0, len(c.PetBaseMap))
	for id := range c.PetBaseMap {
		if id > 0 {
			ids = append(ids, id)
		}
	}
	sort.Ints(ids)
	return ids
}

// DefaultSkillsAtLevel 满级发放用默认技能栏（最多 4）。
// 按 LearningLv 从高到低取可学招；跳过 SkillXMLInfo 中不存在的 ID（否则客户端空槽）。
func (c *Catalog) DefaultSkillsAtLevel(petID, level int) []int {
	if level <= 0 {
		level = 1
	}
	moves := c.MovesUpToLevel(petID, level)
	out := make([]int, 0, 4)
	seen := map[int]bool{}
	for i := len(moves) - 1; i >= 0; i-- {
		id := moves[i].ID
		if id <= 0 || seen[id] {
			continue
		}
		if c.Skill(id) == nil {
			continue
		}
		seen[id] = true
		out = append(out, id)
		if len(out) >= 4 {
			break
		}
	}
	if len(out) == 0 {
		return []int{10001}
	}
	return out
}
