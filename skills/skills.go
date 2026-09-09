// Package skills embeds the agent skill so that `transpareo setup`
// can install it without a network fetch.
package skills

import _ "embed"

// Transpareo is the content of skills/transpareo/SKILL.md.
//
//go:embed transpareo/SKILL.md
var Transpareo []byte

// Plugin is the plugin manifest beside the skill.
//
//go:embed transpareo/.claude-plugin/plugin.json
var Plugin []byte
