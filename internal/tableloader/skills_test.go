package tableloader_test

import (
	"path/filepath"
	"testing"

	"niaohao/server/internal/tableloader"
)

func TestLoadSkills(t *testing.T) {
	dir := filepath.Join("..", "..", "tables", "xml")
	c := tableloader.New(dir)
	if err := c.Load(); err != nil {
		t.Fatal(err)
	}
	_, _, sk, _ := c.StatsFull()
	if sk < 100 {
		t.Fatalf("skills=%d want >=100", sk)
	}
	s := c.Skill(10001)
	if s == nil || s.Power != 35 {
		t.Fatalf("10001=%v", s)
	}
}
