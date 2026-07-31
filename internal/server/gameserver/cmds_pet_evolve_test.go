package gameserver

import (
	"path/filepath"
	"testing"

	"niaohao/server/internal/tableloader"
)

func TestEvolveBranchesLoad(t *testing.T) {
	xmlDir := filepath.Join("..", "..", "..", "tables", "xml")
	if err := tableloader.LoadEvolveXML(filepath.Join(xmlDir, "EvolveXMLInfo.xml")); err != nil {
		t.Fatal(err)
	}
	br, ok := tableloader.EvolveBranches(7) // 悠悠
	if !ok || len(br) < 3 {
		t.Fatalf("flag7 branches=%v ok=%v", br, ok)
	}
	if br[0].MonTo != 92 || br[0].EvolvItem != 400004 {
		t.Fatalf("branch0=%+v", br[0])
	}
}

func TestEssenceBreedLoad(t *testing.T) {
	xmlDir := filepath.Join("..", "..", "..", "tables", "xml")
	cat := tableloader.New(xmlDir)
	if err := cat.Load(); err != nil {
		t.Fatal(err)
	}
	eb, ok := cat.EssenceBreedOf(400107)
	if !ok || eb.BreedMonID != 41 {
		t.Fatalf("essence 400107=%+v ok=%v", eb, ok)
	}
	base := cat.EvolutionBaseForm(42) // 里奥斯 -> 胡里亚 41
	if base != 41 {
		t.Fatalf("base of 42 want 41 got %d", base)
	}
}
