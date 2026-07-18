document.addEventListener('DOMContentLoaded', async () => {
  const container = document.getElementById('graph');
  if (!container || !window.DOCSHUB_GRAPH_ENDPOINT) return;
  const searchInput = document.getElementById('graphSearch');
  const statusInput = document.getElementById('graphStatus');
  const stats = document.getElementById('graphStats');
  const accessibleList = document.getElementById('graphAccessibleList');
  const svgNS = 'http://www.w3.org/2000/svg';
  const nodeWidth = 190;
  const nodeHeight = 62;
  let data = { nodes: [], links: [] };
  let svg = null;
  let viewport = null;
  let transform = { x: 0, y: 0, scale: 1 };
  let canvasSize = { width: 1, height: 1 };
  let dragging = null;
  let pinch = null;
  const activePointers = new Map();
  const compactGraph = window.matchMedia('(max-width: 900px)');

  const createSVG = (name, attrs = {}) => {
    const element = document.createElementNS(svgNS, name);
    Object.entries(attrs).forEach(([key, value]) => element.setAttribute(key, String(value)));
    return element;
  };

  try {
    const response = await fetch(window.DOCSHUB_GRAPH_ENDPOINT);
    if (!response.ok) throw new Error(`HTTP ${response.status}`);
    data = await response.json();
    render();
  } catch (error) {
    showMessage('graph-error', 'Не удалось загрузить граф. Обновите страницу и попробуйте снова.');
  }

  function filteredData() {
    const status = statusInput?.value || '';
    const nodes = (data.nodes || []).filter((node) => !status || node.status === status);
    const ids = new Set(nodes.map((node) => node.id));
    const links = (data.links || []).filter((link) => ids.has(link.source) && ids.has(link.target));
    return { nodes, links };
  }

  function render() {
    container.setAttribute('aria-busy', 'true');
    const graph = filteredData();
    if (!graph.nodes.length) {
      showMessage('graph-empty', 'Нет документов для выбранного фильтра.');
      updateStats(graph);
      renderAccessibleList(graph.nodes);
      return;
    }
    const positions = layeredLayout(graph.nodes, graph.links);
    canvasSize = positions.size;
    container.replaceChildren();
    svg = createSVG('svg', { role: 'img', 'aria-label': `Граф: ${graph.nodes.length} документов и ${graph.links.length} связей` });
    const defs = createSVG('defs');
    const marker = createSVG('marker', { id: 'graphArrow', markerWidth: 8, markerHeight: 8, refX: 7, refY: 4, orient: 'auto', markerUnits: 'strokeWidth' });
    marker.appendChild(createSVG('path', { d: 'M0,0 L8,4 L0,8 Z', fill: 'var(--border-strong)' }));
    const filter = createSVG('filter', { id: 'nodeShadow', x: '-20%', y: '-20%', width: '140%', height: '150%' });
    filter.appendChild(createSVG('feDropShadow', { dx: 0, dy: 2, stdDeviation: 3, 'flood-color': '#111426', 'flood-opacity': '.10' }));
    defs.append(marker, filter);
    svg.appendChild(defs);
    viewport = createSVG('g');
    svg.appendChild(viewport);
    const edgeLayer = createSVG('g', { class: 'graph-edges' });
    const nodeLayer = createSVG('g', { class: 'graph-nodes' });
    viewport.append(edgeLayer, nodeLayer);

    graph.links.forEach((link, index) => drawEdge(edgeLayer, link, positions.map, index));
    graph.nodes.forEach((node) => drawNode(nodeLayer, node, positions.map.get(node.id)));
    container.appendChild(svg);
    bindCanvasEvents();
    fit();
    applySearch();
    updateStats(graph);
    renderAccessibleList(graph.nodes);
    container.setAttribute('aria-busy', 'false');
  }

  function layeredLayout(nodes, links) {
    const nodeMap = new Map(nodes.map((node) => [node.id, node]));
    const outgoing = new Map(nodes.map((node) => [node.id, []]));
    const indegree = new Map(nodes.map((node) => [node.id, 0]));
    links.forEach((link) => {
      if (!nodeMap.has(link.source) || !nodeMap.has(link.target) || link.source === link.target) return;
      outgoing.get(link.source).push(link.target);
      indegree.set(link.target, (indegree.get(link.target) || 0) + 1);
    });
    const level = new Map(nodes.map((node) => [node.id, 0]));
    const queue = nodes.filter((node) => indegree.get(node.id) === 0).map((node) => node.id).sort();
    const processed = new Set();
    while (queue.length) {
      const source = queue.shift();
      processed.add(source);
      (outgoing.get(source) || []).forEach((target) => {
        level.set(target, Math.max(level.get(target) || 0, (level.get(source) || 0) + 1));
        indegree.set(target, indegree.get(target) - 1);
        if (indegree.get(target) === 0) queue.push(target);
      });
    }
    nodes.filter((node) => !processed.has(node.id)).sort((a, b) => a.title?.localeCompare?.(b.title) || a.label.localeCompare(b.label)).forEach((node, index) => {
      level.set(node.id, Math.max(level.get(node.id) || 0, index % 3));
    });
    const logicalLayers = new Map();
    nodes.forEach((node) => {
      const key = level.get(node.id) || 0;
      if (!logicalLayers.has(key)) logicalLayers.set(key, []);
      logicalLayers.get(key).push(node);
    });
    const ordered = Array.from(logicalLayers.keys()).sort((a, b) => a - b);
    const columns = [];
    const maxRows = 10;
    ordered.forEach((key) => {
      const layerNodes = logicalLayers.get(key).sort((a, b) => String(a.label).localeCompare(String(b.label), 'ru'));
      for (let index = 0; index < layerNodes.length; index += maxRows) columns.push(layerNodes.slice(index, index + maxRows));
    });
    const maxColumnRows = Math.max(...columns.map((column) => column.length), 1);
    const gapX = 82;
    const gapY = 28;
    const margin = 56;
    const height = margin * 2 + maxColumnRows * nodeHeight + (maxColumnRows - 1) * gapY;
    const width = margin * 2 + columns.length * nodeWidth + Math.max(0, columns.length - 1) * gapX;
    const positions = new Map();
    columns.forEach((column, columnIndex) => {
      const columnHeight = column.length * nodeHeight + Math.max(0, column.length - 1) * gapY;
      const offsetY = (height - columnHeight) / 2;
      column.forEach((node, rowIndex) => positions.set(node.id, {
        x: margin + columnIndex * (nodeWidth + gapX),
        y: offsetY + rowIndex * (nodeHeight + gapY),
      }));
    });
    return { map: positions, size: { width, height } };
  }

  function drawEdge(layer, link, positions, index) {
    const source = positions.get(link.source);
    const target = positions.get(link.target);
    if (!source || !target) return;
    const group = createSVG('g', { class: 'graph-edge' });
    const sourceRight = source.x + nodeWidth;
    const sourceLeft = source.x;
    const targetLeft = target.x;
    const targetRight = target.x + nodeWidth;
    const sourceY = source.y + nodeHeight / 2;
    const targetY = target.y + nodeHeight / 2;
    let d;
    let labelX;
    let labelY;
    if (target.x > source.x) {
      const middle = (sourceRight + targetLeft) / 2 + ((index % 7) - 3) * 4;
      d = `M ${sourceRight} ${sourceY} H ${middle} V ${targetY} H ${targetLeft - 7}`;
      labelX = middle + 4;
      labelY = (sourceY + targetY) / 2 - 5;
    } else {
      const channel = Math.min(sourceLeft, targetLeft) - 30 - (index % 6) * 7;
      d = `M ${sourceLeft} ${sourceY} H ${channel} V ${targetY} H ${targetRight + 7}`;
      labelX = channel + 4;
      labelY = (sourceY + targetY) / 2 - 5;
    }
    const path = createSVG('path', { d, fill: 'none', stroke: 'var(--border-strong)', 'stroke-width': 1.35, 'marker-end': 'url(#graphArrow)' });
    group.appendChild(path);
    if (link.label) {
      const label = createSVG('text', { x: labelX, y: labelY, fill: 'var(--text-tertiary)', 'font-size': 9, 'font-family': 'Inter, sans-serif' });
      label.textContent = truncate(link.label, 22);
      group.appendChild(label);
    }
    layer.appendChild(group);
  }

  function drawNode(layer, node, position) {
    if (!position) return;
    const link = createSVG('a', { href: `/a/${encodeURIComponent(node.id)}`, class: 'graph-node', 'data-id': node.id, 'data-search': `${node.label} ${node.space || ''}`.toLowerCase() });
    link.setAttribute('aria-label', `${node.label}. ${statusName(node.status)}. ${node.space || 'Без пространства'}`);
    const group = createSVG('g', { transform: `translate(${position.x} ${position.y})`, filter: 'url(#nodeShadow)' });
    const rect = createSVG('rect', { width: nodeWidth, height: nodeHeight, rx: 10, fill: 'var(--surface-primary)', stroke: 'var(--border-subtle)' });
    const strip = createSVG('rect', { width: 4, height: nodeHeight - 16, x: 0, y: 8, rx: 2, fill: statusColor(node.status) });
    const icon = createSVG('rect', { x: 13, y: 15, width: 30, height: 30, rx: 8, fill: 'var(--action-primary-soft)' });
    const glyph = createSVG('text', { x: 28, y: 35, 'text-anchor': 'middle', fill: 'var(--action-primary)', 'font-size': 15, 'font-weight': 700, 'font-family': 'Inter, sans-serif' });
    glyph.textContent = '≡';
    const title = createSVG('text', { x: 52, y: 26, fill: 'var(--text-primary)', 'font-size': 11, 'font-weight': 650, 'font-family': 'Inter, sans-serif' });
    title.textContent = truncate(node.label, 23);
    const meta = createSVG('text', { x: 52, y: 43, fill: 'var(--text-tertiary)', 'font-size': 8.5, 'font-family': 'Inter, sans-serif' });
    meta.textContent = truncate(`${node.space || 'Общее'} · ${statusName(node.status)}`, 29);
    group.append(rect, strip, icon, glyph, title, meta);
    link.appendChild(group);
    layer.appendChild(link);
  }

  function statusColor(status) {
    return ({ published: 'var(--status-success)', draft: 'var(--status-warning)', in_review: 'var(--status-info)', approved: 'var(--status-violet)', archived: 'var(--text-tertiary)', rejected: 'var(--status-danger)' })[status] || 'var(--text-tertiary)';
  }
  function statusName(status) {
    return ({ published: 'Опубликован', draft: 'Черновик', in_review: 'На проверке', approved: 'Одобрен', archived: 'Архив', rejected: 'Отклонён' })[status] || status || 'Без статуса';
  }
  function truncate(value, limit) {
    const text = String(value || '');
    return text.length > limit ? `${text.slice(0, limit - 1)}…` : text;
  }

  function applyTransform() {
    viewport?.setAttribute('transform', `translate(${transform.x} ${transform.y}) scale(${transform.scale})`);
  }
  function fit() {
    if (!svg) return;
    const rect = container.getBoundingClientRect();
    const scale = Math.min((rect.width - 28) / canvasSize.width, (rect.height - 28) / canvasSize.height, 1.15);
    transform.scale = Math.max(compactGraph.matches ? .48 : .3, scale);
    transform.x = (rect.width - canvasSize.width * transform.scale) / 2;
    transform.y = (rect.height - canvasSize.height * transform.scale) / 2;
    applyTransform();
  }
  function zoom(factor, centerX = container.clientWidth / 2, centerY = container.clientHeight / 2) {
    const previous = transform.scale;
    const next = Math.min(2.4, Math.max(compactGraph.matches ? .36 : .2, previous * factor));
    transform.x = centerX - ((centerX - transform.x) / previous) * next;
    transform.y = centerY - ((centerY - transform.y) / previous) * next;
    transform.scale = next;
    applyTransform();
  }

  function bindCanvasEvents() {
    svg.addEventListener('pointerdown', (event) => {
      if (event.target.closest('.graph-node')) return;
      activePointers.set(event.pointerId, { x: event.clientX, y: event.clientY, type: event.pointerType });
      svg.setPointerCapture(event.pointerId);
      if (activePointers.size === 1) {
        dragging = { pointerId: event.pointerId, pointerType: event.pointerType, x: event.clientX, y: event.clientY, tx: transform.x, ty: transform.y };
        pinch = null;
      } else if (activePointers.size === 2) {
        const points = Array.from(activePointers.values());
        const rect = container.getBoundingClientRect();
        const centerX = (points[0].x + points[1].x) / 2 - rect.left;
        const centerY = (points[0].y + points[1].y) / 2 - rect.top;
        pinch = {
          distance: Math.max(1, Math.hypot(points[1].x - points[0].x, points[1].y - points[0].y)),
          scale: transform.scale,
          worldX: (centerX - transform.x) / transform.scale,
          worldY: (centerY - transform.y) / transform.scale,
        };
        dragging = null;
      }
    });
    svg.addEventListener('pointermove', (event) => {
      if (!activePointers.has(event.pointerId)) return;
      activePointers.set(event.pointerId, { x: event.clientX, y: event.clientY, type: event.pointerType });
      if (activePointers.size >= 2 && pinch) {
        const points = Array.from(activePointers.values()).slice(0, 2);
        const rect = container.getBoundingClientRect();
        const centerX = (points[0].x + points[1].x) / 2 - rect.left;
        const centerY = (points[0].y + points[1].y) / 2 - rect.top;
        const distance = Math.max(1, Math.hypot(points[1].x - points[0].x, points[1].y - points[0].y));
        const next = Math.min(2.4, Math.max(.32, pinch.scale * distance / pinch.distance));
        transform.scale = next;
        transform.x = centerX - pinch.worldX * next;
        transform.y = centerY - pinch.worldY * next;
      } else if (dragging?.pointerId === event.pointerId) {
        const dx = event.clientX - dragging.x;
        const dy = event.clientY - dragging.y;
        if (dragging.pointerType === 'touch' && Math.abs(dy) > Math.abs(dx)) return;
        transform.x = dragging.tx + dx;
        transform.y = dragging.ty + dy;
      } else {
        return;
      }
      applyTransform();
    });
    const stop = (event) => {
      activePointers.delete(event.pointerId);
      try { svg.releasePointerCapture(event.pointerId); } catch (_) {}
      pinch = null;
      const remaining = Array.from(activePointers.entries());
      if (remaining.length === 1) {
        const [pointerId, point] = remaining[0];
        dragging = { pointerId, pointerType: point.type, x: point.x, y: point.y, tx: transform.x, ty: transform.y };
      } else {
        dragging = null;
      }
    };
    svg.addEventListener('pointerup', stop);
    svg.addEventListener('pointercancel', stop);
    svg.addEventListener('wheel', (event) => {
      event.preventDefault();
      const rect = container.getBoundingClientRect();
      zoom(event.deltaY < 0 ? 1.12 : .89, event.clientX - rect.left, event.clientY - rect.top);
    }, { passive: false });
    container.onkeydown = (event) => {
      const step = event.shiftKey ? 72 : 36;
      if (event.key === '+' || event.key === '=') zoom(1.2);
      else if (event.key === '-') zoom(.82);
      else if (event.key === '0') fit();
      else if (event.key === 'ArrowLeft') transform.x += step;
      else if (event.key === 'ArrowRight') transform.x -= step;
      else if (event.key === 'ArrowUp') transform.y += step;
      else if (event.key === 'ArrowDown') transform.y -= step;
      else return;
      event.preventDefault();
      applyTransform();
    };
  }

  function applySearch() {
    const query = String(searchInput?.value || '').trim().toLowerCase();
    const nodes = Array.from(container.querySelectorAll('.graph-node'));
    let firstMatch = null;
    nodes.forEach((node) => {
      const match = !query || node.dataset.search.includes(query);
      node.style.opacity = match ? '1' : '.16';
      if (match && query && !firstMatch) firstMatch = node;
    });
    if (query && firstMatch) firstMatch.parentNode.appendChild(firstMatch);
  }

  function updateStats(graph) {
    if (stats) stats.textContent = `${graph.nodes.length} узлов · ${graph.links.length} связей`;
  }
  function renderAccessibleList(nodes) {
    if (!accessibleList) return;
    accessibleList.replaceChildren();
    nodes.forEach((node) => {
      const link = document.createElement('a');
      link.href = `/a/${encodeURIComponent(node.id)}`;
      const title = document.createElement('span');
      title.textContent = node.label;
      const meta = document.createElement('small');
      meta.textContent = `${node.space || 'Общее'} · ${statusName(node.status)}`;
      link.append(title, meta);
      accessibleList.appendChild(link);
    });
  }
  function showMessage(className, message) {
    const block = document.createElement('div');
    block.className = className;
    const paragraph = document.createElement('p');
    paragraph.textContent = message;
    block.appendChild(paragraph);
    container.replaceChildren(block);
    container.setAttribute('aria-busy', 'false');
  }

  searchInput?.addEventListener('input', applySearch);
  statusInput?.addEventListener('change', render);
  document.getElementById('graphFit')?.addEventListener('click', fit);
  document.getElementById('graphZoomIn')?.addEventListener('click', () => zoom(1.2));
  document.getElementById('graphZoomOut')?.addEventListener('click', () => zoom(.82));
  const resizeObserver = new ResizeObserver(() => window.requestAnimationFrame(fit));
  resizeObserver.observe(container);
  compactGraph.addEventListener?.('change', () => window.requestAnimationFrame(fit));
});
