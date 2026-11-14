// SPDX-License-Identifier: MIT
// Copyright (c) 2025 The OpenConductor Authors.

package persona

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/openconductorhq/openconductor/internal/config"
	"github.com/openconductorhq/openconductor/internal/fsutil"
	"github.com/openconductorhq/openconductor/internal/logging"
)

const mcpPrefix = "openconductor:"

// claudeMCP defines an MCP server entry for Claude Code's .mcp.json format.
type claudeMCP struct {
	Command string   `json:"command"`
	Args    []string `json:"args"`
}

// opencodeMCP defines an MCP server entry for OpenCode's opencode.json format.
type opencodeMCP struct {
	Type    string   `json:"type"`
	Command []string `json:"command"`
}

// claudeMCPBundles maps each persona to the MCP servers installed for Claude Code.
var claudeMCPBundles = map[config.PersonaType]map[string]claudeMCP{
	config.PersonaVibe: {},
	config.PersonaPOC: {
		"context7":   {Command: "npx", Args: []string{"-y", "@upstash/context7-mcp"}},
		"playwright": {Command: "npx", Args: []string{"@playwright/mcp@latest"}},
	},
	config.PersonaScale: {
		"context7":            {Command: "npx", Args: []string{"-y", "@upstash/context7-mcp"}},
		"playwright":          {Command: "npx", Args: []string{"@playwright/mcp@latest"}},
		"sequential-thinking": {Command: "npx", Args: []string{"-y", "@anthropic/sequential-thinking-mcp"}},
	},
}

// opencodeMCPBundles maps each persona to the MCP servers installed for OpenCode.
var opencodeMCPBundles = map[config.PersonaType]map[string]opencodeMCP{
	config.PersonaVibe: {},
	config.PersonaPOC: {
		"context7":   {Type: "local", Command: []string{"npx", "-y", "@upstash/context7-mcp"}},
		"playwright": {Type: "local", Command: []string{"npx", "@playwright/mcp@latest"}},
	},
	config.PersonaScale: {
		"context7":            {Type: "local", Command: []string{"npx", "-y", "@upstash/context7-mcp"}},
		"playwright":          {Type: "local", Command: []string{"npx", "@playwright/mcp@latest"}},
		"sequential-thinking": {Type: "local", Command: []string{"npx", "-y", "@anthropic/sequential-thinking-mcp"}},
	},
}

// pluginBundles maps each persona to the Claude Code plugins that should be
// enabled in .claude/settings.json.
var pluginBundles = map[config.PersonaType][]string{
	config.PersonaVibe: {},
	config.PersonaPOC: {
		"context7@claude-plugins-official",
		"playwright@claude-plugins-official",
	},
	config.PersonaScale: {
		"context7@claude-plugins-official",
		"playwright@claude-plugins-official",
	},
}

// skillDef defines a single skill file to be written under .claude/skills/.
type skillDef struct {
	Name    string
	Content string
}

// skillBundles maps each persona to the skills installed for Claude Code.
var skillBundles = map[config.PersonaType][]skillDef{
	config.PersonaScale: {
		{
			Name: "openconductor-tdd",
			Content: `---
name: openconductor-tdd
description: Enforces test-driven development workflow. Use before writing any implementation code.
---

# TDD Workflow

Follow the Red-Green-Refactor cycle for every change:

1. **Red**: Write a failing test that describes the desired behavior
2. **Run**: Execute the test to confirm it fails for the right reason
3. **Green**: Write the minimum code to make the test pass
4. **Run**: Execute the test to confirm it passes
5. **Refactor**: Clean up the code while keeping tests green

Never skip steps. Never write implementation before the test.
`,
		},
		{
			Name: "openconductor-review",
			Content: `---
name: openconductor-review
description: Self-review checklist before committing changes. Use after implementation is complete.
---

# Pre-Commit Review

Before committing, verify each item:

- [ ] No security vulnerabilities (injection, XSS, hardcoded secrets)
- [ ] All error paths handled with meaningful messages
- [ ] Edge cases covered in tests
- [ ] No breaking API changes without migration path
- [ ] Public APIs have documentation
- [ ] No TODO/FIXME left as excuses for incomplete work
- [ ] Commit message explains the why, not just the what
`,
		},
	},
}

// WritePersonaBundle writes all persona artifacts for a project: instructions
// (CLAUDE.md or opencode.json instructions), MCPs, skills, and plugins.
// This is the single function called from the TUI after persona selection.
func WritePersonaBundle(
	repoPath string,
	agentType config.AgentType,
	persona config.PersonaType,
	customPersonas []config.CustomPersona,
) error {
	switch agentType {
	case config.AgentClaudeCode:
		if err := WritePersonaSection(repoPath, agentType, persona, customPersonas); err != nil {
			return fmt.Errorf("writing instructions: %w", err)
		}
		if err := writeClaudeMCPs(repoPath, persona); err != nil {
			return fmt.Errorf("writing MCPs: %w", err)
		}
		if err := writeSkills(repoPath, persona); err != nil {
			return fmt.Errorf("writing skills: %w", err)
		}
		if err := writePlugins(repoPath, persona); err != nil {
			return fmt.Errorf("writing plugins: %w", err)
		}
	case config.AgentOpenCode:
		if err := writeOpenCodeInstructions(repoPath, persona, customPersonas); err != nil {
			return fmt.Errorf("writing instructions: %w", err)
		}
		if err := writeOpenCodeMCPs(repoPath, persona); err != nil {
			return fmt.Errorf("writing MCPs: %w", err)
		}
	}
	return nil
}

// writeClaudeMCPs merges persona MCP servers into the project's .mcp.json.
// Existing non-openconductor entries are preserved.
func writeClaudeMCPs(repoPath string, persona config.PersonaType) error {
	filePath := filepath.Join(repoPath, ".mcp.json")

	doc, err := readJSONDoc(filePath)
	if err != nil {
		return err
	}

	// Ensure mcpServers exists.
	servers, ok := doc["mcpServers"].(map[string]interface{})
	if !ok {
		servers = map[string]interface{}{}
		doc["mcpServers"] = servers
	}

	// Remove all openconductor: prefixed keys.
	for key := range servers {
		if strings.HasPrefix(key, mcpPrefix) {
			delete(servers, key)
		}
	}

	// Add new persona MCPs.
	bundle, ok := claudeMCPBundles[persona]
	if ok {
		for name, mcp := range bundle {
			servers[mcpPrefix+name] = mcp
		}
	}

	logging.Info("persona: wrote MCPs", "file", filePath, "persona", string(persona))
	return writeJSONDoc(filePath, doc)
}

// writeOpenCodeMCPs merges persona MCP servers into the project's opencode.json.
// Existing non-openconductor entries are preserved.
func writeOpenCodeMCPs(repoPath string, persona config.PersonaType) error {
	filePath := filepath.Join(repoPath, "opencode.json")

	doc, err := readJSONDoc(filePath)
	if err != nil {
		return err
	}

	// Ensure mcp key exists.
	mcpSection, ok := doc["mcp"].(map[string]interface{})
	if !ok {
		mcpSection = map[string]interface{}{}
		doc["mcp"] = mcpSection
	}

	// Remove openconductor: prefixed keys.
	for key := range mcpSection {
		if strings.HasPrefix(key, mcpPrefix) {
			delete(mcpSection, key)
		}
	}

	// Add new persona MCPs.
	bundle, ok := opencodeMCPBundles[persona]
	if ok {
		for name, mcp := range bundle {
			mcpSection[mcpPrefix+name] = mcp
		}
	}

	logging.Info("persona: wrote MCPs", "file", filePath, "persona", string(persona))
	return writeJSONDoc(filePath, doc)
}

// writeSkills manages skill directories under .claude/skills/ for Claude Code.
// Existing openconductor-* skill directories are removed before writing the
// new persona's skills.
func writeSkills(repoPath string, persona config.PersonaType) error {
	skillsDir := filepath.Join(repoPath, ".claude", "skills")

	// Remove all existing openconductor-* skill directories.
	entries, _ := os.ReadDir(skillsDir)
	for _, e := range entries {
		if e.IsDir() && strings.HasPrefix(e.Name(), "openconductor-") {
			os.RemoveAll(filepath.Join(skillsDir, e.Name()))
		}
	}

	// Add new persona skills.
	skills, ok := skillBundles[persona]
	if !ok || len(skills) == 0 {
		return nil
	}

	for _, skill := range skills {
		dir := filepath.Join(skillsDir, skill.Name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("creating skill dir %s: %w", skill.Name, err)
		}
		skillFile := filepath.Join(dir, "SKILL.md")
		if err := fsutil.AtomicWrite(skillFile, []byte(skill.Content), 0o644); err != nil {
			return fmt.Errorf("writing skill %s: %w", skill.Name, err)
		}
		logging.Info("persona: wrote skill", "skill", skill.Name)
	}

	return nil
}

// writePlugins merges persona plugin enablement into .claude/settings.json.
// Only plugins known to any persona bundle are managed; user-added plugins
// are left untouched.
func writePlugins(repoPath string, persona config.PersonaType) error {
	settingsDir := filepath.Join(repoPath, ".claude")
	settingsPath := filepath.Join(settingsDir, "settings.json")

	doc, err := readJSONDoc(settingsPath)
	if err != nil {
		return err
	}

	// Ensure enabledPlugins exists.
	plugins, ok := doc["enabledPlugins"].(map[string]interface{})
	if !ok {
		plugins = map[string]interface{}{}
	}

	// Determine which plugins are managed by persona bundles. Remove all of
	// them, then add back the ones for the new persona.
	allManagedPlugins := map[string]bool{}
	for _, bundle := range pluginBundles {
		for _, p := range bundle {
			allManagedPlugins[p] = true
		}
	}
	for p := range allManagedPlugins {
		delete(plugins, p)
	}

	// Add new persona plugins.
	bundle, ok := pluginBundles[persona]
	if ok {
		for _, p := range bundle {
			plugins[p] = true
		}
	}

	if len(plugins) > 0 {
		doc["enabledPlugins"] = plugins
	} else {
		delete(doc, "enabledPlugins")
	}

	// Don't create an empty settings file.
	if len(doc) == 0 {
		return nil
	}

	if err := os.MkdirAll(settingsDir, 0o755); err != nil {
		return fmt.Errorf("creating .claude dir: %w", err)
	}

	logging.Info("persona: wrote plugins", "file", settingsPath, "persona", string(persona))
	return writeJSONDoc(settingsPath, doc)
}

// writeOpenCodeInstructions writes persona instructions to a standalone
// .openconductor-persona.md file and registers it in opencode.json's
// instructions array.
func writeOpenCodeInstructions(repoPath string, persona config.PersonaType, customPersonas []config.CustomPersona) error {
	personaFile := filepath.Join(repoPath, ".openconductor-persona.md")
	configFile := filepath.Join(repoPath, "opencode.json")

	// Resolve instruction text.
	var personaText string
	if persona != config.PersonaNone {
		result := Resolve(persona, customPersonas)
		if !result.Found {
			return fmt.Errorf("unknown persona %q", persona)
		}
		personaText = result.Instructions
	}

	if personaText == "" {
		// Remove persona file and its reference from opencode.json.
		os.Remove(personaFile)
		removeOpenCodeInstruction(configFile, ".openconductor-persona.md")
		return nil
	}

	// Write persona instruction file.
	if err := fsutil.AtomicWrite(personaFile, []byte(personaText+"\n"), 0o644); err != nil {
		return err
	}

	// Add to opencode.json instructions array.
	return addOpenCodeInstruction(configFile, ".openconductor-persona.md")
}

// addOpenCodeInstruction ensures instrFile is present in the opencode.json
// instructions array. Creates the file if it does not exist.
func addOpenCodeInstruction(configFile string, instrFile string) error {
	doc, err := readJSONDoc(configFile)
	if err != nil {
		return err
	}

	// Get or create instructions array.
	var instructions []interface{}
	if existing, ok := doc["instructions"]; ok {
		if arr, ok := existing.([]interface{}); ok {
			instructions = arr
		}
	}

	// Check if already present.
	for _, v := range instructions {
		if s, ok := v.(string); ok && s == instrFile {
			return nil // already there
		}
	}

	instructions = append(instructions, instrFile)
	doc["instructions"] = instructions

	return writeJSONDoc(configFile, doc)
}

// removeOpenCodeInstruction removes instrFile from the opencode.json
// instructions array. Does nothing if the config file does not exist or
// the entry is not present.
func removeOpenCodeInstruction(configFile string, instrFile string) error {
	doc, err := readJSONDoc(configFile)
	if err != nil {
		return nil // unreadable config: nothing we can safely remove from
	}

	existing, ok := doc["instructions"]
	if !ok {
		return nil
	}
	arr, ok := existing.([]interface{})
	if !ok {
		return nil
	}

	// Filter out the instruction.
	var filtered []interface{}
	for _, v := range arr {
		if s, ok := v.(string); ok && s == instrFile {
			continue
		}
		filtered = append(filtered, v)
	}

	if len(filtered) == 0 {
		delete(doc, "instructions")
	} else {
		doc["instructions"] = filtered
	}

	return writeJSONDoc(configFile, doc)
}

// readJSONDoc reads a JSON object from path. A missing file yields an empty
// document so callers can merge into it.
func readJSONDoc(path string) (map[string]interface{}, error) {
	doc := map[string]interface{}{}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return doc, nil
		}
		return nil, fmt.Errorf("reading %s: %w", filepath.Base(path), err)
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", filepath.Base(path), err)
	}
	if doc == nil { // file contained JSON null
		doc = map[string]interface{}{}
	}
	return doc, nil
}

// writeJSONDoc writes doc to path as indented JSON, atomically.
func writeJSONDoc(path string, doc map[string]interface{}) error {
	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling %s: %w", filepath.Base(path), err)
	}
	return fsutil.AtomicWrite(path, append(out, '\n'), 0o644)
}
