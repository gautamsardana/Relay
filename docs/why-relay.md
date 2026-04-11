# Why Relay?

## The Problem with Existing AI Chat Tools

Claude, ChatGPT, and similar tools are powerful for interactive, synchronous tasks. You type a command, the model responds, and you're done. For many tasks, this is sufficient.

But it breaks down in a specific class of problems:

- The task takes longer than a browser tab stays open
- The task needs to run on a schedule without a human present
- You need to see exactly what happened at each step and why
- Multiple users need to run similar workflows simultaneously
- A step fails halfway through and you need to retry just that step

These are not edge cases — they are the norm for any real automation workflow.

---

## What Relay Does Differently

### 1. Asynchronous by Default

Claude chat is synchronous — you wait for a response. Relay accepts a command, returns a workflow ID immediately, and executes in the background. You can close the browser, come back an hour later, and see the completed results with a full audit trail.

This makes Relay viable for long-running tasks: multi-step research workflows, data pipelines, content generation that involves several API calls chained together.

### 2. Durable State

Every step — its inputs, outputs, status, errors, retry count — is persisted in Postgres. Nothing lives in memory. If the server crashes mid-execution, the workflow resumes exactly where it left off on restart.

Claude chat has no memory of what it did. If something goes wrong, you start over. Relay never loses work.

### 3. Full Observability

You can see the exact plan Claude generated, the exact input passed to each tool, the exact output returned, and the exact error if something failed. Every decision is traceable.

This matters when something goes wrong. Instead of "it didn't work," you get "step 3 failed because the Notion API returned a 403 — here's the exact payload that was sent."

### 4. Intelligent Failure Recovery

When a step fails, Relay doesn't just give up or blindly retry. It calls the agent with the full execution context — what was done, what was returned, what failed — and asks it to generate a recovery plan. The workflow adapts rather than halts.

### 5. Scalable Execution

Because workers are decoupled from the API server via RabbitMQ, step execution scales independently. Run one API instance and ten worker instances. Fifty users submitting workflows simultaneously doesn't slow down the API — workers process steps in parallel across workflows.

### 6. Pluggable Tools

Relay's tool system is an interface. Any capability — web search, Notion, email, databases, internal APIs — can be added without changing the core system. Claude automatically discovers available tools and uses them in plans.

A generic `http_request` tool means Claude can call any API even without a dedicated integration.

---

## The Use Cases Relay Unlocks

| Use Case | Why Chat Can't Do It | Why Relay Can |
|---|---|---|
| Daily job digest posted to Notion every morning | Requires scheduling, runs headlessly | Triggered on a cron, runs in background |
| Research 20 competitors and generate a report | Takes 30+ minutes, many API calls | Async execution, full audit trail |
| Monitor a topic and notify on Slack when relevant | Needs to run continuously | Scheduled workflows, persistent state |
| Multi-user automation platform | One chat = one user | Workers handle N workflows in parallel |
| Retry a failed step without restarting | No step-level control in chat | Step state in Postgres, retry at step level |

---

## What Relay Is Not

Relay is not a replacement for Claude chat for interactive tasks. If you need a quick answer or a one-shot task, chat is faster and simpler.

Relay is infrastructure for **automated, observable, long-running agentic workflows** — the layer between an LLM and the real world, when that interaction needs to be durable, scalable, and auditable.
