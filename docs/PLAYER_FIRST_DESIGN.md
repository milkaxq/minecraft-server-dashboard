# Player-first welcome redesign

Reference: https://dribbble.com/shots/26467886-Minecraft-Server-Hosting-Game-Gaming-Landing-Page

The direction is a cinematic block-world scene, dark forest colors, large readable headings and an obvious action. The implementation is original Blockyard artwork and layout; the reference's images, branding, claims and copy are not reused.

The root page is now a public welcome page rather than a dashboard redirect. It explains choose → set up → invite, shows database-backed plan prices, and answers practical joining questions. Public GET /auth/config determines invitation-only messaging. Configuration or plan failures leave sign-in available and do not advertise an open signup/checkout flow. No authenticated server/admin API is requested by the public page. The private beta remains invitation-only; real payments are not introduced.

The signed-in dashboard leads with a world-focused banner. CPU/memory hosting totals remain available under Hosting details, rather than dominating the first screen. Existing server management pages retain their readable light design and their functional controls. Sign-in and server cards share the original scene. Usage failures take priority over stale player counts on cards.

Artwork: frontend/public/art/world-night.webp, 1536×1024, approximately 200 KB. Generated with the built-in image_gen tool, then encoded as WebP with Sharp. The Docker build copies public assets into both build and runtime stages. No Dribbble assets are deployed.

Final image-generation prompt:

> Use case: stylized-concept. Asset type: original wide website hero landscape for Blockyard, a friendly Minecraft Java hosting site. Create an exquisite cinematic voxel/block-built world, lush grassy floating terrain, little cozy wooden cabin with warm windows, winding turquoise river with waterfall, chunky pine and broadleaf trees, distant mountains, tiny warm lanterns. Dark midnight forest atmosphere with teal mist and emerald green highlights, inviting and adventurous rather than threatening. Composition: panoramic 3:2 or wide landscape, main cabin and interesting terrain on RIGHT TWO THIRDS, LEFT THIRD dark calm spacious forest sky to support white HTML headline; bottom fades toward very dark blue-green. High-quality 3D voxel illustration with crisp block geometry, subtle atmospheric light, artistic not an exact Minecraft screenshot. No text, no typography, no logos, no watermarks, no UI, no Minecraft characters, no creepers. This is website artwork, NOT a webpage mockup. Palette dark charcoal #0b1418, rich forest emerald, warm amber lamps, muted teal blue water. Detailed polished professional gaming website art, gentle magical mood.

Validation covers public beta messaging, database-backed prices, failed loading, private API isolation, local image delivery, preserved dashboard details and narrow/mobile/laptop layouts. Gameplay, payments and infrastructure settings are not changed by the redesign.

## Shared page design and server overview

The invite, authentication/recovery forms and all authenticated pages share the
night-world palette, lime primary actions, custom dropdowns and responsive
controls in `frontend/app/dashboard-theme.css`. Public invitations reuse the
original homepage artwork. The missing-page screen also provides a clear path
back to the dashboard.

The server overview presents, in order:

1. Current state and guarded Start/Stop/Restart controls.
2. Feature navigation, then the connection address and player invitation.
3. Expandable joining help, health/activity and version/server management.

The first-session checklist and connection troubleshooting live inside joining
help. Agreement/setup requirements open that help initially. Performance charts,
resource readings and maintenance controls no longer compete with the play flow.
A health summary remains visible even when its detailed section is closed. The
layout does not change server permissions, live game settings, player pack
validation or destructive-action confirmations.

### Why the separate Panel domain exists

`panel.playblockyard.com` serves Pterodactyl's administration UI and APIs. The Go
backend uses those APIs; Panel coordinates Wings, which runs the isolated game
containers. The customer dashboard lives at `app.playblockyard.com` and presents
simpler controls with application authorization. The root domain is the public
website. Separate hostnames keep the hosting tools and customer experience clear;
authentication, server authorization and credential boundaries provide security.
Removing the Panel service would break provisioning and management. Customers do
not need Panel accounts or its administration interface to use Blockyard.

## Daily next actions and installed addons

The play card now derives its guidance from provisioning, Minecraft agreement,
live power status and the current user's permissions. It offers one primary next
action: review the agreement, start the world, invite friends, finish checkout or
get help. Starting/stopping and incomplete checks show waiting guidance. Failed
live reads never use a stale Online state to recommend playing. The Start action
uses the same pending-request and agreement guards as the main controls.

Installed plugins/mods appear before catalog discovery. Their status distinguishes
an installed file from a confirmed runtime load: stopped servers say “Start to
load”; online servers offer “Check load status.” Guided configuration is labelled
as available rather than falsely claiming every addon requires configuration.
Other addons link to configuration files for users with file access. After addon
changes, the next-steps message follows the current power state; it does not claim
a successful start proves the addon loaded. Existing console/plugin checks remain
the way to confirm runtime status.
