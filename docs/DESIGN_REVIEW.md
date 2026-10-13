# Dashboard design review

All dropdowns share `frontend/components/select.js`: custom menu styling, safe arrow spacing, keyboard navigation, typeahead, and viewport-aware placement. The shared stylesheet defines page/card spacing, readable secondary text, consistent buttons/fields, and responsive layouts across account, billing, admin and server management pages. On phones, shared tables become labeled cards so their actions stay visible; desktop tables retain their normal layout.

## Screenshots

From `frontend`, run:

```sh
npm run build
npm run test:visual
npm run design:gallery
```

Open `.local/design-review/index.html` in a browser. The gallery contains every route at laptop (1366 × 900) and phone (390 × 844) sizes, plus open-dropdown and installation-progress screenshots. Accounts and API responses are mocked, so screenshots contain no production credentials or customer data. Artifacts remain ignored by Git. Tests also check horizontal overflow and interaction behavior; the full suite additionally exercises 320, 768 and 1024 pixel widths.

Run `npm test` for the full browser suite and `npm run check` for lint/format checks. Catalog progress/receipt authorization and deletion behavior are covered by the Go/PostgreSQL management integration tests; see `ADDON_CATALOG.md` for operation semantics.
