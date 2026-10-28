// Runs on every page (when problem reporting is switched on). It keeps a
// short, privacy-safe trail of what the person did and any script errors, and
// when they choose "Report a problem" it takes a screenshot of the page they
// are on BEFORE opening the form, so the picture shows the problem.
//
// Nothing here is sent anywhere. It stays in this tab's sessionStorage until
// the person submits the report form themselves.
import { KEY_PAGE, KEY_SHOT, describe, forget, pageInfo, recordError, recordTrail, safePath, save, } from "./report-shared.js";
const MAX_SHOT_WIDTH = 1600;
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
document.addEventListener("securitypolicyviolation", (event) => {
    recordError({
        kind: "csp",
        message: `blocked ${event.violatedDirective}`,
        source: event.blockedURI,
        line: event.lineNumber,
        col: event.columnNumber,
    });
});
/** Elements marked data-private, as rectangles in screenshot pixels. */
function privateBoxes(scaleX, scaleY, width, height) {
    const boxes = [];
    document.querySelectorAll("[data-private]").forEach((el) => {
        const r = el.getBoundingClientRect();
        const x = Math.max(0, r.left * scaleX);
        const y = Math.max(0, r.top * scaleY);
        const w = Math.min(width, r.right * scaleX) - x;
        const h = Math.min(height, r.bottom * scaleY) - y;
        if (w >= 4 && h >= 4)
            boxes.push({ x: Math.round(x), y: Math.round(y), w: Math.round(w), h: Math.round(h) });
    });
    return boxes;
}
/**
 * Takes a picture of this tab using the browser's own screen-capture
 * permission (the browser asks the person first). Returns null where that is
 * not available or the person says no: the form then offers a file upload.
 */
async function captureScreenshot() {
    const devices = navigator.mediaDevices;
    if (!window.isSecureContext || !devices || typeof devices.getDisplayMedia !== "function")
        return null;
    let stream = null;
    try {
        const options = {
            video: { displaySurface: "browser" },
            audio: false,
            preferCurrentTab: true,
            selfBrowserSurface: "include",
            surfaceSwitching: "exclude",
        };
        stream = await devices.getDisplayMedia(options);
        const track = stream.getVideoTracks()[0];
        if (!track)
            return null;
        const isThisTab = track.getSettings().displaySurface === "browser";
        const video = document.createElement("video");
        video.muted = true;
        video.playsInline = true;
        video.srcObject = stream;
        await video.play();
        await new Promise((resolve) => {
            if (typeof video.requestVideoFrameCallback === "function")
                video.requestVideoFrameCallback(() => resolve());
            else
                window.setTimeout(resolve, 300);
        });
        const scale = Math.min(1, MAX_SHOT_WIDTH / video.videoWidth);
        const canvas = document.createElement("canvas");
        canvas.width = Math.max(1, Math.round(video.videoWidth * scale));
        canvas.height = Math.max(1, Math.round(video.videoHeight * scale));
        const ctx = canvas.getContext("2d");
        if (!ctx)
            return null;
        ctx.drawImage(video, 0, 0, canvas.width, canvas.height);
        return {
            dataUrl: canvas.toDataURL("image/jpeg", 0.82),
            width: canvas.width,
            height: canvas.height,
            // Only when the person shared THIS tab can page positions be mapped
            // onto the picture; if they chose another window, do not guess.
            boxes: isThisTab ? privateBoxes(canvas.width / window.innerWidth, canvas.height / window.innerHeight, canvas.width, canvas.height) : [],
            method: isThisTab ? "tab-capture" : "window-capture",
            at: Date.now(),
        };
    }
    catch {
        return null; // declined, or not supported here
    }
    finally {
        stream?.getTracks().forEach((t) => t.stop());
    }
}
/** How long to wait for the person to answer the browser's capture prompt. */
const CAPTURE_TIMEOUT_MS = 45_000;
async function startReport(href) {
    save(KEY_PAGE, pageInfo());
    forget(KEY_SHOT);
    // The link must always lead to the form, even if the browser never answers
    // the capture prompt: after the timeout we go on without a screenshot.
    const shot = await Promise.race([
        captureScreenshot(),
        new Promise((resolve) => window.setTimeout(() => resolve(null), CAPTURE_TIMEOUT_MS)),
    ]);
    if (shot && !save(KEY_SHOT, shot))
        forget(KEY_SHOT); // too big for storage: the form offers a file instead
    const url = new URL(href, window.location.href);
    url.searchParams.set("from", window.location.pathname);
    window.location.assign(url.toString());
}
document.addEventListener("click", (event) => {
    const link = event.target instanceof Element ? event.target.closest("a[data-report-link]") : null;
    if (!link || event.button !== 0 || event.ctrlKey || event.metaKey || event.shiftKey || event.altKey)
        return;
    if (document.getElementById("report-form"))
        return; // already on the form: just follow the link
    event.preventDefault();
    void startReport(link.href);
});
// After a report is sent, the picture is no longer needed.
if (document.querySelector("[data-report-sent]")) {
    forget(KEY_SHOT);
    forget(KEY_PAGE);
}
