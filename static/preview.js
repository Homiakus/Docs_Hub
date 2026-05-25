(() => {
    /* ── Elements ────────────────────────────────────────────── */
    const popup = document.getElementById('pagePreviewPopup');
    if (!popup) return;

    /* ── State ───────────────────────────────────────────────── */
    let showTimer = null;
    let hideTimer = null;
    let activeUrl = null;
    let currentController = null;

    /* ── Cache (30 s TTL) ───────────────────────────────────── */
    const cache = new Map();
    const CACHE_TTL = 30_000; // 30 seconds

    function cacheGet(url) {
        const entry = cache.get(url);
        if (!entry) return null;
        if (Date.now() - entry.ts > CACHE_TTL) {
            cache.delete(url);
            return null;
        }
        return entry.html;
    }

    function cacheSet(url, html) {
        cache.set(url, { html, ts: Date.now() });
    }

    /* ── Styles for skeleton ────────────────────────────────── */
    const SKELETON_STYLE_ID = 'preview-skeleton-style';
    if (!document.getElementById(SKELETON_STYLE_ID)) {
        const style = document.createElement('style');
        style.id = SKELETON_STYLE_ID;
        style.textContent = `
            .preview-skeleton {
                pointer-events: none;
                user-select: none;
                padding: 0;
            }
            .preview-skeleton .sk-bar {
                height: 10px;
                margin-bottom: 8px;
                border-radius: 4px;
                background: #d4d4d4;
                animation: preview-pulse 1.4s ease-in-out infinite;
            }
            .preview-skeleton .sk-bar:nth-child(2) { width: 75%; }
            .preview-skeleton .sk-bar:nth-child(3) { width: 50%; }
            @keyframes preview-pulse {
                0%, 100%   { opacity: 0.4; }
                50%        { opacity: 1.0; }
            }
        `;
        document.head.appendChild(style);
    }

    /* ── Fade helpers ────────────────────────────────────────── */
    function fadeIn() {
        popup.style.transition = 'opacity 0.15s ease';
        popup.style.opacity = '1';
        popup.hidden = false;
    }

    function fadeOut() {
        popup.style.transition = 'opacity 0.12s ease';
        popup.style.opacity = '0';
        // Wait for transition, then hide
        const onTransitionEnd = () => {
            popup.removeEventListener('transitionend', onTransitionEnd);
            if (popup.style.opacity === '0') {
                popup.hidden = true;
                popup.style.transition = '';
            }
        };
        popup.addEventListener('transitionend', onTransitionEnd);
        // Fallback: hide after timeout if transitionend doesn't fire
        setTimeout(() => {
            if (popup.style.opacity === '0') {
                popup.hidden = true;
                popup.style.transition = '';
            }
        }, 150);
    }

    function hideNow() {
        popup.hidden = true;
        popup.style.opacity = '';
        popup.style.transition = '';
        activeUrl = null;
    }

    /* ── URL extraction ──────────────────────────────────────── */
    function previewURL(el) {
        if (el.dataset && el.dataset.notePreview) {
            return el.dataset.notePreview;
        }
        const href = el.getAttribute && el.getAttribute('href');
        if (href && href.startsWith('/a/')) {
            return '/preview/article/' + encodeURIComponent(href.slice(3));
        }
        return '';
    }

    /* ── Skeleton markup ─────────────────────────────────────── */
    function renderSkeleton() {
        return `
            <div class="preview-skeleton">
                <div class="sk-bar"></div>
                <div class="sk-bar"></div>
                <div class="sk-bar"></div>
            </div>
        `;
    }

    /* ── Positioning ─────────────────────────────────────────── */
    function place(e) {
        const pad = 14;
        let x = e.clientX + 18;
        let y = e.clientY + 18;

        popup.style.left = x + 'px';
        popup.style.top = y + 'px';

        const r = popup.getBoundingClientRect();
        if (r.right > innerWidth - pad) {
            popup.style.left = Math.max(pad, e.clientX - r.width - 18) + 'px';
        }
        if (r.bottom > innerHeight - pad) {
            popup.style.top = Math.max(pad, e.clientY - r.height - 18) + 'px';
        }
    }

    /* ── Fetch with abort ────────────────────────────────────── */
    async function loadAndShow(url) {
        // Cancel any in-flight request
        if (currentController) {
            currentController.abort();
        }

        const controller = new AbortController();
        currentController = controller;

        try {
            // Check cache first
            const cached = cacheGet(url);
            if (cached) {
                if (activeUrl !== url) return;
                popup.innerHTML = cached;
                return;
            }

            const res = await fetch(url, { signal: controller.signal });
            if (activeUrl !== url) return;
            if (!res.ok) throw new Error('HTTP ' + res.status);

            const html = await res.text();
            if (activeUrl !== url) return;

            cacheSet(url, html);
            popup.innerHTML = html;
        } catch (err) {
            if (err.name === 'AbortError') return; // silenced – user moved on
            if (activeUrl !== url) return;
            // On real error, just hide
            fadeOut();
        } finally {
            if (currentController === controller) {
                currentController = null;
            }
        }
    }

    /* ── Show ────────────────────────────────────────────────── */
    function show(el, e) {
        const url = previewURL(el);
        if (!url) return;

        // If already showing the same URL, just reposition
        if (activeUrl === url && !popup.hidden) {
            place(e);
            return;
        }

        activeUrl = url;
        place(e);

        // Populate skeleton and fade in
        popup.innerHTML = renderSkeleton();
        fadeIn();

        loadAndShow(url);
    }

    /* ── Cancel & hide ───────────────────────────────────────── */
    function cancelShow() {
        clearTimeout(showTimer);
        showTimer = null;
    }

    function scheduleHide() {
        clearTimeout(hideTimer);
        hideTimer = setTimeout(() => {
            fadeOut();
            activeUrl = null;
        }, 180);
    }

    function cancelHide() {
        clearTimeout(hideTimer);
        hideTimer = null;
    }

    /* ── Mouse events ────────────────────────────────────────── */
    document.addEventListener('mouseover', e => {
        const a = e.target.closest('a[href],a[data-note-preview]');
        if (!a) return;
        const url = previewURL(a);
        if (!url) return;

        cancelHide();
        cancelShow();
        showTimer = setTimeout(() => show(a, e), 260);
    });

    document.addEventListener('mousemove', e => {
        if (!popup.hidden) place(e);
    });

    document.addEventListener('mouseout', e => {
        const a = e.target.closest('a[href],a[data-note-preview]');
        if (!a) return;
        cancelShow();
        scheduleHide();
    });

    /* ── Touch support ───────────────────────────────────────── */
    const TOUCH_LONG_PRESS = 500; // ms
    const TOUCH_MOVE_TOLERANCE = 10; // px – cancel long-press if finger moves
    let touchStartX = 0;
    let touchStartY = 0;
    let touchTimer = null;
    let touchTarget = null;

    function clearTouch() {
        clearTimeout(touchTimer);
        touchTimer = null;
        touchTarget = null;
    }

    document.addEventListener('touchstart', e => {
        const a = e.target.closest('a[href],a[data-note-preview]');
        if (!a) {
            // Touched outside popup → hide
            if (!popup.hidden) {
                fadeOut();
                activeUrl = null;
            }
            return;
        }
        const url = previewURL(a);
        if (!url) return;

        touchTarget = a;
        touchStartX = e.touches[0].clientX;
        touchStartY = e.touches[0].clientY;

        touchTimer = setTimeout(() => {
            if (!touchTarget) return;
            cancelHide();
            cancelShow();
            show(touchTarget, { clientX: touchStartX, clientY: touchStartY });
            clearTouch();
        }, TOUCH_LONG_PRESS);
    }, { passive: false });

    document.addEventListener('touchmove', e => {
        if (!touchTimer || !touchTarget) return;
        const dx = e.touches[0].clientX - touchStartX;
        const dy = e.touches[0].clientY - touchStartY;
        if (Math.abs(dx) > TOUCH_MOVE_TOLERANCE || Math.abs(dy) > TOUCH_MOVE_TOLERANCE) {
            clearTouch();
        }
    }, { passive: false });

    document.addEventListener('touchend', e => {
        // If it was a short tap (not long-press), cancel
        if (touchTimer) {
            clearTouch();
            // On short tap, treat like mouseout so popup doesn't linger
            scheduleHide();
        } else if (!popup.hidden && touchTarget) {
            // Long-press already showed; hide on finger lift
            scheduleHide();
        }
        touchTarget = null;
    });

    /* ── Escape key ──────────────────────────────────────────── */
    document.addEventListener('keydown', e => {
        if (e.key === 'Escape' && !popup.hidden) {
            cancelShow();
            cancelHide();
            hideNow();
        }
    });

})();
