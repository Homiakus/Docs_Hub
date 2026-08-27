(() => {
  'use strict';

  function normalizeDetails(root = document) {
    root.querySelectorAll('details > summary').forEach((summary) => {
      const details = summary.parentElement;
      if (!(details instanceof HTMLDetailsElement)) return;

      // Chromium exposes <summary> as a button consistently; WebKit has had
      // differences in its accessibility tree. Make the contract explicit.
      summary.setAttribute('role', 'button');
      if (!summary.getAttribute('aria-label')) {
        const heading = summary.querySelector('h1, h2, h3, h4, h5, h6');
        const label = heading?.textContent?.trim() || summary.textContent?.trim();
        if (label) summary.setAttribute('aria-label', label);
      }

      const syncExpanded = () => summary.setAttribute('aria-expanded', String(details.open));
      syncExpanded();
      details.addEventListener('toggle', syncExpanded);
    });
  }

  document.addEventListener('DOMContentLoaded', () => {
    // Hidden dialogs are made non-focusable by the synchronous CSS
    // visibility rule keyed from aria-hidden. Avoid using MutationObserver
    // to toggle `inert`: existing dialog managers focus their first control
    // immediately after aria-hidden=false, before observer callbacks run.
    normalizeDetails();
  });
})();
