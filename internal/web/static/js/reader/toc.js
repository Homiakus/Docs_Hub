document.addEventListener('DOMContentLoaded', () => {
  const article = document.querySelector('.doc-main .markdown');
  if (!article) return;
  const headings = Array.from(article.querySelectorAll('h1[id],h2[id],h3[id],h4[id]'));
  const links = Array.from(document.querySelectorAll('.toc-nav a,.toc nav a'));
  const progress = document.getElementById('readingProgressBar');

  if (headings.length && links.length && 'IntersectionObserver' in window) {
    const byID = new Map(links.map((link) => [decodeURIComponent(link.hash.slice(1)), []]));
    links.forEach((link) => {
      const id = decodeURIComponent(link.hash.slice(1));
      if (!byID.has(id)) byID.set(id, []);
      byID.get(id).push(link);
    });
    let activeID = '';
    const setActive = (id) => {
      if (!id || activeID === id) return;
      activeID = id;
      links.forEach((link) => {
        const active = decodeURIComponent(link.hash.slice(1)) === id;
        link.classList.toggle('active', active);
        if (active) link.setAttribute('aria-current', 'location');
        else link.removeAttribute('aria-current');
      });
    };
    const observer = new IntersectionObserver((entries) => {
      const visible = entries.filter((entry) => entry.isIntersecting).sort((a, b) => a.boundingClientRect.top - b.boundingClientRect.top);
      if (visible[0]) setActive(visible[0].target.id);
    }, { rootMargin: '-90px 0px -65% 0px', threshold: [0, 1] });
    headings.forEach((heading) => observer.observe(heading));
    setActive(headings[0].id);
  }

  if (progress) {
    const updateProgress = () => {
      const rect = article.getBoundingClientRect();
      const total = Math.max(1, article.offsetHeight - window.innerHeight * .55);
      const passed = Math.min(total, Math.max(0, -rect.top + 100));
      progress.style.width = `${(passed / total) * 100}%`;
    };
    updateProgress();
    document.addEventListener('scroll', updateProgress, { passive: true });
    window.addEventListener('resize', updateProgress);
  }
});
