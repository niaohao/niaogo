package tableloader

import (
	"path/filepath"
	"testing"
)

func TestGrantablePetIDsUsesFrontendFilter(t *testing.T) {
	xmlDir := filepath.Join("..", "..", "tables", "xml")
	c := New(xmlDir)
	if err := c.Load(); err != nil {
		t.Fatalf("load: %v", err)
	}
	all := c.AllPetIDs()
	grant := c.GrantablePetIDs()
	if len(c.FrontendPetName) == 0 {
		t.Fatal("expected PetXMLInfo.bin loaded")
	}
	if len(grant) == 0 {
		t.Fatal("grantable empty")
	}
	if len(grant) >= len(all) {
		t.Fatalf("grantable=%d should be fewer than pets.xml all=%d", len(grant), len(all))
	}
	// 3434 在扩大版 pets.xml，不在前端 PetXMLInfo → 不可发放
	for _, id := range grant {
		if id == 3434 {
			t.Fatal("3434 should not be grantable (not in frontend PetXMLInfo)")
		}
		if c.FrontendPetNameOf(id) == "" {
			t.Fatalf("grantable id=%d missing frontend name", id)
		}
		if c.PetBase(id) == nil {
			t.Fatalf("grantable id=%d missing pets.xml base", id)
		}
	}
	// 布布种子应可发
	found1 := false
	for _, id := range grant {
		if id == 1 {
			found1 = true
			break
		}
	}
	if !found1 {
		t.Fatal("pet 1 should be grantable")
	}
	t.Logf("frontend=%d grantable=%d pets.xml=%d", len(c.FrontendPetName), len(grant), len(all))
}
