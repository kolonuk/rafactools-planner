// Drag a parade night's activities onto another date in the programme grid.
//
// This only adds a mouse shortcut. Every night also has a "Move" control
// (a plain form, see programme_edit.html) that does the same thing without
// JavaScript, with the keyboard or on a touch screen, and both end up at the
// same server endpoint: POST /programmes/{id}/days/{dayID}/move. The server
// swaps the two dates' activities, so nothing is overwritten.
//
// The page's CSP only allows scripts served from this site: no inline code,
// no eval, no third-party code. Keep it that way.
const SAVE_FAILED = "Could not save your edits first, so nothing was moved.";
const MOVE_FAILED = "Could not move the activities. Nothing was changed.";
/** Which day a node in the grid belongs to, from its row's data-day attribute. */
function rowOf(node) {
    return node instanceof Element ? node.closest("tr[data-day]") : null;
}
function say(message) {
    const status = document.getElementById("grid-status");
    if (status)
        status.textContent = message;
}
/** The grid form as application/x-www-form-urlencoded, which is what the server reads. */
function encode(form) {
    const body = new URLSearchParams();
    for (const [key, value] of new FormData(form)) {
        if (typeof value === "string")
            body.append(key, value);
    }
    return body;
}
async function post(url, body) {
    const response = await fetch(url, {
        method: "POST",
        body,
        headers: { "Content-Type": "application/x-www-form-urlencoded" },
        credentials: "same-origin",
    });
    return response.ok;
}
/**
 * Save the grid as it is on screen (so typed-but-unsaved edits survive the
 * page reload), then move, then reload to show the result.
 */
async function moveActivities(gridForm, fromDay, toDay) {
    const moveForm = document.getElementById(`move-${fromDay}`);
    if (!(moveForm instanceof HTMLFormElement)) {
        say(MOVE_FAILED);
        return;
    }
    say("Moving…");
    try {
        if (!(await post(gridForm.action, encode(gridForm)))) {
            say(SAVE_FAILED);
            return;
        }
        const body = encode(moveForm);
        body.set("target_day", toDay);
        if (!(await post(moveForm.action, body))) {
            say(MOVE_FAILED);
            return;
        }
    }
    catch {
        say(MOVE_FAILED);
        return;
    }
    window.location.reload();
}
function enableDrag(grid, gridForm) {
    let dragging = null;
    let hover = null;
    const clear = () => {
        dragging = null;
        hover?.classList.remove("drop-target");
        hover = null;
        grid.querySelectorAll(".dragging").forEach((el) => el.classList.remove("dragging"));
    };
    grid.addEventListener("dragstart", (event) => {
        const handle = event.target instanceof Element ? event.target.closest(".drag-handle") : null;
        if (!handle || !event.dataTransfer)
            return;
        dragging = handle.dataset.day ?? null;
        event.dataTransfer.setData("text/plain", dragging ?? "");
        event.dataTransfer.effectAllowed = "move";
        rowOf(handle)?.classList.add("dragging");
    });
    grid.addEventListener("dragover", (event) => {
        const row = rowOf(event.target);
        if (dragging === null || !row || row.dataset.day === dragging)
            return;
        event.preventDefault(); // marks the row as a valid drop target
        if (event.dataTransfer)
            event.dataTransfer.dropEffect = "move";
        if (hover !== row) {
            hover?.classList.remove("drop-target");
            hover = row;
            row.classList.add("drop-target");
        }
    });
    grid.addEventListener("drop", (event) => {
        const row = rowOf(event.target);
        const from = dragging;
        clear();
        if (from === null || !row || !row.dataset.day || row.dataset.day === from)
            return;
        event.preventDefault();
        const to = row.dataset.day;
        const fromLabel = grid.querySelector(`tr[data-day="${from}"]`)?.dataset.label ?? "that night";
        const toLabel = row.dataset.label ?? "that date";
        const ok = window.confirm(`Move the activities from ${fromLabel} to ${toLabel}?\n\n` +
            `If ${toLabel} already has activities, the two dates swap. ` +
            `Any other edits on this page are saved first.`);
        if (ok)
            void moveActivities(gridForm, from, to);
    });
    grid.addEventListener("dragend", clear);
}
// The "js" class lets the stylesheet show the drag handles only when they work.
document.documentElement.classList.add("js");
const grid = document.querySelector("table.grid");
const gridForm = document.querySelector("form.grid-form");
if (grid && gridForm)
    enableDrag(grid, gridForm);
export {};
