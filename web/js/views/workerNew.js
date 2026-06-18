import { api } from "../api.js";
import { getUser } from "../session.js";
import { navigate } from "../router.js";
import { el } from "../ui.js";

export function renderWorkerNew(_params, mount) {
  const user = getUser();

  const name = el("input", { class: "input", placeholder: "e.g. Backend Job Hunter" });
  const instructions = el("textarea", {
    class: "input textarea",
    rows: "4",
    placeholder: "e.g. Find backend / infrastructure engineering roles",
  });
  const interval = el("input", { class: "input", type: "number", min: "1", value: "24" });
  const resume = el("textarea", {
    class: "input textarea",
    rows: "6",
    placeholder: "Paste your resume as plain text",
  });
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
      resume_text: resume.value.trim(),
      // Slider reads as "resume match" (100 = all match, 0 = all recency);
      // the backend stores the complementary recency_weight.
      recency_weight: 100 - parseInt(slider.value, 10),
    };
    if (!payload.name || !payload.instructions) {
      errBox.textContent = "Name and instructions are required.";
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
        field("Instructions", instructions),
        field("Run every (hours)", interval, "Minimum 1 hour."),
        field("Resume (plain text)", resume, "Used to score how well each job fits you."),
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
