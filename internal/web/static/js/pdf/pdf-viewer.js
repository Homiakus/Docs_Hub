/* Self-Hosted Embedded PDF Viewer Module */

document.addEventListener('DOMContentLoaded', () => {
  const container = document.getElementById('pdfViewerContainer');
  const pageInput = document.getElementById('pdfPageInput');
  const prevBtn = document.getElementById('pdfPrevPage');
  const nextBtn = document.getElementById('pdfNextPage');
  const pageCountSpan = document.getElementById('pdfTotalPages');

  if (!container) return;

  let currentPage = parseHashPage() || 1;
  let totalPages = parseInt(container.getAttribute('data-total-pages') || '1', 10);

  function parseHashPage() {
    const match = window.location.hash.match(/#page=(\d+)/);
    return match ? parseInt(match[1], 10) : null;
  }

  function updatePageUI(pageNum) {
    if (pageNum < 1) pageNum = 1;
    if (pageNum > totalPages) pageNum = totalPages;

    currentPage = pageNum;
    if (pageInput) pageInput.value = currentPage;
    if (pageCountSpan) pageCountSpan.textContent = totalPages;

    // Update URL Hash Deep Link
    window.location.hash = `#page=${currentPage}`;

    const iframe = container.querySelector('iframe');
    if (iframe && iframe.src) {
      const url = new URL(iframe.src, window.location.origin);
      url.hash = `page=${currentPage}`;
      iframe.src = url.toString();
    }
  }

  prevBtn?.addEventListener('click', () => updatePageUI(currentPage - 1));
  nextBtn?.addEventListener('click', () => updatePageUI(currentPage + 1));
  pageInput?.addEventListener('change', () => {
    const val = parseInt(pageInput.value, 10);
    if (!isNaN(val)) updatePageUI(val);
  });

  updatePageUI(currentPage);
});
