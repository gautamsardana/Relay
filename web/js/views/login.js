import { api } from "../api.js";
import { setUser } from "../session.js";
import { navigate } from "../router.js";
import { el } from "../ui.js";
import { renderTopbar } from "../topbar.js";

export function renderLogin(_params, mount) {
  const errBox = el("div", { class: "form-error" });
  const input = el("input", { type: "email", class: "input", placeholder: "you@example.com" });

  async function submit(mode) {
    const email = input.value.trim();
    errBox.textContent = "";
    if (!email) {
      errBox.textContent = "Enter an email.";
      return;
    }
    try {
      const user = mode === "register" ? await api.register(email) : await api.login(email);
      setUser(user);
      renderTopbar();
      navigate("/workers");
    } catch (e) {
      errBox.textContent =
        mode === "register"
          ? "Could not register — that email may already exist. Try logging in."
          : "No account for that email. Try registering.";
    }
  }

  input.addEventListener("keydown", (e) => {
    if (e.key === "Enter") submit("login");
  });

  const card = el("div", { class: "card auth-card" }, [
    el("h1", { class: "auth-title" }, "Relay"),
    el("p", { class: "muted" }, "Log in or create an account with your email."),
    el("label", { class: "label" }, "Email"),
    input,
    errBox,
    el("div", { class: "auth-actions" }, [
      el("button", { class: "btn btn-primary", onClick: () => submit("login") }, "Log in"),
      el("button", { class: "btn btn-secondary", onClick: () => submit("register") }, "Register"),
    ]),
  ]);

  mount.append(el("div", { class: "auth-wrap" }, card));
}
