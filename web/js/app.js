import { route, start, navigate } from "./router.js";
import { getUser } from "./session.js";
import { renderTopbar } from "./topbar.js";
import { renderLogin } from "./views/login.js";
import { renderWorkers } from "./views/workers.js";
import { renderWorkerNew } from "./views/workerNew.js";
import { renderWorker } from "./views/worker.js";
import { renderRun } from "./views/run.js";

// Redirect to login if there's no session.
function requireAuth(handler) {
  return (params, mount) => {
    if (!getUser()) {
      navigate("/login");
      return;
    }
    return handler(params, mount);
  };
}

route("/login", renderLogin);
route("/workers", requireAuth(renderWorkers));
route("/workers/new", requireAuth(renderWorkerNew)); // must precede /workers/:id
route("/workers/:id", requireAuth(renderWorker));
route("/runs/:id", requireAuth(renderRun));

renderTopbar();
start();
