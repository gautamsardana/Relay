// Holds the logged-in user (user_id + email) in localStorage. No real auth yet —
// this is the session the worker-creation flow reads the user_id/email from.
const KEY = "relay.user";

export function getUser() {
  try {
    return JSON.parse(localStorage.getItem(KEY));
  } catch {
    return null;
  }
}

export function setUser(user) {
  localStorage.setItem(KEY, JSON.stringify(user));
}

export function clearUser() {
  localStorage.removeItem(KEY);
}
