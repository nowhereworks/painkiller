# Exam layout

Desktop starts with 40% question / 60% terminal space, excluding the divider.
Drag the divider or focus it and use Left/Right (Shift for larger steps),
Home, or End to resize. Minimum widths are 270px for questions and 150px
for the terminal. Mobile keeps the vertically stacked layout.
The terminal's existing ResizeObserver fits the shell when its panel changes.

Run `node scripts/e2e/layout.js` with `playwright-core` and Chrome installed
for browser-only layout checks (no server or lab required).

## Question walkthroughs

**Reveal Instructions** shows a question's numbered solution steps and commands;
it does not copy the question to the clipboard. **Hide Instructions** collapses
the block. Switching questions hides and clears it; polling keeps it open.

`exam/tasks.json` stores each question's `steps`, `solution`, and `explanation`.
The tasks API includes these during running and completed attempts. Walkthrough
steps and explanations use the same sanitized Markdown renderer as question text; command blocks
use text content and Bash highlighting. Internal grading checks stay server-side.

Run `node scripts/e2e/ui.js` for isolated browser checks, including all 29
packaged walkthroughs (no lab VMs required).

## Results

Each question has a separate card with its score, per-check evaluations, formatted
explanation, and highlighted solution commands. Correct answers are green, wrong
or partially correct answers red, and grading errors amber; text labels accompany
the colors. Grading messages preserve whitespace in plain code blocks. Captured
answer snapshots remain private and are not exposed by the status API.

**Destroy Lab / Reset** appears above and below the cards; both buttons use the
same confirmation and reset action. Grading errors retain **Retry grading**.
The isolated UI checks also cover these controls and the responsive results layout.
