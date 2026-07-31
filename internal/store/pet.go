package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
)

const MaxBagPets = 6

// bag_pos=-2 且 in_bag=0：放生仓（2320/2321/2322）；
// bag_pos=-3 且 in_bag=0：NoNo 模拟训练中（9015/9017/9018）；
// 其它 in_bag=0 为普通仓库。
const (
	RoweiBagPos = -2
	ExeBagPos   = -3
)

// Pet 背包/仓库精灵（对齐 pets 表）。
type Pet struct {
	UserID    int64
	CatchTime int64
	PetID     int
	Name      string
	Level     int
	Exp       int
	DV        int
	Nature    int
	BagPos   int
	InBag     bool
	Skills    []int // 最多 4 个 skillID
	CurrentHP int   // 存 extra_json.currentHp；0=满血
	// 能量珠（2610）：extra_json
	EnergyBallItemID    int
	EnergyBallLeftCount int
	EnergyBallEffectID  int // NewSeIdx
	// Trait 精灵特性 NewSeIdx 1006-1045（extra_json.trait）
	Trait int
	// 学习力 EV：extra_json.ev[6] = HP/Atk/Def/SA/SD/Spd
	EV [6]int
	// NoNo 模拟训练：extra_json
	ExeStart  int64 // unix 开始时间
	ExeCourse int   // 课程档位（天数）
	// IsElite 仓库精英标记（2333/2334，extra_json.elite）
	IsElite bool
	// 形态固定/还原胶囊（9113）：extra_json
	FormLocked          int // 非0=已锁定展示形态
	DisplayFormID       int // 当前展示形态（0=跟 PetID）
	LockedDisplayFormID int // 锁定时记录的展示形态
	// 雷伊体能特训永久面板加成（extra_json.bonus[6] = HP/Atk/Def/SA/SD/Spd）
	Bonus [6]int
	// GM 直接改面板能力值（extra_json.gmStats[6]）；HasGMStats=true 时覆盖公式结果
	GMStats    [6]int
	HasGMStats bool
	// LearnedSkillBank 曾学会/特训解锁的技能（2336 增量；extra_json.skillBank）
	LearnedSkillBank []int
}

type petExtraJSON struct {
	CurrentHP           int   `json:"currentHp,omitempty"`
	EnergyBallItemID    int   `json:"ebItem,omitempty"`
	EnergyBallLeftCount int   `json:"ebLeft,omitempty"`
	EnergyBallEffectID  int   `json:"ebEff,omitempty"`
	Trait               int   `json:"trait,omitempty"`
	EV                  []int `json:"ev,omitempty"`
	ExeStart            int64 `json:"exeStart,omitempty"`
	ExeCourse           int   `json:"exeCourse,omitempty"`
	Elite               int   `json:"elite,omitempty"`
	FormLocked          int   `json:"formLocked,omitempty"`
	DisplayFormID       int   `json:"displayFormId,omitempty"`
	LockedDisplayFormID int   `json:"lockedDisplayFormId,omitempty"`
	Bonus               []int `json:"bonus,omitempty"`
	GMStats             []int `json:"gmStats,omitempty"`
	SkillBank           []int `json:"skillBank,omitempty"`
}

func (s *sqlBackend) UpsertPet(p *Pet) error {
	if p == nil {
		return fmt.Errorf("nil pet")
	}
	skills := p.Skills
	if skills == nil {
		skills = []int{}
	}
	b, err := json.Marshal(skills)
	if err != nil {
		return err
	}
	inBag := 0
	if p.InBag {
		inBag = 1
	}
	_, err = s.db.Exec(`
INSERT INTO pets(user_id, catch_time, pet_id, pet_name, level, exp, dv, nature, bag_pos, in_bag, skills_json)
VALUES(?,?,?,?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE
 pet_id=VALUES(pet_id), pet_name=VALUES(pet_name), level=VALUES(level), exp=VALUES(exp),
 dv=VALUES(dv), nature=VALUES(nature), bag_pos=VALUES(bag_pos), in_bag=VALUES(in_bag),
 skills_json=VALUES(skills_json)`,
		p.UserID, p.CatchTime, p.PetID, p.Name, p.Level, p.Exp, p.DV, p.Nature, p.BagPos, inBag, string(b))
	return err
}

func (s *sqlBackend) GetPetByCatchTime(uid, catchTime int64) (*Pet, error) {
	row := s.db.QueryRow(`
SELECT user_id, catch_time, pet_id, pet_name, level, exp, dv, nature, bag_pos, in_bag, skills_json, extra_json
FROM pets WHERE user_id=? AND catch_time=?`, uid, catchTime)
	return scanPet(row)
}

// SetPetCurrentHP 写入 extra_json.currentHp；hp<=0 表示满血（清除字段）。不改动其它列。
func (s *sqlBackend) SetPetCurrentHP(uid, catchTime int64, hp int) error {
	if hp <= 0 {
		_, err := s.db.Exec(`
UPDATE pets SET extra_json = JSON_REMOVE(COALESCE(extra_json, JSON_OBJECT()), '$.currentHp')
WHERE user_id=? AND catch_time=?`, uid, catchTime)
		return err
	}
	_, err := s.db.Exec(`
UPDATE pets SET extra_json = JSON_SET(COALESCE(extra_json, JSON_OBJECT()), '$.currentHp', ?)
WHERE user_id=? AND catch_time=?`, hp, uid, catchTime)
	return err
}

// SetPetEnergyBall 写入能量珠字段；itemID<=0 时清除。
func (s *sqlBackend) SetPetEnergyBall(uid, catchTime int64, itemID, leftCount, effectID int) error {
	if itemID <= 0 || leftCount <= 0 {
		_, err := s.db.Exec(`
UPDATE pets SET extra_json = JSON_REMOVE(
  COALESCE(extra_json, JSON_OBJECT()), '$.ebItem', '$.ebLeft', '$.ebEff')
WHERE user_id=? AND catch_time=?`, uid, catchTime)
		return err
	}
	_, err := s.db.Exec(`
UPDATE pets SET extra_json = JSON_SET(
  COALESCE(extra_json, JSON_OBJECT()),
  '$.ebItem', ?, '$.ebLeft', ?, '$.ebEff', ?)
WHERE user_id=? AND catch_time=?`, itemID, leftCount, effectID, uid, catchTime)
	return err
}

// SetPetTrait 写入特性 Idx；trait<=0 清除。
func (s *sqlBackend) SetPetTrait(uid, catchTime int64, trait int) error {
	if trait <= 0 {
		_, err := s.db.Exec(`
UPDATE pets SET extra_json = JSON_REMOVE(COALESCE(extra_json, JSON_OBJECT()), '$.trait')
WHERE user_id=? AND catch_time=?`, uid, catchTime)
		return err
	}
	_, err := s.db.Exec(`
UPDATE pets SET extra_json = JSON_SET(COALESCE(extra_json, JSON_OBJECT()), '$.trait', ?)
WHERE user_id=? AND catch_time=?`, trait, uid, catchTime)
	return err
}

// SetPetEV 写入六维学习力；单项≤255、总和≤510（调用方先裁剪）。
func (s *sqlBackend) SetPetEV(uid, catchTime int64, ev [6]int) error {
	b, err := json.Marshal(ev[:])
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`
UPDATE pets SET extra_json = JSON_SET(COALESCE(extra_json, JSON_OBJECT()), '$.ev', CAST(? AS JSON))
WHERE user_id=? AND catch_time=?`, string(b), uid, catchTime)
	return err
}

// SetPetElite 仓库精英标记（2333/2334）。
func (s *sqlBackend) SetPetElite(uid, catchTime int64, elite bool) error {
	if !elite {
		_, err := s.db.Exec(`
UPDATE pets SET extra_json = JSON_REMOVE(COALESCE(extra_json, JSON_OBJECT()), '$.elite')
WHERE user_id=? AND catch_time=?`, uid, catchTime)
		return err
	}
	_, err := s.db.Exec(`
UPDATE pets SET extra_json = JSON_SET(COALESCE(extra_json, JSON_OBJECT()), '$.elite', 1)
WHERE user_id=? AND catch_time=?`, uid, catchTime)
	return err
}

func (s *sqlBackend) ListBagPets(uid int64) ([]Pet, error) {
	return s.listPets(uid, true, MaxBagPets)
}

func (s *sqlBackend) ListStoragePets(uid int64) ([]Pet, error) {
	return s.listPetsWhere(uid, `user_id=? AND in_bag=0 AND bag_pos<>? AND bag_pos<>?`, uid, RoweiBagPos, ExeBagPos)
}

// ListExePets NoNo 模拟训练中的精灵。
func (s *sqlBackend) ListExePets(uid int64) ([]Pet, error) {
	return s.listPetsWhere(uid, `user_id=? AND in_bag=0 AND bag_pos=?`, uid, ExeBagPos)
}

// SetPetExe 写入训练开始时间与课程；start<=0 时清除。
func (s *sqlBackend) SetPetExe(uid, catchTime int64, start int64, course int) error {
	if start <= 0 {
		_, err := s.db.Exec(`
UPDATE pets SET extra_json = JSON_REMOVE(
  COALESCE(extra_json, JSON_OBJECT()), '$.exeStart', '$.exeCourse')
WHERE user_id=? AND catch_time=?`, uid, catchTime)
		return err
	}
	_, err := s.db.Exec(`
UPDATE pets SET extra_json = JSON_SET(
  COALESCE(extra_json, JSON_OBJECT()), '$.exeStart', ?, '$.exeCourse', ?)
WHERE user_id=? AND catch_time=?`, start, course, uid, catchTime)
	return err
}

// MovePetToExe 背包/仓库 → 训练中。
func (s *sqlBackend) MovePetToExe(uid, catchTime int64, course int, startUnix int64) error {
	p, err := s.GetPetByCatchTime(uid, catchTime)
	if err != nil || p == nil {
		return fmt.Errorf("pet not found")
	}
	if p.BagPos == RoweiBagPos {
		return fmt.Errorf("pet in rowei")
	}
	wasBag := p.InBag
	if err := s.SetPetBagFlag(uid, catchTime, false, ExeBagPos); err != nil {
		return err
	}
	if wasBag {
		bag, _ := s.ListBagPets(uid)
		_ = s.reindexBagPos(uid, bag)
	}
	if course < 1 {
		course = 1
	}
	return s.SetPetExe(uid, catchTime, startUnix, course)
}

// EndPetExe 训练结束 → 普通仓库；返回精灵。
func (s *sqlBackend) EndPetExe(uid, catchTime int64) (*Pet, error) {
	p, err := s.GetPetByCatchTime(uid, catchTime)
	if err != nil || p == nil {
		return nil, err
	}
	if p.BagPos != ExeBagPos {
		return nil, fmt.Errorf("pet not in exe")
	}
	if err := s.SetPetBagFlag(uid, catchTime, false, -1); err != nil {
		return nil, err
	}
	_ = s.SetPetExe(uid, catchTime, 0, 0)
	return s.GetPetByCatchTime(uid, catchTime)
}

func (s *sqlBackend) ListRoweiPets(uid int64) ([]Pet, error) {
	return s.listPetsWhere(uid, `user_id=? AND in_bag=0 AND bag_pos=?`, uid, RoweiBagPos)
}

func (s *sqlBackend) CountRoweiPets(uid int64) (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM pets WHERE user_id=? AND in_bag=0 AND bag_pos=?`, uid, RoweiBagPos).Scan(&n)
	return n, err
}

// MovePetToRowei 普通仓库 → 放生仓。
func (s *sqlBackend) MovePetToRowei(uid, catchTime int64) error {
	p, err := s.GetPetByCatchTime(uid, catchTime)
	if err != nil {
		return err
	}
	if p == nil || p.InBag || p.BagPos == RoweiBagPos || p.BagPos == ExeBagPos {
		return fmt.Errorf("pet not in storage")
	}
	_, err = s.db.Exec(`UPDATE pets SET in_bag=0, bag_pos=? WHERE user_id=? AND catch_time=?`,
		RoweiBagPos, uid, catchTime)
	return err
}

// RetrievePetFromRowei 放生仓 → 普通仓库。
func (s *sqlBackend) RetrievePetFromRowei(uid, catchTime int64) error {
	p, err := s.GetPetByCatchTime(uid, catchTime)
	if err != nil {
		return err
	}
	if p == nil || p.InBag || p.BagPos != RoweiBagPos {
		return fmt.Errorf("pet not in rowei")
	}
	_, err = s.db.Exec(`UPDATE pets SET in_bag=0, bag_pos=-1 WHERE user_id=? AND catch_time=?`,
		uid, catchTime)
	return err
}

func (s *sqlBackend) listPetsWhere(uid int64, where string, args ...any) ([]Pet, error) {
	q := `
SELECT user_id, catch_time, pet_id, pet_name, level, exp, dv, nature, bag_pos, in_bag, skills_json, extra_json
FROM pets WHERE ` + where + `
ORDER BY bag_pos ASC, catch_time ASC`
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Pet, 0)
	for rows.Next() {
		p, err := scanPetRows(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *p)
	}
	return out, rows.Err()
}

func (s *sqlBackend) CountBagPets(uid int64) (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM pets WHERE user_id=? AND in_bag=1`, uid).Scan(&n)
	return n, err
}

func (s *sqlBackend) listPets(uid int64, inBag bool, limit int) ([]Pet, error) {
	flag := 0
	if inBag {
		flag = 1
	}
	q := `
SELECT user_id, catch_time, pet_id, pet_name, level, exp, dv, nature, bag_pos, in_bag, skills_json, extra_json
FROM pets WHERE user_id=? AND in_bag=?
ORDER BY bag_pos ASC, catch_time ASC`
	args := []any{uid, flag}
	if limit > 0 {
		q += ` LIMIT ?`
		args = append(args, limit)
	}
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Pet, 0)
	for rows.Next() {
		p, err := scanPetRows(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *p)
	}
	return out, rows.Err()
}

func (s *sqlBackend) SetPetBagFlag(uid, catchTime int64, inBag bool, bagPos int) error {
	flag := 0
	if inBag {
		flag = 1
	}
	_, err := s.db.Exec(`UPDATE pets SET in_bag=?, bag_pos=? WHERE user_id=? AND catch_time=?`,
		flag, bagPos, uid, catchTime)
	return err
}

// MovePetToStorage 背包 → 仓库。返回剩余背包首发 catchTime（无则 0）。
func (s *sqlBackend) MovePetToStorage(uid, catchTime int64) (firstCatch int64, ok bool, err error) {
	p, err := s.GetPetByCatchTime(uid, catchTime)
	if err != nil || p == nil || !p.InBag {
		return 0, false, err
	}
	if err := s.SetPetBagFlag(uid, catchTime, false, -1); err != nil {
		return 0, false, err
	}
	bag, err := s.ListBagPets(uid)
	if err != nil {
		return 0, true, err
	}
	_ = s.reindexBagPos(uid, bag)
	if len(bag) > 0 {
		return bag[0].CatchTime, true, nil
	}
	return 0, true, nil
}

// MovePetToBag 仓库 → 背包；背包满(>=6)则失败。
func (s *sqlBackend) MovePetToBag(uid, catchTime int64) (*Pet, bool, error) {
	p, err := s.GetPetByCatchTime(uid, catchTime)
	if err != nil || p == nil {
		return nil, false, err
	}
	if p.InBag {
		return p, true, nil // 已在背包：仅取信息
	}
	if p.BagPos == RoweiBagPos || p.BagPos == ExeBagPos {
		return nil, false, fmt.Errorf("pet in special storage")
	}
	n, err := s.CountBagPets(uid)
	if err != nil {
		return nil, false, err
	}
	if n >= MaxBagPets {
		return nil, false, nil
	}
	if err := s.SetPetBagFlag(uid, catchTime, true, n); err != nil {
		return nil, false, err
	}
	p.InBag = true
	p.BagPos = n
	return p, true, nil
}

// SetDefaultPet 将指定精灵移到背包首位（bag_pos=0）。
func (s *sqlBackend) SetDefaultPet(uid, catchTime int64) error {
	p, err := s.GetPetByCatchTime(uid, catchTime)
	if err != nil {
		return err
	}
	if p == nil || !p.InBag {
		return fmt.Errorf("pet not in bag")
	}
	bag, err := s.ListBagPets(uid)
	if err != nil {
		return err
	}
	ordered := make([]Pet, 0, len(bag))
	ordered = append(ordered, *p)
	for i := range bag {
		if bag[i].CatchTime == catchTime {
			continue
		}
		ordered = append(ordered, bag[i])
	}
	return s.reindexBagPos(uid, ordered)
}

// NormalizeBagOverflow 背包超过 6 只时多余的挪进仓库，返回挪出数量。
func (s *sqlBackend) NormalizeBagOverflow(uid int64) (int, error) {
	rows, err := s.db.Query(`
SELECT user_id, catch_time, pet_id, pet_name, level, exp, dv, nature, bag_pos, in_bag, skills_json, extra_json
FROM pets WHERE user_id=? AND in_bag=1
ORDER BY bag_pos ASC, catch_time ASC`, uid)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	all := make([]Pet, 0)
	for rows.Next() {
		p, err := scanPetRows(rows)
		if err != nil {
			return 0, err
		}
		all = append(all, *p)
	}
	if len(all) <= MaxBagPets {
		return 0, s.reindexBagPos(uid, all)
	}
	keep := all[:MaxBagPets]
	moved := all[MaxBagPets:]
	if err := s.reindexBagPos(uid, keep); err != nil {
		return 0, err
	}
	for i := range moved {
		if err := s.SetPetBagFlag(uid, moved[i].CatchTime, false, -1); err != nil {
			return i, err
		}
	}
	return len(moved), nil
}

func (s *sqlBackend) reindexBagPos(uid int64, bag []Pet) error {
	for i := range bag {
		if _, err := s.db.Exec(`UPDATE pets SET bag_pos=?, in_bag=1 WHERE user_id=? AND catch_time=?`,
			i, uid, bag[i].CatchTime); err != nil {
			return err
		}
	}
	return nil
}

type petScanner interface {
	Scan(dest ...any) error
}

func scanPet(row petScanner) (*Pet, error) {
	p := &Pet{}
	var inBag int
	var skillsJSON sql.NullString
	var extraJSON sql.NullString
	err := row.Scan(&p.UserID, &p.CatchTime, &p.PetID, &p.Name, &p.Level, &p.Exp,
		&p.DV, &p.Nature, &p.BagPos, &inBag, &skillsJSON, &extraJSON)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	p.InBag = inBag != 0
	if skillsJSON.Valid && skillsJSON.String != "" {
		_ = json.Unmarshal([]byte(skillsJSON.String), &p.Skills)
	}
	if extraJSON.Valid && extraJSON.String != "" {
		var ex petExtraJSON
		if json.Unmarshal([]byte(extraJSON.String), &ex) == nil {
			p.CurrentHP = ex.CurrentHP
			p.EnergyBallItemID = ex.EnergyBallItemID
			p.EnergyBallLeftCount = ex.EnergyBallLeftCount
			p.EnergyBallEffectID = ex.EnergyBallEffectID
			p.Trait = ex.Trait
			if len(ex.EV) >= 6 {
				for i := 0; i < 6; i++ {
					p.EV[i] = ex.EV[i]
				}
			}
			p.ExeStart = ex.ExeStart
			p.ExeCourse = ex.ExeCourse
			p.IsElite = ex.Elite != 0
			p.FormLocked = ex.FormLocked
			p.DisplayFormID = ex.DisplayFormID
			p.LockedDisplayFormID = ex.LockedDisplayFormID
			if len(ex.Bonus) >= 6 {
				for i := 0; i < 6; i++ {
					p.Bonus[i] = ex.Bonus[i]
				}
			}
			if len(ex.GMStats) >= 6 {
				p.HasGMStats = true
				for i := 0; i < 6; i++ {
					p.GMStats[i] = ex.GMStats[i]
				}
			}
			if len(ex.SkillBank) > 0 {
				p.LearnedSkillBank = append([]int(nil), ex.SkillBank...)
			}
		}
	}
	return p, nil
}

// SetPetTrainBonus 写入雷伊体能特训六维永久加成。
func (s *sqlBackend) SetPetTrainBonus(uid, catchTime int64, bonus [6]int) error {
	b, err := json.Marshal(bonus[:])
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`
UPDATE pets SET extra_json = JSON_SET(COALESCE(extra_json, JSON_OBJECT()), '$.bonus', CAST(? AS JSON))
WHERE user_id=? AND catch_time=?`, string(b), uid, catchTime)
	return err
}

// SetPetGMStats 写入 GM 面板能力值覆盖（持久）；各项应 ≥1。
func (s *sqlBackend) SetPetGMStats(uid, catchTime int64, stats [6]int) error {
	b, err := json.Marshal(stats[:])
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`
UPDATE pets SET extra_json = JSON_SET(COALESCE(extra_json, JSON_OBJECT()), '$.gmStats', CAST(? AS JSON))
WHERE user_id=? AND catch_time=?`, string(b), uid, catchTime)
	return err
}

// ClearPetGMStats 清除 GM 面板覆盖，恢复公式计算。
func (s *sqlBackend) ClearPetGMStats(uid, catchTime int64) error {
	_, err := s.db.Exec(`
UPDATE pets SET extra_json = JSON_REMOVE(COALESCE(extra_json, JSON_OBJECT()), '$.gmStats')
WHERE user_id=? AND catch_time=?`, uid, catchTime)
	return err
}

// SetPetLearnedSkillBank 写入技能银行（唤醒仪增量）。
func (s *sqlBackend) SetPetLearnedSkillBank(uid, catchTime int64, skills []int) error {
	if skills == nil {
		skills = []int{}
	}
	b, err := json.Marshal(skills)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`
UPDATE pets SET extra_json = JSON_SET(COALESCE(extra_json, JSON_OBJECT()), '$.skillBank', CAST(? AS JSON))
WHERE user_id=? AND catch_time=?`, string(b), uid, catchTime)
	return err
}

// SetPetFormDisplay 写入形态固定/展示字段；全 0 时清除。
func (s *sqlBackend) SetPetFormDisplay(uid, catchTime int64, formLocked, displayFormID, lockedDisplayFormID int) error {
	if formLocked == 0 && displayFormID == 0 && lockedDisplayFormID == 0 {
		_, err := s.db.Exec(`
UPDATE pets SET extra_json = JSON_REMOVE(
  COALESCE(extra_json, JSON_OBJECT()), '$.formLocked', '$.displayFormId', '$.lockedDisplayFormId')
WHERE user_id=? AND catch_time=?`, uid, catchTime)
		return err
	}
	_, err := s.db.Exec(`
UPDATE pets SET extra_json = JSON_SET(
  COALESCE(extra_json, JSON_OBJECT()),
  '$.formLocked', ?, '$.displayFormId', ?, '$.lockedDisplayFormId', ?)
WHERE user_id=? AND catch_time=?`, formLocked, displayFormID, lockedDisplayFormID, uid, catchTime)
	return err
}

func scanPetRows(rows *sql.Rows) (*Pet, error) {
	return scanPet(rows)
}
