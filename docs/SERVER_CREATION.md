# Server creation and first start

Creation is a single guided page with three jump links: choose how to play, choose a database-backed plan, then name and review. Paper is the available default for normal Java gameplay with optional server plugins. Fabric and Forge remain available only when configured on the backend. Version/software choices stay in a collapsed Advanced section. Creating a Forge/Fabric server does not import a modpack automatically.

Plan cards show memory, disk and recommended players. CPU, backup and database limits are tucked into Technical limits. Recommendations are not performance guarantees. The backend still resolves prices and resource allocations; the browser submits only plan ID, name, software and version.

Inputs are locked during submission. Failed requests preserve choices on the current page and retain the same idempotency key for an unchanged retry. Changing choices creates a new request key. Unsaved choices are not persisted across a browser refresh. Loading failures offer a read-only reload of plans, version options and checkout configuration; submission is blocked while required data is unavailable.

Provisioning progress uses existing lifecycle states and the existing ten-second polling. It shows confirmed order, installation and readiness without an invented percentage or estimated completion time. Failed setup keeps the existing server request; administrators can open recovery and customers can ask for help. No automatic retry or new purchase is triggered.

The first-session checklist names the next action based on installation, EULA and the live Minecraft power state. Once online, the owner can prepare a player invitation. Copying an address is not proof that a player connected.

Verification uses mocked order/provisioning browser flows and mobile/laptop layout checks. Deployment does not create an actual order/server, accept the EULA, change game settings or start Minecraft.
