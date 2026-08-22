# ADR 0004: Fluid Neo-Swiss Responsive Design System

## Status
Accepted

## Context
Documentation tools must provide an optimal reading and editing experience across a broad spectrum of screen geometries, from mobile devices (320px) to high-density 4K and ultrawide displays (3840px), without relying on heavy external frontend frameworks.

## Decision
We engineered a **Vanilla CSS design token architecture** utilizing:
1. Fluid `clamp()` formulas for typography and gutters.
2. Content-driven breakpoints rather than arbitrary device-specific thresholds.
3. Support for hardware capabilities (touch target sizing via `@media (pointer: coarse)`, safe-area insets, reduced motion).
4. Client-side theme switching and custom accent color computation persisted in `localStorage`.

## Consequences
- Ultra-lightweight footprint with zero external CSS framework bloat.
- Pixel-perfect typography and zero global horizontal overflow across all verified devices.
