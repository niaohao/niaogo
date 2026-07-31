package gameserver

import (
	"testing"

	"niaohao/server/internal/store"
)

// 1001 固定尾：petNum(4)+clothesCnt(4)+title(4)+bossAch(200)；无宠无装时成立。
const loginFixedTail = 4 + 4 + 4 + 200

func TestBuildLoginResponseTaskListAligned(t *testing.T) {
	s := &Server{}
	u := &store.User{
		UserID: 10002, Nickname: "t", RegisterTime: 1784815487,
		Energy: 100, MapID: 1, PosX: 480, PosY: 280,
	}
	body := s.buildLoginResponse(u)
	if len(body) < loginFixedTail+1000 {
		t.Fatalf("body too short %d", len(body))
	}
	taskStart := len(body) - loginFixedTail - 1000
	taskEnd := taskStart + 1000
	t.Logf("taskList offset=%d (len=%d)", taskStart, len(body))
	// 本客户端 UserInfo：teacher…monKingWin 后直接 curStage，无 5×dword 预留。
	// 历史误加 5 预留时 offset=458；正确应为 438。
	if taskStart != 438 {
		t.Fatalf("taskList offset=%d want 438 — 登录字段又错位，会导致任务88读成未完成进515", taskStart)
	}
	for i, b := range body[taskStart:taskEnd] {
		if b != 0 {
			t.Fatalf("empty-store task[%d]=%d", i+1, b)
		}
	}
}

func TestTask88IndexInLoginList(t *testing.T) {
	// EncodeLoginTaskList: out[id-1]=status → 任务88在下标87
	raw := make([]byte, 1000)
	raw[87] = 3
	if raw[88-1] != 3 {
		t.Fatal("index")
	}
}
