# Question panel

Keep the question column in killer.sh order: remaining-time bar, previous/select/next navigation, then formatted instructions and question actions. Inline code uses a light-gray highlight; fenced blocks support shell (`bash`/`sh`/`shell`), YAML, and JSON syntax highlighting. Unknown languages remain plain code.

The timer shows remaining whole minutes rounded up. Its fill is green above 25%, orange above 10% through 25%, and red at 10% or below. The attempt stores its initial duration; legacy attempts fall back to the server's configured duration (120 minutes by default).

Instructions use Marked 15.0.12, DOMPurify 3.3.1 (restricted formatting tags), and Prism 1.30.0. All assets and licenses are vendored locally; no CDN is needed at runtime. Keep highlighting manual so polling does not rewrite code or disrupt text selection.

Run the isolated browser checks from the repository root:

```sh
npm --prefix scripts/e2e install
node scripts/e2e/ui.js
```

These checks serve local assets with mocked APIs and never provision or access VMs. Set `E2E_CHROME` if Chrome is not at `/usr/bin/google-chrome`. Optional screenshots go to `E2E_SCREENSHOT_DIR`.
