package tableloader

import (
	"path/filepath"
	"testing"
)

func TestLoadEggsAndGender(t *testing.T) {
	dir := filepath.Join("..", "..", "tables", "xml")
	c := New(dir)
	if err := c.Load(); err != nil {
		t.Fatal(err)
	}
	if c.BreedEggID(450, 447) != 1 {
		t.Fatalf("egg1 got %d", c.BreedEggID(450, 447))
	}
	if c.BreedEggID(962, 965) != 8 {
		t.Fatalf("egg8 got %d", c.BreedEggID(962, 965))
	}
	if id := c.EggOutputPetID(1); id != 445 && id != 448 && id != 512 {
		t.Fatalf("output %d", id)
	}
	// 伊优 Gender=1 雄；布布种子 Gender=2 雌
	if g := c.PetGender(4); g != 1 {
		t.Fatalf("pet4 gender=%d", g)
	}
	if g := c.PetGender(1); g != 2 {
		t.Fatalf("pet1 gender=%d", g)
	}
}
