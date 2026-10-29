// Shared by the two "Report a problem" scripts. Everything here is about
// recording enough to diagnose a problem WITHOUT recording anything private:
// no typed values are ever stored, paths lose their query strings and any
// token-like segment, and text inside [data-private] is replaced.
export const KEY_TRAIL = "rt.trail";
export const KEY_ERRORS = "rt.errors";
export const KEY_SHOT = "rt.shot";
export const KEY_PAGE = "rt.page";
/** A screenshot older than this is not offered (it is from an earlier visit). */
export const SHOT_MAX_AGE_MS = 10 * 60 * 1000;
const TRAIL_MAX = 20;
const ERRORS_MAX = 10;
/** sessionStorage can throw (private windows, blocked storage): never let it break the page. */
export function load(key) {
    try {
        const raw = window.sessionStorage.getItem(key);
        return raw === null ? null : JSON.parse(raw);
    }
    catch {
        return null;
    }
}
export function save(key, value) {
    try {
        window.sessionStorage.setItem(key, JSON.stringify(value));
        return true;
    }
    catch {
        return false;
    }
}
export function forget(key) {
    try {
        window.sessionStorage.removeItem(key);
    }
    catch {
        /* nothing to do */
    }
}
const EMAIL = /[A-Za-z0-9._%+'-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}/g;
const TOKEN = /[A-Za-z0-9_-]{24,}/g;
/** Removes email addresses and token-like strings from free text. */
export function redact(text) {
    return text.replace(EMAIL, "[email]").replace(TOKEN, "[token]");
}
/** A path with no query, no fragment and no token-like segments. */
export function safePath(raw) {
    let path = raw;
    try {
        path = new URL(raw, window.location.origin).pathname;
    }
    catch {
        path = raw.split(/[?#]/)[0] ?? "";
    }
    return path
        .split("/")
        .map((segment) => (/^[A-Za-z0-9_-]{20,}$/.test(segment) ? "[token]" : segment))
        .join("/")
        .slice(0, 200);
}
function shortText(el) {
    return redact((el.textContent ?? "").trim().replace(/\s+/g, " ").slice(0, 40));
}
/** A short, privacy-safe description of an element: never a field's value. */
export function describe(el) {
    if (el.closest("[data-private]"))
        return "[private]";
    const tag = el.tagName.toLowerCase();
    if (el instanceof HTMLAnchorElement)
        return `link "${shortText(el)}"`;
    if (el instanceof HTMLButtonElement)
        return `button "${shortText(el)}"`;
    if (el instanceof HTMLInputElement) {
        if (el.type === "submit" || el.type === "button")
            return `button "${redact(el.value.slice(0, 40))}"`;
        return `${el.type} ${el.name || el.id}`.trim();
    }
    if (el instanceof HTMLSelectElement || el instanceof HTMLTextAreaElement)
        return `${tag} ${el.name || el.id}`.trim();
    return el.id ? `${tag}#${el.id}` : tag;
}
function push(key, entry, max) {
    const list = load(key) ?? [];
    list.push(entry);
    save(key, list.slice(-max));
}
export function recordTrail(kind, target, path = window.location.pathname) {
    push(KEY_TRAIL, { at: new Date().toISOString(), kind, target, path: safePath(path) }, TRAIL_MAX);
}
export function recordError(entry) {
    push(KEY_ERRORS, { ...entry, message: redact(entry.message).slice(0, 300), source: safePath(entry.source) }, ERRORS_MAX);
}
/** What the page shows right now, for the report. */
export function pageInfo() {
    const messages = Array.from(document.querySelectorAll(".error, .flash, .docs-banner"))
        .filter((el) => !el.closest("[data-private]"))
        .map((el) => redact((el.textContent ?? "").trim().replace(/\s+/g, " ").slice(0, 200)))
        .filter((m) => m !== "")
        .slice(0, 5);
    return {
        path: safePath(window.location.pathname),
        title: redact(document.title.slice(0, 120)),
        heading: redact((document.querySelector("h1")?.textContent ?? "").trim().slice(0, 120)),
        messages,
        viewportW: window.innerWidth,
        viewportH: window.innerHeight,
        dpr: window.devicePixelRatio || 1,
        touch: navigator.maxTouchPoints > 0,
        at: Date.now(),
        requestId: document.querySelector('meta[name="request-id"]')?.content.slice(0, 16) ?? "",
        timing: navigationTiming(),
    };
}
function navigationTiming() {
    const nav = performance.getEntriesByType("navigation")[0];
    if (!nav)
        return { ttfb: 0, domContentLoaded: 0, load: 0 };
    const ms = (v) => (Number.isFinite(v) && v > 0 ? Math.round(v) : 0);
    return { ttfb: ms(nav.responseStart), domContentLoaded: ms(nav.domContentLoadedEventEnd), load: ms(nav.loadEventEnd) };
}
