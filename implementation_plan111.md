# Motion, Icons, Status System, Color & Themes Architecture for Docs_Hub

This plan delivers a comprehensive, production-grade overhaul of Docs_Hub's motion, iconography, state management, color science, themes, and interactive feedback systems in strict accordance with the master design system specifications (**State → Feedback → Motion → Color → Meaning**).

---

## User Review Required

> [!IMPORTANT]
> - **Zero-FOUC / Zero-FART Persistence**: All theme settings (Theme mode: System/Light/Dark, Accent Color preset/custom, Interface Density: Comfortable/Compact, Motion mode: System/Full/Reduced) are applied synchronously before first paint via an inline script in `<head>` and stored in `localStorage`.
> - **Color Independence**: Semantic statuses (Danger, Warning, Success, Information) are strictly decoupled from Accent colors so that selecting a Red or Amber accent will never conflict with or diminish critical alert readability.
> - **Accessibility & Reduced Motion**: Full compliance with `@media (prefers-reduced-motion: reduce)` and `@media (prefers-reduced-transparency: reduce)`, with non-decorational tactile feedback preserved.

---

## Proposed Architectural Changes

### 1. Design Tokens & Color Architecture

```
Primitive Colors (HSL / OKLCH)
        ↓
Theme Tiers (Light: Swiss Paper / Dark: Mineral Obsidian)
        ↓
Semantic Tokens (Surfaces, Text, Borders, Statuses)
        ↓
Accent System (10 Curated Presets + Normalized Custom Accent)
        ↓
Component Tokens & Density Rules
```

#### [MODIFY] [tokens.css](file:///c:/Users/den/Documents/PROGRAMS/Docs_Hub/internal/web/static/css/tokens.css)
- Implement unified motion tokens:
  - `--motion-instant: 80ms;`
  - `--motion-fast: 140ms;`
  - `--motion-base: 220ms;`
  - `--motion-medium: 320ms;`
  - `--motion-slow: 480ms;`
  - `--motion-scene: 700ms;`
  - `--ease-standard: cubic-bezier(0.2, 0, 0, 1);`
  - `--ease-out: cubic-bezier(0.16, 1, 0.3, 1);`
  - `--ease-in-out: cubic-bezier(0.65, 0, 0.35, 1);`
  - `--ease-spring: cubic-bezier(0.175, 0.885, 0.32, 1.15);`
  - `--ease-bounce-soft: cubic-bezier(0.34, 1.56, 0.64, 1);`
- Complete Semantic Color Token definitions for Light theme (Swiss warm paper `#fbfbfa`, soft layered borders, warm charcoal typography) and Dark theme (Obsidian slate `#0d0f13`, mineral surfaces `#15171e`/`#1c1f28`, soft zinc typography, zero pure `#000`/`#fff` eye strain).
- 10 Curated Accent Presets:
  1. **Cobalt / Indigo** (Classic Tech Default)
  2. **Sapphire / Classic Blue**
  3. **Cyan / Electric Teal**
  4. **Emerald / Forest Green**
  5. **Amber / Warm Gold**
  6. **Tangerine / Solar Orange**
  7. **Crimson / Coral Red**
  8. **Rose / Vivid Magenta**
  9. **Iris / Royal Violet**
  10. **Monochrome / Graphite Neutral**
- Auto-generated complete accent families: `--accent`, `--accent-hover`, `--accent-active`, `--accent-subtle`, `--accent-muted`, `--accent-border`, `--accent-contrast`, `--focus-ring`.
- Density tokens (`--density-comfortable` vs `--density-compact`).

---

### 2. Unified Stroke Icon System & Microinteractions

#### [NEW] [icons.css](file:///c:/Users/den/Documents/PROGRAMS/Docs_Hub/internal/web/static/css/icons.css)
- Complete stroke-based SVG icon system with uniform 20x20 / 16x16 bounding box, 1.5px–1.75px stroke width, rounded caps and joins.
- Icon motion primitives:
  - Theme Icon morph (smooth Sun ↔ Moon transition with rotation),
  - Search Icon lens pulse/focus response,
  - Copy to clipboard checkmark draw-in animation (`stroke-dashoffset`),
  - Refresh / Sync controlled rotation and check morph,
  - Chevron smooth 180° rotation on `<details>` and dropdown toggles,
  - Hamburger to X morph on mobile navigation,
  - Upload / Dragzone arrow lift on drag-over,
  - Save status spinner / checkmark morph.

---

### 3. Multi-Tier Status System & Contextual Status Bar

#### [NEW] [status.css](file:///c:/Users/den/Documents/PROGRAMS/Docs_Hub/internal/web/static/css/status.css)
- Unified state matrix: Neutral, Informational, Active, Success, Warning, Danger/Critical, Disabled, Pending, Syncing, Offline, Unknown.
- Indicator variants:
  - Status Dot (live pulsing indicator for syncing/critical, subtle ambient glow),
  - Dot + Label,
  - Status Badges with icons and semantic color pairs,
  - Inline system status indicators.
- Persistent Contextual Bottom Status Bar:
  - Real-time network detection (`navigator.onLine`, `online`/`offline` listeners),
  - Live sync & autosave feedback,
  - Contextual reading stats (word count, reading time in Reader; draft status in Editor; system metrics in Admin),
  - Quick appearance trigger button,
  - Global shortcut hints.
- Minimalist Progress Bars & Skeleton Loaders with smooth shimmer animation and `prefers-reduced-motion` safety.

---

### 4. Appearance Settings Modal & Live Engine

#### [NEW] [appearance.css](file:///c:/Users/den/Documents/PROGRAMS/Docs_Hub/internal/web/static/css/appearance.css)
- Dedicated "Внешний вид и анимация" (Appearance & Motion) modal with:
  - **Theme Cards**: System / Light / Dark with interactive graphical previews.
  - **Accent Swatches**: 10 curated preset buttons with active checkmark + Custom color picker with real-time contrast normalization.
  - **Interface Density**: Comfortable / Compact toggle.
  - **Motion Modes**: System / Full Motion / Reduced Motion.
  - Real-time instant live preview without page refresh.

#### [NEW] [appearance_modal.html](file:///c:/Users/den/Documents/PROGRAMS/Docs_Hub/internal/web/templates/components/appearance_modal.html)
- Clean, accessible modal template with full keyboard accessibility (`Esc`, `Tab` trap, `aria-modal="true"`).

---

### 5. Motion Hierarchy & Component Transitions

#### [NEW] [motion.css](file:///c:/Users/den/Documents/PROGRAMS/Docs_Hub/internal/web/static/css/motion.css)
- **Level 1 (Micro feedback)**: Button press scale `0.985`, active tab highlights, form control focus rings, tag chips hover, copy button feedback.
- **Level 2 (Component transitions)**: Dropdown slide/fade, modal zoom-in, drawer slide-in, toast notification entry/exit.
- **Level 3 (Layout transitions)**: Sidebar expand/collapse, editor split-pane toggle, grid view transitions.
- **Level 4 (Scene transitions)**: Smooth page reveal and brand mark micro-animation.
- Full `@media (prefers-reduced-motion: reduce)` ruleset.

---

### 6. Stylesheet Hub & Component Refinements

#### [MODIFY] [button.css](file:///c:/Users/den/Documents/PROGRAMS/Docs_Hub/internal/web/static/css/components/button.css)
- Integrate tactile states (idle, hover, pressed, loading with inline spinner, success with checkmark, disabled, focus-visible).

#### [MODIFY] [badge.css](file:///c:/Users/den/Documents/PROGRAMS/Docs_Hub/internal/web/static/css/components/badge.css)
- Update with full status matrix tokens and SVG icon integration.

#### [MODIFY] [input.css](file:///c:/Users/den/Documents/PROGRAMS/Docs_Hub/internal/web/static/css/components/input.css)
- Add animated checkboxes, radio buttons, custom toggle switches, input focus and error-shake animations.

#### [MODIFY] [dialog.css](file:///c:/Users/den/Documents/PROGRAMS/Docs_Hub/internal/web/static/css/components/dialog.css)
- Update modal overlay and toast notification animations (categories: success, info, warning, error, progress).

#### [MODIFY] [style.css](file:///c:/Users/den/Documents/PROGRAMS/Docs_Hub/internal/web/static/style.css)
- Wire all new CSS modules: `tokens.css`, `typography.css`, `icons.css`, `status.css`, `appearance.css`, `motion.css`, `components/*.css`, `utilities.css`.

---

### 7. Frontend Client Logic (`app.js` & Modules)

#### [MODIFY] [app.js](file:///c:/Users/den/Documents/PROGRAMS/Docs_Hub/internal/web/static/app.js)
- **AppearanceManager**: Handles Theme (System/Light/Dark), Accent Colors (Presets + OKLCH/perceptually uniform Custom Color computation), Interface Density, Motion mode, and `localStorage` syncing.
- **NetworkStatusManager**: Listens to `online`/`offline` events, updates UI status indicators, and notifies user via non-intrusive status bar and toasts.
- **ToastManager**: Multi-category toasts (success, info, warning, error, progress) with icons, auto-dismiss, and accessibility.
- **StatusBarManager**: Dynamically updates contextual status bar based on page type, word count, and network state.
- **IconRenderer / SVG Helper**: Injects clean animated SVG icons across dynamic components.
- **ButtonFeedback**: Provides tactile in-place async button feedback (loading spinner -> success checkmark -> revert).
- **TableCopy / Details Animation**: Enhanced code block copying with SVG check stroke animation and smooth accordion transitions.

---

### 8. Templates Integration

#### [MODIFY] [base.html](file:///c:/Users/den/Documents/PROGRAMS/Docs_Hub/internal/web/templates/base.html)
- Zero-FOUC theme/accent/density/motion script in `<head>`.
- Topbar appearance button + status indicator dot.
- Embed contextual bottom Status Bar.
- Include `appearance_modal` component.

#### [MODIFY] [sidebar.html](file:///c:/Users/den/Documents/PROGRAMS/Docs_Hub/internal/web/templates/components/sidebar.html)
- Replace all text glyphs with clean SVG icons.
- Add Appearance settings button in sidebar footer.

#### [MODIFY] [home.html](file:///c:/Users/den/Documents/PROGRAMS/Docs_Hub/internal/web/templates/home.html), [article.html](file:///c:/Users/den/Documents/PROGRAMS/Docs_Hub/internal/web/templates/article.html), [edit.html](file:///c:/Users/den/Documents/PROGRAMS/Docs_Hub/internal/web/templates/edit.html), [search.html](file:///c:/Users/den/Documents/PROGRAMS/Docs_Hub/internal/web/templates/search.html), [admin.html](file:///c:/Users/den/Documents/PROGRAMS/Docs_Hub/internal/web/templates/admin.html), [graph.html](file:///c:/Users/den/Documents/PROGRAMS/Docs_Hub/internal/web/templates/graph.html), [login.html](file:///c:/Users/den/Documents/PROGRAMS/Docs_Hub/internal/web/templates/login.html)
- Upgrade all icons, status badges, metric cards, and interactive controls with new design tokens and SVG icons.

---

## Verification Plan

### Automated Verification
1. Run Go test suite to verify templates compile and tests pass:
   ```bash
   go test ./...
   ```
2. Build verification:
   ```bash
   go build ./cmd/docshub
   ```

### Manual & Runtime Verification
1. Verify Theme Switching (Light / Dark / System) with zero FOUC on reload.
2. Test all 10 Accent Presets and Custom Color Picker in both Light and Dark themes.
3. Test offline/online network simulation (`NetworkStatusManager`).
4. Test Contextual Status Bar across Home, Article Reader, Editor, Admin, Search, and Graph pages.
5. Verify Toast notifications across all 5 categories (Success, Info, Warning, Error, Progress).
6. Verify Button tactile feedback (idle -> loading -> success -> revert).
7. Test `@media (prefers-reduced-motion: reduce)` in browser to verify motion reduction without loss of function.
8. Verify keyboard accessibility (`Tab`, `Esc`, `Enter`, `Ctrl+K`) and focus ring visibility across all accents.
