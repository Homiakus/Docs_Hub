(() => {
  'use strict';

  function syncHiddenContainer(container) {
    const hidden = container.getAttribute('aria-hidden') === 'true';
    // inert removes descendants from focus navigation as well as the
    // accessibility tree. It is the semantic counterpart to aria-hidden.
    container.inert = hidden;
  }

  function normalizeHiddenDialogs(root = document) {
    root.querySelectorAll('[aria-hidden]').forEach((container) => {
      if (!(container.matches('[role="dialog"]') || container.querySelector('[role="dialog"]'))) return;
      syncHiddenContainer(container);
      const observer = new MutationObserver(() => syncHiddenContainer(container));
      observer.observe(container, { attributes: true, attributeFilter: ['aria-hidden'] });
    });
  }

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
    // command-palette.js creates its dialog during DOMContentLoaded before
    // this deferred script's listener runs, so both static and dynamic
    // dialogs are normalized here.
    normalizeHiddenDialogs();
    normalizeDetails();
  });
})();
