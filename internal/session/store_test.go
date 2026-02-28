package session

import "testing"

func TestStoreSessionParentAndChildren(t *testing.T) {
	s := NewStore()
	parent := s.CreateSession("/repo")
	child := s.CreateSession("/repo")

	s.SetSessionParent(child.ID, parent.ID)

	got, err := s.GetSession(child.ID)
	if err != nil {
		t.Fatalf("get child session: %v", err)
	}
	if got.ParentID != parent.ID {
		t.Fatalf("parent id = %q, want %q", got.ParentID, parent.ID)
	}

	children := s.Children(parent.ID)
	if len(children) != 1 {
		t.Fatalf("children count = %d, want 1", len(children))
	}
	if children[0].ID != child.ID {
		t.Fatalf("child id = %q, want %q", children[0].ID, child.ID)
	}
}
