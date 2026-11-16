# ompig-slim verification feature map

| Feature | User entry point | Proof |
|---|---|---|
| [Model management](model-management.md) | `/model`, then Roles or Alt+P | Settings persist and the terminal returns to the main screen |
| [Subagents](subagents.md) | Agent tree, Alt+A hub, and child transcript | Named rows, role colors, and thinking update before the child finishes |
| [Headless modes](headless-modes.md) | `-p`, `--mode json`, or `--mode rpc` | Output protocol is valid and the process exits or services stdin as documented |
| [Durable execution](durable-execution.md) | `durable run`, `durable status`, `durable resume` | State/events survive process boundaries and resume continues the request |
