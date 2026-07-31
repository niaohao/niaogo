package store

import (
	"database/sql"
	"fmt"
)

// Task 状态对齐客户端 TasksManager：0 未接 / 1 已接 / 3 完成。
type Task struct {
	TaskID int
	Status int
	Buf    []byte // 最多 20 字节进度
}

func (s *sqlBackend) UpsertTaskStatus(uid int64, taskID, status int) error {
	if taskID <= 0 {
		return fmt.Errorf("invalid taskID")
	}
	if status < 0 {
		status = 0
	}
	if status > 255 {
		status = 255
	}
	_, err := s.db.Exec(`
INSERT INTO user_tasks(user_id, task_id, status, buf_blob)
VALUES(?,?,?,?)
ON DUPLICATE KEY UPDATE status=VALUES(status)`,
		uid, taskID, status, []byte{})
	return err
}

// UpsertTaskBuf 写入 20 字节进度；无记录时视为已接取(status=1)。
func (s *sqlBackend) UpsertTaskBuf(uid int64, taskID int, buf []byte) error {
	if taskID <= 0 {
		return fmt.Errorf("invalid taskID")
	}
	padded := make([]byte, 20)
	copy(padded, buf)
	_, err := s.db.Exec(`
INSERT INTO user_tasks(user_id, task_id, status, buf_blob)
VALUES(?,?,1,?)
ON DUPLICATE KEY UPDATE buf_blob=VALUES(buf_blob)`,
		uid, taskID, padded)
	return err
}

func (s *sqlBackend) GetTask(uid int64, taskID int) (*Task, error) {
	t := &Task{}
	var buf []byte
	err := s.db.QueryRow(`
SELECT task_id, status, buf_blob FROM user_tasks WHERE user_id=? AND task_id=?`,
		uid, taskID).Scan(&t.TaskID, &t.Status, &buf)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	t.Buf = buf
	return t, nil
}

// ListTaskStatuses 返回 taskID -> status（仅非 0）。
func (s *sqlBackend) ListTaskStatuses(uid int64) (map[int]int, error) {
	rows, err := s.db.Query(`SELECT task_id, status FROM user_tasks WHERE user_id=? AND status<>0`, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[int]int)
	for rows.Next() {
		var id, st int
		if err := rows.Scan(&id, &st); err != nil {
			return nil, err
		}
		out[id] = st
	}
	return out, rows.Err()
}

// EncodeLoginTaskList 编码登录包 taskList（任务 ID 1..n，每任务 1 字节）。
// 对齐参考 encodeLoginTaskListBytes；本客户端 UserInfo 读 1000 字节。
func (s *sqlBackend) EncodeLoginTaskList(uid int64, size int) ([]byte, error) {
	if size <= 0 {
		size = 1000
	}
	out := make([]byte, size)
	statuses, err := s.ListTaskStatuses(uid)
	if err != nil {
		return out, err
	}
	for id, st := range statuses {
		if id < 1 || id > size {
			continue
		}
		if st < 0 {
			st = 0
		} else if st > 255 {
			st = 255
		}
		// 任务 8「已接受」登录不下发，避免地图 40 卡加载（参考服）
		if id == 8 && st == 1 {
			st = 0
		}
		out[id-1] = byte(st)
	}
	return out, nil
}

// DeleteTask 删除任务进度行（2205/2232）。
func (s *sqlBackend) DeleteTask(uid int64, taskID int) error {
	if taskID <= 0 {
		return nil
	}
	_, err := s.db.Exec(`DELETE FROM user_tasks WHERE user_id=? AND task_id=?`, uid, taskID)
	return err
}
