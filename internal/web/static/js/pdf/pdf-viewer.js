import { GlobalWorkerOptions, TextLayer, getDocument } from 'https://cdn.jsdelivr.net/npm/pdfjs-dist@6.1.200/build/pdf.min.mjs';

GlobalWorkerOptions.workerSrc = 'https://cdn.jsdelivr.net/npm/pdfjs-dist@6.1.200/build/pdf.worker.min.mjs';

const ready = (callback) => {
  if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', callback, { once: true });
  else callback();
};

ready(() => {
  const container = document.getElementById('pdfViewerContainer');
  const stage = document.getElementById('pdfStage');
  const pageShell = document.getElementById('pdfPage');
  const canvas = document.getElementById('pdfCanvas');
  const textLayerElement = document.getElementById('pdfTextLayer');
  const loading = document.getElementById('pdfLoading');
  const errorPanel = document.getElementById('pdfError');
  const input = document.getElementById('pdfPageInput');
  const totalElement = document.getElementById('pdfTotalPages');
  const previous = document.getElementById('pdfPrevPage');
  const next = document.getElementById('pdfNextPage');
  const zoomOut = document.getElementById('pdfZoomOut');
  const zoomIn = document.getElementById('pdfZoomIn');
  const fitWidth = document.getElementById('pdfFitWidth');
  const scaleLabel = document.getElementById('pdfScaleLabel');
  const searchForm = document.getElementById('pdfSearchForm');
  const searchInput = document.getElementById('pdfSearchInput');
  const searchNext = document.getElementById('pdfSearchNext');
  const searchStatus = document.getElementById('pdfSearchStatus');
  if (!container || !stage || !pageShell || !canvas || !textLayerElement || !input) return;

  const source = container.dataset.pdfUrl;
  let pdfDocument = null;
  let currentPage = 1;
  let zoom = 1;
  let renderTask = null;
  let textLayer = null;
  let renderGeneration = 0;
  let searchGeneration = 0;
  let searchMatches = [];
  let searchMatchIndex = -1;
  const pageText = new Map();

  const normalize = (value) => String(value || '').toLocaleLowerCase('ru').replace(/\s+/g, ' ').trim();
  const totalPages = () => Math.max(1, pdfDocument?.numPages || Number(container.dataset.totalPages || 1));
  const clampPage = (value) => Math.min(totalPages(), Math.max(1, Number(value) || 1));
  const updateControls = () => {
    input.value = String(currentPage);
    input.max = String(totalPages());
    if (totalElement) totalElement.textContent = String(totalPages());
    if (previous) previous.disabled = currentPage <= 1;
    if (next) next.disabled = currentPage >= totalPages();
  };

  async function renderPage() {
    if (!pdfDocument) return;
    const generation = ++renderGeneration;
    renderTask?.cancel?.();
    textLayer?.cancel?.();
    container.setAttribute('aria-busy', 'true');
    try {
      const page = await pdfDocument.getPage(currentPage);
      if (generation !== renderGeneration) return;
      const unscaled = page.getViewport({ scale: 1 });
      const availableWidth = Math.max(260, stage.clientWidth - (stage.clientWidth <= 900 ? 28 : 48));
      const fittedScale = Math.min(2.5, Math.max(.35, availableWidth / unscaled.width));
      const scale = Math.min(4, Math.max(.35, fittedScale * zoom));
      const viewport = page.getViewport({ scale });
      const outputScale = Math.min(2, window.devicePixelRatio || 1);
      const context = canvas.getContext('2d', { alpha: false });
      if (!context) throw new Error('Canvas недоступен');

      canvas.width = Math.floor(viewport.width * outputScale);
      canvas.height = Math.floor(viewport.height * outputScale);
      canvas.style.width = `${Math.floor(viewport.width)}px`;
      canvas.style.height = `${Math.floor(viewport.height)}px`;
      pageShell.style.width = `${Math.floor(viewport.width)}px`;
      pageShell.style.height = `${Math.floor(viewport.height)}px`;
      pageShell.hidden = false;
      errorPanel.hidden = true;

      renderTask = page.render({
        canvas,
        canvasContext: context,
        viewport,
        transform: outputScale === 1 ? null : [outputScale, 0, 0, outputScale, 0, 0],
      });
      await renderTask.promise;
      if (generation !== renderGeneration) return;

      textLayerElement.replaceChildren();
      textLayerElement.style.setProperty('--scale-factor', String(scale));
      textLayerElement.style.setProperty('--total-scale-factor', String(scale));
      const textContent = await page.getTextContent();
      textLayer = new TextLayer({ textContentSource: textContent, container: textLayerElement, viewport });
      await textLayer.render();
      pageText.set(currentPage, normalize(textContent.items.map((item) => item.str).join(' ')));
      highlightSearchMatches();

      loading.hidden = true;
      container.setAttribute('aria-busy', 'false');
      if (scaleLabel) scaleLabel.textContent = zoom === 1 ? 'По ширине' : `${Math.round(scale * 100)}%`;
    } catch (error) {
      if (error?.name === 'RenderingCancelledException') return;
      showError(error);
    }
  }

  function highlightSearchMatches() {
    const query = normalize(searchInput?.value);
    textLayerElement.querySelectorAll('.pdf-search-match').forEach((element) => element.classList.remove('pdf-search-match'));
    if (!query) return;
    textLayerElement.querySelectorAll('span').forEach((element) => {
      if (normalize(element.textContent).includes(query)) element.classList.add('pdf-search-match');
    });
  }

  async function show(page, updateHistory = true) {
    currentPage = clampPage(page);
    updateControls();
    if (updateHistory) history.replaceState(history.state, '', `${window.location.pathname}${window.location.search}#page=${currentPage}`);
    await renderPage();
    stage.scrollTo({ top: 0, left: 0, behavior: 'instant' });
  }

  async function pageTextContent(pageNumber) {
    if (pageText.has(pageNumber)) return pageText.get(pageNumber);
    const page = await pdfDocument.getPage(pageNumber);
    const content = await page.getTextContent();
    const text = normalize(content.items.map((item) => item.str).join(' '));
    pageText.set(pageNumber, text);
    return text;
  }

  async function performSearch() {
    const query = normalize(searchInput?.value);
    const generation = ++searchGeneration;
    searchMatches = [];
    searchMatchIndex = -1;
    searchNext.hidden = true;
    if (!query) { searchStatus.textContent = ''; highlightSearchMatches(); return; }
    searchStatus.textContent = 'Ищем…';
    for (let page = 1; page <= totalPages(); page += 1) {
      const text = await pageTextContent(page);
      if (generation !== searchGeneration) return;
      if (text.includes(query)) searchMatches.push(page);
    }
    if (!searchMatches.length) {
      searchStatus.textContent = 'Совпадений нет';
      highlightSearchMatches();
      return;
    }
    searchMatchIndex = 0;
    searchNext.hidden = searchMatches.length < 2;
    searchStatus.textContent = `${searchMatches.length} стр. с совпадениями`;
    await show(searchMatches[0]);
  }

  async function nextSearchMatch() {
    if (!searchMatches.length) return performSearch();
    searchMatchIndex = (searchMatchIndex + 1) % searchMatches.length;
    searchStatus.textContent = `${searchMatchIndex + 1} из ${searchMatches.length}`;
    await show(searchMatches[searchMatchIndex]);
  }

  function showError(error) {
    console.error('PDF viewer:', error);
    loading.hidden = true;
    pageShell.hidden = true;
    errorPanel.hidden = false;
    container.setAttribute('aria-busy', 'false');
  }

  previous?.addEventListener('click', () => show(currentPage - 1));
  next?.addEventListener('click', () => show(currentPage + 1));
  input.addEventListener('change', () => show(input.value));
  input.addEventListener('keydown', (event) => { if (event.key === 'Enter') { event.preventDefault(); show(input.value); } });
  zoomIn?.addEventListener('click', () => { zoom = Math.min(3, zoom * 1.2); renderPage(); });
  zoomOut?.addEventListener('click', () => { zoom = Math.max(.5, zoom / 1.2); renderPage(); });
  fitWidth?.addEventListener('click', () => { zoom = 1; renderPage(); });
  searchForm?.addEventListener('submit', (event) => { event.preventDefault(); performSearch(); });
  searchNext?.addEventListener('click', nextSearchMatch);
  searchInput?.addEventListener('input', () => { searchGeneration += 1; searchMatches = []; searchMatchIndex = -1; searchNext.hidden = true; searchStatus.textContent = ''; highlightSearchMatches(); });

  stage.addEventListener('keydown', (event) => {
    if (event.key === 'PageDown' || event.key === 'ArrowRight') { event.preventDefault(); show(currentPage + 1); }
    else if (event.key === 'PageUp' || event.key === 'ArrowLeft') { event.preventDefault(); show(currentPage - 1); }
    else if (event.key === '+' || event.key === '=') { event.preventDefault(); zoomIn?.click(); }
    else if (event.key === '-') { event.preventDefault(); zoomOut?.click(); }
    else if (event.key === '0') { event.preventDefault(); fitWidth?.click(); }
  });
  document.addEventListener('keydown', (event) => {
    if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === 'f') {
      event.preventDefault();
      searchInput?.focus();
      searchInput?.select();
    }
  });

  let resizeTimer = null;
  const resizeObserver = new ResizeObserver(() => {
    window.clearTimeout(resizeTimer);
    resizeTimer = window.setTimeout(() => { if (pdfDocument) renderPage(); }, 120);
  });
  resizeObserver.observe(stage);

  (async () => {
    try {
      const hashPage = Number(window.location.hash.match(/page=(\d+)/)?.[1] || 1);
      currentPage = Math.max(1, hashPage);
      const loadingTask = getDocument({ url: source });
      pdfDocument = await loadingTask.promise;
      currentPage = clampPage(currentPage);
      updateControls();
      await show(currentPage, false);
    } catch (error) {
      showError(error);
    }
  })();
});
