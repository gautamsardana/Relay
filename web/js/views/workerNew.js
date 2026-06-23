import { api } from "../api.js";
import { getUser } from "../session.js";
import { navigate } from "../router.js";
import { el } from "../ui.js";

const CATEGORIES = [
  ["software_engineering", "Software Engineering"],
  ["data", "Data / ML"],
  ["design", "Design / UX"],
  ["product", "Product"],
  ["marketing", "Marketing"],
  ["sales", "Sales"],
  ["finance", "Finance / Accounting"],
  ["operations", "Operations"],
];

export function renderWorkerNew(_params, mount) {
  const user = getUser();

  const name = el("input", { class: "input", placeholder: "e.g. Backend Job Hunter" });

  const category = el("select", { class: "input" });
  CATEGORIES.forEach(([v, label]) => category.append(el("option", { value: v }, label)));

  const keywords = el("input", { class: "input", placeholder: "golang, kubernetes (comma separated)" });

  const resumeFile = el("input", { class: "input", type: "file", accept: "application/pdf" });
  const resumeStatus = el("p", { class: "hint" }, "Upload a PDF to auto-fill category and keywords.");
  let resumeText = "";

  resumeFile.addEventListener("change", async () => {
    const f = resumeFile.files && resumeFile.files[0];
    if (!f) return;
    resumeStatus.textContent = "Parsing résumé…";
    try {
      const r = await api.parseResume(f);
      resumeText = r.resume_text;
      if (r.suggested_category) category.value = r.suggested_category;
      if (r.suggested_keywords.length) keywords.value = r.suggested_keywords.join(", ");
      resumeStatus.textContent = "Résumé parsed ✓ — review the suggestions below.";
    } catch (e) {
      resumeStatus.textContent = "Could not parse that PDF: " + e.message;
    }
  });

  const instructions = el("textarea", {
    class: "input textarea",
    rows: "3",
    placeholder: "Optional: extra notes for ranking, e.g. remote only, early-stage startups",
  });
  const interval = el("input", { class: "input", type: "number", min: "1", value: "24" });
  const slider = el("input", { class: "slider", type: "range", min: "0", max: "100", value: "50" });
  const sliderVal = el("span", { class: "slider-val" }, "50");
  slider.addEventListener("input", () => (sliderVal.textContent = slider.value));

  const errBox = el("div", { class: "form-error" });
  const submitBtn = el("button", { class: "btn btn-primary" }, "Create worker");

  submitBtn.addEventListener("click", async () => {
    errBox.textContent = "";
    const payload = {
      user_id: user.user_id,
      name: name.value.trim(),
      instructions: instructions.value.trim(),
      interval_hours: parseInt(interval.value, 10),
      resume_text: resumeText,
      // Slider reads as "resume match" (100 = all match, 0 = all recency);
      // the backend stores the complementary recency_weight.
      recency_weight: 100 - parseInt(slider.value, 10),
      category: category.value,
      keywords: keywords.value.split(",").map((s) => s.trim()).filter(Boolean),
    };
    if (!payload.name) {
      errBox.textContent = "Name is required.";
      return;
    }
    if (!payload.interval_hours || payload.interval_hours < 1) {
      errBox.textContent = "Run interval must be at least 1 hour.";
      return;
    }
    submitBtn.disabled = true;
    submitBtn.textContent = "Creating…";
    try {
      const w = await api.createWorker(payload);
      navigate(`/workers/${w.worker_id}`);
    } catch (e) {
      submitBtn.disabled = false;
      submitBtn.textContent = "Create worker";
      errBox.textContent = "Failed to create worker: " + e.message;
    }
  });

  mount.append(
    el("div", { class: "form-wrap" }, [
      el("div", { class: "page-head" }, [
        el("h1", {}, "New worker"),
        el("button", { class: "btn btn-ghost btn-sm", onClick: () => navigate("/workers") }, "← Back"),
      ]),
      el("div", { class: "card form-card" }, [
        field("Name", name),
        el("div", { class: "field" }, [
          el("label", { class: "label" }, "Résumé (PDF)"),
          resumeFile,
          resumeStatus,
        ]),
        field("Category", category),
        field("Keywords (optional)", keywords, "Comma separated, e.g. golang, kubernetes"),
        field("Notes (optional)", instructions, "Extra context for ranking jobs."),
        field("Run every (hours)", interval, "Minimum 1 hour."),
        sliderField(slider, sliderVal),
        errBox,
        el("div", { class: "form-actions" }, [submitBtn]),
      ]),
    ])
  );
}

function field(label, input, hint) {
  return el("div", { class: "field" }, [
    el("label", { class: "label" }, label),
    input,
    hint ? el("p", { class: "hint" }, hint) : null,
  ]);
}

function sliderField(slider, val) {
  return el("div", { class: "field" }, [
    el("label", { class: "label" }, "Ranking preference"),
    el("div", { class: "slider-row" }, [
      el("span", { class: "slider-end" }, "Recent posts"),
      slider,
      el("span", { class: "slider-end" }, "Resume match"),
    ]),
    el("p", { class: "hint" }, ["Resume match: ", val, " / 100"]),
  ]);
}
