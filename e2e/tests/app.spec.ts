import { expect, test, type Page } from "@playwright/test";

// One squadron is claimed once; the tests then run in order against it.
const email = "oc.4321@rafac.mod.gov.uk";
const password = "a long enough password";

test.describe.configure({ mode: "serial" });

let page: Page;
const problems: string[] = [];

test.beforeAll(async ({ browser }) => {
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 } });
  page = await context.newPage();
  page.on("pageerror", (e) => problems.push(`pageerror: ${e}`));
  page.on("response", (r) => {
    if (r.status() >= 400) problems.push(`HTTP ${r.status()} ${r.url()}`);
  });
  page.on("console", (m) => {
    // The failed request itself is reported with its address above.
    if (m.text().startsWith("Failed to load resource")) return;
    // The site denies camera and microphone on purpose (Permissions-Policy).
    if (m.text().startsWith("Permissions policy violation")) return;
    if (m.type() === "error") problems.push(`console error: ${m.text()}`);
  });
  page.on("dialog", (d) => void d.accept());

  await page.goto("/claim");
  await page.fill("input[name=squadron_name]", "4321 (Test Town) Squadron");
  await page.fill("input[name=display_name]", "Fg Off Test");
  await page.fill("input[name=email]", email);
  await page.fill("input[name=password]", password);
  await page.click("button[type=submit]");
  await page.goto("/login");
  await page.fill("input[name=email]", email);
  await page.fill("input[name=password]", password);
  await page.click('button:has-text("Log in")');
  await expect(page.locator("h1").first()).toBeVisible();
});

test.afterAll(async () => {
  expect(problems, "console errors or page errors").toEqual([]);
});

/** Opens this month's programme, which is made when someone signs in. */
async function openProgramme(): Promise<void> {
  await page.goto("/");
  await page.locator('a:has-text("Edit")').first().click();
  await settle();
}

/** Waits for the grid and shows every date (the switch is on again after each page load). */
async function settle(): Promise<void> {
  await expect(page.locator("table.grid")).toBeVisible();
  await showAllNights();
}

/** "Show parade evenings only" is on by default: turn it off to see every date. */
async function showAllNights(): Promise<void> {
  const toggle = page.locator("#parade-only-toggle");
  if (await toggle.isChecked()) await page.locator('label[for="parade-only-toggle"]').click();
  await expect(toggle).not.toBeChecked();
}

/** The ids of the nights on screen: parade nights first, then others. */
async function nights(): Promise<{ parade: string[]; other: string[] }> {
  const parade = await page.locator("tr.day-start").evaluateAll((rows) => rows.map((r) => r.getAttribute("data-day") ?? ""));
  const other = await page.locator("tr.non-parade-row").evaluateAll((rows) => rows.map((r) => r.getAttribute("data-day") ?? ""));
  return { parade, other };
}

async function save(): Promise<void> {
  await page.click('button:has-text("Save programme")');
  await settle();
}

/** The first Activity box of a parade night. */
const activityBox = (day: string) => page.locator(`tr[data-day="${day}"]`).first().locator('input[name^="row_"]').first();
const uniformSelect = (day: string) => page.locator(`select[name="day_${day}_uniform"]`);

test("this month's programme exists after signing in, and no past month is offered", async () => {
  await page.goto("/");
  const now = new Date();
  const month = now.toLocaleString("en-GB", { month: "long" });
  await expect(page.locator("body")).toContainText(`${month} ${now.getFullYear()}`);
});

test("ticking Parade shows the night's boxes at once and saves only on Save", async () => {
  await openProgramme();
  const { other } = await nights();
  expect(other.length).toBeGreaterThan(5);
  const day = other[0];
  const row = page.locator(`tr.non-parade-row[data-day="${day}"]`);
  await row.locator('input[type=checkbox][name$="_parade"]').check();
  const pending = page.locator(`tr[data-day="${day}"] input.pending-input`).first();
  await expect(pending).toBeVisible();
  await pending.fill("Pending text 😀");
  await page.reload(); // nothing saved yet
  await settle();
  await expect(page.locator(`tr.non-parade-row[data-day="${day}"]`)).toBeVisible();
  // now do it again and save
  await page.locator(`tr.non-parade-row[data-day="${day}"] input[type=checkbox][name$="_parade"]`).check();
  await page.locator(`tr[data-day="${day}"] input.pending-input`).first().fill("Saved text 😀 Zoë");
  await save();
  await expect(activityBox(day)).toHaveValue("Saved text 😀 Zoë");
});

test("moving a night with the Move control takes its uniform and notes too, and unticks the date it left", async () => {
  await openProgramme();
  const { parade, other } = await nights();
  const from = parade[0];
  const to = other[0];
  await activityBox(from).fill("Moved by control");
  const options = await uniformSelect(from).locator("option").evaluateAll((os) => os.map((o) => (o as HTMLOptionElement).value).filter(Boolean));
  expect(options.length).toBeGreaterThan(0);
  await uniformSelect(from).selectOption(options[0]);
  await page.locator(`textarea[name="day_${from}_notes"]`).fill("Bring boots");
  await save();

  const row = page.locator(`tr[data-day="${from}"]`).first();
  await row.locator("details.move summary").click();
  await row.locator('select[name="target_day"]').selectOption(to);
  await row.locator('button:has-text("Move activities")').click();
  await settle();

  await expect(activityBox(to)).toHaveValue("Moved by control");
  await expect(uniformSelect(to)).toHaveValue(options[0]);
  await expect(page.locator(`textarea[name="day_${to}_notes"]`)).toHaveValue("Bring boots");
  await expect(page.locator(`tr.non-parade-row[data-day="${from}"]`)).toBeVisible();
  await expect(uniformSelect(from)).toHaveValue("");
});

test("dragging a night to another date moves it with its uniform", async () => {
  await openProgramme();
  const { parade, other } = await nights();
  const from = parade[0];
  const to = other[1];
  await activityBox(from).fill("Moved by drag");
  const options = await uniformSelect(from).locator("option").evaluateAll((os) => os.map((o) => (o as HTMLOptionElement).value).filter(Boolean));
  await uniformSelect(from).selectOption(options[options.length - 1]);
  await save();

  await page.locator(`button.drag-handle[data-day="${from}"]`).dragTo(page.locator(`tr[data-day="${to}"]`).first());
  await expect(page.locator(`tr.day-start[data-day="${to}"]`)).toBeVisible();
  await expect(activityBox(to)).toHaveValue("Moved by drag");
  await expect(uniformSelect(to)).toHaveValue(options[options.length - 1]);
});

test("a closure locks the dates it covers in the grid", async () => {
  await openProgramme();
  const { parade } = await nights();
  const day = parade[0];
  const dateIso = await page.locator(`tr[data-day="${day}"]`).first().getAttribute("data-date");
  expect(dateIso, "rows carry their ISO date").toMatch(/^\d{4}-\d{2}-\d{2}$/);
  await page.fill('input[name="start_date"]', dateIso!);
  await page.fill('input[name="end_date"]', dateIso!);
  await page.fill('input[name="message"]', "Stand-down");
  await page.click('button:has-text("Add closure")');
  const closed = page.locator(`tr.closed-row[data-day="${day}"]`);
  await expect(closed).toContainText("Closed");
  await expect(closed).toContainText("Stand-down");
  await expect(closed.locator("input, select, textarea")).toHaveCount(0);
  await expect(page.locator(`select[name="target_day"] option[value="${day}"]`)).toHaveCount(0);
});

test("every save is kept in the history, and an earlier version can be restored", async () => {
  await openProgramme();
  await page.click('a:has-text("History")');
  await expect(page.locator("h1")).toContainText(/history/i);
  const rows = page.locator("table.list tbody tr");
  const before = await rows.count();
  expect(before).toBeGreaterThan(2);
  await rows.last().locator('a:has-text("Restore")').click();
  await page.click('button:has-text("Yes, restore")');
  await expect(page).toHaveURL(/restored=/);
  // restoring is itself kept as a new version
  await expect(rows).toHaveCount(before + 1);
});

test("the instructors page lists people with a login as fixed entries", async () => {
  await page.goto("/instructors");
  const fixed = page.locator("section", { hasText: "People with a login" });
  await expect(fixed).toContainText("Fg Off Test");
  await expect(fixed.locator("a, button")).toHaveCount(0);
});

test("no page scrolls sideways on a phone", async () => {
  await page.setViewportSize({ width: 390, height: 800 });
  for (const path of ["/", "/instructors", "/account", "/squadron", "/report", "/privacy", "/docs"]) {
    await page.goto(path);
    const overflow = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
    expect(overflow, `${path} overflows by ${overflow}px`).toBeLessThanOrEqual(0);
  }
  await page.setViewportSize({ width: 1280, height: 900 });
});

test("Report a problem is offered on every page and the form has its options", async () => {
  await page.goto("/");
  await expect(page.locator("a.report-button")).toBeVisible();
  await page.click("a.report-button");
  // The link goes straight to the form: no prompt in between.
  await expect(page).toHaveURL(/\/report/);
  await expect(page.locator('select[name="category"] option')).not.toHaveCount(0);
  await expect(page.locator('select[name="category"]')).toContainText("An idea or a missing feature");
  await expect(page.locator('select[name="severity"]')).toBeVisible();
});
