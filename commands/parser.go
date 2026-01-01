package commands

import (
	"strings"
)

// CommandPrefixes defines all supported prefixes
var CommandPrefixes = []string{".", "!", "-", "/"}

// ParseCommand extracts command name and args from message text
// Supports multiple prefixes: ., !, -, / and no prefix
// Returns (commandName, args, isCommand)
func ParseCommand(text string) (string, string, bool) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "", "", false
	}

	// Check with prefixes first
	for _, prefix := range CommandPrefixes {
		if strings.HasPrefix(text, prefix) {
			content := strings.TrimPrefix(text, prefix)
			parts := strings.Fields(content)
			if len(parts) == 0 {
				return "", "", false
			}

			cmd := strings.ToLower(parts[0])
			args := ""
			if len(parts) > 1 {
				args = strings.TrimSpace(strings.Join(parts[1:], " "))
			}
			return cmd, args, true
		}
	}

	// No prefix - check if it's a known command
	parts := strings.Fields(text)
	if len(parts) == 0 {
		return "", "", false
	}

	cmd := strings.ToLower(parts[0])
	args := ""
	if len(parts) > 1 {
		args = strings.TrimSpace(strings.Join(parts[1:], " "))
	}

	// Only allow no-prefix for specific commands (shorter, common ones)
	allowedNoPrefixCmds := map[string]bool{
		"bot":        true,
		"menu":       true,
		"info":       true,
		"ping":       true,
		"status":     true,
		"online":     true,
		"typing":     true,
		"record":     true,
		"readstory":  true,
		"likestory":  true,
		"storydelay": true,
		"jadibot":    true,
		"listjadibot": true,
		"deljadibot": true,
	}

	if allowedNoPrefixCmds[cmd] {
		return cmd, args, true
	}

	return "", "", false
}

// BuildCommandString creates normalized command string for comparison
// Handles commands with arguments (e.g., "online on" -> checks both cmd and args)
func BuildCommandString(cmd, args string) string {
	if args == "" {
		return cmd
	}
	return cmd + " " + args
}
