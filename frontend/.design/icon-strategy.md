# Icon Strategy: Tabler + MUI Natural Integration

## Context
- Current UI stack is MUI (`@mui/material`), with many existing icons from `@mui/icons-material`.
- Pain point: MUI icon set style consistency is good, but icon variety/quality for some product scenes is limited.
- Opportunity: `@tabler/icons-react` is already installed and offers richer, cleaner icon coverage.

## Goal
Use Tabler as the **primary icon source** while keeping MUI component integration behavior (size, color, spacing, interaction states) unchanged.

## SDLC Plan

### 1) Discovery / Audit
- Inventory all `@mui/icons-material` usage by file and frequency.
- Tag icons by type:
  - **Action icons** (buttons, menus)
  - **Status icons** (success/error/warning)
  - **Navigation / structural icons**
- Identify high-impact pages first (probe dialogs, settings cards, onboarding).

### 2) Architecture
- Introduce a small adapter component that wraps Tabler icons in MUI `SvgIcon`.
- Adapter requirements:
  - Supports `fontSize`, `color`, `sx`, `className` from MUI.
  - Uses `inheritViewBox` for native Tabler scaling.
  - Preserves MUI theme color mapping (`primary`, `error`, etc.).

### 3) Pilot Migration
- Migrate one small feature slice first (e.g. Probe menus).
- Validate:
  - visual consistency with current typography and spacing;
  - hover/disabled states;
  - dark mode readability.

### 4) Gradual Rollout
- Batch migration by module to reduce risk.
- Keep a fallback rule: if a Tabler icon is semantically weaker, temporarily keep MUI icon.

### 5) Governance
- Add icon usage guideline:
  - default to Tabler via adapter;
  - use raw MUI icons only when Tabler has no suitable equivalent.
- Add lint/check script later to detect new direct `@mui/icons-material` imports outside exception list.

## Benefits
- Better icon aesthetics and semantic coverage.
- Minimal refactor cost due to compatibility wrapper.
- Preserves existing MUI design system behavior.

## Risks
- Potential visual mismatch in stroke weight between old and new icons.
- Some MUI icons encode familiar meaning; replacement needs UX review.

## Immediate Next Steps
1. Land adapter component (`MuiTablerIcon`).
2. Pilot replace icons in probe menu module.
3. Screenshot + review with design before wider migration.
