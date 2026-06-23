import { api } from "../api.js";
import { el, statusBadge, formatDate, timeAgo, clear } from "../ui.js";

const POLL_MS = 2500;

export function renderRun(params, mount) {
  mount.append(
    el("button", { class: "btn btn-ghost btn-sm", onClick: () => history.back() }, "← Back")
  );
  const head = el("div", {});
  const body = el("div", {});
  mount.append(head, body);

  let timer = null;
  let stopped = false;
  let lastRun = null;
  let lastSteps = null;

  const rerender = () => {
    if (lastRun) renderBody(body, lastRun, lastSteps, rerender);
  };

  async function load() {
    try {
      const { run, steps } = await api.getRun(params.id);
      if (stopped) return;
      lastRun = run;
      lastSteps = steps;
      renderHead(head, run);
      renderBody(body, run, steps, rerender);
      const active = run.status === "init" || run.status === "processing";
      if (!active && timer) {
        clearInterval(timer);
        timer = null;
      }
    } catch (e) {
      clear(head);
      head.append(el("p", { class: "form-error" }, "Failed to load run: " + e.message));
      if (timer) {
        clearInterval(timer);
        timer = null;
      }
    }
  }

  // When the user returns to our tab (e.g. back from applying), re-render so any
  // job they clicked Apply on shows the "Did you apply?" confirm.
  window.addEventListener("focus", rerender);

  load();
  timer = setInterval(load, POLL_MS);

  return () => {
    stopped = true;
    if (timer) clearInterval(timer);
    window.removeEventListener("focus", rerender);
  };
}

function renderHead(head, run) {
  clear(head);
  head.append(
    el("div", { class: "page-head" }, [
      el("div", {}, [
        el("h1", {}, "Run"),
        el("div", { class: "meta" }, [
          statusBadge(run.status),
          el("span", {}, `Started: ${formatDate(run.started_at)}`),
          el("span", {}, `Finished: ${formatDate(run.finished_at)}`),
        ]),
      ]),
    ])
  );
}

function renderBody(body, run, steps, rerender) {
  clear(body);
  body.append(renderProgress(steps));

  if (run.error) {
    body.append(el("div", { class: "card error-card" }, run.error));
    return;
  }

  // Only show output once the whole run is done.
  if (run.status !== "success") {
    body.append(
      el("p", { class: "center-msg muted" }, steps.length ? "Working on it…" : "Planning…")
    );
    return;
  }

  renderResults(body, steps, rerender);
}

// ---- progress tracker (per-step status, no output) ----
function renderProgress(steps) {
  const wrap = el("div", { class: "card stepper" });
  if (!steps.length) {
    wrap.append(el("span", { class: "muted" }, "Planning steps…"));
    return wrap;
  }
  steps.forEach((s, i) => {
    if (i > 0) {
      const done = steps[i - 1].status === "success";
      wrap.append(el("div", { class: `stepper-line ${done ? "line-done" : ""}` }));
    }
    wrap.append(
      el("div", { class: "stepper-node" }, [
        el("div", { class: `stepper-dot dot-${s.status}` }, dotMark(s.status, s.step_number)),
        el("div", { class: "stepper-label" }, [
          el("div", { class: "stepper-tool" }, s.tool),
          el("div", { class: "stepper-state muted" }, s.status),
        ]),
      ])
    );
  });
  return wrap;
}

function dotMark(status, num) {
  if (status === "success") return "✓";
  if (status === "failed") return "✕";
  return String(num);
}

// ---- final results ----
function renderResults(body, steps, rerender) {
  const jobs = finalRankedJobs(steps);

  if (jobs === null) {
    // Not a job-hunt run — fall back to the last step's raw output.
    const last = steps[steps.length - 1];
    if (last && last.output && Object.keys(last.output).length) {
      body.append(el("h2", { class: "section-title" }, "Result"));
      body.append(el("pre", { class: "output" }, JSON.stringify(last.output, null, 2)));
    }
    return;
  }

  body.append(el("h2", { class: "section-title" }, `Results (${jobs.length})`));
  if (!jobs.length) {
    body.append(el("p", { class: "muted" }, "No new matching jobs this run."));
    return;
  }

  const list = el("div", { class: "job-list" });
  for (const j of jobs) list.append(jobRow(j, rerender));
  body.append(list);
}

// Finds the last step output that carries ranked_jobs (i.e. score_jobs).
function finalRankedJobs(steps) {
  for (let i = steps.length - 1; i >= 0; i--) {
    const out = steps[i].output;
    if (out && Array.isArray(out.ranked_jobs)) return out.ranked_jobs;
  }
  return null;
}

function jobRow(j, rerender) {
  const score = Math.round((Number(j.score) || 0) * 100);
  const posted = timeAgo(j.posted_at);

  return el("div", { class: "job-row" }, [
    companyLogo(j),
    el("div", { class: "job-main" }, [
      el("div", { class: "job-title" }, j.title || "Untitled role"),
      el("div", { class: "job-sub muted" }, [j.company, j.department || j.location].filter(Boolean).join(" · ")),
    ]),
    el("div", { class: "job-posted muted" }, posted ? `Posted ${posted}` : ""),
    el("div", { class: "job-score" }, [
      el("span", { class: "job-score-num" }, String(score)),
      el("span", { class: "job-score-label" }, "match"),
    ]),
    applyButton(j, rerender),
  ]);
}

// applyButton has three states: not-yet (Apply link), pending (clicked Apply,
// waiting to confirm → "Applied? Yes/No"), and done (Applied ✓). Clicking Apply
// opens the job and marks it pending; the run view re-renders on tab-focus, so
// when the user comes back the row asks whether they applied.
function applyButton(j, rerender) {
  const jobId = j.job_id || "";

  if (isApplied(jobId)) {
    return el(
      "a",
      { class: "btn btn-primary btn-sm job-apply applied", href: j.url || "#", target: "_blank", rel: "noopener noreferrer" },
      "Applied ✓"
    );
  }

  if (isPending(jobId)) {
    return el("div", { class: "apply-confirm" }, [
      el("span", { class: "apply-confirm-q muted" }, "Applied?"),
      el("button", {
        class: "btn btn-primary btn-sm",
        onClick: () => { markApplied(jobId); clearPending(jobId); rerender(); },
      }, "Yes"),
      el("button", {
        class: "btn btn-ghost btn-sm",
        onClick: () => { clearPending(jobId); rerender(); },
      }, "No"),
    ]);
  }

  const a = el(
    "a",
    { class: "btn btn-primary btn-sm job-apply", href: j.url || "#", target: "_blank", rel: "noopener noreferrer" },
    "Apply now →"
  );
  a.addEventListener("click", () => setPending(jobId));
  return a;
}

// localStorage-backed sets keyed by job_id. "applied" = confirmed; "pending" =
// clicked Apply, awaiting the "did you apply?" confirmation.
function lsSet(key) {
  try {
    return new Set(JSON.parse(localStorage.getItem(key) || "[]"));
  } catch {
    return new Set();
  }
}
function lsAdd(key, jobId) {
  const s = lsSet(key);
  s.add(jobId);
  localStorage.setItem(key, JSON.stringify([...s]));
}
function lsRemove(key, jobId) {
  const s = lsSet(key);
  s.delete(jobId);
  localStorage.setItem(key, JSON.stringify([...s]));
}

function isApplied(jobId) { return jobId !== "" && lsSet("relay.applied").has(jobId); }
function markApplied(jobId) { if (jobId) lsAdd("relay.applied", jobId); }
function isPending(jobId) { return jobId !== "" && lsSet("relay.pendingApply").has(jobId); }
function setPending(jobId) { if (jobId) lsAdd("relay.pendingApply", jobId); }
function clearPending(jobId) { if (jobId) lsRemove("relay.pendingApply", jobId); }

// Company logo via Clearbit (guessing {slug}.com). Falls back to a monogram
// avatar when the logo 404s or the real domain isn't {slug}.com.
function companyLogo(j) {
  const slug = j.company_id || "";
  const name = j.company || "?";
  const wrap = el("div", { class: "job-logo" });

  const showMonogram = () => {
    wrap.classList.add("job-logo--mono");
    wrap.replaceChildren(el("span", { class: "job-mono" }, (name[0] || "?").toUpperCase()));
  };

  if (!slug) {
    showMonogram();
    return wrap;
  }

  const img = el("img", {
    class: "job-logo-img",
    alt: name,
    loading: "lazy",
    src: `https://logo.clearbit.com/${slug}.com`,
  });
  img.addEventListener("error", showMonogram);
  wrap.append(img);
  return wrap;
}
