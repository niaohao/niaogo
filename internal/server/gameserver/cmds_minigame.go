package gameserver

import (
	"encoding/binary"
	"log"
)

// 小游戏 5001 进入 / 5002 结算：水火草等；按攻略日限发量子碎片 400507（日合计 15）。
// 本客户端 GAME_OVER 包体多为 (per, score) 两×u32，不是 (gameID, score)；gameID 用进场 5001 记住。
const (
	miniGameQuantumItemID = 400507
	miniGameQuantumDaily  = 15
	miniGameQuantumKey    = "quantumFrag"
	miniGameScorePerFrag  = 500 // 每 500 分折合 1 碎片（本局），受日限截断
)

func (s *Server) handleMiniGameEnter(c *Client, uid uint32, body []byte) {
	gameID := uint32(0)
	if len(body) >= 4 {
		gameID = binary.BigEndian.Uint32(body[0:4])
	}
	if c != nil {
		c.mu.Lock()
		c.LastMiniGameID = gameID
		c.mu.Unlock()
	}
	best := uint32(s.lifetimeCount(int64(uid), miniGameBestKey(gameID)))
	out := make([]byte, 16)
	binary.BigEndian.PutUint32(out[0:4], 0)
	binary.BigEndian.PutUint32(out[4:8], best)
	binary.BigEndian.PutUint32(out[8:12], gameID)
	binary.BigEndian.PutUint32(out[12:16], uid)
	s.send(c, 5001, uid, 0, out)
	log.Printf("[CMD] OK     5001 JOIN_GAME UID=%d game=%d best=%d", uid, gameID, best)
}

func (s *Server) handleMiniGameOver(c *Client, uid uint32, body []byte) {
	// 常见：per(4)+score(4)；少数只发 1×u32
	per, score := uint32(0), uint32(0)
	if len(body) >= 8 {
		per = binary.BigEndian.Uint32(body[0:4])
		score = binary.BigEndian.Uint32(body[4:8])
	} else if len(body) >= 4 {
		score = binary.BigEndian.Uint32(body[0:4])
	}
	// MapProcess_6/51 等会发 (n,n)；取较大值作分
	if per > score {
		score = per
	}
	gameID := uint32(0)
	if c != nil {
		c.mu.Lock()
		gameID = c.LastMiniGameID
		c.mu.Unlock()
	}
	bestKey := miniGameBestKey(gameID)
	best := uint32(s.lifetimeCount(int64(uid), bestKey))
	if score > best {
		s.setLifetime(int64(uid), bestKey, int(score))
		best = score
	}
	frags := 0
	if s.cfg.Store != nil && score > 0 {
		want := int(score / miniGameScorePerFrag)
		if want < 1 && score >= 100 {
			want = 1
		}
		if want > 5 {
			want = 5 // 单局上限
		}
		have := s.dailyCount(int64(uid), miniGameQuantumKey)
		left := miniGameQuantumDaily - have
		if left < 0 {
			left = 0
		}
		if want > left {
			want = left
		}
		if want > 0 {
			for i := 0; i < want; i++ {
				s.bumpDaily(int64(uid), miniGameQuantumKey)
			}
			_ = s.cfg.Store.AddItem(int64(uid), miniGameQuantumItemID, want)
			frags = want
			s.sendAlert(int64(uid), "获得量子碎片×"+itoaU32(uint32(want))+
				"（今日"+itoaU32(uint32(have+want))+"/"+itoaU32(miniGameQuantumDaily)+"）")
		}
	}
	out := make([]byte, 16)
	binary.BigEndian.PutUint32(out[0:4], 0)
	binary.BigEndian.PutUint32(out[4:8], best)
	binary.BigEndian.PutUint32(out[8:12], gameID)
	binary.BigEndian.PutUint32(out[12:16], uid)
	s.send(c, 5002, uid, 0, out)
	log.Printf("[CMD] OK     5002 GAME_OVER UID=%d game=%d score=%d best=%d frag=%d", uid, gameID, score, best, frags)
}

func miniGameBestKey(gameID uint32) string {
	return "miniBest:" + itoaU32(gameID)
}
