// The "Report a problem" form: shows the screenshot taken just before this
// page opened (or one the person attaches), lets them black out private
// parts when the site allows it, and fills in the context that goes with the
// report. The form still works as a plain form with this script switched off.

import {
  Box,
  KEY_ERRORS,
  KEY_PAGE,
  KEY_SHOT,
  KEY_TRAIL,
  SHOT_MAX_AGE_MS,
  ErrorEntry,
  PageInfo,
  Shot,
  TrailEntry,
  load,
  pageInfo,
} from "./report-shared.js";

const MAX_WIDTH = 1600;
const MIN_BOX = 6;
const MAX_FILE_BYTES = 3 * 1024 * 1024; // the server's limit for a screenshot

function el<T extends HTMLElement>(id: string, type: { new (): T }): T | null {
  const found = document.getElementById(id);
  return found instanceof type ? found : null;
}

const form = el("report-form", HTMLFormElement);
const canvas = el("shot-canvas", HTMLCanvasElement);
const editor = document.getElementById("shot-editor");
const status = document.getElementById("shot-status");
const include = el("shot-include", HTMLInputElement);
const fileInput = el("shot-file", HTMLInputElement);
const contextField = el("client-context", HTMLInputElement);
const undoButton = el("mask-undo", HTMLButtonElement);
const clearButton = el("mask-clear", HTMLButtonElement);
const captureButton = el("shot-capture", HTMLButtonElement);

if (form && canvas && editor && include && fileInput && contextField) {
  document.documentElement.classList.add("js");
  init(form, canvas, editor, include, fileInput, contextField);
}

function init(
  form: HTMLFormElement,
  canvas: HTMLCanvasElement,
  editor: HTMLElement,
  include: HTMLInputElement,
  fileInput: HTMLInputElement,
  contextField: HTMLInputElement,
): void {
  const maskingAllowed = form.dataset.masking === "1";
  const ctx = canvas.getContext("2d");
  if (!ctx) return;

  let image: HTMLImageElement | null = null;
  let method = "none";
  const boxes: Box[] = []; // black boxes, in picture pixels
  let drag: { x: number; y: number } | null = null;
  let preview: Box | null = null;

  const redraw = (): void => {
    if (!image) return;
    ctx.drawImage(image, 0, 0, canvas.width, canvas.height);
    ctx.fillStyle = "#000";
    for (const b of boxes) ctx.fillRect(b.x, b.y, b.w, b.h);
    if (preview) {
      ctx.fillStyle = "rgba(0, 0, 0, 0.55)";
      ctx.fillRect(preview.x, preview.y, preview.w, preview.h);
    }
  };

  const show = (img: HTMLImageElement, how: string, autoBoxes: Box[]): void => {
    // Never let a huge picture through: scale it down to the same size the
    // automatic capture uses.
    const scale = Math.min(1, MAX_WIDTH / img.naturalWidth);
    canvas.width = Math.max(1, Math.round(img.naturalWidth * scale));
    canvas.height = Math.max(1, Math.round(img.naturalHeight * scale));
    image = img;
    method = how;
    boxes.length = 0;
    if (maskingAllowed) boxes.push(...autoBoxes); // pre-black-out parts the page marked private
    editor.hidden = false;
    include.checked = true;
    redraw();
  };

  const loadImage = (src: string, how: string, autoBoxes: Box[], onFail: () => void): void => {
    const img = new Image();
    img.onload = () => show(img, how, autoBoxes);
    img.onerror = onFail;
    img.src = src;
  };

  // 1. The screenshot taken just before this page opened.
  const shot = load<Shot>(KEY_SHOT);
  if (shot && Date.now() - shot.at < SHOT_MAX_AGE_MS) {
    loadImage(shot.dataUrl, shot.method, shot.boxes, () => setStatus("The screenshot could not be shown. You can attach one below."));
    setStatus("");
  } else {
    setStatus(
      window.isSecureContext && typeof navigator.mediaDevices?.getDisplayMedia === "function"
        ? "No screenshot yet. Use Take a screenshot now, or attach a picture, below."
        : "This browser can't take a screenshot here (it needs a secure https:// address), so please attach a picture instead.",
    );
  }

  // 1b. Or take one now: the browser asks which tab or window to share. Only
  //     offered where the browser can do it (a secure https:// address).
  const canCapture = window.isSecureContext && typeof navigator.mediaDevices?.getDisplayMedia === "function";
  if (captureButton) {
    if (!canCapture) {
      const item = captureButton.closest("li");
      if (item) item.style.display = "none"; // (the hidden attribute loses to the .js-only rule)
    } else {
      captureButton.addEventListener("click", () => {
        void takeScreenshotNow(captureButton, setStatus, (dataUrl) =>
          loadImage(dataUrl, "chosen-capture", [], () => setStatus("The picture could not be shown. Try attaching one instead.")),
        );
      });
    }
  }

  // 2. Or a file the person attaches: it goes through the same preview, so it
  //    can be masked in exactly the same way.
  fileInput.addEventListener("change", () => {
    const file = fileInput.files?.[0];
    if (!file) return;
    if (file.size > MAX_FILE_BYTES) {
      setStatus("That file is larger than 3 MB. Please attach a smaller screenshot.");
      fileInput.value = "";
      return;
    }
    // A data: URL, not a blob: URL: the site's content security policy allows
    // images from the site itself and data: only.
    const reader = new FileReader();
    reader.onload = () => {
      if (typeof reader.result === "string") {
        loadImage(reader.result, "uploaded-file", [], () => setStatus("That file could not be read as an image."));
      }
    };
    reader.onerror = () => setStatus("That file could not be read.");
    reader.readAsDataURL(file);
  });

  // 3. Drawing black boxes (only when the site allows masking).
  if (maskingAllowed) {
    canvas.style.touchAction = "none";
    canvas.style.cursor = "crosshair";
    const at = (e: PointerEvent): { x: number; y: number } => {
      const r = canvas.getBoundingClientRect();
      return {
        x: Math.min(canvas.width, Math.max(0, ((e.clientX - r.left) / r.width) * canvas.width)),
        y: Math.min(canvas.height, Math.max(0, ((e.clientY - r.top) / r.height) * canvas.height)),
      };
    };
    const rect = (a: { x: number; y: number }, b: { x: number; y: number }): Box => ({
      x: Math.min(a.x, b.x),
      y: Math.min(a.y, b.y),
      w: Math.abs(a.x - b.x),
      h: Math.abs(a.y - b.y),
    });
    canvas.addEventListener("pointerdown", (e) => {
      drag = at(e);
      canvas.setPointerCapture(e.pointerId);
    });
    canvas.addEventListener("pointermove", (e) => {
      if (!drag) return;
      preview = rect(drag, at(e));
      redraw();
    });
    const finish = (e: PointerEvent): void => {
      if (!drag) return;
      const b = rect(drag, at(e));
      drag = null;
      preview = null;
      if (b.w >= MIN_BOX && b.h >= MIN_BOX) boxes.push(b);
      redraw();
    };
    canvas.addEventListener("pointerup", finish);
    canvas.addEventListener("pointercancel", () => {
      drag = null;
      preview = null;
      redraw();
    });
    undoButton?.addEventListener("click", () => {
      boxes.pop();
      redraw();
    });
    clearButton?.addEventListener("click", () => {
      boxes.length = 0;
      redraw();
    });
  }

  // 4. Sending: put the (masked) picture into the file field, and the context
  //    into the hidden field, then submit the ordinary form.
  let sending = false;
  form.addEventListener("submit", (event) => {
    if (sending) return;
    event.preventDefault();
    if (!form.reportValidity()) return;
    sending = true;
    contextField.value = JSON.stringify(buildContext(Boolean(image) && include.checked, method, maskingAllowed, boxes.length));

    const submit = (): void => form.submit();
    if (image && include.checked) {
      redraw(); // the preview box is gone; only real boxes are burned in
      canvas.toBlob(
        (blob) => {
          if (blob) {
            const transfer = new DataTransfer();
            transfer.items.add(new File([blob], "screenshot.jpg", { type: "image/jpeg" }));
            fileInput.files = transfer.files;
          } else {
            fileInput.value = "";
          }
          submit();
        },
        "image/jpeg",
        0.85,
      );
    } else {
      fileInput.value = ""; // they chose not to send a picture
      submit();
    }
  });
}

/**
 * Asks the browser to share a tab or window and grabs one frame. The person
 * picks what to share (the browser always asks), so nothing is captured
 * without them choosing it.
 */
async function takeScreenshotNow(button: HTMLButtonElement, say: (text: string) => void, done: (dataUrl: string) => void): Promise<void> {
  button.disabled = true;
  say("Choose the tab or window to share in the box your browser just opened…");
  let stream: MediaStream | null = null;
  try {
    stream = await navigator.mediaDevices.getDisplayMedia({ video: true, audio: false });
    const video = document.createElement("video");
    video.muted = true;
    video.playsInline = true;
    video.srcObject = stream;
    await video.play();
    await new Promise<void>((resolve) => {
      if (typeof video.requestVideoFrameCallback === "function") video.requestVideoFrameCallback(() => resolve());
      else window.setTimeout(resolve, 300);
    });
    const scale = Math.min(1, MAX_WIDTH / video.videoWidth);
    const grab = document.createElement("canvas");
    grab.width = Math.max(1, Math.round(video.videoWidth * scale));
    grab.height = Math.max(1, Math.round(video.videoHeight * scale));
    const grabCtx = grab.getContext("2d");
    if (!grabCtx) throw new Error("no canvas");
    grabCtx.drawImage(video, 0, 0, grab.width, grab.height);
    done(grab.toDataURL("image/jpeg", 0.85));
    say("");
  } catch {
    say("No screenshot was taken (you may have cancelled). You can try again or attach a picture.");
  } finally {
    stream?.getTracks().forEach((t) => t.stop());
    button.disabled = false;
  }
}

function setStatus(text: string): void {
  if (status) status.textContent = text;
}

function buildContext(sent: boolean, method: string, maskingAllowed: boolean, boxCount: number): Record<string, unknown> {
  const page = load<PageInfo>(KEY_PAGE) ?? pageInfo();
  return {
    page: page.path,
    title: page.title,
    heading: page.heading,
    messages: page.messages,
    viewport_w: page.viewportW,
    viewport_h: page.viewportH,
    dpr: page.dpr,
    touch: page.touch,
    language: navigator.language,
    timezone: Intl.DateTimeFormat().resolvedOptions().timeZone,
    color_scheme: window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light",
    user_agent: navigator.userAgent,
    online: navigator.onLine,
    request_id: page.requestId,
    timing_ms: page.timing,
    trail: (load<TrailEntry[]>(KEY_TRAIL) ?? []).map((t) => ({ at: t.at, kind: t.kind, target: t.target, path: t.path })),
    errors: (load<ErrorEntry[]>(KEY_ERRORS) ?? []).map((e) => ({ kind: e.kind, message: e.message, source: e.source, line: e.line, col: e.col })),
    screenshot: { captured: sent, method: sent ? method : "none", masked: sent && maskingAllowed && boxCount > 0, boxes: boxCount },
  };
}
