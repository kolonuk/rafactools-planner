// Keeps the little colour dot beside a date in step with the Uniform dropdown
// on that night, before the programme is saved. Without this script the dot
// updates after saving.
function dot(row) {
    return row.querySelector("svg.swatch circle");
}
for (const select of document.querySelectorAll("table.grid select[name$=_uniform]")) {
    select.addEventListener("change", () => {
        const row = select.closest("tr");
        if (!(row instanceof HTMLTableRowElement))
            return;
        const hex = select.selectedOptions[0]?.dataset.hex ?? "";
        const existing = dot(row);
        if (existing) {
            existing.setAttribute("fill", hex || "transparent");
            return;
        }
        if (!hex)
            return;
        // The night had no colour yet, so there is no dot to change: add one.
        const cell = row.querySelector("td.date-cell");
        if (!cell)
            return;
        const ns = "http://www.w3.org/2000/svg";
        const svg = document.createElementNS(ns, "svg");
        svg.setAttribute("class", "swatch");
        svg.setAttribute("width", "12");
        svg.setAttribute("height", "12");
        svg.setAttribute("viewBox", "0 0 12 12");
        svg.setAttribute("aria-hidden", "true");
        const circle = document.createElementNS(ns, "circle");
        circle.setAttribute("cx", "6");
        circle.setAttribute("cy", "6");
        circle.setAttribute("r", "5.5");
        circle.setAttribute("fill", hex);
        circle.setAttribute("stroke", "currentColor");
        circle.setAttribute("stroke-opacity", "0.35");
        svg.appendChild(circle);
        cell.querySelector(".date-text")?.after(svg);
    });
}
export {};
