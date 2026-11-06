// Light / dark toggle. The page follows the browser or system preference until
// the person presses the button; then each press switches to the other side; the choice is kept in this
// browser only (localStorage, wrapped because storage can be blocked). Loaded
// as a plain script in <head> so a saved choice is applied before the page is
// painted. Without this script the page simply follows the system preference.

(() => {
  type Choice = "light" | "dark";
  const KEY = "theme";
  const root = document.documentElement;
  const ICON: Record<Choice, string> = { light: "\u2600", dark: "\u263E" };

  function saved(): Choice | null {
    try {
      const v = localStorage.getItem(KEY);
      return v === "light" || v === "dark" ? v : null;
    } catch {
      return null;
    }
  }

  /** What the page looks like right now: the saved choice, else the device setting. */
  function shown(): Choice {
    const attr = root.getAttribute("data-theme");
    if (attr === "light" || attr === "dark") return attr;
    return window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light";
  }

  function paint(button: HTMLButtonElement): void {
    const now = shown();
    const next: Choice = now === "dark" ? "light" : "dark";
    button.textContent = ICON[now];
    button.title = `Switch to ${next} mode`;
    button.setAttribute("aria-label", `Switch to ${next} mode. It is ${now} now.`);
  }

  const choice = saved();
  if (choice) root.setAttribute("data-theme", choice);

  document.addEventListener("DOMContentLoaded", () => {
    const button = document.getElementById("theme-toggle");
    if (!(button instanceof HTMLButtonElement)) return;
    paint(button);
    button.hidden = false;
    // Each click switches to the other appearance, so it always visibly changes.
    button.addEventListener("click", () => {
      const next: Choice = shown() === "dark" ? "light" : "dark";
      root.setAttribute("data-theme", next);
      try {
        localStorage.setItem(KEY, next);
      } catch {
        /* storage blocked: the choice lasts until the page is closed */
      }
      paint(button);
    });
    // Until a choice is saved the page follows the device, so follow it live.
    window.matchMedia("(prefers-color-scheme: dark)").addEventListener("change", () => paint(button));
  });
})();
