package gameserver

import (
	"log"
)

// NoviceFinishInfo 虚拟奖励 ID（非背包道具）：1=赛尔豆、3=积累经验、5=金豆。
const (
	taskRewardCoins   = 1
	taskRewardExpPool = 3
	taskRewardGold    = 5
)

const defaultTaskExpPool = 2000

// storyTaskRewards 对齐本客户端 TaskClass_* 写死文案/本地加豆；返回 (奖励, 是否有专用表)。
// 有专用表时不再套默认 2000 经验。
func storyTaskRewards(taskID uint32) ([][2]uint32, bool) {
	switch taskID {
	case 18, 19:
		return [][2]uint32{{taskRewardExpPool, 3000}}, true
	case 79:
		return [][2]uint32{{taskRewardExpPool, 3000}, {taskRewardCoins, 2000}}, true
	case 81, 83:
		return [][2]uint32{{taskRewardExpPool, 1000}, {taskRewardCoins, 1000}}, true
	case 84:
		return [][2]uint32{{taskRewardExpPool, 2000}, {taskRewardCoins, 2000}}, true
	case 89:
		return [][2]uint32{{taskRewardExpPool, 500}, {taskRewardCoins, 1000}}, true
	case 90:
		return [][2]uint32{{taskRewardExpPool, 1000}, {taskRewardCoins, 1000}}, true
	case 91, 92:
		return [][2]uint32{{taskRewardExpPool, 2000}, {taskRewardCoins, 2000}}, true
	case 93:
		return [][2]uint32{{taskRewardExpPool, 1000}, {taskRewardCoins, 500}}, true
	case 94:
		return [][2]uint32{{taskRewardExpPool, 500}, {taskRewardCoins, 2000}}, true
	case 95:
		// TaskClass_95：4000 经验 + 2000 豆；monBallList 含 100346 时弹刺蜂套装
		return [][2]uint32{
			{taskRewardExpPool, 4000},
			{taskRewardCoins, 2000},
			{100346, 1},
		}, true
	case 96:
		return [][2]uint32{{taskRewardExpPool, 500}, {taskRewardCoins, 1000}}, true
	case 97:
		return [][2]uint32{{taskRewardExpPool, 1000}, {taskRewardCoins, 1000}}, true
	case 98:
		return [][2]uint32{{taskRewardExpPool, 2000}, {taskRewardCoins, 1000}}, true
	case 401, 402, 403:
		// 训练营三主宠日常：合计 6w 经验 + 1.5w 豆；闪光尼尔在全部完成后另发
		return [][2]uint32{{taskRewardExpPool, 20000}, {taskRewardCoins, 5000}}, true
	case 404, 405, 406, 407:
		return [][2]uint32{{taskRewardExpPool, 2000}}, true
	case 86, 87, 88:
		// 新手三连由 handleCompleteTask 专办，不套默认经验
		return nil, true
	}
	return nil, false
}

func resolveTaskRewards(taskID uint32, isDaily bool) [][2]uint32 {
	if pairs, ok := storyTaskRewards(taskID); ok {
		return pairs
	}
	if isDaily {
		return [][2]uint32{{taskRewardExpPool, defaultTaskExpPool}}
	}
	// 其它一次性任务：默认 2000 积累经验（对齐参考服兜底，供无 TaskClass 的通用完成提示）
	return [][2]uint32{{taskRewardExpPool, defaultTaskExpPool}}
}

func (s *Server) applyTaskItemRewards(uid int64, pairs [][2]uint32) {
	if s.cfg.Store == nil || len(pairs) == 0 {
		return
	}
	for _, p := range pairs {
		id, cnt := int(p[0]), int(p[1])
		if id <= 0 || cnt <= 0 {
			continue
		}
		switch id {
		case taskRewardCoins:
			if err := s.cfg.Store.AddCoins(uid, cnt); err != nil {
				log.Printf("[task] AddCoins uid=%d +%d: %v", uid, cnt, err)
			}
		case taskRewardExpPool:
			if _, err := s.cfg.Store.AddExpPool(uid, cnt); err != nil {
				log.Printf("[task] AddExpPool uid=%d +%d: %v", uid, cnt, err)
			}
		case taskRewardGold:
			if err := s.cfg.Store.AddGold(uid, cnt); err != nil {
				log.Printf("[task] AddGold uid=%d +%d: %v", uid, cnt, err)
			}
		default:
			if err := s.cfg.Store.AddItem(uid, id, cnt); err != nil {
				log.Printf("[task] AddItem uid=%d item=%d +%d: %v", uid, id, cnt, err)
			}
		}
	}
}

func (s *Server) taskAlreadyComplete(uid int64, taskID int) bool {
	if s.cfg.Store == nil || taskID <= 0 {
		return false
	}
	t, err := s.cfg.Store.GetTask(uid, taskID)
	if err != nil || t == nil {
		return false
	}
	return t.Status >= taskStatusComplete
}
