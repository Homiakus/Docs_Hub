/* Interactive Sticky Table of Contents (TOC) Module */

document.addEventListener('DOMContentLoaded', () => {
  const tocNav = document.querySelector('.toc nav');
  const headings = document.querySelectorAll('.markdown h1[id], .markdown h2[id], .markdown h3[id], .markdown h4[id]');

  if (!tocNav || headings.length === 0) return;

  const tocLinks = Array.from(tocNav.querySelectorAll('a'));

  // IntersectionObserver to highlight active heading on scroll
  const observerOptions = {
    root: null,
    rootMargin: '-80px 0px -60% 0px',
    threshold: 0,
  };

  const observer = new IntersectionObserver((entries) => {
    entries.forEach((entry) => {
      if (entry.isIntersecting) {
        const id = entry.target.getAttribute('id');
        tocLinks.forEach((link) => {
          if (link.getAttribute('href') === `#${id}`) {
            link.classList.add('active');
            link.style.color = 'var(--action-primary)';
            link.style.fontWeight = '600';
          } else {
            link.classList.remove('active');
            link.style.color = '';
            link.style.fontWeight = '';
          }
        });
      }
    });
  }, observerOptions);

  headings.forEach((heading) => observer.observe(heading));
});
