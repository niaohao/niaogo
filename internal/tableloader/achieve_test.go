package tableloader

import (
	"path/filepath"
	"testing"
)

func TestLoadAchieveXML(t *testing.T) {
	dir := filepath.Join("..", "..", "tables", "xml")
	c := New(dir)
	if err := c.loadAchieve(filepath.Join(dir, "AchieveXMLInfo.xml")); err != nil {
		t.Fatal(err)
	}
	// 本客户端 XML 含注释掉的 Branch；有效约 14 支 / 100+ Rule
	if len(c.AchieveByBranch) < 10 {
		t.Fatalf("branches=%d", len(c.AchieveByBranch))
	}
	br := c.AchieveBranchOf(2)
	if br == nil || len(br.Rules) < 10 {
		t.Fatalf("branch2=%v", br)
	}
	r, ok := c.AchieveRuleOf(2, 1)
	if !ok || r.Threshold != 301 {
		t.Fatalf("rule2-1=%v ok=%v", r, ok)
	}
}
