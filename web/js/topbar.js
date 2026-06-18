// Renders the logged-in email + logout button in the top bar. Kept in its own
// module so both app.js and the login view can refresh it without an import cycle.
import { el } from "./ui.js";
import { getUser, clearUser } from "./session.js";
import { navigate } from "./router.js";

export function renderTopbar() {
  const box = document.getElementById("topbar-user");
  if (!box) return;
  box.replaceChildren();

  const user = getUser();
  if (!user) return;

  box.append(
    el("span", { class: "topbar-email" }, user.email),
    el(
      "button",
      {
        class: "btn btn-ghost btn-sm",
        onClick: () => {
          clearUser();
          renderTopbar();
          navigate("/login");
        },
      },
      "Log out"
    )
  );
}
