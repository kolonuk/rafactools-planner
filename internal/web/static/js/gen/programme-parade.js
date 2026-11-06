// Ticking "Parade" on a night that is not a parade night yet shows the boxes
// that night will have once saved (an activity and an instructor box for each
// class), right away, so the row looks as it will. Nothing is saved until "Save
// programme" is pressed; unticking puts the row back. Without this script the
// tickbox works as before and the boxes appear after saving.
const NO_PARADE_ROW = "non-parade-row";
function setPending(row, on) {
    for (const input of row.querySelectorAll("input.pending-input")) {
        input.hidden = !on;
        input.disabled = !on; // a disabled box is not submitted
    }
    // The row now counts as a parade night, so "parade nights only" keeps it.
    row.classList.toggle(NO_PARADE_ROW, !on);
    row.classList.toggle("day-start", on);
}
for (const box of document.querySelectorAll("tr.non-parade-row input[type=checkbox][name$=_parade]")) {
    const row = box.closest("tr");
    if (!(row instanceof HTMLTableRowElement) || !row.querySelector(".pending-input"))
        continue; // only nights that were not parade nights
    box.addEventListener("change", () => setPending(row, box.checked));
    setPending(row, box.checked);
}
export {};
