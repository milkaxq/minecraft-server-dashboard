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

## Browsing files

The file browser searches names in the current folder only, without recursively listing subfolders or sending a new request for each search change. Sorting supports natural name order (including numbered filenames), reverse name order and largest files first, while keeping folders above files. Filtering/sorting does not mutate the loaded file list or server files.

Breadcrumbs return directly to any ancestor through the same unsaved-editor confirmation as folder navigation. Cancelling keeps the current path and draft; confirmed folder changes clear the folder search. Failed listings show an unavailable state instead of claiming the folder is empty, and Refresh retries the read.

Small file sizes display in bytes or KB instead of rounding to 0.0 MB. Memory and disk displays keep their existing MB/GB units for larger values; unavailable or invalid readings show an em dash.

## Finding text in a file

Find in file searches the loaded editor text, including unsaved changes. Search is literal and case-insensitive; punctuation is not interpreted as a regular expression. Next/Previous wrap through matches and select the matching text with its line number. Ctrl+F or Command+F opens search while the editor is focused; Enter/Shift+Enter navigate from the search field, and Escape closes it.

Queries are limited to 200 characters and results to the first 1,000 matches; broader searches show a notice to narrow the query. Searching does not change file contents, mark the editor dirty, send requests or persist queries. Existing save and discard protections still apply.
