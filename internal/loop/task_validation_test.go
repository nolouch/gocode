package loop

import (
	"testing"

	"github.com/nolouch/opengocode/internal/model"
)

func TestValidateTaskTargetSession(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		err := validateTaskTargetSession("sess-main", "/repo", &model.Session{ID: "sess-sub", Directory: "/repo", ParentID: "sess-main"})
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("same session", func(t *testing.T) {
		err := validateTaskTargetSession("sess-main", "/repo", &model.Session{ID: "sess-main", Directory: "/repo"})
		if err == nil {
			t.Fatal("expected error for same session")
		}
	})

	t.Run("different dir", func(t *testing.T) {
		err := validateTaskTargetSession("sess-main", "/repo", &model.Session{ID: "sess-sub", Directory: "/other"})
		if err == nil {
			t.Fatal("expected error for different directory")
		}
	})

	t.Run("different parent", func(t *testing.T) {
		err := validateTaskTargetSession("sess-main", "/repo", &model.Session{ID: "sess-sub", Directory: "/repo", ParentID: "sess-other"})
		if err == nil {
			t.Fatal("expected error for different parent")
		}
	})
}
