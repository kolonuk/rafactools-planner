"use strict";
// Light / dark / system theme toggle. The page follows the browser or system
// preference until the person picks a side; the choice is kept in this
// browser only (localStorage, wrapped because storage can be blocked). Loaded
// as a plain script in <head> so a saved choice is applied before the page is
// painted. Without this script the page simply follows the system preference.
(() => {
    const KEY = "theme";
    const ORDER = ["system", "light", "dark"];
    const LABEL = { system: "Theme: system", light: "Theme: light", dark: "Theme: dark" };
    const ICON = { system: "◐", light: "☀", dark: "☾" };
    const root = document.documentElement;
    function saved() {
        try {
            const v = localStorage.getItem(KEY);
            return v === "light" || v === "dark" ? v : "system";
        }
        catch {
            return "system";
        }
    }
    function apply(mode) {
        if (mode === "system")
            root.removeAttribute("data-theme");
        else
            root.setAttribute("data-theme", mode);
    }
    function show(button, mode) {
        button.textContent = ICON[mode];
        button.title = LABEL[mode] + " (click to change)";
        button.setAttribute("aria-label", LABEL[mode] + ". Click to change.");
    }
    apply(saved());
    document.addEventListener("DOMContentLoaded", () => {
        const button = document.getElementById("theme-toggle");
        if (!(button instanceof HTMLButtonElement))
            return;
        let mode = saved();
        show(button, mode);
        button.hidden = false;
        button.addEventListener("click", () => {
            mode = ORDER[(ORDER.indexOf(mode) + 1) % ORDER.length];
            try {
                if (mode === "system")
                    localStorage.removeItem(KEY);
                else
                    localStorage.setItem(KEY, mode);
            }
            catch {
                /* storage blocked: the choice lasts until the page is closed */
            }
            apply(mode);
            show(button, mode);
        });
    });
})();
