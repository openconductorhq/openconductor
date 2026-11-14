#!/usr/bin/env bash
# SPDX-License-Identifier: MIT
# Copyright (c) 2025 The OpenConductor Authors.
#
# Drive a real openconductor + real agents inside a detached tmux session,
# sandboxed from your own config. Only the agents' login credentials are
# linked in: no plugins, hooks, MCP servers or channels from your setup run.
#
#   scripts/e2e.sh start [config.yaml]  build, sandbox, launch (default config: one
#                                        claude-code and one opencode project)
#   scripts/e2e.sh keys <tmux keys...>   send keys, e.g.: keys 'hello' Enter
#   scripts/e2e.sh screen                print the visible screen
#   scripts/e2e.sh wait <regex> [secs]   wait until the screen matches
#   scripts/e2e.sh stop                  kill the session and delete the sandbox
#
# Project repos are $E2E_DIR/repos/<name>; config.yaml may use $E2E_DIR.
# E2E_LLM=1 points openconductor's L2 classifier at the mock (MOCK_DELAY=secs
# makes it slow, to check the UI stays responsive).
set -euo pipefail

E2E_DIR=${E2E_DIR:-${TMPDIR:-/tmp}/openconductor-e2e}
SESSION=oc-e2e
MOCK_PORT=${MOCK_PORT:-18765}
ROOT=$(cd "$(dirname "$0")/.." && pwd)

start() {
	[ -e "$E2E_DIR" ] && { echo "sandbox $E2E_DIR exists; run stop first" >&2; exit 1; }
	local home=$E2E_DIR/home
	mkdir -p "$home/.claude" "$home/.local/share/opencode" "$home/.config/opencode" \
		"$home/.openconductor" "$E2E_DIR/repos"

	ln -s "$HOME/.claude/.credentials.json" "$home/.claude/.credentials.json"
	ln -s "$HOME/.local/share/opencode/auth.json" "$home/.local/share/opencode/auth.json"
	# OpenCode talks to a local scripted model (scripts/e2e-mock-llm.py): no
	# tokens, no account, deterministic tool calls. It asks before shell
	# commands and edits so auto-approve has dialogs to act on.
	cat >"$home/.config/opencode/opencode.json" <<EOF
{"\$schema":"https://opencode.ai/config.json",
 "provider":{"mock":{"npm":"@ai-sdk/openai-compatible","name":"Mock",
   "options":{"baseURL":"http://127.0.0.1:$MOCK_PORT/v1","apiKey":"mock"},
   "models":{"m":{"name":"Mock"}}}},
 "model":"mock/m","small_model":"mock/m",
 "permission":{"bash":"ask","edit":"ask","webfetch":"ask","external_directory":"ask",
   "list":"ask","glob":"ask","grep":"ask"}}
EOF
	MOCK_DELAY=${MOCK_DELAY:-0} python3 "$ROOT/scripts/e2e-mock-llm.py" "$MOCK_PORT" &
	echo $! >"$E2E_DIR/mock.pid"

	if [ -n "${1:-}" ]; then
		E2E_DIR=$E2E_DIR envsubst <"$1" >"$home/.openconductor/config.yaml"
	else
		cat >"$home/.openconductor/config.yaml" <<EOF
projects:
  - name: cc
    repo: $E2E_DIR/repos/cc
    agent: claude-code
    auto_approve: safe
  - name: oc
    repo: $E2E_DIR/repos/oc
    agent: opencode
    auto_approve: safe
notifications:
  enabled: false
EOF
		[ -n "${E2E_LLM:-}" ] && cat >>"$home/.openconductor/config.yaml" <<EOF
llm:
  provider: openai
  model: mock
  api_key_env: OPENAI_API_KEY
  base_url: http://127.0.0.1:$MOCK_PORT/v1
EOF
	fi

	# Create every repo the config names, and pre-trust it for Claude Code.
	local trust=""
	for repo in $(sed -n 's/^ *repo: *//p' "$home/.openconductor/config.yaml"); do
		mkdir -p "$repo"
		git -C "$repo" init -q 2>/dev/null || true
		trust+="\"$repo\":{\"hasTrustDialogAccepted\":true,\"hasCompletedProjectOnboarding\":true},"
	done
	# openconductor always runs Claude Code with --dangerously-skip-permissions;
	# pre-accept its warning. Haiku keeps test runs cheap.
	echo "{\"hasCompletedOnboarding\":true,\"bypassPermissionsModeAccepted\":true,\"theme\":\"dark\",\"projects\":{${trust%,}}}" \
		>"$home/.claude/.claude.json"
	echo '{"model":"haiku","skipDangerousModePermissionPrompt":true}' >"$home/.claude/settings.json"

	(cd "$ROOT" && go build -o "$E2E_DIR/openconductor" ./cmd/openconductor)

	tmux new-session -d -s "$SESSION" -x 200 -y 50 \
		env HOME="$home" CLAUDE_CONFIG_DIR="$home/.claude" \
		XDG_CONFIG_HOME="$home/.config" XDG_DATA_HOME="$home/.local/share" \
		DISABLE_AUTOUPDATER=1 TERM=xterm-256color \
		OPENAI_API_KEY=mock \
		"$E2E_DIR/openconductor" --debug
	echo "started; sandbox $E2E_DIR"
}

case "${1:-}" in
start) shift; start "$@" ;;
keys) shift; tmux send-keys -t "$SESSION" "$@" ;;
screen) tmux capture-pane -p -t "$SESSION" ;;
wait)
	deadline=$((SECONDS + ${3:-60}))
	until tmux capture-pane -p -t "$SESSION" | grep -qE "$2"; do
		[ $SECONDS -ge $deadline ] && { echo "timeout waiting for: $2" >&2; tmux capture-pane -p -t "$SESSION"; exit 1; }
		sleep 1
	done ;;
stop)
	tmux kill-session -t "$SESSION" 2>/dev/null || true
	[ -f "$E2E_DIR/mock.pid" ] && kill "$(cat "$E2E_DIR/mock.pid")" 2>/dev/null || true
	# Agents may still be writing into the sandbox as they exit.
	for _ in 1 2 3 4 5; do rm -rf "$E2E_DIR" 2>/dev/null && break; sleep 1; done ;;
*) sed -n '6,15p' "$0"; exit 2 ;;
esac
