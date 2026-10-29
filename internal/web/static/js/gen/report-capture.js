// Runs on every page (when problem reporting is switched on). It keeps a
// short, privacy-safe trail of what the person did and any script errors, and
// when they choose "Report a problem" it remembers which page they were on.
//
// Nothing here is sent anywhere. It stays in this tab's sessionStorage until
// the person submits the report form themselves.
import { KEY_PAGE, KEY_SHOT, describe, forget, pageInfo, recordError, recordTrail, safePath, save, } from "./report-shared.js";
// --- the trail -------------------------------------------------------------
recordTrail("view", redactTitle(document.title));
function redactTitle(title) {
    return title.slice(0, 80);
}
document.addEventListener("click", (event) => {
    const target = event.target instanceof Element ? event.target.closest("a, button, input, select, summary, label") : null;
    if (target && !target.hasAttribute("data-report-link"))
        recordTrail("click", describe(target));
}, true);
document.addEventListener("submit", (event) => {
    if (event.target instanceof HTMLFormElement) {
        recordTrail("submit", `${event.target.method.toUpperCase()} ${safePath(event.target.action)}`);
    }
}, true);
document.addEventListener("change", (event) => {
    const t = event.target;
    if (t instanceof HTMLSelectElement || (t instanceof HTMLInputElement && (t.type === "checkbox" || t.type === "radio"))) {
        recordTrail("change", describe(t)); // the name of the control, never its value
    }
}, true);
// --- script errors ---------------------------------------------------------
window.addEventListener("error", (event) => {
    recordError({ kind: "error", message: event.message, source: event.filename, line: event.lineno, col: event.colno });
});
window.addEventListener("unhandledrejection", (event) => {
    const reason = event.reason;
    recordError({ kind: "promise", message: reason instanceof Error ? reason.message : String(reason), source: "", line: 0, col: 0 });
});
// Failed requests the page makes itself (the drag-and-drop move, for one): the
// method, path and status, never the body or the query.
const nativeFetch = window.fetch.bind(window);
window.fetch = async (input, init) => {
    const method = (init?.method ?? (input instanceof Request ? input.method : "GET")).toUpperCase();
    const raw = input instanceof Request ? input.url : String(input);
    let path = "";
    try {
        path = safePath(new URL(raw, window.location.href).pathname);
    }
    catch {
        path = "";
    }
    try {
        const response = await nativeFetch(input, init);
        if (!response.ok) {
            recordError({ kind: "fetch", message: `${method} ${response.status}`, source: path, line: 0, col: 0 });
        }
        return response;
    }
    catch (err) {
        recordError({ kind: "fetch", message: `${method} failed: ${err instanceof Error ? err.name : "error"}`, source: path, line: 0, col: 0 });
        throw err;
    }
};
document.addEventListener("securitypolicyviolation", (event) => {
    recordError({
        kind: "csp",
        message: `blocked ${event.violatedDirective}`,
        source: event.blockedURI,
        line: event.lineNumber,
        col: event.columnNumber,
    });
});
// --- the report link -------------------------------------------------------
// The link goes straight to the form, with nothing in between: an earlier
// version asked the browser to share the tab first, and while that prompt was
// open the link looked dead. The screenshot is added on the form itself ("Take
// a screenshot now", or attach a file). Here we only note which page the person
// was on, so the report can say so.
document.addEventListener("click", (event) => {
    const link = event.target instanceof Element ? event.target.closest("a[data-report-link]") : null;
    if (!link || event.button !== 0)
        return;
    save(KEY_PAGE, pageInfo());
    forget(KEY_SHOT);
    const url = new URL(link.href, window.location.href);
    if (!document.getElementById("report-form"))
        url.searchParams.set("from", window.location.pathname);
    link.href = url.toString();
});
// After a report is sent, the picture is no longer needed.
if (document.querySelector("[data-report-sent]")) {
    forget(KEY_SHOT);
    forget(KEY_PAGE);
}
