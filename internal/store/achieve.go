package store

import (
	"database/sql"
	"fmt"
)

// AchieveRuleRow 用户某条成就规则进度。
type AchieveRuleRow struct {
	BranchID  int
	RuleID    int
	Progress  int
	Completed bool
	Claimed   bool
}

// EnsureAchieveSchema 成就相关表。
func (s *sqlBackend) ensureAchieveSchema() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS user_achieve_rules (
  user_id BIGINT NOT NULL,
  branch_id INT NOT NULL,
  rule_id INT NOT NULL,
  progress INT NOT NULL DEFAULT 0,
  completed TINYINT NOT NULL DEFAULT 0,
  claimed TINYINT NOT NULL DEFAULT 0,
  PRIMARY KEY (user_id, branch_id, rule_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS user_achieve_branch (
  user_id BIGINT NOT NULL,
  branch_id INT NOT NULL,
  value INT NOT NULL DEFAULT 0,
  status INT NOT NULL DEFAULT 0,
  PRIMARY KEY (user_id, branch_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS user_titles (
  user_id BIGINT NOT NULL,
  title_id INT NOT NULL,
  PRIMARY KEY (user_id, title_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
	}
	for _, q := range stmts {
		if _, err := s.db.Exec(q); err != nil {
			return fmt.Errorf("achieve schema: %w", err)
		}
	}
	return nil
}

// ListAchieveRules 用户全部成就规则行。
func (s *sqlBackend) ListAchieveRules(uid int64) ([]AchieveRuleRow, error) {
	rows, err := s.db.Query(`
SELECT branch_id, rule_id, progress, completed, claimed
FROM user_achieve_rules WHERE user_id=?`, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]AchieveRuleRow, 0)
	for rows.Next() {
		var r AchieveRuleRow
		var done, claimed int
		if err := rows.Scan(&r.BranchID, &r.RuleID, &r.Progress, &done, &claimed); err != nil {
			return nil, err
		}
		r.Completed = done != 0
		r.Claimed = claimed != 0
		out = append(out, r)
	}
	return out, rows.Err()
}

// ListAchieveRulesOfBranch 单分支规则进度。
func (s *sqlBackend) ListAchieveRulesOfBranch(uid int64, branchID int) ([]AchieveRuleRow, error) {
	rows, err := s.db.Query(`
SELECT branch_id, rule_id, progress, completed, claimed
FROM user_achieve_rules WHERE user_id=? AND branch_id=?`, uid, branchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]AchieveRuleRow, 0)
	for rows.Next() {
		var r AchieveRuleRow
		var done, claimed int
		if err := rows.Scan(&r.BranchID, &r.RuleID, &r.Progress, &done, &claimed); err != nil {
			return nil, err
		}
		r.Completed = done != 0
		r.Claimed = claimed != 0
		out = append(out, r)
	}
	return out, rows.Err()
}

// UpsertAchieveRule 写入/更新规则进度。
func (s *sqlBackend) UpsertAchieveRule(uid int64, r AchieveRuleRow) error {
	done, claimed := 0, 0
	if r.Completed {
		done = 1
	}
	if r.Claimed {
		claimed = 1
	}
	_, err := s.db.Exec(`
INSERT INTO user_achieve_rules(user_id, branch_id, rule_id, progress, completed, claimed)
VALUES(?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE progress=VALUES(progress), completed=VALUES(completed), claimed=VALUES(claimed)`,
		uid, r.BranchID, r.RuleID, r.Progress, done, claimed)
	return err
}

// GetAchieveBranchState value/status。
func (s *sqlBackend) GetAchieveBranchState(uid int64, branchID int) (value, status int, err error) {
	err = s.db.QueryRow(`
SELECT value, status FROM user_achieve_branch WHERE user_id=? AND branch_id=?`,
		uid, branchID).Scan(&value, &status)
	if err == sql.ErrNoRows {
		return 0, 0, nil
	}
	return
}

// SetAchieveBranchState 写分支 value/status。
func (s *sqlBackend) SetAchieveBranchState(uid int64, branchID, value, status int) error {
	_, err := s.db.Exec(`
INSERT INTO user_achieve_branch(user_id, branch_id, value, status)
VALUES(?,?,?,?)
ON DUPLICATE KEY UPDATE value=VALUES(value), status=VALUES(status)`,
		uid, branchID, value, status)
	return err
}

// ListTitles 已获得称号。
func (s *sqlBackend) ListTitles(uid int64) ([]int, error) {
	rows, err := s.db.Query(`SELECT title_id FROM user_titles WHERE user_id=? ORDER BY title_id`, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]int, 0)
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// GrantTitle 发放称号（幂等）。
func (s *sqlBackend) GrantTitle(uid int64, titleID int) error {
	if titleID <= 0 {
		return nil
	}
	_, err := s.db.Exec(`INSERT IGNORE INTO user_titles(user_id, title_id) VALUES(?,?)`, uid, titleID)
	return err
}

// ListDefeatedSPTKeys 全部 SPT 击败 key。
func (s *sqlBackend) ListDefeatedSPTKeys(uid int64) ([]int, error) {
	rows, err := s.db.Query(`SELECT boss_key FROM user_spt_defeated WHERE user_id=?`, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]int, 0)
	for rows.Next() {
		var k int
		if err := rows.Scan(&k); err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}
