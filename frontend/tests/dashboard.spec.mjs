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
  } = {},
) {
  let running = initiallyRunning;
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
    ...baseServer,
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
    if (path === "/auth/login")
      return reply({
        token: "a".repeat(43),
        expires_at: new Date(Date.now() + 86400000).toISOString(),
        user: authUser,
      });
    if (path === "/auth/register") return reply(user, 201);
    if (path === "/auth/verify-email" || path === "/auth/logout")
      return route.fulfill({ status: 204 });
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
    if (path === "/minecraft/options")
      return reply([
        { server_type: "PAPER", minecraft_version: "1.21.1" },
        { server_type: "PAPER", minecraft_version: "1.21.10" },
      ]);
    if (path === "/servers" && request.method() === "GET")
      return errorList ? reply({ error: "Service temporarily unavailable" }, 503) : reply(servers);
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
    if (path === `/servers/${serverID}/plugins` && request.method() === "GET")
      return reply([{ name: "Essentials.jar", is_file: true, size: 1048576 }]);
    if (path === `/servers/${serverID}/plugins` || path.endsWith("/plugins/upload"))
      return reply({ status: "saved", restart_required: true });
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
      running = ["start", "restart"].includes(path.split("/").at(-1));
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
test("authentication, database-backed plans and in-memory session", async ({ page }, testInfo) => {
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
    "1.21.10",
  );
  await page.getByRole("button", { name: "Continue to checkout" }).click();
  await expect(page).toHaveURL(new RegExp(`/checkout/${orderID}$`));
  const call = calls.find((c) => c.path === "/orders" && c.method === "POST");
  expect(call.body).toEqual({
    plan_id: planID,
    name: "A new adventure",
    minecraft_version: "1.21.10",
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
