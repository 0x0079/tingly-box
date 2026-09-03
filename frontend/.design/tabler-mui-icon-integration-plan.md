# Icon System Research & Integration Plan (Tabler + MUI)

## 1) Context

Current pain point: MUI's built-in icon set is easy to integrate with MUI components, but the visual richness and coverage are limited for our product scenarios. We want to keep MUI-level integration while adopting a richer icon library (Tabler Icons).

Goal: Use Tabler icons as the primary UI icon source while preserving natural compatibility with MUI (`sx`, size/color inheritance, theme mode, and consistency across components).

---

## 2) Findings

### MUI Icons strengths
- Tight integration with MUI theming (`fontSize`, `color`, `sx`).
- Consistent behavior in MUI components (`Button`, `ListItemIcon`, `IconButton`, etc.).
- Clear alignment with Material UI visual system.

### MUI Icons weaknesses
- Visual style may feel generic and less distinctive.
- Some semantic icons are missing or less expressive.

### Tabler Icons strengths
- Much broader icon coverage and clearer semantics.
- More modern/clean stroke style, better visual consistency for tool-like UIs.
- Easy to tree-shake via icon-level imports.

### Tabler Icons risks
- Default style is stroke-based; can feel lighter than Material defaults.
- Needs a compatibility layer to ensure "drop-in" behavior with MUI API conventions.

---

## 3) Recommended Architecture

### Decision
Adopt **Tabler as primary generic UI icon set**, keep **brand logos/custom SVGs** in current local brand-icon system.

### Adapter principle
Build a **single MUI-compatible icon wrapper** to normalize:
- `size` -> `width/height` mapping (with sane defaults).
- `color` -> `currentColor` (inherits MUI text/icon color).
- `stroke` -> theme-aware stroke width (default 1.75, density tunable).
- `sx` passthrough -> full MUI style control.

### Why this works
This avoids replacing all icon call sites with library-specific props. App code should depend on our wrapper API, not on MUI Icons or Tabler internal prop shape.

---

## 4) SDLC Execution Plan

### Phase A — Discovery & Baseline
1. Inventory all generic icon usage in frontend.
2. Split by category: navigation, status, action, form, feature-specific.
3. Identify hot paths requiring strict visual parity.

**Deliverable:** icon usage matrix + replacement map.

### Phase B — Foundation
1. Add `@tabler/icons-react` dependency.
2. Implement `AppIcon` wrapper (`src/components/icons/AppIcon.tsx`).
3. Define canonical icon registry (`src/components/icons/registry.ts`) keyed by semantic names.

**Deliverable:** compile-safe wrapper + registry + story/demo page.

### Phase C — Incremental Migration
1. Migrate low-risk pages first (settings/tools pages).
2. Add fallback strategy for unresolved icon names.
3. Keep MUI icons only where Tabler equivalent is intentionally not used.

**Deliverable:** staged PRs with visual screenshots and regression notes.

### Phase D — Quality Gate
1. Visual regression checks (light/dark mode).
2. Interaction checks in high-density lists/buttons.
3. Bundle-size check before/after (tree-shaken imports).

**Deliverable:** migration sign-off report.

---

## 5) Technical Conventions

### 5.1 Wrapper API (target)
```tsx
<AppIcon name="settings" size={18} sx={{ color: 'text.secondary' }} />
```

### 5.2 Rules
- Use semantic `name`; avoid direct Tabler imports in feature pages.
- Default icon color should inherit text color (`currentColor`).
- Keep stroke width globally configurable (theme token or constant).
- For brand/provider icons, continue using dedicated brand components (do not force Tabler).

### 5.3 Fallback
- Unknown semantic names render a neutral fallback glyph.
- In dev mode, warn once per unknown key to catch mapping gaps.

---

## 6) Candidate File Layout

- `frontend/src/components/icons/AppIcon.tsx`
- `frontend/src/components/icons/registry.ts`
- `frontend/src/components/icons/types.ts`
- `frontend/src/components/icons/FallbackIcon.tsx`
- `frontend/src/pages/system/IconPlaygroundPage.tsx` (optional)

---

## 7) Acceptance Criteria

- 90%+ generic UI icons served by `AppIcon` (Tabler-backed).
- Zero functional regressions in MUI controls containing icons.
- Dark/light mode readability remains acceptable.
- No significant bundle regression (target: neutral to improved).

---

## 8) Risks & Mitigations

1. **Stroke too thin in dense UI**  
   Mitigation: set global default stroke width and allow per-use override.

2. **Semantic mismatch during migration**  
   Mitigation: maintain explicit mapping table + design review checklist.

3. **Inconsistent alignment in buttons/list items**  
   Mitigation: wrapper enforces inline-flex centering and consistent size baseline.

---

## 9) Next Action (Concrete)

1. Implement wrapper + registry in one small PR.
2. Migrate one page end-to-end as pilot.
3. Run design review with side-by-side screenshots (light/dark).
4. Continue by domain-based batches.
