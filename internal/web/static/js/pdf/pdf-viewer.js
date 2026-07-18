document.addEventListener('DOMContentLoaded', () => {
  const container = document.getElementById('pdfViewerContainer');
  const frame = document.getElementById('pdfFrame');
  const input = document.getElementById('pdfPageInput');
  const previous = document.getElementById('pdfPrevPage');
  const next = document.getElementById('pdfNextPage');
  if (!container || !frame || !input) return;

  const total = Math.max(1, Number(container.dataset.totalPages || 1));
  const source = container.dataset.pdfUrl || frame.getAttribute('src').split('#')[0];
  const hashMatch = window.location.hash.match(/page=(\d+)/);
  let current = Math.min(total, Math.max(1, Number(hashMatch?.[1] || 1)));

  function show(page) {
    current = Math.min(total, Math.max(1, Number(page) || 1));
    input.value = String(current);
    previous.disabled = current === 1;
    next.disabled = current === total;
    frame.src = `${source}#page=${current}&view=FitH`;
    history.replaceState(history.state, '', `${window.location.pathname}${window.location.search}#page=${current}`);
  }

  previous.addEventListener('click', () => show(current - 1));
  next.addEventListener('click', () => show(current + 1));
  input.addEventListener('change', () => show(input.value));
  input.addEventListener('keydown', (event) => { if (event.key === 'Enter') { event.preventDefault(); show(input.value); } });
  show(current);
});
