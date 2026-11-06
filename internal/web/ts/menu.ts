// Menus in the month's menu bar are <details> elements, so they open and close
// without script. This makes them behave like a menu: opening one closes the
// others, a click anywhere else or Escape closes it, and choosing an item closes it.

const menus = [...document.querySelectorAll<HTMLDetailsElement>(".menubar details")];

function closeAll(except?: HTMLDetailsElement): void {
  for (const m of menus) if (m !== except) m.open = false;
}

for (const m of menus) {
  m.addEventListener("toggle", () => {
    if (m.open) closeAll(m);
  });
}

document.addEventListener("click", (event) => {
  const target = event.target;
  if (!(target instanceof Node)) return;
  for (const m of menus) {
    if (m.open && (!m.contains(target) || (target instanceof Element && target.closest("a")))) m.open = false;
  }
});

document.addEventListener("keydown", (event) => {
  if (event.key !== "Escape") return;
  const open = menus.find((m) => m.open);
  if (!open) return;
  open.open = false;
  open.querySelector("summary")?.focus();
});

export {};
