// API client. This is the only place that knows the wire format. The Go API
// serializes models with PascalCase keys (no JSON tags) for responses but takes
// snake_case request bodies; we normalize responses here so the rest of the app
// speaks one consistent language that matches the backend's domain terms
// (worker, run, step, recency_weight, ...).

async function request(method, path, body) {
  const opts = { method, headers: {} };
  if (body !== undefined) {
    opts.headers["Content-Type"] = "application/json";
    opts.body = JSON.stringify(body);
  }
  const res = await fetch(path, opts);
  if (!res.ok) {
    const text = (await res.text()).trim();
    const err = new Error(text || res.statusText);
    err.status = res.status;
    throw err;
  }
  const text = await res.text();
  return text ? JSON.parse(text) : null;
}

const normUser = (u) =>
  u && { user_id: u.UserID, email: u.Email, created_at: u.CreatedAt };

const normWorker = (w) =>
  w && {
    worker_id: w.WorkerID,
    user_id: w.UserID,
    name: w.Name,
    instructions: w.Instructions,
    category: w.Category,
    keywords: w.Keywords || [],
    interval_seconds: w.IntervalSeconds,
    status: w.Status,
    resume_text: w.ResumeText,
    recency_weight: w.RecencyWeight,
    next_run_at: w.NextRunAt,
    created_at: w.CreatedAt,
    updated_at: w.UpdatedAt,
  };

const normRun = (r) =>
  r && {
    run_id: r.RunID,
    worker_id: r.WorkerID,
    status: r.Status,
    error: r.Error,
    started_at: r.StartedAt,
    finished_at: r.FinishedAt,
  };

const normStep = (s) =>
  s && {
    step_id: s.StepID,
    run_id: s.RunID,
    step_number: s.StepNumber,
    tool: s.Tool,
    description: s.Description,
    input: s.Input,
    output: s.Output,
    status: s.Status,
    retry_count: s.RetryCount,
    error: s.Error,
  };

export const api = {
  async register(email) {
    return normUser(await request("POST", "/users", { email }));
  },
  async login(email) {
    return normUser(await request("GET", `/users?email=${encodeURIComponent(email)}`));
  },
  async listWorkers(userId) {
    const rows = await request("GET", `/workers?user_id=${encodeURIComponent(userId)}`);
    return (rows || []).map(normWorker);
  },
  async createWorker(payload) {
    return normWorker(await request("POST", "/workers", payload));
  },
  async getWorker(id) {
    const d = await request("GET", `/workers/${id}`);
    return { worker: normWorker(d.worker), runs: (d.runs || []).map(normRun) };
  },
  async createRun(workerId) {
    const d = await request("POST", `/workers/${workerId}/run`);
    return d.run_id;
  },
  async getRun(id) {
    const d = await request("GET", `/runs/${id}`);
    return { run: normRun(d.run), steps: (d.steps || []).map(normStep) };
  },
  async setWorkerStatus(id, status) {
    await request("PATCH", `/workers/${id}/status`, { status });
  },
  // Multipart upload — not via request() (which forces a JSON content-type);
  // the browser must set the multipart boundary itself.
  async parseResume(file) {
    const fd = new FormData();
    fd.append("resume", file);
    const res = await fetch("/resumes/parse", { method: "POST", body: fd });
    if (!res.ok) throw new Error((await res.text()) || res.statusText);
    const d = await res.json();
    return {
      resume_text: d.resume_text || "",
      suggested_category: d.suggested_category || "",
      suggested_keywords: d.suggested_keywords || [],
    };
  },
};
