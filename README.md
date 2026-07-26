# agent-library
My personal library of things related to AI Agents. Tools, Skills, MCP, etc
# Directories
- agentFiles - Things that agents can use
  - agents - Agents and subAgents. Agents are _______. Subagents are ______
    - TODO: FIXME!
  -  commands - Self-contained commands that can be used by agents. Commands are _______
  - TODO: FIXME!
  - skills - Self-contained skills that can be used by agents. Skills are _______
    - TODO: FIXME!
  - tools - Self-contained tools that can be used by agents. Tools are ______
    - TODO: FIXME!
  - hooks - holds custom shell scripts or programs that run automatically during specific events in the Claude Code lifecycle
    - Session Start: Triggers when a new agent session begins or resumes.
    - User Prompt Submit: Triggers right after a user sends a prompt.
    - Pre-Tool Use: Fires before a tool runs, allowing you to validate or block risky commands.
    - Post-Tool Use: Fires after a tool finishes, ideal for running formatters or tests.
    - Subagent Start/Stop: Tracks nested agent workflows.
    - Pre-Impact: Triggers when conversation context limits are approached.
    - Stop: Fires when an agent session ends to finalize logs or clean up resources.
    - TODO: FIXME!
  - .mcp.json - A json file defining mcp servers and related commands that an agent can use. It tells your AI environment which local or remote external tools, databases, or file systems it can launch, talk to, and use
  - settings.json - A json file defining settings for the agent environment, including hooks, ADD MORE!
- mcp - Example MCP (Multi-agent coordination protocol) Servers
  - TODO: FIXME!
- examples - Examples of how to use things
  - Agents - Ignore this for now, this is my testbed for things
  - mcp - Example on an agent that utilizes an MCP server
