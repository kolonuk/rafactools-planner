// Click-to-edit lists (see the .ie-* rules in app.css). The switch between
// reading and editing is a hidden checkbox and works without this script; this
// adds Escape to cancel (also undoing what was typed) and moves the cursor into
// the first box when editing starts.
for (const toggle of document.querySelectorAll("input.ie-toggle")) {
    const row = toggle.closest(".ie-row");
    if (!row)
        continue;
    toggle.addEventListener("change", () => {
        if (!toggle.checked)
            return;
        const box = row.querySelector("input.ie-edit, .ie-edit input");
        box?.focus();
        box?.select();
    });
    row.addEventListener("keydown", (event) => {
        if (!(event instanceof KeyboardEvent) || event.key !== "Escape" || !toggle.checked)
            return;
        event.preventDefault();
        for (const box of row.querySelectorAll("input[form]"))
            box.value = box.defaultValue;
        toggle.checked = false;
    });
}
export {};
