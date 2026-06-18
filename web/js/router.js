// Minimal hash router. Patterns use ":param" segments, e.g. "/workers/:id".
// A view handler may return a cleanup function (called when navigating away) —
// used by the run view to stop its polling timer.
import { clear } from "./ui.js";

const routes = [];
let activeCleanup = null;

export function route(pattern, handler) {
  const keys = [];
  const rx = new RegExp(
    "^" +
      pattern.replace(/:[^/]+/g, (m) => {
        keys.push(m.slice(1));
        return "([^/]+)";
      }) +
      "$"
  );
  routes.push({ rx, keys, handler });
}

export function navigate(path) {
  if (location.hash === "#" + path) render(); // same path → force re-render
  else location.hash = "#" + path;
}

function currentPath() {
  return location.hash.replace(/^#/, "") || "/";
}

async function render() {
  if (activeCleanup) {
    activeCleanup();
    activeCleanup = null;
  }
  const path = currentPath();
  const mount = document.getElementById("app");

  for (const r of routes) {
    const m = path.match(r.rx);
    if (!m) continue;
    const params = {};
    r.keys.forEach((k, i) => (params[k] = decodeURIComponent(m[i + 1])));
    clear(mount);
    const cleanup = await r.handler(params, mount);
    if (typeof cleanup === "function") activeCleanup = cleanup;
    return;
  }

  // no match → default
  navigate("/workers");
}

export function start() {
  window.addEventListener("hashchange", render);
  render();
}
