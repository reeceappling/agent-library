#!/usr/bin/env bash
# block dangerous shell commands before Claude Code executes them


# Claude Code sends the tool execution details as JSON via stdin
# Example stdin: { "tool_name": "Bash", "tool_input": { "command": "rm -rf /" } }

# Read the JSON payload from standard input
INPUT_JSON=$(cat)

# Use jq to extract the exact shell command Claude is trying to run
COMMAND=$(echo "$INPUT_JSON" | jq -r '.tool_input.command' 2>/dev/null)

# Define blocklist patterns (e.g., preventing recursive root deletion or formatting)
if [[ "$COMMAND" == *"rm -rf /"* ]] || [[ "$COMMAND" == *"mkfs"* ]]; then
  # Output a standard JSON decision blocking the tool call
  echo '{
    "decision": "block",
    "reason": "Security Alert: This command matches a critical blocklist pattern."
  }'
  exit 0
fi

# Exit silently with code 0 if the command is completely safe
exit 0