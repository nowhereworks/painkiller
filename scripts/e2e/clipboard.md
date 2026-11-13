# Terminal clipboard shortcuts

With the terminal focused, Ctrl+Shift+C copies its selection and Ctrl+Shift+V
pastes through xterm's paste API (including bracketed-paste handling). Ctrl+C
still interrupts commands. Clipboard access requires HTTPS or localhost and
browser permission. Shortcut events received by the terminal cancel browser
defaults; shortcuts intercepted before reaching the page cannot be overridden.

Run the browser-only Chrome regression without a server or lab:

```sh
node scripts/e2e/clipboard.js
```

This covers copy/paste, browser-default cancellation, unchanged Ctrl+C, empty
selections, clipboard failures, and focus scoping. Headless Chrome does not
prove behavior of browser/extension-reserved shortcuts in every desktop setup.
