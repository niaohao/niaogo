package store

import (
	"database/sql"
	"encoding/json"
	"time"
)

// MailItemReward 邮件附件道具。
type MailItemReward struct {
	ItemID int `json:"itemId"`
	Count  int `json:"count"`
}

// MailReward 读信（2754）一次性领取的奖励。
type MailReward struct {
	Coins    int              `json:"coins,omitempty"`
	Gold     int              `json:"gold,omitempty"`
	PetID    int              `json:"petId,omitempty"`
	PetCount int              `json:"petCount,omitempty"`
	Items    []MailItemReward `json:"items,omitempty"`
}

// HasReward 是否含可发附件。
func (r MailReward) HasReward() bool {
	if r.Coins > 0 || r.Gold > 0 || r.PetID > 0 {
		return true
	}
	for _, it := range r.Items {
		if it.ItemID > 0 && it.Count > 0 {
			return true
		}
	}
	return false
}

// Mail 星际邮件（2751/2753/2754）。
type Mail struct {
	ID       int64
	Template int
	MailTime int64
	FromID   int64
	FromNick string
	Content  string
	Read     bool
	Claimed  bool
	Reward   MailReward
}

const mailPageSize = 50

func encodeMailReward(r MailReward) string {
	if !r.HasReward() {
		return ""
	}
	b, err := json.Marshal(r)
	if err != nil {
		return ""
	}
	return string(b)
}

func decodeMailReward(raw sql.NullString) MailReward {
	if !raw.Valid || raw.String == "" || raw.String == "{}" {
		return MailReward{}
	}
	var r MailReward
	_ = json.Unmarshal([]byte(raw.String), &r)
	return r
}

func scanMail(row interface {
	Scan(dest ...any) error
}) (*Mail, error) {
	m := &Mail{}
	var read, claimed int
	var rewardRaw sql.NullString
	err := row.Scan(&m.ID, &m.Template, &m.MailTime, &m.FromID, &m.FromNick, &m.Content, &read, &claimed, &rewardRaw)
	if err != nil {
		return nil, err
	}
	m.Read = read != 0
	m.Claimed = claimed != 0
	m.Reward = decodeMailReward(rewardRaw)
	return m, nil
}

// ListMails 分页列表；start 为 1-based 页内起始序号。
func (s *sqlBackend) ListMails(uid int64, start int) (total int, mails []Mail, err error) {
	if start < 1 {
		start = 1
	}
	if err = s.db.QueryRow(`SELECT COUNT(1) FROM user_mails WHERE user_id=?`, uid).Scan(&total); err != nil {
		return
	}
	offset := start - 1
	rows, err := s.db.Query(`
SELECT id, template, mail_time, from_id, from_nick, content, is_read, is_claimed, reward_json
FROM user_mails WHERE user_id=? ORDER BY mail_time DESC, id DESC LIMIT ? OFFSET ?`,
		uid, mailPageSize, offset)
	if err != nil {
		return total, nil, err
	}
	defer rows.Close()
	for rows.Next() {
		m, err := scanMail(rows)
		if err != nil {
			return total, mails, err
		}
		mails = append(mails, *m)
	}
	return total, mails, rows.Err()
}

// GetMail 按 id 查单封（校验归属）。
func (s *sqlBackend) GetMail(uid, mailID int64) (*Mail, error) {
	m, err := scanMail(s.db.QueryRow(`
SELECT id, template, mail_time, from_id, from_nick, content, is_read, is_claimed, reward_json
FROM user_mails WHERE user_id=? AND id=?`, uid, mailID))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return m, nil
}

// CountUnreadMails 未读数（2757）。
func (s *sqlBackend) CountUnreadMails(uid int64) (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(1) FROM user_mails WHERE user_id=? AND is_read=0`, uid).Scan(&n)
	return n, err
}

// InsertMail 写入收件箱（无附件）。
func (s *sqlBackend) InsertMail(uid int64, template int, fromID int64, fromNick, content string) (int64, error) {
	return s.InsertMailWithReward(uid, template, fromID, fromNick, content, MailReward{})
}

// InsertMailWithReward 写入带附件的邮件（系统/GM 发奖）。
func (s *sqlBackend) InsertMailWithReward(uid int64, template int, fromID int64, fromNick, content string, reward MailReward) (int64, error) {
	now := time.Now().Unix()
	res, err := s.db.Exec(`
INSERT INTO user_mails (user_id, template, mail_time, from_id, from_nick, content, is_read, is_claimed, reward_json)
VALUES (?,?,?,?,?,?,0,0,?)`, uid, template, now, fromID, fromNick, content, encodeMailReward(reward))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// MarkMailsRead 批量已读（无发奖）。
func (s *sqlBackend) MarkMailsRead(uid int64, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, err := s.db.Exec(`UPDATE user_mails SET is_read=1 WHERE user_id=? AND id=?`, uid, id); err != nil {
			return err
		}
	}
	return nil
}

// MarkMailsReadAndClaim 已读并原子领取附件；返回本次新领取的邮件（含 Reward）。
func (s *sqlBackend) MarkMailsReadAndClaim(uid int64, ids []int64) ([]Mail, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	out := make([]Mail, 0)
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		m, err := s.GetMail(uid, id)
		if err != nil {
			return out, err
		}
		if m == nil {
			continue
		}
		if m.Claimed {
			_, _ = s.db.Exec(`UPDATE user_mails SET is_read=1 WHERE user_id=? AND id=?`, uid, id)
			continue
		}
		res, err := s.db.Exec(`
UPDATE user_mails SET is_read=1, is_claimed=1 WHERE user_id=? AND id=? AND is_claimed=0`, uid, id)
		if err != nil {
			return out, err
		}
		n, _ := res.RowsAffected()
		if n > 0 && m.Reward.HasReward() {
			m.Read = true
			m.Claimed = true
			out = append(out, *m)
		}
	}
	return out, nil
}

// DeleteMails 批量删除（2755）。
func (s *sqlBackend) DeleteMails(uid int64, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, err := s.db.Exec(`DELETE FROM user_mails WHERE user_id=? AND id=?`, uid, id); err != nil {
			return err
		}
	}
	return nil
}

// DeleteAllMails 清空（2756）。
func (s *sqlBackend) DeleteAllMails(uid int64) error {
	_, err := s.db.Exec(`DELETE FROM user_mails WHERE user_id=?`, uid)
	return err
}
