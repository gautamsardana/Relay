# Why Relay?

## The Problem with Existing AI Tools

Claude, ChatGPT, and similar tools are powerful for interactive, synchronous tasks. You type a command, the model responds, and you're done. For many tasks, this is enough.

But it breaks down for a specific class of problems:

- The task takes longer than a browser tab stays open
- The task needs to run on a schedule without a human present
- You need to see exactly what happened at each step and why
- A step fails halfway through and you need to retry just that step
- The task should get smarter over time by remembering what it has already done

These are not edge cases — they are the norm for any real automation workflow.

---

## What Relay Is

Relay is an AI agent runtime. Users define persistent workers — give them instructions, a schedule, and tools. Workers run autonomously in the background, accumulate memory across runs, and produce better results over time.

Internally: distributed async execution, durable Postgres state, RabbitMQ-backed workers, and real observability.

Externally: you create an AI employee and it works forever.

---

## The "AI Employee" Model

Users should not think in terms of workflows.
Users should think in terms of hiring AI employees.

Instead of:
> "Run this workflow."

They say:
> "Create an AI worker whose job is to do this forever."

Each worker owns:
- **Instructions** — what it does and how
- **Schedule** — when it runs
- **Memory** — what it has learned across runs
- **Tools** — what it can execute
- **History** — every run, every step, every output

Examples of workers:
- Find remote Go backend jobs every morning
- Monitor competitors for new product announcements every hour
- Track apartment listings until I move
- Summarize AI research papers published each week

---

## Why This is Better Than Chat

| | Chat | Relay |
|---|---|---|
| Execution | Synchronous, blocks the tab | Async, runs in background |
| State | Forgotten after response | Durable in Postgres |
| Memory | None | Persists across runs, gets smarter |
| Failure | Start over | Retry at step level |
| Observability | Black box | Every prompt, tool call, output traced |
| Scale | One user, one session | N workers running in parallel |

---

## What Relay Is Not

Relay is not a replacement for Claude chat for interactive tasks. If you need a quick answer, chat is faster.

Relay is infrastructure for **automated, observable, long-running agentic workflows** — the layer between an LLM and the real world, when that interaction needs to be durable, scheduled, memory-aware, and auditable.

---

## Why We Are Building This

Three goals, one project:

1. **Learn real distributed systems** — message queues, async workers, backpressure, rate limiting, horizontal scaling. Not toy examples — an actual running system.

2. **Learn AI infrastructure** — how production AI systems are built beyond just calling an API. RAG, vector memory, prompt engineering, evaluation, observability for LLM calls.

3. **Solve real problems** — job hunting is the first use case. It's a genuine recurring problem that benefits from scheduling, memory, deduplication, and structured tool access to ATS platforms.

The job hunter is the proof of concept. If it works well enough that you'd actually use it daily, the platform generalizes to any domain: competitor monitoring, stock tracking, academic research, apartment hunting.
