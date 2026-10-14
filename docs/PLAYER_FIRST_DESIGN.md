# Player-first welcome redesign

Reference: https://dribbble.com/shots/26467886-Minecraft-Server-Hosting-Game-Gaming-Landing-Page

The direction is a cinematic block-world scene, dark forest colors, large readable headings and an obvious action. The implementation is original Blockyard artwork and layout; the reference's images, branding, claims and copy are not reused.

The root page is now a public welcome page rather than a dashboard redirect. It explains choose → set up → invite, shows database-backed plan prices, and answers practical joining questions. Public GET /auth/config determines invitation-only messaging. Configuration or plan failures leave sign-in available and do not advertise an open signup/checkout flow. No authenticated server/admin API is requested by the public page. The private beta remains invitation-only; real payments are not introduced.

The signed-in dashboard leads with a world-focused banner. CPU/memory hosting totals remain available under Hosting details, rather than dominating the first screen. Existing server management pages retain their readable light design and their functional controls. Sign-in and server cards share the original scene. Usage failures take priority over stale player counts on cards.

Artwork: frontend/public/art/world-night.webp, 1536×1024, approximately 200 KB. Generated with the built-in image_gen tool, then encoded as WebP with Sharp. The Docker build copies public assets into both build and runtime stages. No Dribbble assets are deployed.

Final image-generation prompt:

> Use case: stylized-concept. Asset type: original wide website hero landscape for Blockyard, a friendly Minecraft Java hosting site. Create an exquisite cinematic voxel/block-built world, lush grassy floating terrain, little cozy wooden cabin with warm windows, winding turquoise river with waterfall, chunky pine and broadleaf trees, distant mountains, tiny warm lanterns. Dark midnight forest atmosphere with teal mist and emerald green highlights, inviting and adventurous rather than threatening. Composition: panoramic 3:2 or wide landscape, main cabin and interesting terrain on RIGHT TWO THIRDS, LEFT THIRD dark calm spacious forest sky to support white HTML headline; bottom fades toward very dark blue-green. High-quality 3D voxel illustration with crisp block geometry, subtle atmospheric light, artistic not an exact Minecraft screenshot. No text, no typography, no logos, no watermarks, no UI, no Minecraft characters, no creepers. This is website artwork, NOT a webpage mockup. Palette dark charcoal #0b1418, rich forest emerald, warm amber lamps, muted teal blue water. Detailed polished professional gaming website art, gentle magical mood.

Validation covers public beta messaging, database-backed prices, failed loading, private API isolation, local image delivery, preserved dashboard details and narrow/mobile/laptop layouts. Gameplay, payments and infrastructure settings are not changed by the redesign.
