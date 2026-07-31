package gameserver

import (
	"encoding/binary"
	"log"
	"math/rand"
	"time"

	"niaohao/server/internal/cmdname"
)

// handleRemainEmptyAck 客户端不解析包体的命令：空 ACK。
func (s *Server) handleRemainEmptyAck(c *Client, uid uint32, cmd int32) {
	s.send(c, cmd, uid, 0, nil)
	log.Printf("[CMD] OK     %s UID=%d", cmdname.Format(cmd), uid)
}

// handleRemainU32Ack 回 4B 指定值。
func (s *Server) handleRemainU32Ack(c *Client, uid uint32, cmd int32, v uint32) {
	out := make([]byte, 4)
	binary.BigEndian.PutUint32(out, v)
	s.send(c, cmd, uid, 0, out)
	log.Printf("[CMD] OK     %s UID=%d v=%d", cmdname.Format(cmd), uid, v)
}

// handleRemainZeroN 回 n 字节零。
func (s *Server) handleRemainZeroN(c *Client, uid uint32, cmd int32, n int) {
	if n < 0 {
		n = 0
	}
	s.send(c, cmd, uid, 0, make([]byte, n))
	log.Printf("[CMD] OK     %s UID=%d body=%d", cmdname.Format(cmd), uid, n)
}

// handleReadCount CMD 1007：无监听细节，回 count=0。
func (s *Server) handleReadCount(c *Client, uid uint32, body []byte) {
	s.handleRemainU32Ack(c, uid, 1007, 0)
}

// handleOfflineExpQuery CMD 2023：本 CMD 多为推送；若客户端请求则回 exp=0。
func (s *Server) handleOfflineExpQuery(c *Client, uid uint32, body []byte) {
	s.handleRemainU32Ack(c, uid, 2023, 0)
}

// handleRequestCount CMD 2053：回 4B 零。
func (s *Server) handleRequestCount(c *Client, uid uint32, body []byte) {
	s.handleRemainU32Ack(c, uid, 2053, 0)
}

// handleGetRequestAward CMD 2064：空 ACK。
func (s *Server) handleGetRequestAward(c *Client, uid uint32, body []byte) {
	s.handleRemainEmptyAck(c, uid, 2064)
}

// handleChangeTaskStatues CMD 2206：请求 taskID+status；应答 taskID。
func (s *Server) handleChangeTaskStatues(c *Client, uid uint32, body []byte) {
	taskID, status := uint32(0), uint32(0)
	if len(body) >= 4 {
		taskID = binary.BigEndian.Uint32(body[0:4])
	}
	if len(body) >= 8 {
		status = binary.BigEndian.Uint32(body[4:8])
	}
	if s.cfg.Store != nil && taskID > 0 {
		st := 1
		switch status {
		case 0, 2:
			st = 0
		case 3:
			st = 3
		}
		_ = s.cfg.Store.UpsertTaskStatus(int64(uid), int(taskID), st)
	}
	out := make([]byte, 4)
	binary.BigEndian.PutUint32(out, taskID)
	s.send(c, 2206, uid, 0, out)
	log.Printf("[CMD] OK     %s UID=%d task=%d status=%d", cmdname.Format(2206), uid, taskID, status)
}

// handleSkillSort CMD 2328：本客户端名 Skill_Sort；对齐加速器次数 20B 查询。
func (s *Server) handleSkillSort(c *Client, uid uint32, body []byte) {
	out := buildBoostTimesBody(s.boostTimesOf(int64(uid)))
	s.send(c, 2328, uid, 0, out)
	log.Printf("[CMD] OK     %s UID=%d", cmdname.Format(2328), uid)
}

// handlePrizeOfPetKing CMD 2317：plan+petID → petID+catchTime。
func (s *Server) handlePrizeOfPetKing(c *Client, uid uint32, body []byte) {
	empty := make([]byte, 8)
	if len(body) < 8 || s.cfg.Store == nil {
		s.send(c, 2317, uid, 0, empty)
		return
	}
	plan := binary.BigEndian.Uint32(body[0:4])
	petID := int(binary.BigEndian.Uint32(body[4:8]))
	if plan != 301 || (petID != 1 && petID != 4 && petID != 7) {
		s.send(c, 2317, uid, 0, empty)
		return
	}
	if claimed, _ := s.cfg.Store.IsPetCollectClaimed(int64(uid), 301); claimed {
		s.send(c, 2317, uid, 0, empty)
		return
	}
	name := ""
	if s.cfg.Catalog != nil {
		name = s.cfg.Catalog.PetNameOf(petID)
	}
	catch, err := s.cfg.Store.GrantPet(int64(uid), petID, name, 1, rand.Intn(32), rand.Intn(25), nil)
	if err != nil {
		s.send(c, 2317, uid, 0, empty)
		log.Printf("[CMD] WARN  %s UID=%d grant: %v", cmdname.Format(2317), uid, err)
		return
	}
	_ = s.cfg.Store.MarkPetCollectClaimed(int64(uid), 301)
	out := make([]byte, 8)
	binary.BigEndian.PutUint32(out[0:4], uint32(petID))
	binary.BigEndian.PutUint32(out[4:8], uint32(catch))
	s.send(c, 2317, uid, 0, out)
	log.Printf("[CMD] OK     %s UID=%d pet=%d catch=%d", cmdname.Format(2317), uid, petID, catch)
}

// handleGetDiamond CMD 9025：diamond(4)。
func (s *Server) handleGetDiamond(c *Client, uid uint32, body []byte) {
	s.handleRemainU32Ack(c, uid, 9025, 0)
}

// handleHitStone CMD 2105：BossMonsterInfo 最小（无道具列表）。
func (s *Server) handleHitStone(c *Client, uid uint32, body []byte) {
	out := make([]byte, 16) // bonus+pet+capture+itemCount0
	s.send(c, 2105, uid, 0, out)
	log.Printf("[CMD] OK     %s UID=%d", cmdname.Format(2105), uid)
}

// handleFestivalGift 节日/铭牌类：空 ACK 或简单发奖提示。
func (s *Server) handleFestivalGift(c *Client, uid uint32, cmd int32, body []byte) {
	switch cmd {
	case 1108, 1110: // 红包 / 元宵
		if s.cfg.Store != nil {
			_ = s.cfg.Store.AddCoins(int64(uid), 500)
			_, _ = s.cfg.Store.AddExpPool(int64(uid), 1000)
		}
		s.handleRemainEmptyAck(c, uid, cmd)
	case 1111, 1112: // 铭牌兑换/领取
		s.handleRemainU32Ack(c, uid, cmd, 0)
	case 2065:
		s.handleRemainEmptyAck(c, uid, cmd)
	default:
		s.handleRemainEmptyAck(c, uid, cmd)
	}
}

// handleUseBagItem CMD 80009：空 ACK（客户端自处理）。
func (s *Server) handleUseBagItem(c *Client, uid uint32, body []byte) {
	s.handleRemainEmptyAck(c, uid, 80009)
}

// handlePetKingJoin CMD 2413：空 ACK（匹配 stub）。
func (s *Server) handlePetKingJoin(c *Client, uid uint32, body []byte) {
	s.handleRemainEmptyAck(c, uid, 2413)
}

// handleNPCJoin CMD 2427。
func (s *Server) handleNPCJoin(c *Client, uid uint32, body []byte) {
	s.handleRemainEmptyAck(c, uid, 2427)
}

// handlePetTopLevelJoin CMD 2458。
func (s *Server) handlePetTopLevelJoin(c *Client, uid uint32, body []byte) {
	s.handleRemainEmptyAck(c, uid, 2458)
}

// handleMasterRewards CMD 2611：空列表 count=0。
func (s *Server) handleMasterRewards(c *Client, uid uint32, body []byte) {
	s.handleRemainU32Ack(c, uid, 2611, 0)
}

// handleGetPetKingRewards CMD 2612：count=0。
func (s *Server) handleGetPetKingRewards(c *Client, uid uint32, body []byte) {
	s.handleRemainU32Ack(c, uid, 2612, 0)
}

// handleExchangeMasterCards CMD 2621。
func (s *Server) handleExchangeMasterCards(c *Client, uid uint32, body []byte) {
	s.handleRemainEmptyAck(c, uid, 2621)
}

// handleAwardCode CMD 3301。
func (s *Server) handleAwardCode(c *Client, uid uint32, body []byte) {
	s.handleRemainU32Ack(c, uid, 3301, 0)
}

// handleNoteTransformUser CMD 2107：广播变身（同 2111 布局）。
func (s *Server) handleNoteTransformUser(c *Client, uid uint32, body []byte) {
	suit := uint32(0)
	if len(body) >= 4 {
		suit = binary.BigEndian.Uint32(body[0:4])
	}
	out := make([]byte, 8)
	binary.BigEndian.PutUint32(out[0:4], uid)
	binary.BigEndian.PutUint32(out[4:8], suit)
	s.send(c, 2107, uid, 0, out)
	s.broadcastToMap(c, 2107, out)
	log.Printf("[CMD] OK     %s UID=%d suit=%d", cmdname.Format(2107), uid, suit)
}

// handleUseFightHPItem CMD 2342：战斗用药走 2406；此处空 ACK。
func (s *Server) handleUseFightHPItem(c *Client, uid uint32, body []byte) {
	s.handleRemainEmptyAck(c, uid, 2342)
}

// handleSendEggToFriend CMD 2373。
func (s *Server) handleSendEggToFriend(c *Client, uid uint32, body []byte) {
	s.handleRemainEmptyAck(c, uid, 2373)
}

// handleUpgradeMedicine CMD 2377。
func (s *Server) handleUpgradeMedicine(c *Client, uid uint32, body []byte) {
	s.handleRemainEmptyAck(c, uid, 2377)
}

// handleExchangeOre CMD 2251。
func (s *Server) handleExchangeOre(c *Client, uid uint32, body []byte) {
	s.handleRemainEmptyAck(c, uid, 2251)
}

// handleIcicleGame CMD 2120/2121。
func (s *Server) handleIcicleGame(c *Client, uid uint32, cmd int32, body []byte) {
	if cmd == 2120 {
		s.handleRemainZeroN(c, uid, cmd, 8)
		return
	}
	s.handleRemainEmptyAck(c, uid, cmd)
}

// handleGetTimePoke CMD 2110。
func (s *Server) handleGetTimePoke(c *Client, uid uint32, body []byte) {
	s.handleRemainU32Ack(c, uid, 2110, uint32(time.Now().Unix()))
}

// handleAttackBailuen CMD 2109。
func (s *Server) handleAttackBailuen(c *Client, uid uint32, body []byte) {
	s.handleRemainEmptyAck(c, uid, 2109)
}

// handleNoteStatus CMD 2108。
func (s *Server) handleNoteStatus(c *Client, uid uint32, body []byte) {
	s.handleRemainEmptyAck(c, uid, 2108)
}

// handleUserTimePassword CMD 2821。
func (s *Server) handleUserTimePassword(c *Client, uid uint32, body []byte) {
	s.handleRemainU32Ack(c, uid, 2821, 1)
}

// handleDSStatus CMD 2851/2852。
func (s *Server) handleDSStatus(c *Client, uid uint32, cmd int32, body []byte) {
	s.handleRemainU32Ack(c, uid, cmd, 0)
}

// handleBuersiguangFix CMD 4140。
func (s *Server) handleBuersiguangFix(c *Client, uid uint32, body []byte) {
	s.handleRemainEmptyAck(c, uid, 4140)
}

// handleLeaveGame CMD 5003。
func (s *Server) handleLeaveGame(c *Client, uid uint32, body []byte) {
	s.handleRemainEmptyAck(c, uid, 5003)
}

// handleFBGameOver CMD 5052：8B 零。
func (s *Server) handleFBGameOver(c *Client, uid uint32, body []byte) {
	s.handleRemainZeroN(c, uid, 5052, 8)
}

// handleWorkConnection CMD 6001/6003。
func (s *Server) handleWorkConnection(c *Client, uid uint32, cmd int32, body []byte) {
	s.handleRemainEmptyAck(c, uid, cmd)
}

// handleComplainUser CMD 7001/7002/7003。
func (s *Server) handleComplainUser(c *Client, uid uint32, cmd int32, body []byte) {
	s.handleRemainEmptyAck(c, uid, cmd)
}

// handleFireEdgeBreedConvert CMD 9145。
func (s *Server) handleFireEdgeBreedConvert(c *Client, uid uint32, body []byte) {
	s.handleRemainEmptyAck(c, uid, 9145)
}

// handleBreedIntroMovieGift CMD 9222。
func (s *Server) handleBreedIntroMovieGift(c *Client, uid uint32, body []byte) {
	if s.cfg.Store != nil {
		_, _ = s.cfg.Store.AddExpPool(int64(uid), 500)
	}
	s.handleRemainEmptyAck(c, uid, 9222)
}

// handleAresUnionTrain CMD 9388/9394。
func (s *Server) handleAresUnionTrain(c *Client, uid uint32, cmd int32, body []byte) {
	s.handleRemainZeroN(c, uid, cmd, 8)
}

// handleTestCMD CMD 30000。
func (s *Server) handleTestCMD(c *Client, uid uint32, body []byte) {
	s.handleRemainEmptyAck(c, uid, 30000)
}

// handlePvpDeleveling CMD 70008。
func (s *Server) handlePvpDeleveling(c *Client, uid uint32, body []byte) {
	s.handleRemainEmptyAck(c, uid, 70008)
}

// handlePlayVideo CMD 80011。
func (s *Server) handlePlayVideo(c *Client, uid uint32, body []byte) {
	s.handleRemainEmptyAck(c, uid, 80011)
}

// handleSukeExchange CMD 80012。
func (s *Server) handleSukeExchange(c *Client, uid uint32, body []byte) {
	s.handleRemainEmptyAck(c, uid, 80012)
}
