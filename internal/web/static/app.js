(() => {
  // ---- Mobile navigation ----
  const navToggle = document.querySelector('.mobile-nav-toggle');
  const navClose = document.querySelector('.mobile-nav-close');
  const sidepanel = document.querySelector('.sidepanel');
  const setNavOpen = (open) => {
    document.body.classList.toggle('nav-open', open);
    navToggle?.setAttribute('aria-expanded', String(open));
  };
  navToggle?.addEventListener('click', () => setNavOpen(!document.body.classList.contains('nav-open')));
  navClose?.addEventListener('click', () => setNavOpen(false));
  document.addEventListener('click', (event) => {
    if (!document.body.classList.contains('nav-open')) return;
    if (sidepanel?.contains(event.target) || navToggle?.contains(event.target)) return;
    setNavOpen(false);
  });
  sidepanel?.addEventListener('click', (event) => {
    if (event.target.closest('a, button[type="submit"]')) setNavOpen(false);
  });
  document.addEventListener('keydown', (event) => {
    if (event.key === 'Escape') setNavOpen(false);
  });

  // ---- Theme toggle ----
  const themeToggle = document.getElementById('themeToggle');
  const storedTheme = localStorage.getItem('docs-hub-theme');
  if (storedTheme) {
    document.documentElement.setAttribute('data-theme', storedTheme);
    updateThemeIcon(storedTheme);
  }
  themeToggle?.addEventListener('click', () => {
    const current = document.documentElement.getAttribute('data-theme');
    const next = current === 'light' ? 'dark' : 'light';
    document.documentElement.setAttribute('data-theme', next);
    localStorage.setItem('docs-hub-theme', next);
    updateThemeIcon(next);
    // Re-render mermaid if present
    if (window.mermaid && document.querySelector('.mermaid')) {
      document.querySelectorAll('.mermaid').forEach(el => {
        if (!el.hasAttribute('data-processed')) mermaid.run({ nodes: [el] });
      });
    }
  });
  function updateThemeIcon(theme) {
    const icon = themeToggle?.querySelector('.theme-icon');
    if (icon) icon.textContent = theme === 'light' ? '🌙' : '☀️';
  }

  // ---- Table wrapping (scrollable containers) ----
  document.querySelectorAll('.markdown table').forEach(table => {
    if (table.closest('.table-wrap')) return; // already wrapped
    const wrap = document.createElement('div');
    wrap.className = 'table-wrap';
    table.parentNode.insertBefore(wrap, table);
    wrap.appendChild(table);
  });

  // ---- Code copy buttons ----
  document.querySelectorAll('.markdown pre').forEach(pre => {
    const btn = document.createElement('button');
    btn.className = 'code-copy';
    btn.textContent = 'Copy';
    btn.addEventListener('click', () => {
      const code = pre.querySelector('code');
      if (!code) return;
      navigator.clipboard.writeText(code.textContent || '').then(() => {
        btn.textContent = 'Copied!';
        btn.classList.add('copied');
        setTimeout(() => { btn.textContent = 'Copy'; btn.classList.remove('copied'); }, 2000);
      });
    });
    pre.appendChild(btn);
  });

  // ---- Editor / preview ----
  const editor = document.getElementById('content');
  const preview = document.getElementById('preview');
  if (editor && preview) {
    const dropzone = document.getElementById('dropzone');
    const mediaInput = document.getElementById('mediaInput');
    const mediaPicker = document.getElementById('mediaPicker');
    const render = async () => {
      const res = await fetch('/api/preview', {method:'POST', headers:{'content-type':'text/plain'}, body:editor.value});
      if (res.redirected && new URL(res.url, window.location.href).pathname === '/login') {
        window.location.href = '/login';
        return;
      }
      if (!res.ok) {
        showToast((await res.text()).trim() || 'Не удалось обновить предпросмотр');
        return;
      }
      preview.innerHTML = await res.text();
      // Re-run mermaid in preview
      if (window.mermaid) {
        preview.querySelectorAll('.mermaid:not([data-processed])').forEach(el => mermaid.run({ nodes: [el] }));
      }
    };
    editor.addEventListener('input', debounce(render, 250));
    mediaPicker?.addEventListener('click', () => mediaInput?.click());
    mediaInput?.addEventListener('change', () => {
      uploadFiles(Array.from(mediaInput.files || []), editor, render, dropzone).finally(() => {
        mediaInput.value = '';
      });
    });
    [dropzone, editor].filter(Boolean).forEach((target) => {
      target.addEventListener('dragenter', (event) => {
        if (!hasFiles(event)) return;
        event.preventDefault();
        dropzone?.classList.add('is-dragging');
      });
      target.addEventListener('dragover', (event) => {
        if (!hasFiles(event)) return;
        event.preventDefault();
        event.dataTransfer.dropEffect = 'copy';
      });
      target.addEventListener('dragleave', (event) => {
        if (!dropzone?.contains(event.relatedTarget)) dropzone?.classList.remove('is-dragging');
      });
      target.addEventListener('drop', (event) => {
        if (!hasFiles(event)) return;
        event.preventDefault();
        dropzone?.classList.remove('is-dragging');
        uploadFiles(Array.from(event.dataTransfer.files || []), editor, render, dropzone);
      });
    });
    render();
  }

  // ---- Graph ----
  if (window.DOCSHUB_GRAPH_ENDPOINT) {
    fetch(window.DOCSHUB_GRAPH_ENDPOINT).then(r=>r.json()).then(g => {
      const graph = document.getElementById('graph');
      const renderGraph = debounce(() => drawGraph(graph, g), 120);
      drawGraph(graph, g);
      window.addEventListener('resize', renderGraph);
    });
  }

  // ---- Keyboard shortcuts ----
  document.addEventListener('keydown', (event) => {
    // Ctrl+S / Cmd+S: save (if editor focused)
    if ((event.ctrlKey || event.metaKey) && event.key === 's') {
      if (document.activeElement?.id === 'content') {
        event.preventDefault();
        document.querySelector('form.editor')?.submit();
      }
    }
    // / — focus search (when not in input)
    if (event.key === '/' && !event.ctrlKey && !event.metaKey) {
      const tag = document.activeElement?.tagName;
      if (tag !== 'INPUT' && tag !== 'TEXTAREA' && tag !== 'SELECT') {
        event.preventDefault();
        document.querySelector('.search input')?.focus();
      }
    }
  });

  // ---- Helpers ----
  function debounce(fn, ms){let t;return()=>{clearTimeout(t);t=setTimeout(fn,ms)}}
  function hasFiles(event){
    return Array.from(event.dataTransfer?.types || []).includes('Files');
  }
  async function uploadFiles(files, editor, render, dropzone){
    const allowed = files.filter(isSupportedMediaFile);
    if (!allowed.length) return;
    dropzone?.classList.add('is-uploading');
    try {
      for (const file of allowed) {
        const form = new FormData();
        form.append('file', file);
        const res = await fetch(editor.dataset.uploadEndpoint || '/api/uploads', {method:'POST', body:form});
        if (!res.ok) throw new Error(await res.text());
        const payload = await res.json();
        insertAtCursor(editor, `\n\n${payload.markdown}\n\n`);
      }
      editor.dispatchEvent(new Event('input', {bubbles:true}));
      await render();
      editor.focus();
    } catch (err) {
      showToast(`Не удалось загрузить медиа: ${String(err.message || err).trim()}`);
    } finally {
      dropzone?.classList.remove('is-uploading');
    }
  }
  function showToast(message){
    const region = ensureToastRegion();
    const toast = document.createElement('div');
    toast.className = 'toast';
    toast.setAttribute('role', 'status');
    toast.textContent = message;
    region.appendChild(toast);
    window.setTimeout(() => {
      toast.classList.add('is-hiding');
      toast.addEventListener('transitionend', () => toast.remove(), {once:true});
      window.setTimeout(() => toast.remove(), 260);
    }, 3600);
  }
  function ensureToastRegion(){
    let region = document.querySelector('.toast-region');
    if (!region) {
      region = document.createElement('div');
      region.className = 'toast-region';
      document.body.appendChild(region);
    }
    return region;
  }
  function insertAtCursor(textarea, text){
    const start = textarea.selectionStart ?? textarea.value.length;
    const end = textarea.selectionEnd ?? textarea.value.length;
    textarea.value = textarea.value.slice(0, start) + text + textarea.value.slice(end);
    const cursor = start + text.length;
    textarea.setSelectionRange(cursor, cursor);
  }
  function isSupportedMediaFile(file){
    if (/^(image|audio|video)\//.test(file.type || '')) return true;
    return /\.(avif|gif|jpe?g|png|webp|aac|flac|m4a|mp3|oga|ogg|wav|webm|mov|m4v|mp4)$/i.test(file.name || '');
  }
  function drawGraph(el, g){
    if(!el) return;
    const nodes = g.nodes || [], links = g.links || [];
    const w=el.clientWidth||900,h=el.clientHeight||500;
    const svgNS='http://www.w3.org/2000/svg';
    const svg=document.createElementNS(svgNS,'svg');
    svg.setAttribute('viewBox',`0 0 ${w} ${h}`);
    svg.style.width='100%'; svg.style.height='60vh';
    el.innerHTML=''; el.appendChild(svg);
    const pos = new Map(nodes.map((n,i)=>[n.id,{x:w/2+Math.cos(i/nodes.length*6.28)*w*.32,y:h/2+Math.sin(i/nodes.length*6.28)*h*.32}]));
    links.forEach(l=>{const a=pos.get(l.source),b=pos.get(l.target); if(!a||!b)return; const line=document.createElementNS(svgNS,'line'); line.setAttribute('x1',a.x);line.setAttribute('y1',a.y);line.setAttribute('x2',b.x);line.setAttribute('y2',b.y);line.setAttribute('stroke','var(--line-strong, rgba(67,232,199,.34))');svg.appendChild(line)});
    nodes.forEach(n=>{const p=pos.get(n.id); const a=document.createElementNS(svgNS,'a'); a.setAttribute('href','/a/'+encodeURIComponent(n.id)); const c=document.createElementNS(svgNS,'circle'); c.setAttribute('cx',p.x);c.setAttribute('cy',p.y);c.setAttribute('r',w < 520 ? 14 : 18);c.setAttribute('fill','var(--accent, #43e8c7)'); const t=document.createElementNS(svgNS,'text'); t.setAttribute('x',p.x);t.setAttribute('y',p.y+(w < 520 ? 28 : 34));t.setAttribute('text-anchor','middle');t.setAttribute('fill','var(--text, #f4f1ff)');t.setAttribute('font-size',w < 520 ? '11' : '13');t.textContent=shortLabel(n.label, w < 520 ? 16 : 28); a.appendChild(c);a.appendChild(t);svg.appendChild(a)});
  }
  function shortLabel(value, limit){
    const text = String(value || '');
    return text.length > limit ? `${text.slice(0, limit - 1)}...` : text;
  }
})();
