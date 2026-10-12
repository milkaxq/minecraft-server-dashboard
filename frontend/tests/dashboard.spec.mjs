import { test, expect } from "@playwright/test";
const serverID = "30000000-0000-4000-8000-000000000001";
const orderID = "50000000-0000-4000-8000-000000000001";
const planID = "10000000-0000-4000-8000-000000000001";
const user = {
  id: "20000000-0000-4000-8000-000000000001",
  name: "Alex",
  email: "alex@example.com",
  verified_at: "2026-10-01T10:00:00Z",
  created_at: "2026-10-01T10:00:00Z",
};
const plan = {
  id: planID,
  name: "Campfire",
  description: "A world for close friends.",
  price_cents: 975,
  currency: "USD",
  billing_period: "MONTHLY",
  ram_mb: 2048,
  disk_mb: 10240,
  cpu_limit: 100,
  backup_limit: 2,
  database_limit: 0,
  player_recommendation: 10,
  enabled: true,
};
const baseServer = {
  id: serverID,
  owner_id: user.id,
  plan_id: planID,
  name: "The Lantern Lodge",
  minecraft_version: "1.21.1",
  server_type: "PAPER",
  status: "RUNNING",
  hostname: "play.example.com",
  port: 25565,
  ram_mb: 2048,
  disk_mb: 10240,
  cpu_limit: 100,
  backup_limit: 2,
  database_limit: 0,
  role: "OWNER",
  permissions: [
    "view",
    "console",
    "start",
    "stop",
    "restart",
    "kill",
    "files",
    "settings",
    "plugins",
    "worlds",
    "members",
    "backups",
    "delete",
  ],
  management_available: true,
  console_available: true,
};
async function mockAPI(
  page,
  {
    role = "OWNER",
    servers = [baseServer],
    errorList = false,
    isAdmin = false,
    paymentDisabled = false,
    failedServer = false,
    eulaAccepted = true,
    eulaUnavailable = false,
    serverState = "RUNNING",
    initiallyRunning = false,
    inviteRequired = false,
    holdStop = false,
    ignoreWhitelist = false,
  } = {},
) {
  let loggedIn = false;
  let running = initiallyRunning;
  let deletionPending = false;
  let whitelistEnabled = true;
  let whitelistPlayers = [{ name: "milkaxq", uuid: "60000000-0000-4000-8000-000000000001" }];
  let safetyBackup = false;
  let worldGenerated = true;
  let sharedMembers = [{ user_id: user.id, name: user.name, email: user.email, role: "OWNER" }];
  let paid = false;
  let ending = false;
  const calls = [];
  const authUser = { ...user, is_admin: isAdmin };
  const order = {
    id: orderID,
    user_id: user.id,
    server_id: serverID,
    plan_id: planID,
    plan_name: plan.name,
    price_cents: plan.price_cents,
    currency: plan.currency,
    billing_period: plan.billing_period,
    kind: "NEW",
    status: "PENDING",
    created_at: "2026-10-08T10:00:00Z",
  };
  const server = {
    ...(servers[0] || baseServer),
    status: serverState,
    role,
    permissions:
      role === "MODERATOR" ? ["view", "console", "start", "restart"] : baseServer.permissions,
  };
  await page.route("http://localhost:8080/**", async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    const body = request.headers()["content-type"]?.startsWith("application/json")
      ? request.postDataJSON()
      : null;
    const path = url.pathname;
    calls.push({
      path,
      query: url.searchParams.toString(),
      bytes: request.postDataBuffer()?.length || 0,
      method: request.method(),
      body,
      headers: request.headers(),
    });
    const reply = (data, status = 200) =>
      route.fulfill({ status, contentType: "application/json", body: JSON.stringify(data) });
    if (path === "/auth/session")
      return loggedIn
        ? reply({
            csrf_token: "c".repeat(43),
            expires_at: new Date(Date.now() + 86400000).toISOString(),
            user: authUser,
          })
        : reply({ error: "invalid credentials" }, 401);
    if (path === "/auth/session-login") {
      loggedIn = true;
      return reply({
        csrf_token: "c".repeat(43),
        expires_at: new Date(Date.now() + 86400000).toISOString(),
        user: authUser,
      });
    }
    if (path === "/auth/forgot-password")
      return reply(
        {
          message:
            "If a verified account exists for that email, a password reset link will be sent. Check your inbox and spam folder.",
        },
        202,
      );
    if (path === "/auth/reset-password" || path === "/account/password") {
      loggedIn = false;
      return route.fulfill({ status: 204 });
    }
    if (path === "/auth/config") return reply({ invite_required: inviteRequired });
    if (path === "/admin/invitations") return reply([]);
    if (path === "/auth/register") return reply(user, 201);
    if (path === "/auth/logout") {
      loggedIn = false;
      return route.fulfill({ status: 204 });
    }
    if (path === "/auth/verify-email") return route.fulfill({ status: 204 });
    if (path === "/auth/resend-verification")
      return reply({ message: "Verification email sent." }, 202);
    if (path === "/plans") return reply([plan]);
    if (path === "/account") return reply(authUser);

    if (path === "/billing/config")
      return reply({
        provider: paymentDisabled ? "disabled" : "mock",
        checkout_enabled: !paymentDisabled,
        collects_money: false,
      });
    if (path === "/orders" && request.method() === "POST") return reply(order, 201);
    if (path === "/orders") return reply([{ ...order, status: paid ? "PAID" : "PENDING" }]);
    if (path === `/orders/${orderID}`)
      return reply({ ...order, status: paid ? "PAID" : "PENDING" });
    if (path === `/orders/${orderID}/checkout`)
      return reply({
        provider: "mock",
        url: `/checkout/${orderID}`,
        reference: "test-only-reference",
      });
    if (path === `/orders/${orderID}/mock-pay`) {
      paid = true;
      return reply({ status: "confirmed" });
    }
    if (path === "/subscriptions")
      return reply([
        {
          id: "60000000-0000-4000-8000-000000000001",
          server_id: serverID,
          plan_id: planID,
          server_name: baseServer.name,
          status: "ACTIVE",
          current_period_end: new Date(Date.now() + 30 * 86400000).toISOString(),
          price_cents: 975,
          currency: "USD",
          billing_period: "MONTHLY",
          cancel_at_period_end: ending,
        },
      ]);
    if (path === `/servers/${serverID}/renewals`) return reply({ ...order, kind: "RENEWAL" }, 201);
    if (path === `/servers/${serverID}/subscription`) {
      ending = body.cancel_at_period_end;
      return route.fulfill({ status: 204 });
    }
    if (path === "/admin/plans" && request.method() === "GET") return reply([plan]);
    if (path === "/admin/plans" || path === `/admin/plans/${planID}`)
      return reply({ ...plan, ...body });
    if (path === "/admin/servers")
      return reply([
        {
          ...baseServer,
          status: failedServer ? "FAILED" : "RUNNING",
          owner_email: user.email,
          plan_name: plan.name,
          job_status: failedServer ? "FAILED" : "SUCCEEDED",
          job_action: "PROVISION",
          job_error: failedServer ? "node_capacity_unavailable" : null,
        },
      ]);
    if (path === "/admin/users") return reply([authUser]);
    if (path === "/admin/orders") return reply([order]);
    if (path === "/admin/nodes")
      return reply([
        {
          id: planID,
          name: "EU node",
          cpu_capacity: 1000,
          ram_capacity_mb: 20480,
          disk_capacity_mb: 102400,
          allocated_cpu: 100,
          allocated_ram_mb: 2048,
          allocated_disk_mb: 10240,
          enabled: true,
        },
      ]);
    if (path === "/admin/audit")
      return reply([
        {
          id: orderID,
          action: "plan.updated",
          actor_id: user.id,
          server_id: serverID,
          request_id: "test-request",
          created_at: "2026-10-08T10:00:00Z",
        },
      ]);
    if (path.startsWith("/admin/servers/")) return reply({ id: orderID, status: "PENDING" }, 202);
    if (path === "/admin/beta-servers" && request.method() === "POST")
      return reply({ ...server, ...body, status: "PAYMENT_CONFIRMED" }, 201);
    if (path === "/minecraft/options")
      return reply([
        { server_type: "PAPER", minecraft_version: "1.21.1" },
        { server_type: "PAPER", minecraft_version: "1.21.10" },
        { server_type: "PAPER", minecraft_version: "26.3" },
        ...["VANILLA", "SPIGOT", "PURPUR", "FABRIC", "FORGE", "NEOFORGE"].map((server_type) => ({
          server_type,
          minecraft_version: "1.21.1",
        })),
      ]);
    if (path === "/servers" && request.method() === "GET")
      return errorList
        ? reply({ error: "Service temporarily unavailable" }, 503)
        : reply(
            servers.map((s) =>
              s.id === serverID ? { ...s, deletion_pending: deletionPending } : s,
            ),
          );
    if (path === "/servers" && request.method() === "POST")
      return reply({ ...server, ...body, status: "PENDING_PAYMENT" }, 201);
    if (path === `/servers/${serverID}`) return reply(server);
    if (path === `/servers/${serverID}/eula`) {
      if (eulaUnavailable) return reply({ error: "Cannot check agreement" }, 502);
      if (request.method() === "POST") eulaAccepted = body.accepted === true;
      return reply({
        accepted: eulaAccepted,
        can_accept: role === "OWNER",
        url: "https://www.minecraft.net/en-us/eula",
      });
    }
    if (path === `/servers/${serverID}/usage`)
      return reply({
        power_state: running ? "running" : "offline",
        cpu_percent: running ? 32.5 : 0,
        memory_bytes: running ? 1200000000 : 0,
        disk_bytes: 2147483648,
        uptime_ms: running ? 420000 : 0,
        players_online: running ? 3 : 0,
        players_max: 10,
        players_available: true,
      });

    if (path === `/servers/${serverID}/settings` && request.method() === "GET")
      return reply({
        motd: "Welcome friends",
        difficulty: "normal",
        gamemode: "survival",
        hardcore: false,
        pvp: true,
        max_players: 10,
        whitelist: false,
        online_mode: true,
        view_distance: 10,
        spawn_protection: 16,
        revision: "a".repeat(64),
      });
    if (path === `/servers/${serverID}/whitelist`) {
      if (request.method() === "GET")
        return reply({
          enabled: whitelistEnabled,
          players: whitelistPlayers,
          revision: "d".repeat(64),
        });
      if (!ignoreWhitelist) {
        if (body.action === "add")
          whitelistPlayers.push({ name: body.name, uuid: "60000000-0000-4000-8000-000000000002" });
        if (body.action === "remove")
          whitelistPlayers = whitelistPlayers.filter((p) => p.name !== body.name);
        if (body.action === "on" || body.action === "off") whitelistEnabled = body.action === "on";
      }
      return reply({ confirmed: !ignoreWhitelist });
    }
    if (path === `/servers/${serverID}/settings`)
      return reply({ status: "saved", restart_required: true });
    if (path === `/servers/${serverID}/files` && request.method() === "GET")
      return reply([
        { name: "server.properties", is_file: true, is_symlink: false, size: 24 },
        { name: "world", is_file: false, is_symlink: false, size: 0 },
      ]);
    if (path === `/servers/${serverID}/files/text`)
      return reply({
        path: "/server.properties",
        content: "motd=Welcome friends\n",
        revision: "b".repeat(64),
      });
    if (path === `/servers/${serverID}/files` || path.endsWith("/files/upload"))
      return reply({ status: "saved", restart_required: true });
    if (
      (path === `/servers/${serverID}/plugins` || path === `/servers/${serverID}/mods`) &&
      request.method() === "GET"
    )
      return reply([{ name: "Essentials.jar", is_file: true, size: 1048576 }]);
    if (
      path === `/servers/${serverID}/plugins` ||
      path === `/servers/${serverID}/mods` ||
      path.endsWith("/mods/upload") ||
      path.endsWith("/plugins/upload")
    )
      return reply({ status: "saved", restart_required: true });
    if (path.endsWith("/catalog"))
      return reply([
        {
          project_id: "viaversion",
          title: "ViaVersion",
          description: "Allow newer clients to join.",
        },
      ]);
    if (path.endsWith("/catalog/viaversion") && request.method() === "GET")
      return reply({
        project: "viaversion",
        software: "PAPER",
        minecraft_version: "1.21.1",
        revision: "c".repeat(64),
        files: [{ version_id: "stable", filename: "ViaVersion.jar", size: 6500000 }],
      });
    if (path.endsWith("/catalog/viaversion"))
      return reply({ status: "installed", restart_required: true });
    if (path === `/servers/${serverID}/members` && request.method() === "GET")
      return reply(sharedMembers);
    if (path === `/servers/${serverID}/members`) {
      const member = {
        user_id: "60000000-0000-4000-8000-000000000001",
        name: "Sam",
        email: body.email,
        role: body.role,
      };
      sharedMembers.push(member);
      return reply(member, 201);
    }
    if (path.startsWith(`/servers/${serverID}/members/`)) {
      const id = path.split("/").at(-1);
      if (request.method() === "DELETE")
        sharedMembers = sharedMembers.filter((m) => m.user_id !== id);
      else
        sharedMembers = sharedMembers.map((m) =>
          m.user_id === id ? { ...m, role: body.role } : m,
        );
      return reply({ status: "saved" });
    }
    if (path === `/servers/${serverID}/world`)
      return reply({
        name: "world",
        directories: ["world", "world_nether", "world_the_end"],
        generated: worldGenerated,
        revision: "a".repeat(64),
        safety_backup: safetyBackup ? "40000000-0000-4000-8000-000000000001" : "",
      });
    if (path === `/servers/${serverID}/world/backup`) {
      safetyBackup = true;
      return reply({ uuid: "40000000-0000-4000-8000-000000000001" }, 202);
    }
    if (path === `/servers/${serverID}/world/download`)
      return route.fulfill({
        status: 200,
        contentType: "application/zip",
        body: Buffer.from("world-archive"),
      });
    if (
      path === `/servers/${serverID}/world/reset` ||
      path === `/servers/${serverID}/world/upload`
    ) {
      worldGenerated = path.endsWith("upload");
      safetyBackup = false;
      return reply({ status: "saved", restart_required: true });
    }
    if (path === `/servers/${serverID}/backups` && request.method() === "GET")
      return reply([
        {
          uuid: "40000000-0000-4000-8000-000000000001",
          name: "Before changes",
          is_successful: true,
          is_locked: true,
          bytes: 1048576,
          created_at: "2026-10-07T10:00:00Z",
          completed_at: "2026-10-07T10:00:10Z",
        },
      ]);
    if (path === `/servers/${serverID}/backups` || path.includes("/backups/"))
      return reply({ status: "accepted" }, 202);
    if (path === `/servers/${serverID}/console-ticket`)
      return reply(
        { ticket: "t".repeat(43), expires_at: new Date(Date.now() + 45000).toISOString() },
        201,
      );
    if (path === `/servers/${serverID}/commands`) return reply({ status: "accepted" }, 202);
    if (path.startsWith(`/servers/${serverID}/`)) {
      const action = path.split("/").at(-1);
      if (action === "terminate") {
        deletionPending = true;
        return reply({ id: orderID, status: "PENDING" }, 202);
      }
      if (!(action === "stop" && holdStop)) running = ["start", "restart"].includes(action);
      return reply({ status: "accepted" }, 202);
    }
    return reply({ error: "Not found" }, 404);
  });
  return calls;
}
async function login(page) {
  await page.goto("/login");
  await page.getByLabel("Email address").fill(user.email);
  await page.getByLabel("Password", { exact: true }).fill("test-password-long");
  await page.getByRole("button", { name: "Sign in", exact: true }).click();
  await expect(page.getByRole("heading", { name: "Welcome back, Alex." })).toBeVisible();
}
test("authentication, database-backed plans and cookie session", async ({ page }, testInfo) => {
  await mockAPI(page);
  await login(page);
  await expect(page.getByRole("heading", { name: baseServer.name })).toBeVisible();
  await expect(page.getByText("Campfire", { exact: true })).toBeVisible();
  await expect(page.getByText("$9.75")).toBeVisible();
  await page.screenshot({ path: testInfo.outputPath("dashboard.png"), fullPage: true });
  expect(await page.evaluate(() => localStorage.length)).toBe(0);
  expect(await page.evaluate(() => sessionStorage.length)).toBe(0);
  await page.getByRole("link", { name: "Account", exact: true }).click();
  await expect(page.getByRole("heading", { name: "Keep your account safe." })).toBeVisible();
  await page.getByRole("button", { name: "Sign out of this session" }).click();
  await expect(page.getByRole("heading", { name: "Good to see you again." })).toBeVisible();
});
test("order request sends approved choices without resource or price fields", async ({ page }) => {
  const calls = await mockAPI(page);
  await login(page);
  await page.getByRole("link", { name: "New server", exact: true }).click();
  await page.getByLabel("Server name", { exact: true }).fill("A new adventure");
  await expect(page.getByRole("combobox", { name: "Minecraft version", exact: true })).toHaveValue(
    "26.3",
  );
  await page.getByRole("button", { name: "Continue to checkout" }).click();
  await expect(page).toHaveURL(new RegExp(`/checkout/${orderID}$`));
  const call = calls.find((c) => c.path === "/orders" && c.method === "POST");
  expect(call.body).toEqual({
    plan_id: planID,
    name: "A new adventure",
    minecraft_version: "26.3",
    server_type: "PAPER",
  });
});
test("owner power controls use a request key and refresh real status", async ({ page }) => {
  const calls = await mockAPI(page);
  await login(page);
  await page
    .getByRole("link")
    .filter({ has: page.getByRole("heading", { name: baseServer.name }) })
    .click();
  await expect(page.getByRole("button", { name: "Stop", exact: true })).toBeDisabled();
  await page.getByRole("button", { name: "Start", exact: true }).click();
  await expect(page.getByRole("button", { name: "Start", exact: true })).toBeDisabled();
  await expect(page.getByRole("button", { name: "Stop", exact: true })).toBeEnabled();
  const call = calls.find((c) => c.path.endsWith("/start") && c.method === "POST");
  expect(call.headers["idempotency-key"]).toMatch(/^[0-9a-f-]{36}$/);
  expect(call.body).toEqual({});
  await expect(page.getByText("7m", { exact: true })).toBeVisible();
});
test("moderator UI excludes Stop and Force stop; console commands use REST", async ({ page }) => {
  const calls = await mockAPI(page, { role: "MODERATOR" });
  await page.routeWebSocket(`ws://localhost:8080/servers/${serverID}/console`, (ws) => {
    ws.send(JSON.stringify({ event: "connected" }));
    ws.send(
      JSON.stringify({ event: "console output", args: ["[Server] Welcome back, adventurer."] }),
    );
  });
  await login(page);
  await page
    .getByRole("link")
    .filter({ has: page.getByRole("heading", { name: baseServer.name }) })
    .click();
  await expect(page.getByRole("button", { name: "Stop", exact: true })).toHaveCount(0);
  await page.getByRole("link", { name: "Advanced", exact: true }).click();
  await expect(page.getByRole("button", { name: "Force stop", exact: true })).toHaveCount(0);
  await expect(page.getByText("[Server] Welcome back, adventurer.", { exact: true })).toBeVisible();
  await page.getByLabel("Minecraft console command").fill("list");
  await page.getByRole("button", { name: "Send", exact: true }).click();
  await expect(page.getByText("Command accepted.", { exact: true })).toBeVisible();
  expect(calls.find((c) => c.path.endsWith("/commands")).body).toEqual({ command: "list" });
});
test("force stop requires explicit confirmation", async ({ page }) => {
  const calls = await mockAPI(page);
  await page.routeWebSocket(`ws://localhost:8080/servers/${serverID}/console`, (ws) =>
    ws.send(JSON.stringify({ event: "connected" })),
  );
  await login(page);
  await page
    .getByRole("link")
    .filter({ has: page.getByRole("heading", { name: baseServer.name }) })
    .click();
  await page.getByRole("button", { name: "Start", exact: true }).click();
  await expect(page.getByRole("button", { name: "Stop", exact: true })).toBeEnabled();
  await page.getByRole("link", { name: "Advanced", exact: true }).click();
  await page.getByRole("button", { name: "Force stop", exact: true }).click();
  await expect(page.getByRole("dialog")).toBeVisible();
  expect(calls.filter((c) => c.path.endsWith("/kill"))).toHaveLength(0);
  await page.getByRole("dialog").getByRole("button", { name: "Force stop", exact: true }).click();
  await expect(page.getByRole("dialog")).not.toBeVisible();
  expect(calls.find((c) => c.path.endsWith("/kill")).body).toEqual({ confirmation: serverID });
});
test("empty workspace and unavailable API show useful states", async ({ page }) => {
  await mockAPI(page, { servers: [] });
  await login(page);
  await expect(
    page.getByRole("heading", { name: "Your next adventure starts here" }),
  ).toBeVisible();
  await page.unroute("http://localhost:8080/**");
  await mockAPI(page, { errorList: true });
  await page.getByRole("link", { name: "Servers", exact: true }).click();
  await expect(
    page.getByRole("alert").filter({ hasText: "Service temporarily unavailable" }),
  ).toBeVisible();
  await expect(page.getByRole("button", { name: "Try again" })).toBeVisible();
});
test("registration and verification flows", async ({ page }) => {
  const calls = await mockAPI(page);
  await page.goto("/register");
  await page.getByLabel("Your name").fill("Alex");
  await page.getByLabel("Email address").fill(user.email);
  await page.getByLabel("Password", { exact: true }).fill("a-long-test-password");
  await page.getByRole("button", { name: "Create account" }).click();
  await expect(page).toHaveURL(/\/verify$/);
  await page.getByLabel("Verification token", { exact: true }).fill("v".repeat(43));
  await page.getByRole("button", { name: "Verify email", exact: true }).click();
  await expect(page.getByRole("status")).toContainText("Email verified");
  expect(calls.find((c) => c.path === "/auth/register").body.name).toBe("Alex");
});
test("dashboard and server controls fit a mobile viewport", async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await mockAPI(page);
  await login(page);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(
    true,
  );
  await page
    .getByRole("link")
    .filter({ has: page.getByRole("heading", { name: baseServer.name }) })
    .click();
  await expect(page.getByRole("button", { name: "Start", exact: true })).toBeVisible();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(
    true,
  );
});

async function openServer(page) {
  await login(page);
  await page
    .getByRole("link")
    .filter({ has: page.getByRole("heading", { name: baseServer.name }) })
    .click();
}
test("settings save actual values and revision while preserving plan allocations", async ({
  page,
}, testInfo) => {
  const calls = await mockAPI(page);
  await openServer(page);
  await page.getByRole("link", { name: "Settings", exact: true }).click();
  await page.getByRole("textbox", { name: /Server message/ }).fill("Our next adventure");
  await page.getByRole("combobox", { name: "Difficulty" }).selectOption("hard");
  await page.getByRole("spinbutton", { name: "Maximum players" }).fill("30");
  await page.getByRole("button", { name: "Save settings" }).click();
  await expect(page.getByRole("status")).toContainText("Settings saved");
  const call = calls.find((c) => c.path.endsWith("/settings") && c.method === "PUT");
  expect(call.body.motd).toBe("Our next adventure");
  expect(call.body.revision).toBe("a".repeat(64));
  expect(call.body).not.toHaveProperty("ram_mb");
  await page.screenshot({ path: testInfo.outputPath("settings.png"), fullPage: true });
});
test("advanced files edit with revision and require deletion confirmation", async ({ page }) => {
  const calls = await mockAPI(page);
  await page.routeWebSocket(`ws://localhost:8080/servers/${serverID}/console`, (ws) =>
    ws.send(JSON.stringify({ event: "connected" })),
  );
  await openServer(page);
  await page.getByRole("link", { name: "Advanced", exact: true }).click();
  await page.getByRole("link", { name: "Files", exact: true }).click();
  await page.getByRole("button", { name: "server.properties", exact: true }).click();
  await page.getByRole("textbox", { name: "File content" }).fill("motd=Changed");
  await page.getByRole("button", { name: "Save file", exact: true }).click();
  await expect(page.getByRole("status")).toContainText("File saved");
  const save = calls.find((c) => c.path.endsWith("/files") && c.method === "POST");
  expect(save.body.revision).toBe("b".repeat(64));
  expect(save.body.path).toBe("/server.properties");
  await page
    .getByRole("row")
    .filter({ hasText: "server.properties" })
    .getByRole("button", { name: "Delete", exact: true })
    .click();
  await expect(page.getByRole("dialog")).toBeVisible();
  expect(calls.filter((c) => c.body?.action === "delete")).toHaveLength(0);
  await page.getByRole("dialog").getByRole("button", { name: "Delete", exact: true }).click();
  await expect(page.getByRole("dialog")).not.toBeVisible();
  expect(calls.find((c) => c.body?.action === "delete").body.confirmation).toBe(
    "/server.properties",
  );
});
test("Paper plugin uploads raw binary and confirms removal", async ({ page }) => {
  const calls = await mockAPI(page);
  await openServer(page);
  await page.getByRole("link", { name: "Plugins", exact: true }).click();
  await expect(page.getByText("Essentials.jar", { exact: true })).toBeVisible();
  await page.locator("input[type=file]").setInputFiles({
    name: "Example.jar",
    mimeType: "application/java-archive",
    buffer: Buffer.from("test-only binary jar"),
  });
  await expect(page.getByRole("status")).toContainText("Plugin uploaded");
  const upload = calls.find((c) => c.path.endsWith("/plugins/upload"));
  expect(upload.headers["content-type"]).toBe("application/octet-stream");
  expect(upload.bytes).toBeGreaterThan(0);
  await page.getByRole("button", { name: "Delete plugin", exact: true }).click();
  await expect(page.getByRole("dialog")).toBeVisible();
  await page
    .getByRole("dialog")
    .getByRole("button", { name: "Delete plugin", exact: true })
    .click();
  await expect(page.getByRole("dialog")).not.toBeVisible();
  expect(calls.find((c) => c.path.endsWith("/plugins") && c.method === "DELETE").body).toEqual({
    name: "Essentials.jar",
    confirmation: "Essentials.jar",
  });
});
test("backup restore replaces files only after explicit confirmation", async ({ page }) => {
  const calls = await mockAPI(page);
  await openServer(page);
  await page.getByRole("link", { name: "Backups", exact: true }).click();
  await expect(page.getByText("Before changes", { exact: true })).toBeVisible();
  await page.getByRole("button", { name: "Restore", exact: true }).click();
  await expect(page.getByRole("dialog")).toContainText("replaces all current server files");
  expect(calls.filter((c) => c.path.endsWith("/restore"))).toHaveLength(0);
  await page
    .getByRole("dialog")
    .getByRole("button", { name: "Restore backup", exact: true })
    .click();
  await expect(page.getByRole("status")).toContainText("Restore requested");
  expect(calls.find((c) => c.path.endsWith("/restore")).body.confirmation).toBe(
    "40000000-0000-4000-8000-000000000001",
  );
});

test("mock checkout confirms payment and links to the provisioned server", async ({
  page,
}, testInfo) => {
  const calls = await mockAPI(page);
  await login(page);
  await page.getByRole("link", { name: "New server", exact: true }).click();
  await page.getByLabel("Server name", { exact: true }).fill("Our adventure");
  await page.getByRole("button", { name: "Continue to checkout" }).click();
  await expect(page.getByText("Development demo checkout", { exact: true })).toBeVisible();
  await page.getByRole("button", { name: "Confirm demo payment" }).click();
  await expect(page.getByRole("status")).toContainText("Demo payment confirmed");
  await page.getByRole("link", { name: "Go to your server" }).click();
  await expect(page).toHaveURL(new RegExp(`/servers/${serverID}$`));
  expect(calls.find((c) => c.path.endsWith("/mock-pay")).body).toEqual({});
  expect(
    calls.find((c) => c.path === "/orders" && c.method === "POST").headers["idempotency-key"],
  ).toMatch(/^[0-9a-f-]{36}$/);
  await page.screenshot({ path: testInfo.outputPath("paid-server.png"), fullPage: true });
});
test("billing displays paid periods and renews without client-side price input", async ({
  page,
}) => {
  const calls = await mockAPI(page);
  await login(page);
  await page.getByRole("link", { name: "Billing", exact: true }).click();
  await expect(page.getByRole("heading", { name: "Your subscriptions" })).toBeVisible();
  await page.getByRole("button", { name: "End after this period" }).click();
  await expect(page.getByRole("button", { name: "Keep service" })).toBeVisible();
  await page.getByRole("button", { name: "Renew", exact: true }).click();
  await expect(page).toHaveURL(new RegExp(`/checkout/${orderID}$`));
  expect(calls.find((c) => c.path.endsWith("/renewals")).body).toEqual({});
  expect(calls.find((c) => c.path.endsWith("/subscription")).body).toEqual({
    cancel_at_period_end: true,
  });
});
test("admin edits plans and confirms server termination with a reason", async ({
  page,
}, testInfo) => {
  const calls = await mockAPI(page, { isAdmin: true });
  await login(page);
  await page.getByRole("link", { name: "Admin", exact: true }).click();
  await expect(page.getByRole("heading", { name: "Admin workspace." })).toBeVisible();
  await page.getByRole("button", { name: "Plans", exact: true }).click();
  await page.getByRole("button", { name: "Edit Campfire" }).click();
  await page.getByRole("spinbutton", { name: "Price (cents)" }).fill("1250");
  await page.getByRole("button", { name: "Save plan" }).click();
  await expect(page.getByRole("status")).toContainText("Plan saved");
  expect(
    calls.find((c) => c.path === `/admin/plans/${planID}` && c.method === "PUT").body.price_cents,
  ).toBe(1250);
  await page.getByRole("button", { name: "Servers", exact: true }).click();
  await page.getByRole("button", { name: "Terminate", exact: true }).click();
  await expect(page.getByRole("dialog")).toContainText("all server data");
  expect(calls.filter((c) => c.path.endsWith("/terminate"))).toHaveLength(0);
  await page.getByRole("textbox", { name: "Reason" }).fill("Customer requested deletion");
  await page.getByRole("button", { name: "Confirm action" }).click();
  await expect(page.getByRole("dialog")).not.toBeVisible();
  expect(calls.find((c) => c.path.endsWith("/terminate")).body.confirmation).toBe(serverID);
  await page.screenshot({ path: testInfo.outputPath("admin.png"), fullPage: true });
});
test("non-admin account has no admin navigation", async ({ page }) => {
  const calls = await mockAPI(page);
  await login(page);
  await expect(page.getByRole("link", { name: "Admin", exact: true })).toHaveCount(0);
  await page.getByRole("link", { name: "Account", exact: true }).click();
  await page.getByRole("link", { name: "Dashboard", exact: true }).click();
  expect(calls.some((c) => c.path.startsWith("/admin/"))).toBe(false);
});
test("disabled payment configuration prevents creating an order", async ({ page }) => {
  const calls = await mockAPI(page, { paymentDisabled: true });
  await login(page);
  await page.getByRole("link", { name: "New server", exact: true }).click();
  await page.getByLabel("Server name", { exact: true }).fill("Our adventure");
  await expect(page.getByRole("button", { name: "Continue to checkout" })).toBeDisabled();
  expect(calls.filter((c) => c.path === "/orders" && c.method === "POST")).toHaveLength(0);
});

test("beta operator creates the latest configured Paper server without checkout", async ({
  page,
}) => {
  const calls = await mockAPI(page, { paymentDisabled: true, isAdmin: true, inviteRequired: true });
  await login(page);
  await page.getByRole("link", { name: "New server", exact: true }).click();
  await page.getByLabel("Server name", { exact: true }).fill("Latest world");
  await page.getByLabel("Minecraft version").selectOption("26.3");
  await page.getByRole("button", { name: "Create beta server" }).click();
  await expect(page).toHaveURL(new RegExp(`/servers/${serverID}$`));
  const create = calls.find((call) => call.path === "/admin/beta-servers");
  expect(create.body.minecraft_version).toBe("26.3");
  expect(create.body).not.toHaveProperty("docker_image");
  expect(calls.filter((call) => call.path === "/orders" && call.method === "POST")).toHaveLength(0);
});

test("stop remains visibly pending until the server goes offline", async ({ page }) => {
  const calls = await mockAPI(page, { initiallyRunning: true, holdStop: true });
  await login(page);
  await page.locator(`a[href="/servers/${serverID}"]`).first().click();
  await page.getByRole("button", { name: "Stop", exact: true }).click();
  await expect(page.getByRole("button", { name: "Shutting down…" })).toBeDisabled();
  await expect(page.getByText("Stopping", { exact: true }).first()).toBeVisible();
  await expect(page.getByRole("status")).toContainText("Shutting down safely");
  expect(calls.filter((call) => call.path === `/servers/${serverID}/stop`)).toHaveLength(1);
});

test("owner must type the server name before deletion is queued", async ({ page }) => {
  const calls = await mockAPI(page);
  await login(page);
  await page.locator(`a[href="/servers/${serverID}"]`).first().click();
  await page.getByRole("button", { name: "Delete server" }).click();
  await expect(page.getByRole("dialog")).toContainText("world, files and backups");
  await expect(
    page.getByRole("dialog").getByRole("button", { name: "Delete server" }),
  ).toBeDisabled();
  await page.getByRole("dialog").getByRole("textbox").fill(baseServer.name);
  await page.getByRole("dialog").getByRole("button", { name: "Delete server" }).click();
  await expect(page).toHaveURL(/\/servers$/);
  const deletion = calls.find((call) => call.path === `/servers/${serverID}/terminate`);
  expect(deletion.body.confirmation).toBe(serverID);
  await expect(page.getByRole("status")).toContainText("being deleted");
  await expect(page.locator(`a.server-card[href="/servers/${serverID}"]`)).toHaveCount(0);
  await page.getByLabel("Server view").selectOption("all");
  await expect(page.getByText("Deleting", { exact: true })).toBeVisible();
  await page.reload();
  await page.getByLabel("Server view").selectOption("all");
  await expect(page.getByText("Deleting", { exact: true })).toBeVisible();
});

test("first start requires an explicit owner agreement", async ({ page }) => {
  const calls = await mockAPI(page, { eulaAccepted: false });
  await login(page);
  await page.locator(`a[href="/servers/${serverID}"]`).first().click();
  const accept = page.getByRole("button", { name: "Accept agreement", exact: true });
  await expect(accept).toBeDisabled();
  await expect(page.getByRole("button", { name: "Start", exact: true })).toBeDisabled();
  for (const width of [1280, 390]) {
    await page.setViewportSize({ width, height: 900 });
    const checkbox = await page
      .getByRole("checkbox", { name: /I have read and accept/ })
      .boundingBox();
    const label = await page.locator(".eula-checkbox").boundingBox();
    expect(checkbox.width).toBeLessThanOrEqual(20);
    expect(checkbox.height).toBeLessThanOrEqual(20);
    expect(label.x + label.width).toBeLessThanOrEqual(width);
  }
  expect(calls.some((c) => c.path.endsWith("/eula") && c.method === "POST")).toBe(false);
  await page.getByRole("checkbox", { name: /I have read and accept/ }).check();
  await accept.click();
  await expect(page.getByRole("button", { name: "Start", exact: true })).toBeEnabled();
  const call = calls.find((c) => c.path.endsWith("/eula") && c.method === "POST");
  expect(call.body).toEqual({ accepted: true });
  expect(call.headers["idempotency-key"]).toMatch(/^[0-9a-f-]{36}$/);
  await expect(page.getByText("Agreement accepted. Your server is ready to start.")).toBeVisible();
});

test("moderators wait for the owner to accept the agreement", async ({ page }) => {
  await mockAPI(page, { role: "MODERATOR", eulaAccepted: false });
  await login(page);
  await page.locator(`a[href="/servers/${serverID}"]`).first().click();
  await expect(page.getByText(/Ask the server owner to accept/)).toBeVisible();
  await expect(page.getByRole("button", { name: "Accept agreement" })).toHaveCount(0);
  await expect(page.getByRole("button", { name: "Start", exact: true })).toBeDisabled();
});

test("agreement read failure blocks restart but keeps stop available", async ({ page }) => {
  await mockAPI(page, { eulaUnavailable: true, initiallyRunning: true });
  await login(page);
  await page.locator(`a[href="/servers/${serverID}"]`).first().click();
  await expect(page.getByText("Cannot check agreement")).toBeVisible();
  await expect(page.getByRole("button", { name: "Restart", exact: true })).toBeDisabled();
  await expect(page.getByRole("button", { name: "Stop", exact: true })).toBeEnabled();
});

for (const [serverState, message] of [
  ["PROVISIONING", "Minecraft is installing."],
  ["FAILED", "Setup didn’t finish."],
]) {
  test(`setup guidance for ${serverState}`, async ({ page }) => {
    const calls = await mockAPI(page, { serverState });
    await login(page);
    await page.locator(`a[href="/servers/${serverID}"]`).first().click();
    await expect(page.getByText(message, { exact: true })).toBeVisible();
    await expect(page.getByRole("button", { name: "Start", exact: true })).toBeDisabled();
    expect(calls.some((c) => c.path.endsWith("/eula"))).toBe(false);
  });
}

test("world reset requires a completed safety backup and typed confirmation", async ({ page }) => {
  const calls = await mockAPI(page);
  await openServer(page);
  await page.getByRole("link", { name: "World", exact: true }).click();
  await expect(page.getByRole("heading", { name: "Your world", exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: "Reset world", exact: true })).toBeDisabled();
  await page.getByRole("button", { name: "Create safety backup" }).click();
  await expect(page.getByRole("status").filter({ hasText: "Safety backup ready." })).toBeVisible();
  await page.getByRole("button", { name: "Reset world", exact: true }).click();
  const dialog = page.getByRole("dialog");
  await expect(dialog.getByRole("button", { name: "Reset world", exact: true })).toBeDisabled();
  await dialog.getByRole("textbox").fill("world");
  await dialog.getByRole("button", { name: "Reset world", exact: true }).click();
  await expect(dialog).not.toBeVisible();
  expect(calls.find((c) => c.path.endsWith("/world/reset")).body).toEqual({
    backup: "40000000-0000-4000-8000-000000000001",
    revision: "a".repeat(64),
    confirmation: "world",
  });
  await expect(
    page.getByText("Your world will be generated when you start the server."),
  ).toBeVisible();
});
test("world upload confirms replacement and sends archive bytes", async ({ page }) => {
  const calls = await mockAPI(page);
  await openServer(page);
  await page.getByRole("link", { name: "World", exact: true }).click();
  await page.getByRole("button", { name: "Create safety backup" }).click();
  await expect(page.getByRole("button", { name: "Upload world .zip" })).toBeEnabled();
  await page.locator('input[type="file"]').setInputFiles({
    name: "world.zip",
    mimeType: "application/zip",
    buffer: Buffer.from("zip-fixture"),
  });
  const dialog = page.getByRole("dialog");
  await expect(dialog.getByRole("button", { name: "Replace world" })).toBeDisabled();
  await dialog.getByRole("textbox").fill("world");
  await dialog.getByRole("button", { name: "Replace world" }).click();
  await expect(dialog).not.toBeVisible();
  const call = calls.find((c) => c.path.endsWith("/world/upload"));
  expect(call.bytes).toBe(11);
  expect(new URLSearchParams(call.query).get("confirmation")).toBe("world");
  expect(call.headers["idempotency-key"]).toBeTruthy();
});
test("online worlds block changes and moderators do not see world management", async ({ page }) => {
  await mockAPI(page, { initiallyRunning: true });
  await openServer(page);
  await page.getByRole("link", { name: "World", exact: true }).click();
  await expect(page.getByRole("button", { name: "Create safety backup" })).toBeDisabled();
  await expect(page.getByRole("button", { name: "Download world" })).toBeDisabled();
  await page.unrouteAll();
  await mockAPI(page, { role: "MODERATOR" });
  await openServer(page);
  await expect(page.getByRole("link", { name: "World", exact: true })).toHaveCount(0);
});

test("world management and admin navigation fit a mobile screen", async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await mockAPI(page, { isAdmin: true });
  await openServer(page);
  await page.getByRole("link", { name: "World", exact: true }).click();
  await expect(page.getByRole("heading", { name: "Your world", exact: true })).toBeVisible();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(
    true,
  );
});

test("owner adds members, updates their role, and confirms removal", async ({ page }) => {
  const calls = await mockAPI(page);
  await openServer(page);
  await page.getByRole("link", { name: "Members", exact: true }).click();
  await page.getByLabel("Member email").fill("sam@example.com");
  await page.getByLabel("Server role", { exact: true }).selectOption("MODERATOR");
  await page.getByRole("button", { name: "Add member" }).click();
  await expect(page.getByText("sam@example.com", { exact: true })).toBeVisible();
  expect(calls.find((c) => c.path.endsWith("/members") && c.method === "POST").body).toEqual({
    email: "sam@example.com",
    role: "MODERATOR",
  });
  await page.getByLabel("Role for Sam").selectOption("ADMIN");
  await expect(page.getByRole("status")).toContainText("Member role updated");
  await page.getByRole("button", { name: "Remove member", exact: true }).click();
  await expect(page.getByRole("dialog")).toContainText("sam@example.com");
  await page
    .getByRole("dialog")
    .getByRole("button", { name: "Remove member", exact: true })
    .click();
  await expect(page.getByText("sam@example.com", { exact: true })).toHaveCount(0);
  expect(
    calls.find((c) => c.method === "DELETE" && c.path.includes("/members/")).body.confirmation,
  ).toBe("60000000-0000-4000-8000-000000000001");
});
test("moderator cannot manage members", async ({ page }) => {
  await mockAPI(page, { role: "MODERATOR" });
  await openServer(page);
  await expect(page.getByRole("link", { name: "Members", exact: true })).toHaveCount(0);
});

test("guide explains software ecosystems and custom code", async ({ page }) => {
  await mockAPI(page);
  await login(page);
  await page.getByRole("link", { name: "Guide", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: "Your first server, step by step." }),
  ).toBeVisible();
  await page.getByRole("button", { name: "New blocks, machines or a modpack" }).click();
  await expect(
    page.getByRole("button", { name: "New blocks, machines or a modpack" }),
  ).toHaveAttribute("aria-pressed", "true");
  await expect(page.getByText("NeoForge", { exact: true })).toBeVisible();
  await expect(page.getByText("/premium", { exact: true }).last()).toBeVisible();
  await expect(page.getByText(/future visits skip the server password/)).toBeVisible();
  await expect(page.getByText(/New Paper and Purpur servers include AuthMe/)).toBeVisible();
  await page.getByText("Can I write my own plugin or mod?", { exact: true }).click();
  await expect(page.getByText(/build a .jar with the correct plugin descriptor/)).toBeVisible();
});
for (const type of ["VANILLA", "SPIGOT", "PURPUR", "FABRIC", "FORGE", "NEOFORGE"]) {
  test(`purchase resolves ${type} from approved software options`, async ({ page }) => {
    const calls = await mockAPI(page);
    await login(page);
    await page.getByRole("link", { name: "New server", exact: true }).click();
    await page.getByLabel("Server software").selectOption(type);
    await page.getByLabel("Server name", { exact: true }).fill("Custom world");
    await expect(page.getByLabel("Minecraft version")).toHaveValue("1.21.1");
    await page.getByRole("button", { name: "Continue to checkout" }).click();
    await expect(page).toHaveURL(/checkout/);
    expect(calls.find((c) => c.path === "/orders" && c.method === "POST").body.server_type).toBe(
      type,
    );
  });
}
test("Fabric mods have a guided upload page", async ({ page }) => {
  const calls = await mockAPI(page, { servers: [{ ...baseServer, server_type: "FABRIC" }] });
  await login(page);
  await page
    .getByRole("link")
    .filter({ has: page.getByRole("heading", { name: baseServer.name }) })
    .click();
  await page.getByRole("link", { name: "Mods", exact: true }).click();
  await expect(page.getByRole("heading", { name: "Your mods" })).toBeVisible();
  await page.locator('input[type="file"]').setInputFiles({
    name: "example.jar",
    mimeType: "application/java-archive",
    buffer: Buffer.from("browser fixture"),
  });
  await expect(page.getByRole("status")).toContainText("Mod uploaded");
  expect(
    calls.find((c) => c.path.endsWith("/mods/upload")).headers["idempotency-key"],
  ).toBeTruthy();
});

test("guide and software comparison fit a mobile viewport", async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await mockAPI(page);
  await login(page);
  await page.getByRole("button", { name: "Open navigation" }).click();
  await page.getByRole("link", { name: "Guide", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: "Your first server, step by step." }),
  ).toBeVisible();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(
    true,
  );
});

test("guide topic links reveal and focus the matching answer", async ({ page }) => {
  await mockAPI(page);
  await login(page);
  await page.getByRole("link", { name: "Guide", exact: true }).click();
  await page
    .getByRole("navigation", { name: "Guide topics" })
    .getByRole("link", { name: "Write your own code" })
    .click();
  await expect(page).toHaveURL(/\/guide#custom-code$/);
  await expect(page.locator("#custom-code")).toHaveAttribute("open", "");
  await expect(page.locator("#custom-code summary")).toBeFocused();
  await expect(page.getByText(/build a .jar with the correct plugin descriptor/)).toBeVisible();
});

test("mods contextual help links directly to installation instructions", async ({ page }) => {
  await mockAPI(page, { servers: [{ ...baseServer, server_type: "FORGE" }] });
  await openServer(page);
  await page.getByRole("link", { name: "Mods", exact: true }).click();
  const help = page.getByRole("complementary", { name: "Help with mods" });
  await help.locator("summary").click();
  await expect(help).toContainText("Minecraft version and loader");
  await help.getByRole("link", { name: "Open the step-by-step guide" }).click();
  await expect(page).toHaveURL(/\/guide#add-extras$/);
  await expect(page.getByRole("heading", { name: "3. Add plugins or mods" })).toBeInViewport();
});

test("console help deep link opens startup troubleshooting", async ({ page }) => {
  await mockAPI(page);
  await page.routeWebSocket(`ws://localhost:8080/servers/${serverID}/console`, (ws) =>
    ws.send(JSON.stringify({ event: "connected" })),
  );
  await openServer(page);
  await page.getByRole("link", { name: "Advanced", exact: true }).click();
  const help = page.getByRole("complementary", { name: "Help with console" });
  await help.locator("summary").click();
  await expect(help).toContainText("without the chat’s leading slash");
  await help.getByRole("link", { name: "Open the step-by-step guide" }).click();
  await expect(page).toHaveURL(/\/guide#server-errors$/);
  await expect(page.locator("#server-errors")).toHaveAttribute("open", "");
  await expect(page.getByText(/find the first error/)).toBeVisible();
});

test("forgot password requests mail without exposing account existence", async ({ page }) => {
  const calls = await mockAPI(page);
  await page.goto("/login");
  await page.getByRole("link", { name: "Forgot your password?" }).click();
  await expect(page).toHaveURL(/\/forgot-password$/);
  await page.getByLabel("Email address").fill("unknown@example.com");
  await expect(page.locator('input[type="password"]')).toHaveCount(0);
  await page.getByRole("button", { name: "Send reset link" }).click();
  await expect(page.getByRole("status")).toContainText("If a verified account exists");
  expect(calls.find((c) => c.path === "/auth/forgot-password").body).toEqual({
    email: "unknown@example.com",
  });
});
test("reset reads and removes the fragment token and never sends it in URLs", async ({ page }) => {
  const calls = await mockAPI(page);
  const token = "r".repeat(43);
  const urls = [];
  page.on("request", (r) => urls.push(r.url()));
  await page.goto(`/reset-password#token=${token}`);
  await expect(page.getByLabel("Reset token")).toHaveValue(token);
  await expect(page).toHaveURL(/\/reset-password$/);
  await page.getByLabel("New password", { exact: true }).fill("replacement-password");
  await page.getByLabel("Confirm new password").fill("replacement-password");
  await page.getByRole("button", { name: "Reset password", exact: true }).click();
  await expect(page.getByRole("status")).toContainText(
    "Password reset. All sessions were signed out",
  );
  expect(calls.find((c) => c.path === "/auth/reset-password").body).toEqual({
    token,
    password: "replacement-password",
  });
  expect(urls.some((url) => url.includes(token))).toBe(false);
  await expect(page.getByRole("button", { name: "Reset password", exact: true })).toHaveCount(0);
});
test("reset password mismatch prevents the API request", async ({ page }) => {
  const calls = await mockAPI(page);
  await page.goto("/reset-password");
  await page.getByLabel("Reset token").fill("r".repeat(43));
  await page.getByLabel("New password", { exact: true }).fill("replacement-password");
  await page.getByLabel("Confirm new password").fill("different-password");
  await page.getByRole("button", { name: "Reset password", exact: true }).click();
  await expect(page.getByRole("alert").filter({ hasText: "Passwords do not match" })).toContainText(
    "Passwords do not match",
  );
  expect(calls.filter((c) => c.path === "/auth/reset-password")).toHaveLength(0);
});
test("expired reset links offer a new request", async ({ page }) => {
  await mockAPI(page);
  await page.route("http://localhost:8080/auth/reset-password", (route) =>
    route.fulfill({
      status: 400,
      contentType: "application/json",
      body: JSON.stringify({ error: "reset link is invalid or expired; request a new one" }),
    }),
  );
  await page.goto("/reset-password");
  await page.getByLabel("Reset token").fill("r".repeat(43));
  await page.getByLabel("New password", { exact: true }).fill("replacement-password");
  await page.getByLabel("Confirm new password").fill("replacement-password");
  await page.getByRole("button", { name: "Reset password", exact: true }).click();
  await expect(page.getByRole("alert").filter({ hasText: "invalid or expired" })).toContainText(
    "invalid or expired",
  );
  await page.getByRole("link", { name: "Request a new reset link" }).click();
  await expect(page).toHaveURL(/\/forgot-password$/);
});
test("account password change signs out without reusing the revoked session", async ({ page }) => {
  const calls = await mockAPI(page);
  await login(page);
  await page.getByRole("link", { name: "Account", exact: true }).click();
  await page.getByLabel("Current password").fill("original-password");
  await page.getByLabel("New password", { exact: true }).fill("replacement-password");
  await page.getByLabel("Confirm new password").fill("replacement-password");
  await page.getByRole("button", { name: "Change password and sign out" }).click();
  await expect(page).toHaveURL(/\/login$/);
  await expect(page.getByRole("status")).toContainText("All sessions were signed out");
  expect(calls.find((c) => c.path === "/account/password").body).toEqual({
    current_password: "original-password",
    new_password: "replacement-password",
  });
  expect(calls.filter((c) => c.path === "/auth/logout")).toHaveLength(0);
});
test("wrong current password keeps the account form available", async ({ page }) => {
  await mockAPI(page);
  await page.route("http://localhost:8080/account/password", (route) =>
    route.fulfill({
      status: 400,
      contentType: "application/json",
      body: JSON.stringify({ error: "current password is incorrect" }),
    }),
  );
  await login(page);
  await page.getByRole("link", { name: "Account", exact: true }).click();
  await page.getByLabel("Current password").fill("wrong-password");
  await page.getByLabel("New password", { exact: true }).fill("replacement-password");
  await page.getByLabel("Confirm new password").fill("replacement-password");
  await page.getByRole("button", { name: "Change password and sign out" }).click();
  await expect(
    page.getByRole("alert").filter({ hasText: "current password is incorrect" }),
  ).toContainText("current password is incorrect");
  await expect(page).toHaveURL(/\/account$/);
  await expect(page.getByRole("button", { name: "Change password and sign out" })).toBeEnabled();
});

test("mobile navigation exposes every destination and sign-out with keyboard dismissal", async ({
  page,
}) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await mockAPI(page, { isAdmin: true });
  await login(page);
  const trigger = page.getByRole("button", { name: "Open navigation" });
  await trigger.click();
  const menu = page.getByRole("dialog", { name: "Your workspace" });
  await expect(menu).toBeVisible();
  await expect(menu.getByRole("button", { name: "Close navigation" })).toBeFocused();
  for (const label of ["Dashboard", "Servers", "Billing", "Account", "Guide", "Admin"])
    await expect(menu.getByRole("link", { name: label, exact: true })).toBeInViewport();
  await expect(menu.getByRole("button", { name: "Sign out", exact: true })).toBeInViewport();
  await page.keyboard.press("Tab");
  await expect(menu.getByRole("link", { name: "Dashboard", exact: true })).toBeFocused();
  await page.keyboard.press("Tab");
  await expect(menu.getByRole("link", { name: "Servers", exact: true })).toBeFocused();
  await page.keyboard.press("Escape");
  await expect(menu).not.toBeVisible();
  await expect(trigger).toBeFocused();
  await trigger.click();
  await menu.getByRole("link", { name: "Account", exact: true }).click();
  await expect(page).toHaveURL(/\/account$/);
  await expect(menu).not.toBeVisible();
  await page.getByRole("button", { name: "Open navigation" }).click();
  await page.getByRole("dialog").getByRole("button", { name: "Sign out", exact: true }).click();
  await expect(page).toHaveURL(/\/login$/);
});

test("current servers come first and cancelled hosting stays available in history", async ({
  page,
}) => {
  await mockAPI(page, {
    servers: [
      {
        ...baseServer,
        id: "30000000-0000-4000-8000-000000000002",
        name: "Old world",
        status: "CANCELLED",
      },
      {
        ...baseServer,
        id: "30000000-0000-4000-8000-000000000003",
        name: "New world",
        status: "PROVISIONING",
      },
      baseServer,
    ],
  });
  await login(page);
  await expect(page.locator(".server-card")).toHaveCount(2);
  await expect(page.locator(".server-card").first()).toContainText(baseServer.name);
  await expect(page.getByRole("heading", { name: "Old world", exact: true })).toHaveCount(0);
  await page.getByLabel("Server view").selectOption("all");
  await expect(page.locator(".server-card")).toHaveCount(3);
  await page.getByLabel("Search servers").fill("Old world");
  await expect(page.locator(".server-card")).toHaveCount(1);
  await expect(page.locator(".server-card")).toContainText("Old world");
});

for (const width of [320, 768, 1024, 1366]) {
  test(`workspace pages fit ${width}px with readable controls`, async ({ page }) => {
    await page.setViewportSize({ width, height: 900 });
    await mockAPI(page);
    await login(page);
    const navigate = async (label) => {
      if (width <= 900) await page.getByRole("button", { name: "Open navigation" }).click();
      await page
        .getByRole("navigation", { name: "Main navigation" })
        .getByRole("link", { name: label, exact: true })
        .click();
    };
    const fits = async () =>
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
        true,
      );
    await fits();
    await page.getByRole("link", { name: "New server", exact: true }).click();
    await expect(page.getByLabel("Server name", { exact: true })).toBeVisible();
    await fits();
    await navigate("Billing");
    await expect(page.getByRole("heading", { name: "Keep your adventures going." })).toBeVisible();
    await fits();
    await navigate("Account");
    await expect(page.getByLabel("Current password")).toBeVisible();
    await fits();
    if (width <= 600) {
      expect(
        await page
          .getByLabel("Current password")
          .evaluate((el) => parseFloat(getComputedStyle(el).fontSize)),
      ).toBeGreaterThanOrEqual(16);
      expect(
        (await page.getByRole("button", { name: "Change password and sign out" }).boundingBox())
          .height,
      ).toBeGreaterThanOrEqual(44);
    }
    await navigate("Servers");
    await page.locator(`a[href="/servers/${serverID}"]`).first().click();
    await expect(page.getByRole("button", { name: "Start", exact: true })).toBeVisible();
    await fits();
    await page.getByRole("link", { name: "Settings", exact: true }).click();
    await expect(page.getByRole("combobox", { name: "Difficulty" })).toBeVisible();
    await fits();
  });
}

test("verification email links prepare a confirmation without leaking tokens in URLs", async ({
  page,
}) => {
  const calls = await mockAPI(page);
  const token = "v".repeat(43);
  const urls = [];
  page.on("request", (request) => urls.push(request.url()));
  await page.goto(`/verify#token=${token}`);
  await expect(page).toHaveURL(/\/verify$/);
  await expect(page.getByText("Your verification link is ready.", { exact: false })).toBeVisible();
  await expect(page.getByLabel("Verification token", { exact: true })).toHaveCount(0);
  expect(calls.filter((c) => c.path === "/auth/verify-email")).toHaveLength(0);
  await page.getByRole("button", { name: "Verify email", exact: true }).click();
  await expect(page.getByRole("status")).toContainText("Email verified");
  await expect(page.getByRole("button", { name: "Verify email", exact: true })).toHaveCount(0);
  await expect(page.getByRole("button", { name: "Resend verification" })).toHaveCount(0);
  expect(calls.find((c) => c.path === "/auth/verify-email").body).toEqual({ token });
  expect(urls.some((url) => url.includes(token))).toBe(false);
  await page.getByRole("link", { name: "Continue to sign in" }).click();
  await expect(page).toHaveURL(/\/login$/);
});

test("expired verification links preserve a way to request another email", async ({ page }) => {
  await mockAPI(page);
  await page.route("http://localhost:8080/auth/verify-email", (route) =>
    route.fulfill({
      status: 400,
      contentType: "application/json",
      body: JSON.stringify({ error: "verification token is invalid or expired" }),
    }),
  );
  await page.goto(`/verify#token=${"v".repeat(43)}`);
  await expect(page).toHaveURL(/\/verify$/);
  await page.getByRole("button", { name: "Verify email", exact: true }).click();
  await expect(page.getByRole("alert").filter({ hasText: "invalid or expired" })).toBeVisible();
  await page.getByLabel("Need another verification email?").fill("alex@example.com");
  await page.getByRole("button", { name: "Resend verification" }).click();
  await expect(page.getByRole("status")).toContainText(/verification/i);
});

test("private beta signup reads an email-bound invitation without sending secrets in URLs", async ({
  page,
}) => {
  const calls = await mockAPI(page, { inviteRequired: true });
  const token = "i".repeat(43);
  const urls = [];
  page.on("request", (r) => urls.push(r.url()));
  await page.goto(`/register#invite=${token}&email=guest%40example.com`);
  await expect(page.getByLabel("Invitation code")).toHaveValue(token);
  await expect(page.getByLabel("Email address")).toHaveValue("guest@example.com");
  await expect(page).toHaveURL(/\/register$/);
  await page.getByLabel("Your name").fill("Guest");
  await page.getByLabel("Password", { exact: true }).fill("guest-long-password");
  await page.getByRole("button", { name: "Create account" }).click();
  await expect(page).toHaveURL(/\/verify$/);
  expect(calls.find((c) => c.path === "/auth/register").body.invitation_token).toBe(token);
  expect(urls.some((url) => url.includes(token))).toBe(false);
});

test("registration fails closed when registration settings are unavailable", async ({ page }) => {
  const calls = await mockAPI(page);
  await page.route("http://localhost:8080/auth/config", (r) =>
    r.fulfill({
      status: 503,
      contentType: "application/json",
      body: JSON.stringify({ error: "Registration settings unavailable" }),
    }),
  );
  await page.goto("/register");
  await expect(
    page.getByRole("alert").filter({ hasText: "Registration settings unavailable" }),
  ).toBeVisible();
  await expect(page.getByRole("button", { name: "Create account" })).toBeDisabled();
  expect(calls.filter((c) => c.path === "/auth/register")).toHaveLength(0);
});

test("admins create one-time private invitation links and revoke them", async ({ page }) => {
  await mockAPI(page, { isAdmin: true });
  let issued = false;
  let revoked = false;
  const id = "90000000-0000-4000-8000-000000000001";
  await page.route("http://localhost:8080/admin/invitations", (r) => {
    if (r.request().method() === "POST") {
      issued = true;
      return r.fulfill({
        status: 201,
        contentType: "application/json",
        body: JSON.stringify({
          id,
          email: "guest@example.com",
          link: "http://localhost:3000/register#invite=" + "i".repeat(43),
          expires_at: "2026-10-16T00:00:00Z",
        }),
      });
    }
    return r.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify(
        issued
          ? [
              {
                id,
                email: "guest@example.com",
                status: revoked ? "REVOKED" : "ACTIVE",
                expires_at: "2026-10-16T00:00:00Z",
              },
            ]
          : [],
      ),
    });
  });
  await page.route(`http://localhost:8080/admin/invitations/${id}/revoke`, (r) => {
    revoked = true;
    return r.fulfill({ status: 204 });
  });
  await login(page);
  await page.getByRole("link", { name: "Admin", exact: true }).click();
  await page.getByRole("button", { name: "Invitations", exact: true }).click();
  await page.getByLabel("Invite email").fill("guest@example.com");
  await page.getByRole("button", { name: "Create invitation" }).click();
  await expect(page.getByLabel("Invitation link")).toHaveValue(/register#invite=/);
  await page.getByRole("button", { name: "Revoke invitation" }).click();
  await expect(page.getByLabel("Invitation link")).toHaveCount(0);
  await expect(page.getByRole("cell", { name: "REVOKED", exact: true })).toBeVisible();
});

test("beta onboarding explains manual access instead of checkout", async ({ page }) => {
  await page.setViewportSize({ width: 320, height: 900 });
  await mockAPI(page, { inviteRequired: true, paymentDisabled: true, servers: [] });
  await login(page);
  await expect(page.getByRole("heading", { name: "Welcome to the private beta" })).toBeVisible();
  await expect(
    page.getByText("payments are disabled for this beta", { exact: false }),
  ).toBeVisible();
  await expect(page.getByRole("link", { name: "New server", exact: true })).toHaveCount(0);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
});

test("first-session checklist reflects the EULA and real startup state", async ({ page }) => {
  await mockAPI(page, { eulaAccepted: false });
  await openServer(page);
  const checklist = page.locator(".first-server-checklist");
  await expect(checklist).toHaveAttribute("open", "");
  await expect(checklist.locator("summary")).toContainText("1 of 4 steps complete");
  await expect(checklist).toContainText("Minecraft Java 1.21.1");
  await page.getByRole("checkbox", { name: /I have read and accept/ }).check();
  await page.getByRole("button", { name: "Accept agreement", exact: true }).click();
  await expect(checklist.locator("summary")).toContainText("2 of 4 steps complete");
  await page.getByRole("button", { name: "Start", exact: true }).click();
  await expect(checklist.locator("summary")).toContainText("3 of 4 steps complete");
});

const feedbackFixture = {
  id: "90000000-0000-4000-8000-000000000011",
  author_id: user.id,
  author_name: user.name,
  author_email: user.email,
  server_id: serverID,
  category: "BUG",
  title: "Cannot join the world",
  message: "I started Minecraft but cannot connect to the world.",
  status: "NEW",
  response: "",
  revision: 1,
  created_at: "2026-10-11T10:00:00Z",
  updated_at: "2026-10-11T10:00:00Z",
};
test("beta feedback retries safely and displays host replies on mobile", async ({ page }) => {
  await mockAPI(page, { inviteRequired: true, paymentDisabled: true });
  let report = null;
  const submissions = [];
  await page.route("http://localhost:8080/feedback*", (route) => {
    if (route.request().method() === "POST") {
      submissions.push({
        key: route.request().headers()["idempotency-key"],
        body: route.request().postDataJSON(),
      });
      if (submissions.length === 1)
        return route.fulfill({
          status: 503,
          contentType: "application/json",
          body: JSON.stringify({ error: "Please try again" }),
        });
      report = {
        ...feedbackFixture,
        ...submissions.at(-1).body,
        status: "REVIEWED",
        response: "Use the server address shown on Overview.",
        revision: 2,
      };
      return route.fulfill({
        status: 201,
        contentType: "application/json",
        body: JSON.stringify(report),
      });
    }
    return route.fulfill({
      contentType: "application/json",
      body: JSON.stringify(report ? [report] : []),
    });
  });
  await login(page);
  await page.getByRole("link", { name: "Feedback", exact: true }).click();
  await expect(page.getByRole("heading", { name: "Help us improve." })).toBeVisible();
  await page.evaluate(
    (id) => window.history.replaceState(null, "", "/feedback?server=" + id),
    serverID,
  );
  await expect(page.getByLabel("Related server (optional)")).toHaveValue(serverID);
  await page.getByLabel("Short summary").fill("Cannot join the world");
  await page
    .getByLabel("What happened?")
    .fill("I started Minecraft but cannot connect to the world.");
  await page.getByRole("button", { name: "Send report", exact: true }).click();
  await expect(page.getByRole("alert").filter({ hasText: "Please try again" })).toBeVisible();
  await page.getByRole("button", { name: "Send report", exact: true }).click();
  await expect(page.getByRole("status")).toContainText("received");
  expect(submissions[0].key).toBe(submissions[1].key);
  expect(submissions[1].body.server_id).toBe(serverID);
  await expect(page.getByRole("article")).toContainText(
    "Use the server address shown on Overview.",
  );
  await page.setViewportSize({ width: 320, height: 900 });
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
});
test("admin reviews reports without rendering submitted markup", async ({ page }) => {
  await mockAPI(page, { isAdmin: true });
  let report = {
    ...feedbackFixture,
    message: '<img src=x onerror="window.reportExecuted=true"> is the text I saw.',
  };
  await page.route("http://localhost:8080/admin/feedback*", (route) => {
    if (route.request().method() === "PUT") {
      const update = route.request().postDataJSON();
      expect(update.revision).toBe(1);
      report = { ...report, ...update, revision: 2 };
      return route.fulfill({ contentType: "application/json", body: JSON.stringify(report) });
    }
    return route.fulfill({ contentType: "application/json", body: JSON.stringify([report]) });
  });
  await login(page);
  await page.getByRole("link", { name: "Admin", exact: true }).click();
  await page.getByRole("button", { name: "Feedback", exact: true }).click();
  await expect(page.getByRole("article")).toContainText("<img src=x");
  expect(await page.evaluate(() => window.reportExecuted)).toBeUndefined();
  await page.getByRole("button", { name: "Review report", exact: true }).click();
  await page.getByLabel("Report status").selectOption("RESOLVED");
  await page.getByLabel("Reply to tester").fill("This is fixed. Try joining again.");
  await page.getByRole("button", { name: "Save response", exact: true }).click();
  await expect(page.getByRole("article")).toContainText("Resolved");
  await expect(page.getByRole("article")).toContainText("This is fixed. Try joining again.");
});
test("feedback only offers accessible servers and paginates personal reports", async ({ page }) => {
  await mockAPI(page, { servers: [] });
  const offsets = [];
  await page.route("http://localhost:8080/feedback*", (route) => {
    const offset = new URL(route.request().url()).searchParams.get("offset");
    offsets.push(offset);
    const reports =
      offset === "0"
        ? Array.from({ length: 50 }, (_, i) => ({
            ...feedbackFixture,
            id: `report-${i}`,
            title: `Report number ${i}`,
            server_id: null,
          }))
        : [];
    return route.fulfill({ contentType: "application/json", body: JSON.stringify(reports) });
  });
  await login(page);
  await page.getByRole("link", { name: "Feedback", exact: true }).click();
  await expect(page.getByRole("heading", { name: "Help us improve." })).toBeVisible();
  await page.evaluate(
    (id) => window.history.replaceState(null, "", "/feedback?server=" + id),
    serverID,
  );
  await expect(page.getByLabel("Related server (optional)")).toHaveValue("");
  await expect(page.getByLabel("Related server (optional)").locator("option")).toHaveCount(1);
  await page.getByRole("button", { name: "Next", exact: true }).click();
  await expect(page.getByText("Page 2", { exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: "Next", exact: true })).toBeDisabled();
  expect(offsets).toContain("50");
});

test("refresh restores the session on a server page without browser token storage", async ({
  page,
}) => {
  const calls = await mockAPI(page);
  await login(page);
  await page.locator(`a[href="/servers/${serverID}"]`).first().click();
  await expect(page.getByRole("heading", { name: baseServer.name, exact: true })).toBeVisible();
  await expect(page).toHaveURL(new RegExp(`/servers/${serverID}$`));
  await page.reload();
  await expect(page.getByRole("heading", { name: baseServer.name, exact: true })).toBeVisible();
  await expect(page).toHaveURL(new RegExp(`/servers/${serverID}$`));
  await page.getByRole("button", { name: "Start", exact: true }).click();
  await expect(page.getByText("Online", { exact: true }).first()).toBeVisible();
  const power = calls.find((c) => c.path === `/servers/${serverID}/start`);
  expect(power.headers["x-csrf-token"]).toBe("c".repeat(43));
  expect(power.headers.authorization).toBeUndefined();
  expect(await page.evaluate(() => localStorage.length + sessionStorage.length)).toBe(0);
});
test("logout stays signed out after refresh", async ({ page }) => {
  await mockAPI(page);
  await login(page);
  await page.getByRole("link", { name: "Account", exact: true }).click();
  await page.getByRole("button", { name: "Sign out of this session" }).click();
  await expect(page).toHaveURL(/\/login$/);
  await page.reload();
  await expect(page.getByRole("heading", { name: "Good to see you again." })).toBeVisible();
  await expect(page).toHaveURL(/\/login$/);
});
test("session restoration outage offers retry without discarding the login", async ({ page }) => {
  await mockAPI(page);
  await login(page);
  let unavailable = true;
  await page.route("http://localhost:8080/auth/session", (route) =>
    route.fulfill({
      status: unavailable ? 503 : 200,
      contentType: "application/json",
      body: JSON.stringify(
        unavailable
          ? { error: "Hosting service temporarily unavailable" }
          : {
              user,
              csrf_token: "c".repeat(43),
              expires_at: new Date(Date.now() + 86400000).toISOString(),
            },
      ),
    }),
  );
  await page.reload();
  await expect(
    page.getByRole("alert").filter({ hasText: "Hosting service temporarily unavailable" }),
  ).toBeVisible();
  await expect(page).toHaveURL(/\/dashboard$/);
  unavailable = false;
  await page.getByRole("button", { name: "Retry connection" }).click();
  await expect(page.getByRole("heading", { name: "Welcome back, Alex." })).toBeVisible();
});
test("failed logout keeps the session and explains how to retry", async ({ page }) => {
  await mockAPI(page);
  await login(page);
  await page.route("http://localhost:8080/auth/logout", (route) =>
    route.fulfill({
      status: 503,
      contentType: "application/json",
      body: JSON.stringify({ error: "Service unavailable" }),
    }),
  );
  await page.getByRole("button", { name: "A Alex Sign out" }).click();
  await expect(page.getByRole("alert").filter({ hasText: "Could not sign out" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "Welcome back, Alex." })).toBeVisible();
});

test("plugin catalog reviews compatible files before installing", async ({ page }) => {
  const calls = await mockAPI(page);
  await openServer(page);
  await page.getByRole("link", { name: "Plugins", exact: true }).click();
  await page.getByLabel("Search plugins", { exact: true }).fill("ViaVersion");
  await page.getByRole("button", { name: "Search Modrinth" }).click();
  await page.getByRole("button", { name: "Review install" }).click();
  await expect(page.getByRole("dialog")).toContainText("PAPER 1.21.1");
  await expect(page.getByRole("dialog")).toContainText("ViaVersion.jar");
  expect(
    calls.filter((c) => c.path.endsWith("/catalog/viaversion") && c.method === "POST"),
  ).toHaveLength(0);
  await page.getByRole("button", { name: "Install now", exact: true }).click();
  await expect(page.getByRole("dialog")).not.toBeVisible();
  const install = calls.find((c) => c.path.endsWith("/catalog/viaversion") && c.method === "POST");
  expect(install.body).toEqual({ revision: "c".repeat(64) });
  expect(install.headers["idempotency-key"]).toBeTruthy();
});

test("catalog search and review fit a narrow mobile screen", async ({ page }) => {
  await page.setViewportSize({ width: 320, height: 740 });
  await mockAPI(page);
  await openServer(page);
  await page.getByRole("link", { name: "Plugins", exact: true }).click();
  await page.getByRole("button", { name: "Search Modrinth" }).click();
  await expect(page.getByRole("button", { name: "Review install" })).toBeVisible();
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(320);
  await page.getByRole("button", { name: "Review install" }).click();
  await expect(page.getByRole("button", { name: "Cancel", exact: true })).toBeVisible();
  await page.getByRole("button", { name: "Cancel", exact: true }).click();
  await expect(page.getByRole("dialog")).not.toBeVisible();
});

for (const width of [320, 1366]) {
  test(`players whitelist controls fit ${width}px and confirm access changes`, async ({
    page,
  }, testInfo) => {
    await page.setViewportSize({ width, height: 900 });
    const calls = await mockAPI(page, { initiallyRunning: true });
    await openServer(page);
    await page.getByRole("link", { name: "Players", exact: true }).click();
    await expect(page.getByText(/Stop your server before changing players/)).toHaveCount(0);
    await expect(page.getByRole("link", { name: "Players", exact: true })).toHaveAttribute(
      "aria-current",
      "page",
    );
    await expect(page.getByRole("link", { name: "Settings", exact: true })).not.toHaveAttribute(
      "aria-current",
      "page",
    );
    await expect(page.getByText("milkaxq", { exact: true })).toBeVisible();
    await page.getByLabel("Minecraft username", { exact: true }).fill("@a");
    await expect(page.getByRole("button", { name: "Add player", exact: true })).toBeDisabled();
    await page.getByLabel("Minecraft username", { exact: true }).fill("Friend_123");
    await page.getByRole("button", { name: "Add player", exact: true }).click();
    await expect(page.getByText("Friend_123", { exact: true })).toBeVisible();
    await page.screenshot({ path: testInfo.outputPath(`players-${width}.png`), fullPage: true });
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
      true,
    );
    await page.getByLabel("Find a player", { exact: true }).fill("Friend");
    await expect(page.getByText("milkaxq", { exact: true })).not.toBeVisible();
    await page.getByRole("button", { name: "Remove Friend_123", exact: true }).click();
    await expect(page.getByRole("dialog", { name: "Remove Friend_123?" })).toBeVisible();
    expect(calls.filter((c) => c.body?.action === "remove")).toHaveLength(0);
    await page.getByRole("button", { name: "Remove player", exact: true }).click();
    await expect(page.getByText("Friend_123", { exact: true })).not.toBeVisible();
    await page.getByRole("button", { name: "Turn whitelist off", exact: true }).click();
    expect(calls.filter((c) => c.body?.action === "off")).toHaveLength(0);
    await page.getByRole("dialog").getByRole("button", { name: "Turn whitelist off" }).click();
    await expect(page.getByRole("button", { name: "Turn whitelist on" })).toBeVisible();
    await page.getByRole("button", { name: "Turn whitelist on" }).click();
    await expect(page.getByRole("button", { name: "Turn whitelist off" })).toBeVisible();
    for (const call of calls.filter((c) => c.path.endsWith("/whitelist") && c.method === "POST")) {
      expect(call.headers["idempotency-key"]).toMatch(/^[a-f0-9-]{36}$/);
      expect(call.body.revision).toHaveLength(64);
      expect(call.body.uuid).toBeUndefined();
      if (["remove", "off"].includes(call.body.action)) expect(call.body.confirmed).toBe(true);
    }
  });
}
test("players page is read-only while stopped and hidden from moderators", async ({ page }) => {
  await mockAPI(page);
  await openServer(page);
  await page.getByRole("link", { name: "Players", exact: true }).click();
  await expect(page.getByText("milkaxq", { exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: "Remove milkaxq" })).toBeDisabled();
  await expect(page.getByRole("button", { name: "Turn whitelist off" })).toBeDisabled();
  await mockAPI(page, { role: "MODERATOR" });
  await login(page);
  await page.goto(`/servers/${serverID}`);
  await expect(page.getByRole("link", { name: "Players", exact: true })).toHaveCount(0);
  await page.goto(`/servers/${serverID}/players`);
  await expect(page.getByText("Your server role does not allow this page.")).toBeVisible();
});
test("players page does not claim success when Minecraft has not confirmed a command", async ({
  page,
}) => {
  await mockAPI(page, { initiallyRunning: true, ignoreWhitelist: true });
  await openServer(page);
  await page.getByRole("link", { name: "Players", exact: true }).click();
  await page.getByLabel("Minecraft username", { exact: true }).fill("NotResolved");
  await page.getByRole("button", { name: "Add player" }).click();
  await expect(page.getByRole("status")).toContainText("Minecraft has not confirmed");
  await expect(page.getByText("NotResolved", { exact: true })).not.toBeVisible();
});
