package gameserver

import (
	"encoding/binary"
	"testing"

	"niaohao/server/internal/tableloader"
)

func TestHandleAchieveCurrentBodyLen72(t *testing.T) {
	// AchieveNewPanel.onGetCurrent：4 tip + 7×(a,b) = 18×u32 = 72B；短包 #2030
	cat := &tableloader.Catalog{
		AchieveByBranch: map[int]*tableloader.AchieveBranch{
			2: {
				ID: 2,
				Rules: []tableloader.AchieveRule{
					{BranchID: 2, RuleID: 1, AchievementPoint: 10},
					{BranchID: 2, RuleID: 2, AchievementPoint: 20},
				},
			},
		},
		AchieveBranchOrder: []int{2},
	}
	s := &Server{cfg: Config{Catalog: cat}}
	tips := s.pickAchieveTips(0, 2)
	if len(tips) != 2 || tips[0] != [2]int{2, 1} || tips[1] != [2]int{2, 2} {
		t.Fatalf("tips=%v", tips)
	}
	out := make([]byte, 72)
	for i, tip := range tips {
		binary.BigEndian.PutUint32(out[i*8:i*8+4], uint32(tip[0]))
		binary.BigEndian.PutUint32(out[i*8+4:i*8+8], uint32(tip[1]))
	}
	points, completed := s.achieveCurrentTotals(0)
	binary.BigEndian.PutUint32(out[16:20], uint32(points))
	binary.BigEndian.PutUint32(out[20:24], uint32(completed))
	if len(out) != 72 {
		t.Fatalf("len=%d", len(out))
	}
	if binary.BigEndian.Uint32(out[0:4]) != 2 || binary.BigEndian.Uint32(out[4:8]) != 1 {
		t.Fatalf("out tip0 %v", out[:8])
	}
}