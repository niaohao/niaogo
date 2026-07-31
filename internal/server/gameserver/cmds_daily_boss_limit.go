package gameserver

// markDailyBossChallengeLimit：MapProcess_305/306 用 2701 cate 200013/200014 作「捕捉/挑战日限」。
// 客户端 count!=0 则拒绝开战；战胜后 +1。
func (s *Server) markDailyBossChallengeLimit(uid uint32, st *BattleState) {
	if st == nil || st.isPvP() {
		return
	}
	var cate uint32
	switch st.MapID {
	case 305:
		cate = 200013 // 700013-500000
	case 306, 307:
		cate = 200014 // 700014-500000
	default:
		return
	}
	key := "talk:" + itoaU32(cate)
	if s.dailyCount(int64(uid), key) == 0 {
		s.bumpDaily(int64(uid), key)
	}
}
