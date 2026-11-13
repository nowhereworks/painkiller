# Exam layout

Desktop starts with 40% question / 60% terminal space, excluding the divider.
Drag the divider or focus it and use Left/Right (Shift for larger steps),
Home, or End to resize. Minimum widths are 270px for questions and 150px
for the terminal. Mobile keeps the vertically stacked layout.
The terminal's existing ResizeObserver fits the shell when its panel changes.

Run `node scripts/e2e/layout.js` with `playwright-core` and Chrome installed
for browser-only layout checks (no server or lab required).
