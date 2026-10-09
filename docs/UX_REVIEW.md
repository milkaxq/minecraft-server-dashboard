# Responsive workspace review

The dashboard keeps its existing green visual style while prioritizing server
management over promotional content.

## Layout decisions

- Established workspaces show summaries and current servers immediately. The
  welcome banner is reserved for an empty workspace.
- Current servers appear first; cancelled hosting remains accessible through
  the All servers filter. Filtering changes presentation only, not stored data.
- Below 901px, a sticky header opens a native modal navigation menu. Every page,
  including Guide and Admin when authorized, remains reachable. Sign-out is
  available in the menu. Escape dismisses it and restores focus to its trigger;
  widening the window closes the mobile menu.
- Phone layouts use larger text, 44px controls and 16px form inputs. Summaries
  remain compact, search and filtering fill the available width, and long names
  and account identifiers wrap.
- Server power controls stay together. The joining address appears before the
  performance chart on phones. Feature navigation and file tables scroll within
  their own containers rather than widening the page.
- Keyboard users can skip directly to the main content. Navigation identifies
  the current page, and visible focus indicators remain available.

## Validation

Run `npm --prefix frontend run check`, `npm --prefix frontend run build`, and
`npm --prefix frontend test`. Browser coverage includes navigation dismissal and
sign-out, current/history filtering, phone input and button sizes, and overflow
checks at 320px, 768px, 1024px and 1366px.

Before/after screenshots and local live-review scripts are kept in the ignored
`.local/design-review` directory because they contain development account data.
No new assets or dependencies are needed for these changes.
