/* Knowledge Graph Explorer Interactive Module */

document.addEventListener('DOMContentLoaded', () => {
  const container = document.getElementById('graph');
  if (!container || !window.DOCSHUB_GRAPH_ENDPOINT) return;

  fetch(window.DOCSHUB_GRAPH_ENDPOINT)
    .then(res => res.json())
    .then(graphData => {
      renderGraphExplorer(container, graphData);
    })
    .catch(err => console.error('Graph data fetch error:', err));

  function renderGraphExplorer(el, data) {
    const nodes = data.nodes || [];
    const links = data.links || [];

    const w = el.clientWidth || 900;
    const h = el.clientHeight || 600;
    const svgNS = 'http://www.w3.org/2000/svg';

    el.innerHTML = '';
    const svg = document.createElementNS(svgNS, 'svg');
    svg.setAttribute('viewBox', `0 0 ${w} ${h}`);
    svg.style.width = '100%';
    svg.style.height = '65vh';
    svg.style.backgroundColor = 'var(--surface-primary)';
    svg.style.borderRadius = 'var(--radius-lg)';
    svg.style.border = '1px solid var(--border-subtle)';

    el.appendChild(svg);

    // Calculate node coordinates in a circular topology layout
    const posMap = new Map();
    nodes.forEach((node, i) => {
      const angle = (i / nodes.length) * 2 * Math.PI;
      const radius = Math.min(w, h) * 0.35;
      const x = w / 2 + Math.cos(angle) * radius;
      const y = h / 2 + Math.sin(angle) * radius;
      posMap.set(node.id, { x, y, label: node.label });
    });

    // Draw connecting edges
    links.forEach(link => {
      const src = posMap.get(link.source);
      const tgt = posMap.get(link.target);
      if (!src || !tgt) return;

      const line = document.createElementNS(svgNS, 'line');
      line.setAttribute('x1', src.x);
      line.setAttribute('y1', src.y);
      line.setAttribute('x2', tgt.x);
      line.setAttribute('y2', tgt.y);
      line.setAttribute('stroke', 'var(--border-strong)');
      line.setAttribute('stroke-width', '1.5');
      line.setAttribute('opacity', '0.6');
      svg.appendChild(line);
    });

    // Draw graph nodes
    nodes.forEach(node => {
      const pos = posMap.get(node.id);
      const group = document.createElementNS(svgNS, 'g');
      group.style.cursor = 'pointer';

      const circle = document.createElementNS(svgNS, 'circle');
      circle.setAttribute('cx', pos.x);
      circle.setAttribute('cy', pos.y);
      circle.setAttribute('r', '16');
      circle.setAttribute('fill', 'var(--action-primary)');

      const text = document.createElementNS(svgNS, 'text');
      text.setAttribute('x', pos.x);
      text.setAttribute('y', pos.y + 30);
      text.setAttribute('text-anchor', 'middle');
      text.setAttribute('fill', 'var(--text-primary)');
      text.setAttribute('font-size', '12');
      text.textContent = node.label.length > 20 ? node.label.slice(0, 18) + '...' : node.label;

      group.appendChild(circle);
      group.appendChild(text);

      group.addEventListener('click', () => {
        window.location.href = `/a/${encodeURIComponent(node.id)}`;
      });

      svg.appendChild(group);
    });
  }
});
