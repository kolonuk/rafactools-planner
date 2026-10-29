// Ticking "Parade" on a night that is not a parade night yet shows the boxes
// that night will have once saved (the Activity and Instructor rows), right
// away, so the row looks as it will. Nothing is saved until "Save programme"
// is pressed; unticking puts the row back. Without this script the tickbox
// works as before and the boxes appear after saving.

const NO_PARADE_ROW = "non-parade-row";

function setPending(row: HTMLTableRowElement, on: boolean): void {
  const extra = row.nextElementSibling instanceof HTMLTableRowElement && row.nextElementSibling.classList.contains("pending-row") ? row.nextElementSibling : null;

  const none = row.querySelector<HTMLElement>(".pending-none");
  const label = row.querySelector<HTMLElement>(".pending-label");
  if (none) none.hidden = on;
  if (label) label.hidden = !on;

  const inputs = [...row.querySelectorAll<HTMLInputElement>("input.pending-input"), ...(extra ? extra.querySelectorAll<HTMLInputElement>("input.pending-input") : [])];
  for (const input of inputs) {
    input.hidden = !on;
    input.disabled = !on; // a disabled box is not submitted
  }
  if (extra) extra.hidden = !on;

  // The notes box spans both lines, as it does after saving, and the row
  // counts as a parade night (so "show parade evenings only" keeps it).
  const notes = row.querySelector<HTMLTableCellElement>("td.pending-notes");
  if (notes) notes.rowSpan = on && extra ? 2 : 1;
  row.classList.toggle(NO_PARADE_ROW, !on);
  row.classList.toggle("day-start", on);
}

for (const box of document.querySelectorAll<HTMLInputElement>("tr.non-parade-row input[type=checkbox][name$=_parade], tr.day-start input[type=checkbox][name$=_parade]")) {
  const row = box.closest("tr");
  if (!(row instanceof HTMLTableRowElement) || !row.querySelector(".pending-label")) continue; // only nights that were not parade nights
  box.addEventListener("change", () => setPending(row, box.checked));
  setPending(row, box.checked);
}
