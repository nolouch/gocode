package agent

import (
	"testing"

	"github.com/nolouch/gcode/internal/permission"
)

func TestPlanPermissions_DefaultReadOnly(t *testing.T) {
	reg := NewRegistry()
	plan := reg.Get("plan")

	if permission.ToolAllowed("write", plan.Permissions) {
		t.Fatal("plan should deny write by default")
	}
	if !permission.ToolAllowed("read", plan.Permissions) {
		t.Fatal("plan should allow read by default")
	}
	if !permission.ToolAllowed("bash", plan.Permissions) {
		t.Fatal("plan should allow bash by default")
	}
}
