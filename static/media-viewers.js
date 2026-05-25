(() => {
  const PLYR_CSS = "https://cdn.plyr.io/3.7.8/plyr.css";
  const PLYR_JS = "https://cdn.plyr.io/3.7.8/plyr.polyfilled.js";
  const PDFJS_URL = "https://cdn.jsdelivr.net/npm/pdfjs-dist@5.7.284/build/pdf.mjs";
  const PDFJS_WORKER_URL = "https://cdn.jsdelivr.net/npm/pdfjs-dist@5.7.284/build/pdf.worker.mjs";
  const MODEL_VIEWER_URL = "https://unpkg.com/@google/model-viewer/dist/model-viewer.min.js";

  let plyrPromise = null;
  let pdfPromise = null;
  let modelPromise = null;

  function loadStyle(href) {
    if (document.querySelector('link[href="' + href + '"]')) return;
    const link = document.createElement("link");
    link.rel = "stylesheet";
    link.href = href;
    document.head.appendChild(link);
  }

  function loadScript(src) {
    return new Promise((resolve, reject) => {
      if (document.querySelector('script[src="' + src + '"]')) {
        resolve();
        return;
      }
      const script = document.createElement("script");
      script.src = src;
      script.defer = true;
      script.onload = resolve;
      script.onerror = reject;
      document.head.appendChild(script);
    });
  }

  function loadModuleScript(src) {
    return new Promise((resolve, reject) => {
      if (document.querySelector('script[src="' + src + '"]')) {
        resolve();
        return;
      }
      const script = document.createElement("script");
      script.type = "module";
      script.src = src;
      script.onload = resolve;
      script.onerror = reject;
      document.head.appendChild(script);
    });
  }

  function loadPlyr() {
    if (!plyrPromise) {
      loadStyle(PLYR_CSS);
      plyrPromise = loadScript(PLYR_JS).then(() => window.Plyr);
    }
    return plyrPromise;
  }

  async function loadPDFJS() {
    if (!pdfPromise) {
      pdfPromise = import(PDFJS_URL).then((pdfjs) => {
        pdfjs.GlobalWorkerOptions.workerSrc = PDFJS_WORKER_URL;
        return pdfjs;
      });
    }
    return pdfPromise;
  }

  function loadModelViewer() {
    if (!modelPromise) {
      modelPromise = customElements.get("model-viewer")
        ? Promise.resolve()
        : loadModuleScript(MODEL_VIEWER_URL);
    }
    return modelPromise;
  }

  async function initPlayers(root) {
    const media = Array.from(root.querySelectorAll("audio.md-media, video.md-media"))
      .filter((el) => !el.dataset.viewerReady);
    if (media.length === 0) return;
    media.forEach((el) => {
      el.dataset.viewerReady = "pending";
    });
    try {
      const Plyr = await loadPlyr();
      if (!Plyr) throw new Error("Plyr unavailable");
      media.forEach((el) => {
        if (el.dataset.viewerReady === "done") return;
        new Plyr(el, {
          controls: [
            "play",
            "progress",
            "current-time",
            "duration",
            "mute",
            "volume",
            "fullscreen",
          ],
        });
        el.dataset.viewerReady = "done";
      });
    } catch (err) {
      media.forEach((el) => {
        el.dataset.viewerReady = "native";
      });
      console.warn("Plyr failed, native media controls remain active", err);
    }
  }

  async function renderPDF(box) {
    if (box.dataset.viewerReady) return;
    box.dataset.viewerReady = "pending";
    const src = box.dataset.pdfSrc;
    const canvas = box.querySelector("canvas");
    const status = box.querySelector("[data-pdf-status]");
    const pageLabel = box.querySelector("[data-pdf-page]");
    const prev = box.querySelector("[data-pdf-prev]");
    const next = box.querySelector("[data-pdf-next]");
    const zoomIn = box.querySelector("[data-pdf-zoom-in]");
    const zoomOut = box.querySelector("[data-pdf-zoom-out]");
    if (!src || !canvas) return;

    let doc = null;
    let pageNumber = 1;
    let zoom = 1;
    let renderTask = null;

    function setStatus(text) {
      if (status) status.textContent = text;
    }

    function updateControls() {
      if (!doc) return;
      if (pageLabel) pageLabel.textContent = pageNumber + " / " + doc.numPages;
      if (prev) prev.disabled = pageNumber <= 1;
      if (next) next.disabled = pageNumber >= doc.numPages;
      if (zoomOut) zoomOut.disabled = zoom <= 0.7;
      if (zoomIn) zoomIn.disabled = zoom >= 2;
    }

    async function paint() {
      if (!doc) return;
      setStatus("Рендеринг...");
      if (renderTask) {
        try {
          renderTask.cancel();
        } catch (_) {}
      }
      const page = await doc.getPage(pageNumber);
      const base = page.getViewport({ scale: 1 });
      const width = Math.max(280, box.querySelector(".md-pdf-stage").clientWidth || box.clientWidth || 720);
      const fit = Math.min(1.7, Math.max(0.35, width / base.width));
      const viewport = page.getViewport({ scale: fit * zoom });
      const ratio = window.devicePixelRatio || 1;
      canvas.width = Math.floor(viewport.width * ratio);
      canvas.height = Math.floor(viewport.height * ratio);
      canvas.style.width = Math.floor(viewport.width) + "px";
      canvas.style.height = Math.floor(viewport.height) + "px";
      const ctx = canvas.getContext("2d");
      ctx.setTransform(ratio, 0, 0, ratio, 0, 0);
      renderTask = page.render({ canvasContext: ctx, viewport });
      await renderTask.promise;
      renderTask = null;
      updateControls();
      setStatus("");
      box.dataset.viewerReady = "done";
    }

    try {
      setStatus("Загрузка PDF...");
      const pdfjs = await loadPDFJS();
      doc = await pdfjs.getDocument(src).promise;
      updateControls();
      await paint();
    } catch (err) {
      box.dataset.viewerReady = "fallback";
      setStatus("PDF не удалось отрисовать. Используйте ссылку открытия.");
      console.warn("PDF.js failed", err);
      return;
    }

    prev?.addEventListener("click", () => {
      if (pageNumber > 1) {
        pageNumber -= 1;
        paint();
      }
    });
    next?.addEventListener("click", () => {
      if (doc && pageNumber < doc.numPages) {
        pageNumber += 1;
        paint();
      }
    });
    zoomIn?.addEventListener("click", () => {
      zoom = Math.min(2, zoom + 0.15);
      paint();
    });
    zoomOut?.addEventListener("click", () => {
      zoom = Math.max(0.7, zoom - 0.15);
      paint();
    });

    if ("ResizeObserver" in window) {
      let resizeTimer = null;
      const observer = new ResizeObserver(() => {
        clearTimeout(resizeTimer);
        resizeTimer = setTimeout(paint, 140);
      });
      observer.observe(box);
    }
  }

  async function initPDFs(root) {
    const boxes = Array.from(root.querySelectorAll(".md-pdf[data-pdf-src]"));
    for (const box of boxes) {
      renderPDF(box);
    }
  }

  async function initModels(root) {
    const models = root.querySelectorAll("model-viewer");
    if (models.length === 0) return;
    try {
      await loadModelViewer();
    } catch (err) {
      console.warn("model-viewer failed, fallback links remain active", err);
    }
  }

  function init(root = document) {
    initPlayers(root);
    initPDFs(root);
    initModels(root);
  }

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", () => init());
  } else {
    init();
  }

  const observer = new MutationObserver((records) => {
    for (const record of records) {
      for (const node of record.addedNodes) {
        if (node.nodeType === Node.ELEMENT_NODE) init(node);
      }
    }
  });
  observer.observe(document.documentElement, { childList: true, subtree: true });

  window.DocsHubMediaViewers = { init };
})();
