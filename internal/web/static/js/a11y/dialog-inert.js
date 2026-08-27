document.addEventListener('DOMContentLoaded', () => {
  const syncOverlay = (overlay) => {
    if (!(overlay instanceof HTMLElement)) return;
    const hidden = overlay.getAttribute('aria-hidden') !== 'false';
    if (hidden) overlay.setAttribute('inert', '');
    else overlay.removeAttribute('inert');
  };

  const observeOverlay = (overlay) => {
    syncOverlay(overlay);
    const observer = new MutationObserver(() => syncOverlay(overlay));
    observer.observe(overlay, { attributes: true, attributeFilter: ['aria-hidden'] });
  };

  document.querySelectorAll('.dialog-overlay').forEach(observeOverlay);

  const bodyObserver = new MutationObserver((records) => {
    for (const record of records) {
      for (const node of record.addedNodes) {
        if (!(node instanceof HTMLElement)) continue;
        if (node.matches('.dialog-overlay')) observeOverlay(node);
        node.querySelectorAll?.('.dialog-overlay').forEach(observeOverlay);
      }
    }
  });
  bodyObserver.observe(document.body, { childList: true, subtree: true });

  /* Chromium exposes <summary> inconsistently through role queries. Keep the
   * native disclosure behavior while making the interactive contract explicit
   * to assistive technology and regression tests. */
  document.querySelectorAll('.editor-settings > summary').forEach((summary) => {
    summary.setAttribute('role', 'button');
  });
});
