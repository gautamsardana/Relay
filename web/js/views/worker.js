import { api } from "../api.js";
import { navigate } from "../router.js";
import { el, statusBadge, formatDate, shortId } from "../ui.js";

export async function renderWorker(params, mount) {
  mount.append(
    el("button", { class: "btn btn-ghost btn-sm", onClick: () => navigate("/workers") }, "← Workers")
  );

  const container = el("div", {});
  mount.append(container);

  try {
    const { worker, runs } = await api.getWorker(params.id);
    renderDetail(container, worker, runs);
  } catch (e) {
    container.append(el("p", { class: "form-error" }, "Failed to load worker: " + e.message));
  }
}

function renderDetail(container, worker, runs) {
  const hours = Math.round((worker.interval_seconds || 0) / 3600);

  const runNow = el("button", { class: "btn btn-primary" }, "Run now");
  runNow.addEventListener("click", async () => {
    runNow.disabled = true;
    runNow.textContent = "Starting…";
    try {
      const runId = await api.createRun(worker.worker_id);
      navigate(`/runs/${runId}`);
    } catch (e) {
      runNow.disabled = false;
      runNow.textContent = "Run now";
      alert("Failed to start run: " + e.message);
    }
  });

  const pauseBtn = el("button", { class: "btn btn-secondary" }, worker.status === "paused" ? "Resume" : "Pause");
  pauseBtn.addEventListener("click", async () => {
    const next = worker.status === "paused" ? "active" : "paused";
    try {
      await api.setWorkerStatus(worker.worker_id, next);
      navigate(`/workers/${worker.worker_id}`); // re-render
    } catch (e) {
      alert("Failed: " + e.message);
    }
  });

  const delBtn = el("button", { class: "btn btn-ghost" }, "Delete");
  delBtn.addEventListener("click", async () => {
    if (!confirm(`Delete worker "${worker.name}"? It will be archived and hidden.`)) return;
    try {
      await api.setWorkerStatus(worker.worker_id, "archived");
      navigate("/workers");
    } catch (e) {
      alert("Failed: " + e.message);
    }
  });

  container.append(
    el("div", { class: "page-head" }, [
      el("div", {}, [
        el("h1", {}, worker.name),
        el("div", { class: "meta" }, [
          statusBadge(worker.status),
          el("span", {}, `Every ${hours}h`),
          el("span", {}, `Next: ${formatDate(worker.next_run_at)}`),
        ]),
      ]),
      el("div", { class: "head-actions" }, [pauseBtn, delBtn, runNow]),
    ])
  );

  const bits = [];
  if (worker.category) bits.push(prettyCategory(worker.category));
  if (worker.keywords && worker.keywords.length) bits.push(worker.keywords.join(", "));
  container.append(
    el("div", { class: "card" }, [
      el("h4", { class: "section-label" }, "Search"),
      el("p", {}, bits.length ? bits.join(" · ") : "—"),
      worker.instructions ? el("p", { class: "muted" }, worker.instructions) : null,
    ])
  );

  container.append(el("h2", { class: "section-title" }, "Runs"));

  if (!runs.length) {
    container.append(el("p", { class: "muted" }, "No runs yet. Hit “Run now” to start one."));
    return;
  }

  const list = el("div", { class: "run-list" });
  for (const r of runs) {
    list.append(
      el("div", { class: "run-row", onClick: () => navigate(`/runs/${r.run_id}`) }, [
        statusBadge(r.status),
        el("span", { class: "run-time" }, formatDate(r.started_at)),
        el("span", { class: "run-id muted" }, shortId(r.run_id)),
        el("span", { class: "chev" }, "›"),
      ])
    );
  }
  container.append(list);
}

function prettyCategory(c) {
  return c
    .split("_")
    .map((w) => w.charAt(0).toUpperCase() + w.slice(1))
    .join(" ");
}
