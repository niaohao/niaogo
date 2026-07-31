package store

import (
	"encoding/json"
	"fmt"
	"time"
)

// PerfectBalancedEV 极品学习力：六维均分且总和=510。
func PerfectBalancedEV() [6]int {
	return [6]int{85, 85, 85, 85, 85, 85}
}

// GrantPetsBatch 批量发放精灵（GM 一键全图鉴）。
// 背包未满时优先填满 6 只，其余进仓库；每只须带好 PetID/Name/Level/DV/Nature/Skills，可选 EV。
// 返回成功写入数量与第一只的 catchTime。
func (s *sqlBackend) GrantPetsBatch(uid int64, pets []Pet) (granted int, firstCatch int64, err error) {
	if uid <= 0 {
		return 0, 0, fmt.Errorf("bad uid")
	}
	if len(pets) == 0 {
		return 0, 0, nil
	}
	used := map[int64]struct{}{}
	rows, qerr := s.db.Query(`SELECT catch_time FROM pets WHERE user_id=?`, uid)
	if qerr != nil {
		return 0, 0, qerr
	}
	for rows.Next() {
		var ct int64
		if err := rows.Scan(&ct); err != nil {
			rows.Close()
			return 0, 0, err
		}
		used[ct] = struct{}{}
	}
	rows.Close()

	bagCount, _ := s.CountBagPets(uid)
	catchTime := time.Now().Unix()

	tx, err := s.db.Begin()
	if err != nil {
		return 0, 0, err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	stmt, err := tx.Prepare(`
INSERT INTO pets(user_id, catch_time, pet_id, pet_name, level, exp, dv, nature, bag_pos, in_bag, skills_json, extra_json)
VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`)
	if err != nil {
		return 0, 0, err
	}
	defer stmt.Close()

	for i := range pets {
		p := &pets[i]
		if p.PetID <= 0 {
			continue
		}
		if p.Level <= 0 {
			p.Level = 1
		}
		if p.Name == "" {
			p.Name = "精灵"
		}
		if p.Skills == nil {
			p.Skills = []int{10001}
		}
		for {
			if _, ok := used[catchTime]; !ok {
				break
			}
			catchTime++
		}
		used[catchTime] = struct{}{}

		inBag := 0
		bagPos := -1
		if bagCount < MaxBagPets {
			inBag = 1
			bagPos = 99
			bagCount++
		}
		skillsJSON, mErr := json.Marshal(p.Skills)
		if mErr != nil {
			err = mErr
			return 0, 0, err
		}
		extra := "{}"
		if evTotal(p.EV) > 0 {
			ex := petExtraJSON{EV: []int{p.EV[0], p.EV[1], p.EV[2], p.EV[3], p.EV[4], p.EV[5]}}
			b, mErr := json.Marshal(ex)
			if mErr != nil {
				err = mErr
				return 0, 0, err
			}
			extra = string(b)
		}
		if _, err = stmt.Exec(uid, catchTime, p.PetID, p.Name, p.Level, p.Exp, p.DV, p.Nature, bagPos, inBag, string(skillsJSON), extra); err != nil {
			return 0, 0, err
		}
		if firstCatch == 0 {
			firstCatch = catchTime
		}
		granted++
		catchTime++
	}
	if err = tx.Commit(); err != nil {
		return 0, 0, err
	}
	return granted, firstCatch, nil
}

func evTotal(ev [6]int) int {
	n := 0
	for _, v := range ev {
		n += v
	}
	return n
}
