import { api } from "../api.js";
import { getUser } from "../session.js";
import { navigate } from "../router.js";
import { el, statusBadge, formatDate } from "../ui.js";

export async function renderWorkers(_params, mount) {
  const user = getUser();

  mount.append(
    el("div", { class: "page-head" }, [
      el("h1", {}, "Workers"),
      el("button", { class: "btn btn-primary", onClick: () => navigate("/workers/new") }, "+ New worker"),
    ])
  );

  const list = el("div", { class: "grid" });
  mount.append(list);

  try {
    const workers = await api.listWorkers(user.user_id);
    if (!workers.length) {
      list.append(el("p", { class: "muted" }, "No workers yet. Create your first one."));
      return;
    }
    for (const w of workers) list.append(workerCard(w));
  } catch (e) {
    list.append(el("p", { class: "form-error" }, "Failed to load workers: " + e.message));
  }
}

function workerCard(w) {
  const hours = Math.round((w.interval_seconds || 0) / 3600);
  return el(
    "div",
    { class: "card worker-card", onClick: () => navigate(`/workers/${w.worker_id}`) },
    [
      el("div", { class: "card-head" }, [el("h3", {}, w.name), statusBadge(w.status)]),
      el("p", { class: "muted clamp" }, w.instructions || ""),
      el("div", { class: "meta" }, [
        el("span", {}, `Every ${hours}h`),
        el("span", {}, `Next: ${formatDate(w.next_run_at)}`),
      ]),
    ]
  );
}
