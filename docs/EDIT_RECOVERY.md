# Editing and recovery

File content, Minecraft settings and Voice Chat settings remain in the current page's memory while editing; configuration text is not persisted to browser storage.

## File editor

- Closing, reloading, opening another file or moving between folders asks for confirmation when content differs from its loaded version.
- Cancel preserves the draft and does not request a replacement file. Confirming a reload/open replaces the draft only after the read succeeds; a failed read leaves it available to copy or save.
- Refreshing the file list does not discard the open editor.
- Saving is disabled when unchanged or when the server is running. During a request the text remains readable/copyable but cannot be edited, and file navigation is disabled.
- Failed saves keep text and revision intact. An identical retry reuses the operation key. Successful saves close the editor and clear its dirty state.

## Settings

Minecraft settings use the same operation key when retrying identical input. The form is disabled during save so text typed after submission cannot be silently discarded when the saved data reloads. Input changes get a new key; successful saves clear the attempt. Existing server-side revision and ownership checks remain authoritative.

## Leaving the page

All three editing flows warn on full navigation, refresh or tab close using the browser's own warning. Ordinary same-tab HTTP(S) links use a themed confirmation dialog with a safe default of staying. Native dialog Escape keeps edits. New-tab links, downloads and same-page hash links do not discard the page and are not intercepted.

This is not an autosave or a universal router blocker: browser back/forward actions, sign-out and other programmatic redirects are not intercepted by the link dialog. Save or copy important edits before those actions. No server file is changed by either confirmation dialog alone.
