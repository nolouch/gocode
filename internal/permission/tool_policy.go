package permission

import "strings"

func AuthorizeTool(agentName, toolName string, args map[string]any, rules Ruleset) (bool, string) {
	if !ToolAllowed(toolName, rules) {
		return true, "tool \"" + toolName + "\" is disabled by permission rules"
	}

	name := strings.ToLower(strings.TrimSpace(toolName))
	agent := strings.ToLower(strings.TrimSpace(agentName))

	if len(rules) == 0 && (agent == "plan" || agent == "explore") {
		switch name {
		case "write", "write_file", "edit", "apply_patch", "todo_write":
			return true, "tool \"" + toolName + "\" is disabled in " + agent + " mode"
		}
	}

	if name == "bash" {
		cmd, _ := args["command"].(string)
		if cmd != "" && (agent == "plan" || agent == "explore") && IsMutatingShellCommand(cmd) {
			return true, "bash command rejected in " + agent + " mode: mutating command"
		}
		if cmd != "" && IsDangerousShellCommand(cmd) {
			return true, "bash command rejected: dangerous operation"
		}
	}

	return false, ""
}

func IsMutatingShellCommand(cmd string) bool {
	s := strings.ToLower(cmd)
	hints := []string{
		" rm ", " mv ", " cp ", " chmod ", " chown ", " mkdir ", " rmdir ",
		" touch ", " tee ", " sed -i", " perl -i", " git add", " git commit",
		" git push", " npm install", " pnpm install", " yarn add", " gofmt",
		">", " >>",
	}
	padded := " " + s + " "
	for _, hint := range hints {
		if strings.Contains(padded, hint) {
			return true
		}
	}
	return false
}

func IsDangerousShellCommand(cmd string) bool {
	s := strings.ToLower(cmd)
	dangerous := []string{
		"rm -rf /",
		"mkfs",
		"shutdown",
		"reboot",
		"poweroff",
		":(){ :|:& };:",
	}
	for _, pattern := range dangerous {
		if strings.Contains(s, pattern) {
			return true
		}
	}
	return false
}
