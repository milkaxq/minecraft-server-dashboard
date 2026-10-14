# Homepage and dashboard domains

- https://playblockyard.com: public homepage, artwork and public plan/registration metadata.
- https://www.playblockyard.com: redirects to the root domain.
- https://app.playblockyard.com: authenticated dashboard, server controls, billing, account recovery and player invitation pages. Its root redirects to /dashboard.
- Panel and Wings remain on their existing subdomains.

Cloudflare DNS-only records are @ A 169.58.54.42 and www CNAME playblockyard.com. An importable zone is in deploy/homepage-dns.zone. Caddy manages HTTPS automatically once DNS resolves.

The homepage reads only GET/HEAD /api/plans and /api/auth/config on its own domain. Root-domain private URLs redirect to app with the path/query preserved. Login and registration links point explicitly to app. The marketing host does not restore an authenticated session or request private API data.

Go's FRONTEND_ORIGIN and public API URL remain on app. Cookie scope, CSRF checks, email verification, password recovery and existing invitation URLs are unchanged. The root domain does not proxy authentication mutations or private server APIs.

The Next layout reads Host to supply the correct public-page links and marketing-only session behavior. All authenticated pages share the forest/night dashboard theme. Server actions still use their existing permission, resource, EULA and confirmation checks.
