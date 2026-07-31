package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"time"
)

// UserOpsState 荣誉、日常/周常/月常计数、融合特性池、终身标记（落库，跨重启保留）。
type UserOpsState struct {
	Honor           int            `json:"honor"`
	CurTopLevel     int            `json:"curTopLevel,omitempty"` // 巅峰之战积分/等级（46001 score；与 honor 独立）
	Day             string         `json:"day"`                   // 游戏日（东八区，6:00 换日）
	Daily           map[string]int `json:"daily,omitempty"`
	Week            string         `json:"week"` // ISO 周键
	Weekly          map[string]int `json:"weekly,omitempty"`
	Month           string         `json:"month,omitempty"` // 东八区 YYYY-MM
	Monthly         map[string]int `json:"monthly,omitempty"`
	Lifetime        map[string]int `json:"lifetime,omitempty"` // 终身计数/标记（不随日月清）
	FusionTraits    map[string]int `json:"fusionTraits,omitempty"`
	TeacherID       uint32         `json:"teacherId,omitempty"`
	StudentID       uint32         `json:"studentId,omitempty"`
	GraduationCount int            `json:"graduationCount,omitempty"`
	TeacherExpPond  int            `json:"teacherExpPond,omitempty"` // 教官积累、学员可领的共享经验
}

// 巅峰积分上下限（排行 score / GM 灌分）。
const (
	TopLevelMin = 0
	TopLevelMax = 999999
)

// ClampTopLevel 将巅峰积分钳到合法区间。
func ClampTopLevel(v int) int {
	if v < TopLevelMin {
		return TopLevelMin
	}
	if v > TopLevelMax {
		return TopLevelMax
	}
	return v
}

// TopWarRankEntry 巅峰赛季排行一项（46001）。
type TopWarRankEntry struct {
	UserID   int64
	Nickname string
	Score    int
}

func (s *sqlBackend) ensureUserOpsSchema() error {
	_, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS user_ops_state (
  user_id BIGINT NOT NULL PRIMARY KEY,
  state_json JSON NOT NULL,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`)
	return err
}

func (s *sqlBackend) GetUserOps(uid int64) (UserOpsState, error) {
	_ = s.ensureUserOpsSchema()
	var raw []byte
	err := s.db.QueryRow(`SELECT state_json FROM user_ops_state WHERE user_id=?`, uid).Scan(&raw)
	if err == sql.ErrNoRows {
		return UserOpsState{}, nil
	}
	if err != nil {
		return UserOpsState{}, err
	}
	var st UserOpsState
	if err := json.Unmarshal(raw, &st); err != nil {
		return UserOpsState{}, nil
	}
	return st, nil
}

func (s *sqlBackend) SetUserOps(uid int64, st UserOpsState) error {
	_ = s.ensureUserOpsSchema()
	raw, err := json.Marshal(st)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`
INSERT INTO user_ops_state(user_id, state_json) VALUES(?,?)
ON DUPLICATE KEY UPDATE state_json=VALUES(state_json)`, uid, raw)
	return err
}

// ListTopWarRanks 按 curTopLevel 降序返回排行；score≤0 不上榜。limit≤0 默认 100。
func (s *sqlBackend) ListTopWarRanks(limit int) ([]TopWarRankEntry, error) {
	_ = s.ensureUserOpsSchema()
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.Query(`
SELECT o.user_id,
       COALESCE(u.nickname, ''),
       CAST(COALESCE(JSON_UNQUOTE(JSON_EXTRACT(o.state_json, '$.curTopLevel')), '0') AS UNSIGNED) AS score
FROM user_ops_state o
LEFT JOIN users u ON u.user_id = o.user_id
WHERE CAST(COALESCE(JSON_UNQUOTE(JSON_EXTRACT(o.state_json, '$.curTopLevel')), '0') AS UNSIGNED) > 0
ORDER BY score DESC, o.user_id ASC
LIMIT ?`, limit)
	if err != nil {
		// 部分环境 JSON_* 不可用时回落全表扫描
		return s.listTopWarRanksScan(limit)
	}
	defer rows.Close()
	out := make([]TopWarRankEntry, 0)
	for rows.Next() {
		var e TopWarRankEntry
		if err := rows.Scan(&e.UserID, &e.Nickname, &e.Score); err != nil {
			return out, err
		}
		e.Score = ClampTopLevel(e.Score)
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return out, err
	}
	return out, nil
}

func (s *sqlBackend) listTopWarRanksScan(limit int) ([]TopWarRankEntry, error) {
	rows, err := s.db.Query(`SELECT user_id, state_json FROM user_ops_state`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type scored struct {
		uid   int64
		score int
	}
	var scoredList []scored
	for rows.Next() {
		var uid int64
		var raw []byte
		if err := rows.Scan(&uid, &raw); err != nil {
			return nil, err
		}
		var st UserOpsState
		if json.Unmarshal(raw, &st) != nil {
			continue
		}
		sc := ClampTopLevel(st.CurTopLevel)
		if sc <= 0 {
			continue
		}
		scoredList = append(scoredList, scored{uid: uid, score: sc})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.Slice(scoredList, func(i, j int) bool {
		if scoredList[i].score != scoredList[j].score {
			return scoredList[i].score > scoredList[j].score
		}
		return scoredList[i].uid < scoredList[j].uid
	})
	if len(scoredList) > limit {
		scoredList = scoredList[:limit]
	}
	out := make([]TopWarRankEntry, 0, len(scoredList))
	for _, it := range scoredList {
		nick := ""
		if u, err := s.FindByUserID(it.uid); err == nil && u != nil {
			nick = u.Nickname
		}
		out = append(out, TopWarRankEntry{UserID: it.uid, Nickname: nick, Score: it.score})
	}
	return out, nil
}

// ChinaGameDayKey 东八区、每日 6:00 刷新的日期键。
func ChinaGameDayKey(now time.Time) string {
	t := now.In(chinaLoc())
	if t.Hour() < 6 {
		t = t.Add(-24 * time.Hour)
	}
	return t.Format("2006-01-02")
}

// ChinaWeekKey 东八区 ISO 周键（周常螳螂 / 6v6 周上限）。
func ChinaWeekKey(now time.Time) string {
	t := now.In(chinaLoc())
	y, w := t.ISOWeek()
	return fmt.Sprintf("%04d-W%02d", y, w)
}

// ChinaMonthKey 东八区年月键（发明室月领胶囊等）。
func ChinaMonthKey(now time.Time) string {
	t := now.In(chinaLoc())
	return t.Format("2006-01")
}

// NormalizeUserOps 跨日清 daily、跨周清 weekly、跨月清 monthly。
func NormalizeUserOps(st UserOpsState, now time.Time) UserOpsState {
	day := ChinaGameDayKey(now)
	week := ChinaWeekKey(now)
	month := ChinaMonthKey(now)
	if st.Day != day {
		st.Day = day
		st.Daily = map[string]int{}
	}
	if st.Week != week {
		st.Week = week
		st.Weekly = map[string]int{}
	}
	if st.Month != month {
		st.Month = month
		st.Monthly = map[string]int{}
	}
	if st.Daily == nil {
		st.Daily = map[string]int{}
	}
	if st.Weekly == nil {
		st.Weekly = map[string]int{}
	}
	if st.Monthly == nil {
		st.Monthly = map[string]int{}
	}
	if st.Lifetime == nil {
		st.Lifetime = map[string]int{}
	}
	if st.FusionTraits == nil {
		st.FusionTraits = map[string]int{}
	}
	if st.Honor < 0 {
		st.Honor = 0
	}
	st.CurTopLevel = ClampTopLevel(st.CurTopLevel)
	if st.GraduationCount < 0 {
		st.GraduationCount = 0
	}
	if st.TeacherExpPond < 0 {
		st.TeacherExpPond = 0
	}
	return st
}
