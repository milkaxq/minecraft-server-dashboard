import { test, expect } from "@playwright/test";
import { parseChunkyProgress, parseChunkyEvent } from "../lib/chunky-progress.js";
import {
  installedAddonInfo,
  hasGuidedSetup,
  filterInstalledAddons,
} from "../lib/installed-addons.js";
import { diagnosticReport } from "../lib/lag-diagnostics.js";
import { findInFile } from "../lib/find-in-file.js";
import { formatBytes, request as apiRequest } from "../lib/api.js";
import { PLAYER_GROUPS, recommendGroupPlan } from "../lib/group-plans.js";
import { availableSlots, monthlySale, planMargin, mixForecast } from "../lib/margins.js";
import { nextServerAction } from "../lib/next-action.js";
async function selectValue(page, control, value) {
  await control.waitFor({ state: "attached" });
  const ancestors = control.locator("xpath=ancestor::details");
  for (let index = (await ancestors.count()) - 1; index >= 0; index--) {
    const details = ancestors.nth(index);
    if (!(await details.evaluate((element) => element.open)))
      await details.locator("summary").first().click();
  }
  await control.click();
  await page.locator(`[role="option"][data-value="${value}"]`).click();
}

async function reviewCreation(page) {
  const review = page.getByRole("button", { name: /Name & create/ });
  if ((await review.getAttribute("aria-current")) !== "step") await review.click();
}

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
const voiceDefaults = {
  max_voice_distance: 48,
  whisper_distance: 24,
  enable_groups: true,
  force_voice_chat: false,
  allow_recording: true,
  spectator_interaction: false,
};
const voiceFixture = {
  id: "simple-voice-chat",
  name: "Simple Voice Chat",
  installed: true,
  config_exists: true,
  configured: true,
  revision: "v".repeat(64),
  network: { status: "READY", hostname: "play.example.com", port: 24454 },
  defaults: voiceDefaults,
  values: { ...voiceDefaults, max_voice_distance: 72 },
  fields: [
    {
      key: "max_voice_distance",
      label: "Voice distance",
      help: "Nearby voice range in blocks.",
      kind: "number",
      min: 1,
      max: 256,
    },
    {
      key: "whisper_distance",
      label: "Whisper distance",
      help: "No greater than the voice distance.",
      kind: "number",
      min: 1,
      max: 256,
    },
    {
      key: "enable_groups",
      label: "Group chats",
      help: "Let players create groups.",
      kind: "boolean",
    },
    {
      key: "force_voice_chat",
      label: "Require the client mod",
      help: "Players without the mod will be disconnected.",
      kind: "boolean",
    },
    {
      key: "allow_recording",
      label: "Allow recording",
      help: "Allow built-in recording.",
      kind: "boolean",
    },
    {
      key: "spectator_interaction",
      label: "Spectators can talk",
      help: "Spectators can talk to players.",
      kind: "boolean",
    },
  ],
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
    backupSchedule = {},
    scheduleConflict = false,
    scheduleChangesAfterRead = false,
  } = {},
) {
  let scheduleReads = 0;
  let schedule = {
    enabled: false,
    hour: 3,
    minute: 0,
    timezone: "UTC",
    revision: 0,
    available: true,
    can_manage: role === "OWNER",
    next_run: null,
    last_success: null,
    runs: [],
    warning: "",
    ...backupSchedule,
  };
  let loggedIn = false;
  let running = initiallyRunning;
  let playerBans = [];
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
    if (path.endsWith("/addons/chunky/worlds"))
      return reply([
        { name: "world", label: "Main world" },
        { name: "world_nether", label: "Nether" },
      ]);
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
    if (path === "/admin/finances")
      return reply({ month: url.searchParams.get("month"), costs: [], sales: [] });
    if (path === "/admin/invitations") return reply([]);
    if (["/feedback", "/admin/feedback"].includes(path) && request.method() === "GET")
      return reply([]);
    if (path === "/auth/register") return reply(user, 201);
    if (path === "/auth/logout") {
      loggedIn = false;
      return route.fulfill({ status: 204 });
    }
    if (path === "/auth/verify-email") return route.fulfill({ status: 204 });
    if (path === "/auth/resend-verification")
      return reply({ message: "Verification email sent." }, 202);
    if (path === `/servers/${serverID}/startup-check`)
      return reply({
        checked: true,
        checked_at: "2026-10-13T11:00:00Z",
        issues: [],
        power_state: running ? "running" : "offline",
      });
    if (path === `/servers/${serverID}/client-requirements`)
      return reply({
        software: server.server_type,
        minecraft_version: server.minecraft_version,
        addons: [],
      });
    if (path === `/servers/${serverID}/player-invite`) return reply(null);
    if (path === "/notifications") return reply([]);
    if (path === `/servers/${serverID}/activity`) return reply([]);
    if (path === `/servers/${serverID}/storage`)
      return reply({
        used_bytes: 0,
        limit_bytes: 10737418240,
        backup_bytes: 0,
        backup_count: 0,
        backup_limit: 2,
        categories: [],
        partial: false,
      });
    if (
      path === `/servers/${serverID}/plugins/catalog/updates` ||
      path === `/servers/${serverID}/mods/catalog/updates`
    )
      return reply([]);
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
    if (path === `/servers/${serverID}/players/bans`)
      return reply({ players: playerBans, revision: "e".repeat(64) });
    if (path === `/servers/${serverID}/players/actions`) {
      if (body.action === "ban")
        playerBans.push({
          uuid: "80000000-0000-4000-8000-000000000001",
          name: body.name,
          reason: body.reason,
        });
      if (body.action === "unban")
        playerBans = playerBans.filter((player) => player.name !== body.name);
      return reply({ confirmed: body.action !== "kick", sent: true });
    }
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
    if (path === `/servers/${serverID}/addons/simple-voice-chat` && request.method() === "GET")
      return reply(voiceFixture);
    if (path === `/servers/${serverID}/addons/simple-voice-chat`)
      return reply({ status: "saved", restart_required: true });
    if (
      path === `/servers/${serverID}/addons/simple-voice-chat/network` &&
      request.method() === "GET"
    )
      return reply(voiceFixture.network);
    if (path === `/servers/${serverID}/addons/simple-voice-chat/network`)
      return reply({ id: orderID, status: "PENDING" }, 202);
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
      return reply({
        projects: [
          {
            project_id: "viaversion",
            title: "ViaVersion",
            description: "Allow newer clients to join.",
            downloads: 10000,
            release_version: "5.12.1",
          },
        ],
        has_more: false,
        next_offset: 1,
        software: "PAPER",
        minecraft_version: "1.21.1",
      });
    if (path.endsWith("/catalog/installed")) return reply({ projects: [] });
    if (path.includes("/catalog/operations/"))
      return reply({
        status: "PENDING",
        phase: "Downloading and checking files",
        completed: 1,
        total: 2,
      });
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
    if (path === `/servers/${serverID}/backup-schedule`) {
      if (role === "MODERATOR") return reply({ error: "server not found" }, 404);
      if (request.method() === "PUT") {
        if (role !== "OWNER") return reply({ error: "server not found" }, 404);
        if (scheduleConflict) {
          schedule = { ...schedule, revision: 5 };
          return reply({ error: "Backup schedule changed. Refresh before saving again." }, 409);
        }
        schedule = {
          ...schedule,
          ...request.postDataJSON(),
          revision: schedule.revision + 1,
          next_run: request.postDataJSON().enabled ? "2026-10-14T03:00:00Z" : null,
        };
      }
      if (request.method() === "GET" && ++scheduleReads > 1 && scheduleChangesAfterRead)
        schedule = { ...schedule, revision: 5 };
      return reply(schedule);
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
test("authentication and cookie session keep the dashboard focused", async ({ page }, testInfo) => {
  await mockAPI(page);
  await login(page);
  await expect(page.getByRole("heading", { name: baseServer.name })).toBeVisible();
  await expect(page.locator(".plans-section")).toHaveCount(0);
  await expect(page.getByRole("heading", { name: "Continue playing", exact: true })).toBeVisible();
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
  await reviewCreation(page);
  await page.getByLabel("Server name", { exact: true }).fill("A new adventure");
  await expect(page.getByRole("heading", { name: "Name your world", exact: true })).toBeVisible();
  await expect(
    page.getByRole("combobox", { name: "Minecraft version", exact: true, includeHidden: true }),
  ).toHaveAttribute("data-value", "26.3");
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
  await page.getByRole("button", { name: "Health", exact: true }).click();
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
  await selectValue(page, page.getByRole("combobox", { name: "Difficulty" }), "hard");
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
  await page
    .getByRole("dialog")
    .getByRole("button", { name: "Upload plugin", exact: true })
    .click();
  await expect(page.getByRole("status").filter({ hasText: "Plugin uploaded" })).toContainText(
    "Plugin uploaded",
  );
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
  await expect(
    page.getByRole("dialog").getByRole("button", { name: "Restore backup", exact: true }),
  ).toBeDisabled();
  await page.getByLabel("I understand that the current server files will be replaced.").check();
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
  await reviewCreation(page);
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
  await reviewCreation(page);
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
  await reviewCreation(page);
  await page.getByLabel("Server name", { exact: true }).fill("Latest world");
  await selectValue(page, page.getByLabel("Minecraft version"), "26.3");
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
  await page.getByRole("button", { name: "Manage", exact: true }).click();
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
  await selectValue(page, page.getByLabel("Server view"), "all");
  await expect(page.getByText("Deleting", { exact: true })).toBeVisible();
  await page.reload();
  await selectValue(page, page.getByLabel("Server view"), "all");
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
  await selectValue(page, page.getByLabel("Server role", { exact: true }), "MODERATOR");
  await page.getByRole("button", { name: "Add member" }).click();
  await expect(page.getByText("sam@example.com", { exact: true })).toBeVisible();
  expect(calls.find((c) => c.path.endsWith("/members") && c.method === "POST").body).toEqual({
    email: "sam@example.com",
    role: "MODERATOR",
  });
  await selectValue(page, page.getByLabel("Role for Sam"), "ADMIN");
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
  await page.getByText("Detailed help, troubleshooting & custom code", { exact: true }).click();
  await expect(
    page.getByRole("heading", { name: "Your first server, step by step." }),
  ).toBeVisible();
  await page.getByRole("button", { name: "New blocks, machines or a modpack" }).click();
  await expect(
    page.getByRole("button", { name: "New blocks, machines or a modpack" }),
  ).toHaveAttribute("aria-pressed", "true");
  await expect(page.getByText("Fabric", { exact: true })).toBeVisible();
  await expect(page.getByText("/premium", { exact: true })).toBeVisible();
  await expect(page.getByText(/future visits skip the server password/)).toBeVisible();
  await page.getByText("Can I write my own plugin or mod?", { exact: true }).click();
  await expect(page.getByText(/build a .jar with the correct plugin descriptor/)).toBeVisible();
});
for (const type of ["FORGE", "PAPER", "FABRIC"]) {
  test(`purchase resolves ${type} from approved software options`, async ({ page }) => {
    const calls = await mockAPI(page);
    await login(page);
    await page.getByRole("link", { name: "New server", exact: true }).click();
    await reviewCreation(page);
    await selectValue(page, page.getByLabel("Server software"), type);
    await reviewCreation(page);
    await page.getByLabel("Server name", { exact: true }).fill("Custom world");
    await expect(page.getByLabel("Minecraft version")).toHaveAttribute(
      "data-value",
      type === "PAPER" ? "26.3" : "1.21.1",
    );
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
  await page.getByRole("dialog").getByRole("button", { name: "Upload mod", exact: true }).click();
  await expect(page.getByRole("status").filter({ hasText: "Mod uploaded" })).toContainText(
    "Mod uploaded",
  );
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
  await page.getByText("Detailed help, troubleshooting & custom code", { exact: true }).click();
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
  await page.getByText("Detailed help, troubleshooting & custom code", { exact: true }).click();
  await page
    .getByRole("navigation", { name: "Guide topics" })
    .getByRole("link", { name: "Write your own code" })
    .click();
  await expect(page).toHaveURL(/\/guide#custom-code$/);
  await expect(page.locator("#custom-code")).toHaveAttribute("open", "");
  await expect(page.locator("#custom-code summary")).toBeFocused();
  await expect(page.getByText(/build a .jar with the correct plugin descriptor/)).toBeVisible();
});

test("management pages omit generic help boxes", async ({ page }) => {
  await mockAPI(page, { servers: [{ ...baseServer, server_type: "FORGE" }] });
  await openServer(page);
  await page.getByRole("link", { name: "Mods", exact: true }).click();
  await expect(page.getByRole("complementary", { name: "Help with mods" })).toHaveCount(0);
  await expect(page.getByRole("heading", { name: "Find mods", exact: true })).toBeVisible();
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
  const menu = page.getByRole("dialog", { name: "Your worlds" });
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
  await selectValue(page, page.getByLabel("Server view"), "all");
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
    await expect(page.getByRole("heading", { name: "How do you want to play?" })).toBeVisible();
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

test("next action follows agreement acceptance and real startup without a checklist", async ({
  page,
}) => {
  await mockAPI(page, { eulaAccepted: false });
  await openServer(page);
  const play = page.getByRole("region", { name: "Play on your server" });
  await expect(play).toContainText("One agreement before your first adventure.");
  await expect(page.locator(".first-server-checklist")).toHaveCount(0);
  await page.getByRole("checkbox", { name: /I have read and accept/ }).check();
  await page.getByRole("button", { name: "Accept agreement", exact: true }).click();
  await expect(play).toContainText("Start your next adventure.");
  await page.getByRole("button", { name: "Start", exact: true }).click();
  await expect(play).toContainText("Your world is ready. Bring your friends.");
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
  await expect(page.getByLabel("Related server (optional)")).toHaveAttribute(
    "data-value",
    serverID,
  );
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
  await selectValue(page, page.getByLabel("Report status"), "RESOLVED");
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
  await expect(page.getByLabel("Related server (optional)")).toHaveAttribute("data-value", "");
  await page.getByLabel("Related server (optional)").click();
  await expect(page.getByRole("option")).toHaveCount(1);
  await page.getByRole("combobox", { name: "Related server (optional)" }).press("Escape");
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
  expect(install.body).toEqual({
    revision: "c".repeat(64),
    backup: "40000000-0000-4000-8000-000000000001",
    acknowledge_no_backup: false,
  });
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

for (const width of [320, 1366]) {
  test(`joining instructions fit ${width}px and use saved login settings`, async ({ page }) => {
    await mockAPI(page, { initiallyRunning: true });
    await page.setViewportSize({ width, height: 900 });
    await login(page);
    await page.goto(`/servers/${serverID}`);
    await page.getByRole("button", { name: "Join", exact: true }).click();
    const card = page.getByRole("region", { name: "How to join" });
    await expect(card.getByText("Sign in with Microsoft", { exact: true })).toBeVisible();
    await expect(card.getByText("Whitelist is disabled.", { exact: false })).toBeVisible();
    await expect(card.getByText("play.example.com:25565", { exact: true })).toBeVisible();
    await expect(card.getByText("Invalid session:", { exact: false })).toBeVisible();
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
      true,
    );
  });
}
test("moderator joining help does not request privileged settings", async ({ page }) => {
  const calls = await mockAPI(page, { role: "MODERATOR", initiallyRunning: true });
  await login(page);
  await page.goto(`/servers/${serverID}`);
  await page.getByRole("button", { name: "Join", exact: true }).click();
  const card = page.getByRole("region", { name: "How to join" });
  await expect(card.getByText("Check which account you need", { exact: true })).toBeVisible();
  await expect(card.getByRole("link", { name: "Manage allowed players →" })).toHaveCount(0);
  expect(calls.some((c) => c.path.endsWith("/settings"))).toBe(false);
});
test("offline joining guidance does not assume an authentication plugin is installed", async ({
  page,
}) => {
  await mockAPI(page, { initiallyRunning: true });
  await page.route(`http://localhost:8080/servers/${serverID}/settings`, (route) =>
    route.fulfill({ json: { online_mode: false, whitelist: true } }),
  );
  await login(page);
  await page.goto(`/servers/${serverID}`);
  await page.getByRole("button", { name: "Join", exact: true }).click();
  const card = page.getByRole("region", { name: "How to join" });
  await expect(
    card.getByText("Check the server’s login instructions", { exact: true }),
  ).toBeVisible();
  await expect(
    card.getByText("plugins are not installed automatically.", { exact: false }),
  ).toBeVisible();
  await expect(card.getByText("Whitelist is enabled.", { exact: false })).toBeVisible();
});

for (const width of [320, 1366]) {
  test(`admin capacity and failure guidance fit ${width}px`, async ({ page }, testInfo) => {
    const calls = await mockAPI(page, { isAdmin: true, failedServer: true });
    await page.setViewportSize({ width, height: 900 });
    await login(page);
    await page.goto("/admin");
    await expect(page.getByText("Not enough available capacity", { exact: true })).toBeVisible();
    await page.getByRole("button", { name: "Retry setup", exact: true }).click();
    await expect(page.getByRole("dialog")).toContainText("Free capacity or enable another node");
    await page.getByRole("textbox", { name: "Reason" }).fill("Capacity restored after inspection");
    await page.getByRole("button", { name: "Confirm action" }).click();
    const retry = calls.find((c) => c.path.endsWith("/retry") && c.method === "POST");
    expect(retry.body).toEqual({ reason: "Capacity restored after inspection" });
    expect(retry.headers["idempotency-key"]).toBeTruthy();
    await page.getByRole("button", { name: "Nodes", exact: true }).click();
    const capacity = page.getByRole("region", { name: "Available hosting capacity" });
    await expect(capacity.getByText("9 more servers", { exact: true })).toBeVisible();
    await expect(
      capacity.getByRole("progressbar", { name: "EU node RAM allocated" }),
    ).toHaveAttribute("value", "2048");
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
      true,
    );
    await page.screenshot({ path: testInfo.outputPath("capacity.png"), fullPage: true });
  });
}
test("unknown Panel creation outcome requires inspection confirmation before retry", async ({
  page,
}) => {
  await mockAPI(page, { isAdmin: true });
  await page.route("http://localhost:8080/admin/servers?**", (route) =>
    route.fulfill({
      json: [
        {
          ...baseServer,
          status: "FAILED",
          job_error: "panel_create_outcome_unknown",
          job_status: "FAILED",
        },
      ],
    }),
  );
  await login(page);
  await page.goto("/admin");
  await page.getByRole("button", { name: "Retry setup", exact: true }).click();
  await page.getByRole("textbox", { name: "Reason" }).fill("Panel operations finished and absent");
  await expect(page.getByRole("button", { name: "Confirm action" })).toBeDisabled();
  await page.getByRole("checkbox").check();
  await expect(page.getByRole("button", { name: "Confirm action" })).toBeEnabled();
});
test("disabled and full nodes do not advertise capacity for new servers", async ({ page }) => {
  await mockAPI(page, { isAdmin: true });
  await page.route("http://localhost:8080/admin/nodes", (route) =>
    route.fulfill({
      json: [
        {
          id: "disabled",
          name: "Disabled node",
          enabled: false,
          ram_capacity_mb: 20480,
          disk_capacity_mb: 102400,
          cpu_capacity: 1000,
          allocated_ram_mb: 0,
          allocated_disk_mb: 0,
          allocated_cpu: 0,
        },
        {
          id: "full",
          name: "Full node",
          enabled: true,
          ram_capacity_mb: 20480,
          disk_capacity_mb: 102400,
          cpu_capacity: 1000,
          allocated_ram_mb: 20480,
          allocated_disk_mb: 0,
          allocated_cpu: 0,
        },
      ],
    }),
  );
  await login(page);
  await page.goto("/admin");
  await page.getByRole("button", { name: "Nodes", exact: true }).click();
  await expect(
    page
      .getByRole("region", { name: "Available hosting capacity" })
      .getByText("0 more servers", { exact: true }),
  ).toHaveCount(2);
});
test("catalog displays safe project icons and recovers from failed images", async ({
  page,
}, testInfo) => {
  await mockAPI(page);
  await page.setViewportSize({ width: 320, height: 900 });
  await page.route("http://localhost:8080/servers/*/plugins/catalog?**", (route) =>
    route.fulfill({
      json: {
        projects: [
          {
            project_id: "viaversion",
            title: "ViaVersion",
            description: "Version compatibility",
            icon_url: "https://cdn.modrinth.com/data/test/icon.png",
          },
          {
            project_id: "unsafe",
            title: "Unsafe image",
            description: "Fallback",
            icon_url: "https://example.com/tracker.png",
          },
          { project_id: "missing", title: "No image", description: "Fallback" },
        ],
        has_more: false,
        next_offset: 3,
        software: "PAPER",
        minecraft_version: "1.21.1",
      },
    }),
  );
  await page.route("https://cdn.modrinth.com/data/test/icon.png", (route) =>
    route.fulfill({
      contentType: "image/png",
      body: Buffer.from(
        "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR4nGNwmRn6HwAEiQIyWIHUyQAAAABJRU5ErkJggg==",
        "base64",
      ),
    }),
  );
  await openServer(page);
  await page.getByRole("link", { name: "Plugins", exact: true }).click();
  await page.getByRole("button", { name: "Search Modrinth" }).click();
  const icons = page.locator(".project-icon");
  await expect(icons).toHaveCount(3);
  await expect(icons.locator("img")).toHaveCount(1);
  await expect(icons.locator("svg")).toHaveCount(2);
  await icons.locator("img").evaluate((img) => img.dispatchEvent(new Event("error")));
  await expect(icons.locator("img")).toHaveCount(0);
  await expect(icons.locator("svg")).toHaveCount(3);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await page.screenshot({ path: testInfo.outputPath("plugin-images.png"), fullPage: true });
});

for (const width of [320, 1366]) {
  test(`catalog defaults to most downloaded and preserves filters across pages at ${width}px`, async ({
    page,
  }) => {
    await mockAPI(page);
    const searches = [];
    await page.route(
      `http://localhost:8080/servers/${serverID}/plugins/catalog?**`,
      async (route) => {
        const url = new URL(route.request().url());
        searches.push(Object.fromEntries(url.searchParams));
        const more = url.searchParams.get("offset") === "12";
        await route.fulfill({
          json: {
            projects: [
              {
                project_id: more ? "second" : "viaversion",
                title: more ? "Second plugin" : "ViaVersion",
                description: "Compatible release",
                release_version: "5.0",
                downloads: 10000,
              },
            ],
            has_more: !more,
            next_offset: more ? 24 : 12,
            software: "PAPER",
            minecraft_version: "1.21.1",
          },
        });
      },
    );
    await page.setViewportSize({ width, height: 900 });
    await openServer(page);
    await page.getByRole("link", { name: "Plugins", exact: true }).click();
    await expect(page.getByLabel("Sort catalog")).toHaveAttribute("data-value", "downloads", {
      timeout: 1500,
    });
    await expect(
      page.locator(".addon-catalog").getByText("ViaVersion", { exact: true }),
    ).toBeVisible();
    expect(searches[0]).toEqual({ q: "", sort: "downloads", category: "", offset: "0" });
    await selectValue(page, page.getByLabel("Category filter"), "utility");
    await selectValue(page, page.getByLabel("Sort catalog"), "updated");
    await page.getByLabel("Search plugins", { exact: true }).fill("versions");
    await page.getByRole("button", { name: "Search Modrinth" }).click();
    await expect
      .poll(() => searches.at(-1))
      .toEqual({ q: "versions", sort: "updated", category: "utility", offset: "0" });
    await page.getByRole("button", { name: "Load more" }).click();
    await expect(
      page.locator(".addon-catalog").getByText("Second plugin", { exact: true }),
    ).toBeVisible();
    await expect(
      page.locator(".addon-catalog").getByText("ViaVersion", { exact: true }),
    ).toBeVisible();
    expect(searches.at(-1)).toEqual({
      q: "versions",
      sort: "updated",
      category: "utility",
      offset: "12",
    });
    await page.getByRole("button", { name: "Reset filters" }).click();
    await expect(page.getByLabel("Sort catalog")).toHaveAttribute("data-value", "downloads");
    await expect(page.getByLabel("Category filter")).toHaveAttribute("data-value", "");
    await expect(page.getByLabel("Search plugins", { exact: true })).toHaveValue("");
    await expect(
      page.locator(".addon-catalog").getByText("Second plugin", { exact: true }),
    ).toHaveCount(0);
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
      true,
    );
  });
}

test("catalog ignores late responses from earlier sorting choices", async ({ page }) => {
  await mockAPI(page);
  await page.route(
    `http://localhost:8080/servers/${serverID}/plugins/catalog?**`,
    async (route) => {
      const sort = new URL(route.request().url()).searchParams.get("sort");
      if (sort === "newest") await new Promise((resolve) => setTimeout(resolve, 400));
      await route
        .fulfill({
          json: {
            projects: [
              {
                project_id: sort,
                title: sort === "newest" ? "Old response" : "Current response",
                description: "Matching release",
              },
            ],
            has_more: false,
            next_offset: 1,
            software: "PAPER",
            minecraft_version: "1.21.1",
          },
        })
        .catch(() => {});
    },
  );
  await openServer(page);
  await page.getByRole("link", { name: "Plugins", exact: true }).click();
  await selectValue(page, page.getByLabel("Sort catalog"), "newest");
  await selectValue(page, page.getByLabel("Sort catalog"), "updated");
  await expect(page.getByText("Current response", { exact: true })).toBeVisible();
  await page.waitForTimeout(500);
  await expect(page.getByText("Old response", { exact: true })).toHaveCount(0);
});
test("catalog failure offers retry instead of claiming no compatible projects", async ({
  page,
}) => {
  await mockAPI(page);
  let failing = true;
  await page.route(`http://localhost:8080/servers/${serverID}/plugins/catalog?**`, (route) =>
    failing
      ? route.fulfill({ status: 503, json: { error: "Catalog temporarily unavailable" } })
      : route.fulfill({
          json: {
            projects: [],
            has_more: false,
            next_offset: 0,
            software: "PAPER",
            minecraft_version: "1.21.1",
          },
        }),
  );
  await openServer(page);
  await page.getByRole("link", { name: "Plugins", exact: true }).click();
  await expect(page.getByText("Catalog temporarily unavailable", { exact: true })).toBeVisible();
  await expect(page.getByText("No compatible projects found.", { exact: false })).toHaveCount(0);
  failing = false;
  await page.getByRole("button", { name: "Retry catalog load" }).click();
  await expect(page.getByText("No compatible projects found.", { exact: false })).toBeVisible();
});

test("daily backups enable and disable with time timezone and revision", async ({ page }) => {
  const calls = await mockAPI(page);
  await openServer(page);
  await page.getByRole("link", { name: "Backups", exact: true }).click();
  const card = page.getByRole("region", { name: "Daily backups" });
  await expect(card.getByRole("checkbox", { name: "Enable daily backups" })).not.toBeChecked();
  await card.getByRole("checkbox", { name: "Enable daily backups" }).check();
  await card.getByLabel("Backup time").fill("04:15");
  await card.getByLabel("Timezone").fill("Asia/Ashgabat");
  await card.getByRole("button", { name: "Save schedule" }).click();
  await expect(card.getByRole("status")).toContainText("Schedule saved");
  const call = calls.find((c) => c.path.endsWith("/backup-schedule") && c.method === "PUT");
  expect(call.body).toEqual({
    enabled: true,
    hour: 4,
    minute: 15,
    timezone: "Asia/Ashgabat",
    revision: 0,
  });
  await card.getByRole("checkbox", { name: "Enable daily backups" }).uncheck();
  await card.getByRole("button", { name: "Save schedule" }).click();
  await expect(card.getByRole("checkbox")).not.toBeChecked();
  await expect(card).toContainText("Off");
});
test("daily backups stay readable for admins with no edit controls", async ({ page }) => {
  await mockAPI(page, {
    role: "ADMIN",
    backupSchedule: {
      enabled: true,
      revision: 2,
      next_run: "2026-10-14T03:00:00Z",
      timezone: "Europe/Berlin",
    },
  });
  await openServer(page);
  await page.getByRole("link", { name: "Backups", exact: true }).click();
  const card = page.getByRole("region", { name: "Daily backups" });
  await expect(card).toContainText("Europe/Berlin");
  await expect(card.getByRole("button", { name: "Save schedule" })).toHaveCount(0);
});
test("daily backups refresh conflicts without silently resubmitting", async ({ page }) => {
  const calls = await mockAPI(page, { scheduleConflict: true });
  await openServer(page);
  await page.getByRole("link", { name: "Backups", exact: true }).click();
  const card = page.getByRole("region", { name: "Daily backups" });
  await card.getByRole("checkbox").check();
  await card.getByRole("button", { name: "Save schedule" }).click();
  await expect(card.getByRole("alert")).toContainText("changed");
  await card.getByRole("button", { name: "Reload schedule" }).click();
  await expect(card.getByRole("checkbox")).not.toBeChecked();
  expect(
    calls.filter((c) => c.path.endsWith("/backup-schedule") && c.method === "PUT"),
  ).toHaveLength(1);
});
for (const width of [390, 1366])
  test(`daily backup warnings fit ${width}px`, async ({ page }, testInfo) => {
    await page.setViewportSize({ width, height: 900 });
    await mockAPI(page, {
      backupSchedule: {
        enabled: true,
        revision: 3,
        timezone: "Asia/Ashgabat",
        last_success: "2026-10-12T03:00:00Z",
        warning: "needs_inspection",
        runs: [
          {
            id: "run",
            status: "NEEDS_INSPECTION",
            scheduled_at: "2026-10-13T03:00:00Z",
            error_code: "create_unconfirmed",
          },
        ],
      },
    });
    await openServer(page);
    await page.getByRole("link", { name: "Backups", exact: true }).click();
    const card = page.getByRole("region", { name: "Daily backups" });
    await expect(card).toContainText("needs inspection");
    await expect(card).toContainText("Last successful backup");
    await expect(card).toContainText("Asia/Ashgabat");
    expect(
      await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
    ).toBe(true);
    await page.screenshot({ path: testInfo.outputPath("daily-backups.png"), fullPage: true });
  });
test("daily backups require two slots and cannot enable without host support", async ({ page }) => {
  await mockAPI(page, {
    servers: [{ ...baseServer, backup_limit: 1 }],
    backupSchedule: { available: false, warning: "insufficient_allowance" },
  });
  await openServer(page);
  await page.getByRole("link", { name: "Backups", exact: true }).click();
  const card = page.getByRole("region", { name: "Daily backups" });
  await expect(card).toContainText("two backup slots");
  await expect(card.getByRole("checkbox")).toBeDisabled();
});

test("daily backup polling preserves unsaved edits and their original revision", async ({
  page,
}) => {
  const calls = await mockAPI(page, {
    scheduleChangesAfterRead: true,
    backupSchedule: { revision: 2 },
  });
  await openServer(page);
  await page.getByRole("link", { name: "Backups", exact: true }).click();
  const card = page.getByRole("region", { name: "Daily backups" });
  await card.getByLabel("Backup time").fill("05:15");
  await expect
    .poll(
      () => calls.filter((c) => c.path.endsWith("/backup-schedule") && c.method === "GET").length,
      { timeout: 20000 },
    )
    .toBeGreaterThanOrEqual(2);
  await expect(card.getByLabel("Backup time")).toHaveValue("05:15");
  await card.getByRole("button", { name: "Save schedule" }).click();
  const call = calls.find((c) => c.path.endsWith("/backup-schedule") && c.method === "PUT");
  expect(call.body.revision).toBe(2);
});

test("suspended owners can turn off daily backups", async ({ page }) => {
  await mockAPI(page, {
    serverState: "SUSPENDED",
    backupSchedule: { enabled: true, revision: 2, warning: "server_unavailable" },
  });
  await openServer(page);
  await page.goto(`/servers/${serverID}/backups`);
  const card = page.getByRole("region", { name: "Daily backups" });
  await expect(card).toContainText("unavailable");
  await card.getByRole("checkbox").uncheck();
  await card.getByRole("button", { name: "Save schedule" }).click();
  await expect(card.getByRole("status").filter({ hasText: "Schedule saved" })).toContainText(
    "Schedule saved",
  );
});
test("moderators cannot see daily backup controls", async ({ page }) => {
  const calls = await mockAPI(page, { role: "MODERATOR" });
  await openServer(page);
  await page.goto(`/servers/${serverID}/backups`);
  await expect(page.getByRole("region", { name: "Daily backups" })).toHaveCount(0);
  await expect(page.getByText("Your server role does not allow this page.")).toBeVisible();
  expect(calls.filter((c) => c.path.endsWith("/backup-schedule"))).toHaveLength(0);
});
for (const status of ["FAILED", "SKIPPED"])
  test(`daily backup ${status.toLowerCase()} keeps last success visible`, async ({ page }) => {
    await mockAPI(page, {
      backupSchedule: {
        enabled: true,
        revision: 2,
        last_success: "2026-10-12T03:00:00Z",
        warning: status === "FAILED" ? "backup_failed" : "capacity_full",
        runs: [{ id: "run", status, scheduled_at: "2026-10-13T03:00:00Z" }],
      },
    });
    await openServer(page);
    await page.getByRole("link", { name: "Backups", exact: true }).click();
    const card = page.getByRole("region", { name: "Daily backups" });
    await expect(card).toContainText("Last successful backup");
    await expect(card).toContainText(status === "FAILED" ? "Failed" : "Skipped");
    await expect(card).toContainText("preserved");
  });

for (const viewport of [
  { name: "laptop", width: 1366, height: 900 },
  { name: "mobile", width: 390, height: 844 },
]) {
  test(`visual audit of every page at ${viewport.name} size`, async ({ page }, testInfo) => {
    test.setTimeout(120000);
    await page.setViewportSize(viewport);
    await mockAPI(page, { isAdmin: true });
    for (const route of [
      "/",
      "/login",
      "/register",
      "/forgot-password",
      "/reset-password",
      "/verify",
    ]) {
      await page.goto(route);
      await expect(page.locator("h1").first()).toBeVisible();
      await page.screenshot({
        path: testInfo.outputPath(`${viewport.name}-${route.replaceAll("/", "") || "home"}.png`),
        fullPage: true,
      });
      expect(
        await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
        route,
      ).toBe(true);
    }
    await login(page);
    for (const route of [
      "/dashboard",
      "/servers",
      "/servers/new",
      "/billing",
      `/checkout/${orderID}`,
      "/account",
      "/guide",
      "/feedback",
      "/admin",
      `/servers/${serverID}`,
      `/servers/${serverID}/addons/simple-voice-chat`,
      ...["console", "files", "members", "worlds", "players", "plugins", "settings", "backups"].map(
        (section) => `/servers/${serverID}/${section}`,
      ),
    ]) {
      await page.goto(route);
      await expect(page.locator("main h1").first()).toBeVisible();
      await expect(page.getByText("Loading…", { exact: true })).toHaveCount(0);
      await page.screenshot({
        path: testInfo.outputPath(
          `${viewport.name}-${route.replace(serverID, "server").replace(orderID, "order").replaceAll("/", "-")}.png`,
        ),
        fullPage: true,
      });
      expect(
        await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
        route,
      ).toBe(true);
    }
    await page.goto("/admin");
    for (const tab of ["Plans", "Users", "Orders", "Nodes", "Audit", "Invitations", "Feedback"]) {
      await page
        .getByRole("navigation", { name: "Admin sections" })
        .getByRole("button", { name: tab, exact: true })
        .click();
      await expect(page.getByText("Loading…", { exact: true })).toHaveCount(0);
      await page.evaluate(() => window.scrollTo(0, 0));
      await page.screenshot({
        path: testInfo.outputPath(`${viewport.name}-admin-${tab.toLowerCase()}.png`),
        fullPage: true,
      });
      expect(
        await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
        tab,
      ).toBe(true);
    }
    // Mods use the same catalog but their own software and content guidance.
    await page.route(`http://localhost:8080/servers/${serverID}`, (route) =>
      route.fulfill({
        contentType: "application/json",
        body: JSON.stringify({ ...baseServer, server_type: "FABRIC" }),
      }),
    );
    await page.goto(`/servers/${serverID}/mods`);
    await expect(page.getByRole("heading", { name: "Find mods" })).toBeVisible();
    await page.screenshot({
      path: testInfo.outputPath(`${viewport.name}-mods.png`),
      fullPage: true,
    });
  });
}

test("custom dropdown supports keyboard selection and keeps its menu inside a phone viewport", async ({
  page,
}, testInfo) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await mockAPI(page);
  await login(page);
  await page.goto(`/servers/${serverID}/settings`);
  const control = page.getByRole("combobox", { name: "Difficulty" });
  await control.focus();
  await control.press("Enter");
  await expect(control).toHaveAttribute("aria-expanded", "true");
  await control.press("End");
  await control.press("Enter");
  await expect(control).toContainText("hard");
  await control.click();
  const bounds = await page.getByRole("listbox").boundingBox();
  await page.screenshot({ path: testInfo.outputPath("custom-dropdown.png") });
  expect(bounds.x).toBeGreaterThanOrEqual(0);
  expect(bounds.x + bounds.width).toBeLessThanOrEqual(390);
  await control.press("Escape");
  await expect(page.getByRole("listbox")).toHaveCount(0);
  await expect(control).toBeFocused();
});

test("catalog shows actual installation progress and a durable installed badge", async ({
  page,
}, testInfo) => {
  await mockAPI(page);
  let installed = false;
  let release;
  const waiting = new Promise((resolve) => {
    release = resolve;
  });
  await page.route(`http://localhost:8080/servers/${serverID}/plugins/catalog/installed`, (route) =>
    route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({ projects: installed ? ["viaversion"] : [] }),
    }),
  );
  await page.route(
    `http://localhost:8080/servers/${serverID}/plugins/catalog/viaversion`,
    async (route) => {
      if (route.request().method() === "GET") return route.fallback();
      await waiting;
      installed = true;
      await route.fulfill({
        contentType: "application/json",
        body: JSON.stringify({
          status: "installed",
          files: [{ project_id: "viaversion", filename: "ViaVersion.jar", size: 6500000 }],
        }),
      });
    },
  );
  await login(page);
  await page.goto(`/servers/${serverID}/plugins`);
  await page.getByRole("button", { name: "Review install" }).click();
  await page.getByRole("button", { name: "Install now" }).click();
  const progress = page.getByRole("progressbar", { name: "Installation progress" });
  await expect(progress).toHaveAttribute("value", "1");
  await expect(progress).toHaveAttribute("max", "2");
  await expect(page.getByText("Downloading and checking files", { exact: true })).toBeVisible();
  await page.screenshot({ path: testInfo.outputPath("installation-progress.png") });
  release();
  await expect(page.getByRole("button", { name: "File installed", exact: true })).toBeDisabled();
  await page.reload();
  await expect(page.getByRole("button", { name: "File installed", exact: true })).toBeDisabled();
  installed = false;
  await page.reload();
  await expect(page.getByRole("button", { name: "Review install" })).toBeEnabled();
});

for (const width of [390, 1366]) {
  test(`guided voice settings load real values, reset defaults, and preserve server-owned networking at ${width}px`, async ({
    page,
  }, testInfo) => {
    const calls = await mockAPI(page);
    await page.setViewportSize({ width, height: 900 });
    await login(page);
    await page.goto(`/servers/${serverID}/addons/simple-voice-chat`);
    await expect(page.getByLabel("Voice distance", { exact: true })).toHaveValue("72");
    await expect(page.getByText("play.example.com:24454", { exact: true })).toBeVisible();
    await page.getByRole("button", { name: "Reset to defaults" }).click();
    await expect(page.getByLabel("Voice distance", { exact: true })).toHaveValue("48");
    await expect(page.getByLabel("Require the client mod", { exact: true })).not.toBeChecked();
    await page.getByLabel("Voice distance", { exact: true }).fill("80");
    await page.getByLabel("Group chats", { exact: true }).uncheck();
    await page.getByRole("button", { name: "Save settings", exact: true }).click();
    await expect(page.getByRole("status").filter({ hasText: "Settings saved" })).toBeVisible();
    const save = calls.find(
      (call) => call.path.endsWith("/addons/simple-voice-chat") && call.method === "PUT",
    );
    expect(save.body).toEqual({
      ...voiceDefaults,
      max_voice_distance: 80,
      enable_groups: false,
      revision: "v".repeat(64),
    });
    await selectValue(page, page.getByRole("combobox", { name: "Client loader" }), "neoforge");
    await expect(page.getByRole("link", { name: "Find client download" })).toHaveAttribute(
      "href",
      /g=1.21.1&l=neoforge$/,
    );
    await page.getByLabel("Player’s Minecraft version", { exact: true }).fill("26.3");
    await expect(page.getByRole("link", { name: "Find client download" })).toHaveAttribute(
      "href",
      /g=26.3&l=neoforge$/,
    );
    await expect(
      page.getByText("Minecraft’s built-in download prompt", { exact: false }),
    ).toBeVisible();
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
      true,
    );
    await page.screenshot({
      path: testInfo.outputPath(`voice-settings-${width}.png`),
      fullPage: true,
    });
  });
}

test("voice settings preserve dirty edits and block stale or online saves", async ({ page }) => {
  await mockAPI(page, { initiallyRunning: true });
  let revision = "v".repeat(64);
  await page.route(`http://localhost:8080/servers/${serverID}/addons/simple-voice-chat`, (route) =>
    route.fulfill({ json: { ...voiceFixture, revision } }),
  );
  await login(page);
  await page.goto(`/servers/${serverID}/addons/simple-voice-chat`);
  await page.getByLabel("Voice distance", { exact: true }).fill("100");
  await expect(page.getByRole("button", { name: "Save settings", exact: true })).toBeDisabled();
  revision = "z".repeat(64);
  await expect(
    page.getByText("The configuration changed while you were editing", { exact: false }),
  ).toBeVisible({ timeout: 20000 });
  await expect(page.getByLabel("Voice distance", { exact: true })).toHaveValue("100");
  await page.getByRole("button", { name: "Reload settings" }).click();
  await expect(page.getByLabel("Voice distance", { exact: true })).toHaveValue("72");
});

test("voice networking is prepared by a job and configured pages exclude moderators", async ({
  page,
}) => {
  const calls = await mockAPI(page);
  let state = "NONE";
  await page.route(`http://localhost:8080/servers/${serverID}/addons/simple-voice-chat`, (route) =>
    route.fulfill({
      json: {
        ...voiceFixture,
        network: {
          status: state,
          ...(state === "READY" ? { hostname: "play.example.com", port: 24454 } : {}),
        },
      },
    }),
  );
  await page.route(
    `http://localhost:8080/servers/${serverID}/addons/simple-voice-chat/network`,
    (route) => {
      if (route.request().method() === "GET")
        return route.fulfill({
          json: {
            status: state,
            ...(state === "READY" ? { hostname: "play.example.com", port: 24454 } : {}),
          },
        });
      state = "PREPARING";
      return route.fulfill({ json: { id: orderID, status: "PENDING" }, status: 202 });
    },
  );
  await login(page);
  await page.goto(`/servers/${serverID}/addons/simple-voice-chat`);
  await expect(page.getByRole("button", { name: "Save settings", exact: true })).toBeDisabled();
  await page.getByRole("button", { name: "Prepare voice connection" }).click();
  await expect(page.getByText("Preparing your voice connection", { exact: false })).toBeVisible();
  state = "READY";
  await expect(page.getByRole("button", { name: "Save settings", exact: true })).toBeEnabled({
    timeout: 6000,
  });
  expect(calls.some((call) => call.body?.port || call.body?.pterodactyl_allocation_id)).toBe(false);
});

test("moderators cannot load addon configurations", async ({ page }) => {
  const calls = await mockAPI(page, { role: "MODERATOR" });
  await login(page);
  await page.goto(`/servers/${serverID}/addons/simple-voice-chat`);
  await expect(page.getByText("An owner or administrator can configure this addon.")).toBeVisible();
  expect(calls.some((call) => call.path.includes("/addons/"))).toBe(false);
});

test("installed supported catalog addons lead directly to guided configuration", async ({
  page,
}) => {
  await mockAPI(page);
  const project = {
    project_id: "9eGKb6K1",
    title: "Simple Voice Chat",
    description: "Proximity voice chat",
    downloads: 100000,
    configuration_id: "simple-voice-chat",
  };
  let installed = false;
  const file = {
    project_id: project.project_id,
    version_id: "voice-release",
    filename: "voicechat-bukkit-2.6.jar",
    size: 10000,
  };
  await page.route(`http://localhost:8080/servers/${serverID}/plugins/catalog?**`, (route) =>
    route.fulfill({ json: { projects: [project], has_more: false } }),
  );
  await page.route(`http://localhost:8080/servers/${serverID}/plugins/catalog/installed`, (route) =>
    route.fulfill({ json: { projects: installed ? [project.project_id] : [] } }),
  );
  await page.route(`http://localhost:8080/servers/${serverID}/plugins`, (route) =>
    route.fulfill({
      json: installed ? [{ name: file.filename, is_file: true, size: file.size }] : [],
    }),
  );
  await page.route(
    `http://localhost:8080/servers/${serverID}/plugins/catalog/${project.project_id}`,
    (route) => {
      if (route.request().method() === "GET")
        return route.fulfill({
          json: {
            project: project.project_id,
            software: "PAPER",
            minecraft_version: "1.21.1",
            revision: "v".repeat(64),
            files: [file],
          },
        });
      installed = true;
      return route.fulfill({
        json: { status: "installed", files: [file], restart_required: true },
      });
    },
  );
  await login(page);
  await page.goto(`/servers/${serverID}/plugins`);
  await page.getByRole("button", { name: "Review install" }).click();
  await page.getByRole("button", { name: "Install now" }).click();
  await page
    .locator(".plugin-list li")
    .filter({ hasText: file.filename })
    .getByRole("link", { name: "Configure", exact: true })
    .click();
  await expect(page).toHaveURL(new RegExp(`/servers/${serverID}/addons/simple-voice-chat$`));
  await expect(page.getByLabel("Voice distance", { exact: true })).toHaveValue("72");
});

for (const [software, collection, width] of [
  ["PAPER", "plugins", 1366],
  ["FABRIC", "mods", 390],
]) {
  test(`installed ${collection} expose configuration without catalog results`, async ({
    page,
  }, testInfo) => {
    await page.setViewportSize({ width, height: 900 });
    await mockAPI(page, {
      servers: [{ ...baseServer, server_type: software }],
      initiallyRunning: true,
    });
    await page.route(
      `http://localhost:8080/servers/${serverID}/${collection}/catalog?**`,
      (route) => route.fulfill({ json: { projects: [], has_more: false } }),
    );
    await page.route(`http://localhost:8080/servers/${serverID}/${collection}`, (route) =>
      route.fulfill({
        json: [
          { name: "voicechat-2.6.jar", is_file: true, size: 10000 },
          { name: "other-addon.jar", is_file: true, size: 10000 },
        ],
      }),
    );
    await login(page);
    await page.goto(`/servers/${serverID}/${collection}`);
    const row = page.locator(".plugin-list li").filter({ hasText: "voicechat-2.6.jar" });
    await expect(row.getByRole("link", { name: "Configure", exact: true })).toBeVisible();
    await expect(
      row.getByRole("button", { name: `Delete ${collection === "mods" ? "mod" : "plugin"}` }),
    ).toBeDisabled();
    await expect(
      page
        .locator(".plugin-list li")
        .filter({ hasText: "other-addon.jar" })
        .getByRole("link", { name: "Configure" }),
    ).toHaveCount(0);
    expect(
      await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
    ).toBe(true);
    await row.scrollIntoViewIfNeeded();
    await page.screenshot({
      path: testInfo.outputPath(`installed-${collection}.png`),
      fullPage: true,
    });
    await row.getByRole("link", { name: "Configure", exact: true }).click();
    await expect(page).toHaveURL(new RegExp(`/servers/${serverID}/addons/simple-voice-chat$`));
  });
}

for (const [width, voiceRequired] of [
  [1366, true],
  [390, false],
]) {
  test(`player setup labels distinguish voice join requirements at ${width}px`, async ({
    page,
  }, testInfo) => {
    await page.setViewportSize({ width, height: 900 });
    await mockAPI(page, { initiallyRunning: true });
    const voice = {
      filename: "voicechat.jar",
      title: "Simple Voice Chat",
      project_id: "9eGKb6K1",
      configuration_id: "simple-voice-chat",
      client_side: "optional",
      voice_required: voiceRequired,
    };
    const addons = [voice, { filename: "manual.jar", title: "manual.jar", client_side: "unknown" }];
    await page.route(`http://localhost:8080/servers/${serverID}/client-requirements`, (route) =>
      route.fulfill({ json: { addons } }),
    );
    await openServer(page);
    await page.getByRole("button", { name: "Join", exact: true }).click();
    const notice = page.getByLabel("Player client requirements");
    await expect(
      notice.getByText(voiceRequired ? "Modded client required" : "Check client requirements", {
        exact: true,
      }),
    ).toBeVisible();
    await expect(
      notice.getByText(voiceRequired ? "Client mod required to join" : "Client mod for voice", {
        exact: true,
      }),
    ).toBeVisible();
    await expect(notice.getByText("Client details not provided", { exact: true })).toBeVisible();
    await notice.scrollIntoViewIfNeeded();
    await page.screenshot({
      path: testInfo.outputPath(`client-requirements-${width}.png`),
      fullPage: true,
    });
    expect(
      await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
    ).toBe(true);
    await page.route(`http://localhost:8080/servers/${serverID}/plugins`, (route) =>
      route.fulfill({
        json: [
          { name: "voicechat.jar", size: 3 },
          { name: "manual.jar", size: 6 },
        ],
      }),
    );
    await page.goto(`/servers/${serverID}/plugins`);
    const row = page.locator(".plugin-list li").filter({ hasText: "voicechat.jar" });
    await expect(
      row.getByText(voiceRequired ? "Client mod required to join" : "Client mod for voice", {
        exact: true,
      }),
    ).toBeVisible();
  });
}

test("catalog and installation preview show client requirements", async ({ page }) => {
  await mockAPI(page);
  const projects = [
    { project_id: "required", title: "Required addon", client_side: "required" },
    { project_id: "optional", title: "Optional addon", client_side: "optional" },
    { project_id: "serveronly", title: "Server addon", client_side: "unsupported" },
    { project_id: "unknown", title: "Unknown addon" },
    { project_id: "9eGKb6K1", title: "Simple Voice Chat", client_side: "required" },
  ];
  await page.route(`http://localhost:8080/servers/${serverID}/plugins/catalog?**`, (route) =>
    route.fulfill({ json: { projects, has_more: false } }),
  );
  await page.route(`http://localhost:8080/servers/${serverID}/plugins/catalog/required`, (route) =>
    route.fulfill({
      json: {
        software: "PAPER",
        minecraft_version: "1.21.1",
        files: [{ version_id: "v", filename: "required.jar", size: 1, client_side: "required" }],
      },
    }),
  );
  await login(page);
  await page.goto(`/servers/${serverID}/plugins`);
  const row = (title) => page.locator(".plugin-list li").filter({ hasText: title });
  await expect(
    row("Required addon").getByText("Client mod required", { exact: true }),
  ).toBeVisible();
  await expect(
    row("Optional addon").getByText("Client mod optional", { exact: true }),
  ).toBeVisible();
  await expect(row("Server addon").getByText("Server only", { exact: true })).toBeVisible();
  await expect(
    row("Unknown addon").getByText("Client details not provided", { exact: true }),
  ).toBeVisible();
  await expect(
    row("Simple Voice Chat").getByText("Client mod for voice · check join rules", { exact: true }),
  ).toBeVisible();
  await row("Required addon").getByRole("button", { name: "Review install" }).click();
  await expect(
    page.getByRole("dialog").getByText("Client mod required", { exact: true }),
  ).toBeVisible();
});

test("failed requirement lookup never claims vanilla compatibility", async ({ page }) => {
  await mockAPI(page);
  await page.route(`http://localhost:8080/servers/${serverID}/client-requirements`, (route) =>
    route.fulfill({ status: 503, json: { message: "Unavailable" } }),
  );
  await openServer(page);
  await page.getByRole("button", { name: "Join", exact: true }).click();
  const notice = page.getByLabel("Player client requirements");
  await expect(
    notice.getByText("Player requirements could not be checked", { exact: true }),
  ).toBeVisible();
  await expect(notice.getByText("Vanilla Java supported", { exact: true })).toHaveCount(0);
  await expect(notice.getByRole("button", { name: "Check again" })).toBeVisible();
});

for (const width of [390, 1366]) {
  test(`compact overview shares complete join instructions at ${width}px`, async ({
    page,
  }, testInfo) => {
    await page.setViewportSize({ width, height: 900 });
    await page.addInitScript(() =>
      Object.defineProperty(navigator, "clipboard", {
        configurable: true,
        value: {
          writeText: async (text) => {
            window.sharedJoinInstructions = text;
          },
        },
      }),
    );
    await mockAPI(page, { initiallyRunning: true });
    await page.route(`http://localhost:8080/servers/${serverID}/settings`, (route) =>
      route.fulfill({ json: { online_mode: true, whitelist: true } }),
    );
    await page.route(`http://localhost:8080/servers/${serverID}/client-requirements`, (route) =>
      route.fulfill({
        json: {
          addons: [
            {
              filename: "voicechat.jar",
              title: "Simple Voice Chat",
              project_id: "9eGKb6K1",
              configuration_id: "simple-voice-chat",
              voice_required: true,
            },
            { filename: "custom.jar", client_side: "unknown" },
          ],
        },
      }),
    );
    await openServer(page);
    await page.getByRole("button", { name: "Join", exact: true }).click();
    const card = page.getByRole("region", { name: "How to join" });
    await expect(card.getByText("Sign in with Microsoft", { exact: true })).toBeVisible();
    await expect(page.locator(".metric-grid")).not.toBeVisible();
    await expect(page.getByRole("region", { name: "Play on your server" })).not.toBeVisible();
    // Let the two independent setup reads finish before sharing the snapshot.
    await expect(card.getByText("Client mod required to join", { exact: true })).toBeVisible();
    await expect(card.getByText("Sign in with Microsoft", { exact: true })).toBeVisible();
    await card.getByRole("button", { name: "Copy join instructions" }).click();
    await expect(card.getByRole("status")).toContainText("Join instructions copied");
    const text = await page.evaluate(() => window.sharedJoinInstructions);
    expect(text).toContain("Minecraft Java 1.21.1");
    expect(text).toContain("play.example.com:25565");
    expect(text).toContain("Client mod required to join");
    expect(text).toContain("Client details not provided");
    expect(text).toContain("Sign in through Microsoft");
    expect(text).toContain("Whitelist is enabled");
    expect(text).not.toContain("csrf");
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
      true,
    );
    await page.evaluate(() => window.scrollTo(0, 0));
    await page.screenshot({
      path: testInfo.outputPath(`compact-overview-${width}.png`),
      fullPage: true,
    });
  });
}

test("join instruction copy falls back safely when clipboard and setup are unavailable", async ({
  page,
}) => {
  await page.addInitScript(() =>
    Object.defineProperty(navigator, "clipboard", {
      configurable: true,
      value: {
        writeText: async () => {
          throw new Error("Denied");
        },
      },
    }),
  );
  await mockAPI(page, { role: "MODERATOR", initiallyRunning: true });
  await page.route(`http://localhost:8080/servers/${serverID}/client-requirements`, (route) =>
    route.fulfill({ status: 503, json: { error: "Unavailable" } }),
  );
  await openServer(page);
  await page.getByRole("button", { name: "Join", exact: true }).click();
  await expect(
    page.getByText("Player requirements could not be checked", { exact: true }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Copy join instructions" }).click();
  const text = await page.getByLabel("Join instructions to copy").inputValue();
  expect(text).toContain("Client requirements could not be checked");
  expect(text).toContain("Ask the host which account");
  expect(text).not.toContain("No client mods required");
});

test("connection troubleshooting gives specific version mods whitelist and voice guidance", async ({
  page,
}) => {
  await mockAPI(page, { initiallyRunning: true });
  await openServer(page);
  await page.getByRole("button", { name: "Join", exact: true }).click();
  const card = page.getByRole("region", { name: "How to join" });
  const control = card.getByRole("combobox", { name: "What does Minecraft show?" });
  await selectValue(page, control, "version");
  await expect(card.getByText("Open Minecraft Java 1.21.1.", { exact: false })).toBeVisible();
  await selectValue(page, control, "whitelist");
  await expect(card.getByRole("link", { name: "Open Players", exact: true })).toBeVisible();
  await selectValue(page, control, "mods");
  await card.getByRole("button", { name: "Show player setup" }).click();
  await expect(card.getByText("Sign in with Microsoft", { exact: true })).toBeVisible();
  await selectValue(page, control, "voice");
  await expect(card.getByText("Check microphone permissions", { exact: false })).toBeVisible();
});

test("plugin checks distinguish installed files from enabled disabled and unconfirmed runtime", async ({
  page,
}) => {
  await mockAPI(page, { initiallyRunning: true });
  let confirmed = true;
  let request;
  await page.route(`http://localhost:8080/servers/${serverID}/plugins/check`, (route) => {
    request = route.request();
    return route.fulfill({
      json: {
        confirmed,
        checked_at: new Date().toISOString(),
        uptime_ms: 0,
        plugins: confirmed
          ? [
              { name: "Essentials", enabled: true },
              { name: "BrokenPlugin", enabled: false },
            ]
          : [],
        reason: confirmed ? "" : "No coloured response received.",
      },
    });
  });
  await login(page);
  await page.goto(`/servers/${serverID}/plugins`);
  await expect(page.getByText("File installed", { exact: false }).first()).toBeVisible();
  await expect(page.getByText("Loaded and enabled", { exact: true })).toHaveCount(0);
  await page.getByRole("button", { name: "Check loaded plugins" }).click();
  await expect(page.getByText("Loaded and enabled", { exact: true })).toBeVisible();
  await expect(page.getByText("Disabled or failed to load", { exact: true })).toBeVisible();
  expect(request.postDataJSON()).toEqual({});
  expect(request.headers()["idempotency-key"]).toBeTruthy();
  confirmed = false;
  await page.getByRole("button", { name: "Check loaded plugins" }).click();
  await expect(
    page.getByText("Load status could not be confirmed.", { exact: false }),
  ).toBeVisible();
  await expect(page.getByText("Loaded and enabled", { exact: true })).toHaveCount(0);
});

test("offline plugins and mod loaders do not claim runtime confirmation", async ({ page }) => {
  const calls = await mockAPI(page);
  await login(page);
  await page.goto(`/servers/${serverID}/plugins`);
  await expect(page.getByRole("button", { name: "Check loaded plugins" })).toBeDisabled();
  await page.route(`http://localhost:8080/servers/${serverID}`, (route) =>
    route.fulfill({ json: { ...baseServer, server_type: "FABRIC" } }),
  );
  await page.goto(`/servers/${serverID}/mods`);
  await expect(
    page.getByText("Automatic per-mod load checks are not available", { exact: false }),
  ).toBeVisible();
  await expect(page.getByRole("button", { name: "Check loaded plugins" })).toHaveCount(0);
  expect(calls.some((call) => call.path.endsWith("/plugins/check"))).toBe(false);
});

test("addon installation waits for a successful backup or an explicit risk choice", async ({
  page,
}, testInfo) => {
  await page.setViewportSize({ width: 390, height: 844 });
  const calls = await mockAPI(page);
  let complete = false;
  const backup = {
    uuid: "40000000-0000-4000-8000-000000000099",
    name: "Before addon change",
    is_successful: false,
    completed_at: null,
  };
  await page.route(`http://localhost:8080/servers/${serverID}/backups`, (route) =>
    route.fulfill({
      json:
        route.request().method() === "POST"
          ? backup
          : [
              {
                ...backup,
                is_successful: complete,
                completed_at: complete ? "2026-10-13T11:00:00Z" : null,
              },
            ],
    }),
  );
  await login(page);
  await page.goto(`/servers/${serverID}/plugins`);
  await page.getByRole("button", { name: "Review install" }).click();
  const dialog = page.getByRole("dialog");
  await expect(dialog.getByRole("button", { name: "Install now", exact: true })).toBeDisabled();
  await dialog.getByRole("button", { name: "Create backup first" }).click();
  await expect(dialog.getByText("Waiting for the selected backup", { exact: false })).toBeVisible();
  await expect(dialog.getByRole("button", { name: "Install now", exact: true })).toBeDisabled();
  await page.screenshot({ path: testInfo.outputPath("backup-safety-mobile.png"), fullPage: true });
  complete = true;
  await expect(dialog.getByRole("button", { name: "Install now", exact: true })).toBeEnabled();
  await dialog.getByRole("button", { name: "Install now", exact: true }).click();
  expect(
    calls.find((c) => c.path.endsWith("/catalog/viaversion") && c.method === "POST").body.backup,
  ).toBe(backup.uuid);
});

test("no-backup risk acknowledgement is explicit", async ({ page }) => {
  const calls = await mockAPI(page);
  await page.route(`http://localhost:8080/servers/${serverID}/backups`, (route) =>
    route.fulfill({ json: [] }),
  );
  await login(page);
  await page.goto(`/servers/${serverID}/plugins`);
  await page.getByRole("button", { name: "Review install" }).click();
  const dialog = page.getByRole("dialog");
  await expect(dialog.getByRole("button", { name: "Install now", exact: true })).toBeDisabled();
  await dialog.getByLabel("I understand I am continuing without a backup.").check();
  await dialog.getByRole("button", { name: "Install now", exact: true }).click();
  const body = calls.find(
    (c) => c.path.endsWith("/catalog/viaversion") && c.method === "POST",
  ).body;
  expect(body).toMatchObject({ acknowledge_no_backup: true, backup: null });
});

test("version changes offer a backup before navigating to a new server", async ({ page }) => {
  await mockAPI(page);
  await openServer(page);
  await page.getByRole("button", { name: "Manage", exact: true }).click();
  await page.getByRole("button", { name: "Prepare version change" }).click();
  await expect(page.getByRole("combobox", { name: "Completed backup" })).toContainText(
    "Before changes",
  );
  await expect(page.getByText("This creates a separate server.", { exact: false })).toBeVisible();
  await expect(page.getByRole("link", { name: "Download your world →" })).toHaveAttribute(
    "href",
    `/servers/${serverID}/worlds`,
  );
});

test("startup help shows safe actionable issues and preserves the console fallback", async ({
  page,
}) => {
  await mockAPI(page);
  await page.route(`http://localhost:8080/servers/${serverID}/startup-check`, (route) =>
    route.fulfill({
      json: {
        checked: true,
        checked_at: "2026-10-13T11:00:00Z",
        issues: [
          {
            code: "MISSING_DEPENDENCY",
            title: "An addon is missing a dependency",
            help: "Check Console for the required addon names and versions.",
          },
        ],
      },
    }),
  );
  await openServer(page);
  await page.getByRole("button", { name: "Health", exact: true }).click();
  await expect(page.getByRole("status", { name: "Startup alerts" })).toBeVisible();
  await expect(page.getByText("An addon is missing a dependency", { exact: true })).toBeVisible();
  await expect(
    page
      .getByRole("region", { name: "Server health" })
      .getByRole("link", { name: "Open Console →" }),
  ).toHaveAttribute("href", `/servers/${serverID}/console`);
});

test("error reports attach server page and reference without query tokens or private logs", async ({
  page,
}) => {
  await mockAPI(page);
  const reference = "066aa59b-590d-4296-951c-81d70922d4f9";
  let submitted;
  await page.route(`http://localhost:8080/servers/${serverID}/plugins/catalog?**`, (route) =>
    route.fulfill({
      status: 502,
      headers: { "X-Request-ID": reference },
      json: { error: "Catalog temporarily unavailable", request_id: reference },
    }),
  );
  await page.route("http://localhost:8080/feedback", (route) => {
    submitted = route.request().postDataJSON();
    return route.fulfill({ status: 201, json: { ...feedbackFixture, id: orderID } });
  });
  await login(page);
  await page.goto(`/servers/${serverID}/plugins?token=private-query-value`);
  await page.getByRole("link", { name: "Report this problem →" }).first().click();
  await expect(page.getByText(`Support reference: ${reference}`, { exact: false })).toBeVisible();
  await page.getByLabel("Short summary").fill("Catalog search failed");
  await page.getByLabel("What happened?").fill("I tried searching for a compatible plugin.");
  await page.getByRole("button", { name: "Send report", exact: true }).click();
  expect(submitted).toMatchObject({
    server_id: serverID,
    request_reference: reference,
    source_page: `/servers/${serverID}/plugins`,
  });
  expect(JSON.stringify(submitted)).not.toContain("private-query-value");
  expect(submitted).not.toHaveProperty("logs");
});

test("full backup allowance offers existing snapshots without silently deleting them", async ({
  page,
}) => {
  await mockAPI(page);
  await page.route(`http://localhost:8080/servers/${serverID}/backups`, (route) =>
    route.fulfill({
      json: [
        {
          uuid: "40000000-0000-4000-8000-000000000001",
          name: "Completed copy",
          is_successful: true,
          completed_at: "2026-10-13T11:00:00Z",
        },
        {
          uuid: "40000000-0000-4000-8000-000000000002",
          name: "Pending copy",
          is_successful: false,
          completed_at: null,
        },
      ],
    }),
  );
  await login(page);
  await page.goto(`/servers/${serverID}/plugins`);
  await page.getByRole("button", { name: "Review install" }).click();
  const dialog = page.getByRole("dialog");
  await expect(dialog.getByRole("button", { name: "Create backup first" })).toBeDisabled();
  await expect(dialog.getByText("All 2 backup slots are used.", { exact: false })).toBeVisible();
  await expect(dialog.getByRole("button", { name: "Install now", exact: true })).toBeEnabled();
});

test("known backup preflight failures retry with a new key while uncertain writes retain their key", async ({
  page,
}) => {
  await mockAPI(page);
  const keys = [];
  let code = "BACKUP_CHECK_FAILED";
  await page.route(
    `http://localhost:8080/servers/${serverID}/plugins/catalog/viaversion`,
    (route) => {
      if (route.request().method() === "GET")
        return route.fulfill({
          json: {
            project: "viaversion",
            software: "PAPER",
            minecraft_version: "1.21.1",
            revision: "c".repeat(64),
            files: [{ version_id: "stable", filename: "ViaVersion.jar", size: 1 }],
          },
        });
      keys.push(route.request().headers()["idempotency-key"]);
      return route.fulfill({
        status: 502,
        json: {
          error: code
            ? "Backup availability could not be checked"
            : "Operation outcome could not be confirmed",
          code,
        },
      });
    },
  );
  await login(page);
  await page.goto(`/servers/${serverID}/plugins`);
  await page.getByRole("button", { name: "Review install" }).click();
  const install = page
    .getByRole("dialog")
    .getByRole("button", { name: "Install now", exact: true });
  await install.click();
  await expect(page.getByRole("dialog").getByRole("alert")).toContainText("Backup availability");
  code = null;
  await install.click();
  await expect(page.getByRole("dialog").getByRole("alert")).toContainText("Operation outcome");
  expect(keys[1]).not.toBe(keys[0]);
  await install.click();
  await expect.poll(() => keys.length).toBe(3);
  expect(keys[2]).toBe(keys[1]);
});

test("server activity and bounded storage show safe results on mobile", async ({
  page,
}, testInfo) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await mockAPI(page);
  await page.route(`http://localhost:8080/servers/${serverID}/activity`, (route) =>
    route.fulfill({
      json: [
        {
          id: "event",
          actor: "Alex",
          title: "Stop requested",
          outcome: "SUCCEEDED",
          created_at: "2026-10-13T12:00:00Z",
        },
      ],
    }),
  );
  await page.route(`http://localhost:8080/servers/${serverID}/storage`, (route) =>
    route.fulfill({
      json: {
        used_bytes: 950,
        limit_bytes: 1000,
        backup_bytes: 50,
        backup_count: 1,
        backup_limit: 2,
        categories: [
          { name: "Worlds", bytes: 900 },
          { name: "Addons", bytes: 50 },
        ],
        partial: true,
      },
    }),
  );
  await login(page);
  await page.goto(`/servers/${serverID}`);
  await page.getByRole("button", { name: "Health", exact: true }).click();
  await expect(page.getByText("Stop requested", { exact: true })).toBeVisible();
  await expect(page.getByText("Storage is nearly full.", { exact: false })).toBeVisible();
  await expect(page.getByText("This scan is partial:", { exact: false })).toBeVisible();
  await page
    .locator(".server-insights-grid")
    .screenshot({ path: testInfo.outputPath("server-insights-mobile.png") });
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
});

test("notifications mark read through authenticated API and disappear", async ({ page }) => {
  await mockAPI(page);
  let read = false;
  const notificationID = "60000000-0000-4000-8000-000000000001";
  await page.route("http://localhost:8080/notifications", (route) =>
    route.fulfill({
      json: read
        ? []
        : [
            {
              id: notificationID,
              server_id: serverID,
              server_name: baseServer.name,
              title: "Your server is ready",
              created_at: "2026-10-13T12:00:00Z",
            },
          ],
    }),
  );
  await page.route(`http://localhost:8080/notifications/${notificationID}/read`, (route) => {
    expect(route.request().method()).toBe("POST");
    read = true;
    return route.fulfill({ json: { read: true } });
  });
  await login(page);
  await page.goto("/dashboard");
  await expect(page.getByRole("heading", { name: "Notifications", exact: true })).toBeVisible();
  await expect(page.getByText("Your server is ready", { exact: true })).toBeVisible();
  await page.getByRole("button", { name: "Mark read" }).click();
  await expect(page.getByRole("region", { name: "Notifications", exact: true })).toHaveCount(0);
});

test("addon update review names replaced files and requires backup choice", async ({ page }) => {
  await mockAPI(page);
  await page.route(`http://localhost:8080/servers/${serverID}/plugins/catalog/updates`, (route) =>
    route.fulfill({
      json: [
        {
          project: "update-project",
          title: "Test addon",
          current: "1.0",
          latest: "2.0",
          status: "available",
        },
      ],
    }),
  );
  await page.route(
    `http://localhost:8080/servers/${serverID}/plugins/catalog/update-project`,
    (route) =>
      route.fulfill({
        json: {
          project: "update-project",
          software: "PAPER",
          minecraft_version: "1.21.1",
          revision: "a".repeat(64),
          files: [
            { version_id: "v2", filename: "Test-2.jar", size: 100, client_side: "unsupported" },
          ],
          replaces: [{ filename: "Test-1.jar", name: "1.0" }],
        },
      }),
  );
  await page.route(`http://localhost:8080/servers/${serverID}/backups`, (route) =>
    route.fulfill({ json: [] }),
  );
  await login(page);
  await page.goto(`/servers/${serverID}/plugins`);
  await page.getByRole("button", { name: "Check addon updates" }).click();
  await page.getByRole("button", { name: "Review update" }).click();
  const dialog = page.getByRole("dialog", { name: "Review addon update" });
  await expect(dialog.getByText("Test-1.jar · 1.0")).toBeVisible();
  await expect(dialog.getByRole("button", { name: "Update now" })).toBeDisabled();
});

test("player checklist download contains requirements and no automatic install claim", async ({
  page,
}) => {
  await mockAPI(page);
  await login(page);
  await page.goto(`/servers/${serverID}`);
  await page.getByRole("button", { name: "Join", exact: true }).click();
  const downloadPromise = page.waitForEvent("download");
  await page.getByRole("button", { name: "Download player setup checklist" }).click();
  const download = await downloadPromise;
  expect(download.suggestedFilename()).toBe("minecraft-player-setup.txt");
  await expect(page.locator(".join-server details")).toHaveCount(0);
});

test("new server selector hides legacy loaders even in stale options", async ({ page }) => {
  await mockAPI(page);
  await login(page);
  await page.goto("/servers/new");
  await reviewCreation(page);
  await page.getByText("Advanced settings", { exact: true }).click();
  await page.getByLabel("Server software").click();
  await expect(page.getByRole("option")).toHaveCount(3);
  for (const kind of ["SPIGOT", "PURPUR", "VANILLA", "NEOFORGE"])
    await expect(page.locator(`[role="option"][data-value="${kind}"]`)).toHaveCount(0);
});

test("dashboard polling respects Retry-After after rate limiting", async ({ page }) => {
  await mockAPI(page);
  await login(page);
  await page.clock.install();
  let requests = 0;
  await page.route(
    `http://localhost:8080/servers/${serverID}/addons/simple-voice-chat/network`,
    (route) => {
      requests++;
      return route.fulfill({
        status: 429,
        headers: { "Retry-After": "60" },
        json: { error: "too many requests" },
      });
    },
  );
  await page.goto(`/servers/${serverID}/addons/simple-voice-chat`);
  await expect(page.getByText("too many requests", { exact: true })).toBeVisible();
  expect(requests).toBe(1);
  await page.clock.fastForward(10000);
  expect(requests).toBe(1);
  await page.evaluate(() => {
    Object.defineProperty(document, "visibilityState", { configurable: true, value: "hidden" });
    document.dispatchEvent(new Event("visibilitychange"));
    Object.defineProperty(document, "visibilityState", { configurable: true, value: "visible" });
    document.dispatchEvent(new Event("visibilitychange"));
  });
  await page.clock.fastForward(1);
  expect(requests).toBe(1);
  await page.clock.fastForward(50001);
  await expect.poll(() => requests).toBe(2);
});

const playerSlug = "A".repeat(43);
const playerSetup = {
  name: "The Lantern Lodge",
  address: "play.example.com:25565",
  minecraft_version: "1.21.1",
  loader: "fabric",
  loader_version: "0.19.5",
  verified_login: false,
  whitelist: false,
  addons: [{ title: "Simple Voice Chat", project: "9eGKb6K1", required: true }],
  pack_ready: true,
  notes: [
    "VeinMiner: Modrinth does not specify a client requirement. It stays installed on the server and is not added to the player download.",
  ],
  published_at: "2026-10-13T12:00:00Z",
};

for (const width of [390, 1366])
  test(`guest player setup offers download import play at ${width}px`, async ({
    page,
  }, testInfo) => {
    await page.setViewportSize({ width, height: 900 });
    const calls = await mockAPI(page);
    await page.route(`http://localhost:8080/join/${playerSlug}`, (route) =>
      route.fulfill({ json: playerSetup }),
    );
    await page.route(`http://localhost:8080/join/${playerSlug}/pack`, (route) =>
      route.fulfill({
        body: Buffer.from("test download"),
        contentType: "application/x-modrinth-modpack+zip",
      }),
    );
    await page.goto(`/join/${playerSlug}`);
    await page.getByText("What will be installed?", { exact: true }).click();
    await expect(page.getByText(playerSetup.notes[0], { exact: true })).toBeVisible();
    await page
      .getByRole("dialog", { name: "Player requirements", exact: true })
      .getByRole("button", { name: "Close", exact: true })
      .click();
    await expect(page.getByRole("heading", { name: "Join The Lantern Lodge" })).toBeVisible();
    await expect(page.getByRole("heading", { name: "Import and launch" })).toBeVisible();
    const downloadPromise = page.waitForEvent("download");
    await page.getByRole("button", { name: "Download player pack" }).click();
    const download = await downloadPromise;
    expect(download.suggestedFilename()).toBe("blockyard-player-setup.mrpack");
    await expect(page.getByText("Downloaded. Import it in your launcher next.")).toBeVisible();
    expect(calls.some((call) => call.path.startsWith("/servers"))).toBe(false);
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
      true,
    );
    await page.screenshot({
      path: testInfo.outputPath(`player-join-${width}.png`),
      fullPage: true,
    });
  });

test("owner publishes copies and revokes a player invite", async ({ page }) => {
  await page.context().grantPermissions(["clipboard-read", "clipboard-write"]);
  await mockAPI(page);
  let invite = null;
  let publishedOptions = null;
  await page.route(`http://localhost:8080/servers/${serverID}/player-invite`, (route) => {
    const method = route.request().method();
    if (method === "POST") {
      publishedOptions = route.request().postDataJSON();
      invite = {
        slug: playerSlug,
        expires_at: "2026-11-13T12:00:00Z",
        setup: { ...playerSetup, include_beta: publishedOptions.include_beta },
      };
    }
    if (method === "DELETE") invite = null;
    return route.fulfill({ json: method === "DELETE" ? { revoked: true } : invite });
  });
  await openServer(page);
  const panel = page.getByRole("region", { name: "Invite your friends" });
  await panel.getByRole("button", { name: "Download options", exact: true }).click();
  await expect(panel.getByLabel("Include optional client features")).not.toBeChecked();
  await expect(panel.getByLabel("Include beta client builds")).not.toBeChecked();

  await panel.getByLabel("Include beta client builds").check();
  await page
    .getByRole("dialog")
    .getByRole("button", { name: "Update invitation", exact: true })
    .click();
  await expect
    .poll(() => publishedOptions)
    .toEqual({ include_beta: true, include_optional: false });
  await expect(panel.getByText("Player download ready")).toBeVisible();
  await panel.getByRole("button", { name: "Copy invite link" }).click();
  await expect
    .poll(() => page.evaluate(() => navigator.clipboard.readText()))
    .toContain(`/join/${playerSlug}`);
  await panel.getByRole("button", { name: "Download options", exact: true }).click();
  await panel.getByRole("button", { name: "Disable invite link" }).click();
  await expect(panel.getByRole("button", { name: "Prepare invite link" })).toBeVisible();
});

test("an unverified guest setup never offers an automatic download", async ({ page }) => {
  await mockAPI(page);
  await page.route(`http://localhost:8080/join/${playerSlug}`, (route) =>
    route.fulfill({
      json: {
        ...playerSetup,
        pack_ready: false,
        pack_reason: "Your host needs to review an unverified addon.",
      },
    }),
  );
  await page.goto(`/join/${playerSlug}`);
  await expect(page.getByText("Your host needs to review an unverified addon.")).toBeVisible();
  await expect(page.getByRole("button", { name: "Download player pack" })).toHaveCount(0);
});

test("revoked guest invite does not require dashboard sign in", async ({ page }) => {
  await mockAPI(page);
  await page.route(`http://localhost:8080/join/${playerSlug}`, (route) =>
    route.fulfill({ status: 404, json: { error: "resource not found" } }),
  );
  await page.goto(`/join/${playerSlug}`);
  await expect(page.getByRole("heading", { name: "This invite is unavailable" })).toBeVisible();
  expect(page.url()).toContain(`/join/${playerSlug}`);
});

for (const width of [390, 1366])
  test(`SKlauncher 4 offers one pack with manual fallback at ${width}px`, async ({ page }) => {
    await page.setViewportSize({ width, height: 900 });
    const calls = await mockAPI(page);
    let fileReads = 0;
    await page.route(`http://localhost:8080/join/${playerSlug}`, (route) =>
      route.fulfill({ json: playerSetup }),
    );
    await page.route(
      `http://localhost:8080/join/${playerSlug}/files`,
      (route) => (
        fileReads++,
        route.fulfill({
          json: [
            {
              path: "mods/voicechat-fabric.jar",
              downloads: ["https://cdn.modrinth.com/voicechat.jar"],
              hashes: { sha1: "b".repeat(40), sha512: "a".repeat(128) },
              fileSize: 1048576,
            },
          ],
        })
      ),
    );
    await page.route(`http://localhost:8080/join/${playerSlug}/pack`, (route) =>
      route.fulfill({
        body: Buffer.from("test pack"),
        contentType: "application/x-modrinth-modpack+zip",
      }),
    );
    await page.route(`http://localhost:8080/join/${playerSlug}/bundle`, (route) =>
      route.fulfill({ body: Buffer.from("test zip"), contentType: "application/zip" }),
    );
    await page.goto(`/join/${playerSlug}`);
    const selector = page.getByRole("combobox", { name: "Your Minecraft launcher" });
    await selectValue(page, selector, "sklauncher");
    await expect(page.getByRole("heading", { name: "Import into SKlauncher 4" })).toBeVisible();
    await expect(page.getByRole("button", { name: "Download client mods ZIP" })).toHaveCount(0);
    expect(fileReads).toBe(0);
    const packDownload = page.waitForEvent("download");
    await page.getByRole("button", { name: "Download player pack" }).click();
    expect((await packDownload).suggestedFilename()).toBe("blockyard-player-setup.mrpack");
    await page.getByText("Import failed? Use the manual ZIP", { exact: true }).click();
    await page.getByText("Individual downloads", { exact: true }).click();
    await expect(
      page.getByRole("link", { name: "Download voicechat-fabric.jar ↗" }),
    ).toHaveAttribute("href", "https://cdn.modrinth.com/voicechat.jar");
    await expect(page.getByText("Fabric 0.19.5", { exact: true })).toBeVisible();
    await expect(
      page.getByText("Modrinth App requires a Microsoft account that owns Minecraft Java Edition."),
    ).toHaveCount(0);
    const downloadPromise = page.waitForEvent("download");
    await page.getByRole("button", { name: "Download client mods ZIP" }).click();
    expect((await downloadPromise).suggestedFilename()).toBe("blockyard-client-mods.zip");
    await expect(
      page.getByText(
        "ZIP downloaded. Extract it and copy the mods folder into your installation.",
        { exact: true },
      ),
    ).toBeVisible();
    await page.reload();
    await expect(page.getByRole("heading", { name: "Import into SKlauncher 4" })).toBeVisible();
    await selectValue(page, selector, "prism");
    await expect(page.getByRole("heading", { name: "Import into Prism" })).toBeVisible();
    await expect(page.getByRole("link", { name: "Download voicechat-fabric.jar ↗" })).toHaveCount(
      0,
    );
    await selectValue(page, selector, "official");
    await expect(page.getByRole("heading", { name: "Set up your installation" })).toBeVisible();
    expect(calls.some((call) => call.path.startsWith("/servers"))).toBe(false);
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
      true,
    );
  });

test("owner invite polling reflects changed server setup instead of a cached published result", async ({
  page,
}) => {
  await page.clock.install();
  await mockAPI(page);
  let current = { slug: playerSlug, expires_at: "2077-11-13T12:00:00Z", setup: playerSetup };
  await page.route(`http://localhost:8080/servers/${serverID}/player-invite`, (route) =>
    route.fulfill({ json: current }),
  );
  await openServer(page);
  const panel = page.getByRole("region", { name: "Invite your friends" });
  await expect(panel.getByRole("button", { name: "Copy invite link" })).toHaveClass(/primary/);
  await expect(panel.getByLabel("Include optional client features")).toHaveCount(0);
  await panel.getByRole("button", { name: "Refresh player setup" }).click();
  await expect(panel.getByText("Player download ready", { exact: true })).toBeVisible();
  current = {
    ...current,
    setup: {
      ...playerSetup,
      pack_ready: false,
      pack_reason: "The server setup changed. Ask your host to refresh the player setup.",
    },
  };
  await page.clock.fastForward(31000);
  await expect(panel.getByText("Player setup needs a check", { exact: true })).toBeVisible();
  await expect(panel.getByRole("button", { name: "Refresh player setup" })).toHaveClass(/primary/);
  await expect(panel.getByRole("button", { name: "Copy invite link" })).toHaveClass(/secondary/);
  await expect(panel.getByRole("button", { name: "Copy invite link" })).toBeDisabled();
});
test("expired owner invitation offers refresh and never copies its expired link", async ({
  page,
}) => {
  await mockAPI(page);
  await page.route(`http://localhost:8080/servers/${serverID}/player-invite`, (route) =>
    route.fulfill({
      json: { slug: playerSlug, expires_at: "2000-01-01T00:00:00Z", setup: playerSetup },
    }),
  );
  await openServer(page);
  const panel = page.getByRole("region", { name: "Invite your friends" });
  await expect(panel.getByText("Invite link expired", { exact: true })).toBeVisible();
  await expect(panel.getByRole("button", { name: "Copy invite link" })).toHaveCount(0);
  await expect(panel.getByRole("button", { name: "Refresh player setup" })).toBeEnabled();
});

test("failed and unfinished backups never imply a usable recovery copy", async ({ page }) => {
  await page.clock.install();
  await mockAPI(page);
  let pending = false;
  await page.route(`http://localhost:8080/servers/${serverID}/backups`, (route) =>
    route.fulfill({
      json: [
        ...(pending
          ? [
              {
                uuid: "pending",
                name: "New copy",
                is_successful: false,
                completed_at: null,
                created_at: "2026-10-14T10:00:00Z",
                bytes: 0,
              },
            ]
          : []),
        {
          uuid: "failed",
          name: "Failed copy",
          is_successful: false,
          created_at: "2026-10-13T10:00:00Z",
          completed_at: "2026-10-13T10:01:00Z",
          bytes: 0,
        },
      ],
    }),
  );
  await openServer(page);
  await page.goto(`/servers/${serverID}/backups`);
  const protection = page.getByRole("region", { name: "Backup protection" });
  await expect(
    protection.getByRole("heading", { name: "No usable recovery copy yet" }),
  ).toBeVisible();
  await expect(page.getByRole("button", { name: "Restore", exact: true })).toBeDisabled();
  pending = true;
  await page.clock.fastForward(11000);
  await expect(
    protection.getByRole("heading", { name: "Your first recovery copy is being created" }),
  ).toBeVisible();
  await expect(page.getByRole("button", { name: "Restore", exact: true }).first()).toBeDisabled();
  await expect(page.getByRole("button", { name: "Restore", exact: true }).last()).toBeDisabled();
});

test("restore confirmation stops when the server starts while the dialog is open", async ({
  page,
}) => {
  await page.clock.install();
  const calls = await mockAPI(page);
  let power = "offline";
  await page.route(`http://localhost:8080/servers/${serverID}/usage`, (route) =>
    route.fulfill({ json: { power_state: power } }),
  );
  await openServer(page);
  await page.goto(`/servers/${serverID}/backups`);
  await page.getByRole("button", { name: "Restore", exact: true }).click();
  const dialog = page.getByRole("dialog");
  await dialog.getByLabel("I understand that the current server files will be replaced.").check();
  await expect(dialog.getByRole("button", { name: "Restore backup", exact: true })).toBeEnabled();
  power = "running";
  await page.clock.fastForward(11000);
  await expect(dialog.getByRole("button", { name: "Restore backup", exact: true })).toBeDisabled();
  expect(calls.filter((call) => call.path.endsWith("/restore"))).toHaveLength(0);
});

test("backup shortcuts choose the newest usable snapshot rather than a failed or slow older copy", async ({
  page,
}) => {
  await page.clock.install({ time: new Date("2026-10-14T12:00:00Z") });
  const calls = await mockAPI(page);
  const newest = "40000000-0000-4000-8000-000000000002";
  await page.route(`http://localhost:8080/servers/${serverID}/backups`, (route) =>
    route.fulfill({
      json: [
        {
          uuid: "40000000-0000-4000-8000-000000000001",
          name: "Old slow backup",
          is_successful: true,
          created_at: "2026-10-07T10:00:00Z",
          completed_at: "2026-10-14T11:00:00Z",
          bytes: 100,
        },
        {
          uuid: newest,
          name: "Newest world copy",
          is_successful: true,
          created_at: "2026-10-11T10:00:00Z",
          completed_at: "2026-10-11T10:01:00Z",
          bytes: 100,
        },
        {
          uuid: "failed",
          name: "New failed copy",
          is_successful: false,
          created_at: "2026-10-14T10:00:00Z",
          completed_at: "2026-10-14T10:01:00Z",
          bytes: 0,
        },
      ],
    }),
  );
  await openServer(page);
  await page.goto(`/servers/${serverID}/backups`);
  const summary = page.getByRole("region", { name: "Backup protection" });
  await expect(summary).toContainText("Your newest usable copy is “Newest world copy”");
  await expect(summary).toContainText("over two days old");
  await summary.getByRole("button", { name: "Restore latest copy" }).click();
  const dialog = page.getByRole("dialog");
  await expect(dialog).toContainText("Newest world copy");
  await expect(dialog.getByRole("button", { name: "Restore backup", exact: true })).toBeDisabled();
  expect(calls.filter((call) => call.path.endsWith("/restore"))).toHaveLength(0);
});

test("server health explains pressure without claiming measured lag or player ping", async ({
  page,
}) => {
  await mockAPI(page, { initiallyRunning: true });
  await page.route(`http://localhost:8080/servers/${serverID}/usage`, (route) =>
    route.fulfill({
      json: {
        power_state: "running",
        cpu_percent: 10,
        memory_bytes: 100,
        disk_bytes: 20 * 1024 * 1024 * 1024,
      },
    }),
  );
  await openServer(page);
  await page.getByRole("button", { name: "Health", exact: true }).click();
  const card = page.getByRole("region", { name: "Server health" });
  await expect(card.getByRole("heading", { name: "Storage is nearly full" })).toBeVisible();
  await expect(card).toContainText("Tick rate and player ping aren’t measured here.");
});
test("health support links attach safe dashboard state and check time to a reviewed report", async ({
  page,
}) => {
  await mockAPI(page);
  let submitted;
  await page.route("http://localhost:8080/feedback", (route) => {
    submitted = route.request().postDataJSON();
    return route.fulfill({ status: 201, json: { ...feedbackFixture, id: orderID } });
  });
  await openServer(page);
  await page.getByRole("button", { name: "Health", exact: true }).click();
  await page
    .getByRole("region", { name: "Server health" })
    .getByRole("link", { name: "Report a problem →" })
    .click();
  await expect(page.getByText("Attached dashboard state:", { exact: false })).toBeVisible();
  await page.getByLabel("Short summary").fill("Minecraft did not start");
  await page.getByLabel("What happened?").fill("I pressed Start and the world stayed offline.");
  await page.getByRole("button", { name: "Send report", exact: true }).click();
  expect(submitted.server_id).toBe(serverID);
  expect(submitted.message).toContain("Displayed state: offline");
  expect(submitted.message).toContain("Startup check time:");
  expect(submitted).not.toHaveProperty("logs");
});

test("unavailable live readings never claim online and support uses the failing request reference", async ({
  page,
}) => {
  await mockAPI(page, { initiallyRunning: true });
  const reference = "066aa59b-590d-4296-951c-81d70922d4f9";
  await page.route(`http://localhost:8080/servers/${serverID}/usage`, (route) =>
    route.fulfill({
      status: 502,
      headers: { "X-Request-ID": reference },
      json: { error: "Live readings unavailable", request_id: reference },
    }),
  );
  await openServer(page);
  await page.getByRole("button", { name: "Health", exact: true }).click();
  const card = page.getByRole("region", { name: "Server health" });
  await expect(card.getByRole("heading", { name: "Live status unavailable" })).toBeVisible();
  await expect(card.getByRole("link", { name: "Report a problem →" })).toHaveAttribute(
    "href",
    new RegExp(`reference=${reference}`),
  );
});
test("health report context rejects arbitrary query values", async ({ page }) => {
  await mockAPI(page);
  let submitted;
  await page.route("http://localhost:8080/feedback", (route) => {
    submitted = route.request().postDataJSON();
    return route.fulfill({ status: 201, json: { ...feedbackFixture, id: orderID } });
  });
  await login(page);
  await page.goto(
    `/feedback?server=${serverID}&observed_state=private-health-value&checked_at=private-time-value`,
  );
  await page.getByLabel("Short summary").fill("Check dashboard state");
  await page.getByLabel("What happened?").fill("The live dashboard reading did not load.");
  await page.getByRole("button", { name: "Send report", exact: true }).click();
  expect(submitted.message).toBe("The live dashboard reading did not load.");
  expect(JSON.stringify(submitted)).not.toContain("private-health-value");
  expect(JSON.stringify(submitted)).not.toContain("private-time-value");
});

test("gameplay choices review edits and preserve joining rules before saving", async ({ page }) => {
  const calls = await mockAPI(page);
  await openServer(page);
  await page.getByRole("link", { name: "Settings", exact: true }).click();
  await expect(page.getByRole("button", { name: "Save settings", exact: true })).toBeDisabled();
  await page.getByRole("button", { name: /Creative building/ }).click();
  const review = page.getByRole("region", { name: "Review settings changes" });
  await expect(review).toContainText("survival → creative");
  await expect(page.getByRole("button", { name: "Reload settings", exact: true })).toBeDisabled();
  expect(calls.some((call) => call.method === "PUT" && call.path.endsWith("/settings"))).toBe(
    false,
  );
  await page.getByRole("button", { name: "Discard changes", exact: true }).click();
  await expect(review).toContainText("No unsaved changes");
  await expect(page.getByRole("button", { name: "Reload settings", exact: true })).toBeEnabled();
  await page.getByRole("button", { name: /Creative building/ }).click();
  await page.getByRole("button", { name: "Save settings", exact: true }).click();
  await expect(page.getByRole("status")).toContainText("Settings saved");
  const saved = calls.find((call) => call.method === "PUT" && call.path.endsWith("/settings")).body;
  expect(saved).toMatchObject({
    gamemode: "creative",
    difficulty: "peaceful",
    hardcore: false,
    pvp: false,
    online_mode: true,
    whitelist: false,
    max_players: 10,
    revision: "a".repeat(64),
  });
  expect(saved).not.toHaveProperty("cpu_limit");
});

test("game settings keep offline login defaults and block saves while running", async ({
  page,
}) => {
  const calls = await mockAPI(page, { initiallyRunning: true });
  await page.route(`http://localhost:8080/servers/${serverID}/settings`, async (route) => {
    if (route.request().method() !== "GET") return route.fallback();
    await route.fulfill({
      json: {
        motd: "Welcome",
        difficulty: "normal",
        gamemode: "survival",
        hardcore: false,
        pvp: true,
        max_players: 10,
        whitelist: false,
        online_mode: false,
        view_distance: 10,
        spawn_protection: 16,
        revision: "a".repeat(64),
      },
    });
  });
  await openServer(page);
  await page.getByRole("link", { name: "Settings", exact: true }).click();
  await page.getByRole("button", { name: /Peaceful survival/ }).click();
  await expect(page.getByRole("checkbox", { name: /Verify Minecraft accounts/ })).not.toBeChecked();
  await expect(page.getByRole("checkbox", { name: /^Whitelist/ })).not.toBeChecked();
  await expect(page.getByRole("button", { name: "Save settings", exact: true })).toBeDisabled();
  await expect(page.getByRole("region", { name: "Review settings changes" })).toContainText(
    "Stop your server before saving",
  );
  expect(calls.some((call) => call.path.endsWith("/settings") && call.method === "PUT")).toBe(
    false,
  );
});

test("addon installation explains compatibility and next steps without starting Minecraft", async ({
  page,
}) => {
  const calls = await mockAPI(page);
  await openServer(page);
  await page.getByRole("link", { name: "Plugins", exact: true }).click();
  await expect(page.getByText("Matches your server", { exact: true })).toBeVisible();
  await page.getByRole("button", { name: "Review install" }).click();
  await expect(page.getByRole("dialog")).toContainText("1 file to install");
  await expect(page.getByRole("dialog")).toContainText("Client downloads are prepared separately");
  await page.getByRole("button", { name: "Install now", exact: true }).click();
  const steps = page.getByRole("region", { name: "Next steps after addon changes" });
  await expect(steps).toContainText("Addon files installed");
  await expect(steps.getByRole("link", { name: "Refresh player invitation" })).toHaveAttribute(
    "href",
    `/servers/${serverID}#invite-friends`,
  );
  expect(
    calls.some(
      (call) => call.method === "POST" && /\/(start|restart|player-invite)$/.test(call.path),
    ),
  ).toBe(false);
});

test("failed addon review offers a read-only retry and does not install files", async ({
  page,
}) => {
  const calls = await mockAPI(page);
  let reviews = 0;
  await page.route(
    `http://localhost:8080/servers/${serverID}/plugins/catalog/viaversion`,
    async (route) => {
      if (route.request().method() !== "GET") return route.fallback();
      reviews++;
      if (reviews === 1)
        return route.fulfill({ status: 503, json: { error: "Package unavailable" } });
      return route.fallback();
    },
  );
  await openServer(page);
  await page.getByRole("link", { name: "Plugins", exact: true }).click();
  await page.getByRole("button", { name: "Review install" }).click();
  await expect(page.getByLabel("Addon failure help")).toContainText(
    "This review has not installed anything",
  );
  await expect(page.getByRole("region", { name: "Next steps after addon changes" })).toHaveCount(0);
  await page.getByRole("button", { name: "Retry addon review" }).click();
  await expect(page.getByRole("dialog")).toContainText("ViaVersion.jar");
  expect(reviews).toBe(2);
  expect(calls.some((call) => call.method === "POST" && call.path.includes("/catalog/"))).toBe(
    false,
  );
});

test("interrupted addon installation keeps the request key and prompts inspection", async ({
  page,
}) => {
  await mockAPI(page);
  const attempts = [];
  await page.route(
    `http://localhost:8080/servers/${serverID}/plugins/catalog/viaversion`,
    async (route) => {
      if (route.request().method() !== "POST") return route.fallback();
      attempts.push(route.request().headers()["idempotency-key"]);
      if (attempts.length === 1)
        return route.fulfill({ status: 503, json: { error: "Installation outcome unknown" } });
      return route.fulfill({ json: { status: "installed", restart_required: true } });
    },
  );
  await openServer(page);
  await page.getByRole("link", { name: "Plugins", exact: true }).click();
  await page.getByRole("button", { name: "Review install" }).click();
  await page.getByRole("button", { name: "Install now", exact: true }).click();
  await expect(page.getByLabel("Addon failure help")).toContainText(
    "Some files may have been copied",
  );
  await expect(page.getByRole("region", { name: "Next steps after addon changes" })).toHaveCount(0);
  await page.getByRole("button", { name: "Install now", exact: true }).click();
  await expect(page.getByRole("region", { name: "Next steps after addon changes" })).toBeVisible();
  expect(attempts).toHaveLength(2);
  expect(attempts[0]).toBeTruthy();
  expect(attempts[1]).toBe(attempts[0]);
});

test("failed creation preserves choices and safely retries the same order", async ({ page }) => {
  await mockAPI(page);
  const requests = [];
  await page.route("http://localhost:8080/orders", async (route) => {
    if (route.request().method() !== "POST") return route.fallback();
    requests.push({
      key: route.request().headers()["idempotency-key"],
      body: route.request().postDataJSON(),
    });
    if (requests.length === 1)
      return route.fulfill({ status: 503, json: { error: "Checkout temporarily unavailable" } });
    return route.fallback();
  });
  await login(page);
  await page.goto("/servers/new");
  await expect(page.getByRole("navigation", { name: "Server creation steps" })).toContainText(
    "Name & create",
  );
  await reviewCreation(page);
  await selectValue(page, page.getByLabel("Server software"), "FORGE");
  await reviewCreation(page);
  await page.getByLabel("Server name", { exact: true }).fill("Friends' world");
  await page.getByRole("button", { name: "Continue to checkout" }).click();
  await expect(page.getByRole("status")).toContainText("Your choices are saved here");
  await expect(page.getByLabel("Server name", { exact: true })).toHaveValue("Friends' world");
  await expect(page.getByLabel("Server software")).toHaveAttribute("data-value", "FORGE");
  await expect(page.getByLabel("Minecraft version")).toHaveAttribute("data-value", "1.21.1");
  await page.getByRole("button", { name: "Continue to checkout" }).click();
  await expect(page).toHaveURL(/checkout/);
  expect(requests).toHaveLength(2);
  expect(requests[1]).toEqual(requests[0]);
  expect(requests[0].key).toBeTruthy();
});

test("setup progress follows provisioning status and offers recovery without creating another server", async ({
  page,
}) => {
  await page.clock.install();
  const calls = await mockAPI(page, { serverState: "PROVISIONING", isAdmin: true });
  let state = "PROVISIONING";
  await page.route(`http://localhost:8080/servers/${serverID}`, (route) =>
    route.fulfill({
      json: { ...baseServer, status: state, management_available: state === "RUNNING" },
    }),
  );
  await openServer(page);
  const progress = page.getByRole("region", { name: "Server setup progress" });
  await expect(progress).toContainText("Minecraft is installing.");
  await expect(progress.locator('[aria-current="step"]')).toContainText("Install Minecraft");
  await expect(page.getByRole("button", { name: "Start", exact: true })).toBeDisabled();
  state = "FAILED";
  await page.clock.fastForward(11000);
  await expect(progress).toContainText("Your server request and configuration are kept");
  await expect(progress.getByRole("link", { name: "Open setup recovery" })).toHaveAttribute(
    "href",
    "/admin",
  );
  await expect(progress.getByRole("link", { name: "Ask for setup help" })).toHaveAttribute(
    "href",
    `/feedback?server=${serverID}&page=server-setup`,
  );
  state = "RUNNING";
  await page.clock.fastForward(11000);
  await expect(progress).toHaveCount(0);
  await expect(page.getByRole("button", { name: "Start", exact: true })).toBeEnabled();
  expect(
    calls.some(
      (call) =>
        call.method === "POST" &&
        (call.path === "/orders" ||
          call.path === "/admin/beta-servers" ||
          /\/(start|stop|restart|kill|eula)$/.test(call.path)),
    ),
  ).toBe(false);
});

test("players show reported names honestly and review ban kick and unban", async ({ page }) => {
  const calls = await mockAPI(page, { initiallyRunning: true });
  await page.route(`http://localhost:8080/servers/${serverID}/usage`, (route) =>
    route.fulfill({
      json: {
        power_state: "running",
        cpu_percent: 10,
        memory_bytes: 1000,
        disk_bytes: 2000,
        uptime_ms: 1000,
        players_max: 10,
        players_available: true,
        players_online: 3,
        player_names: ["Steve", "Alex"],
      },
    }),
  );
  await openServer(page);
  await page.getByRole("link", { name: "Players", exact: true }).click();
  const live = page.getByRole("region", { name: "Live players" });
  await expect(live).toContainText("3 players online");
  await expect(live).toContainText("not a complete attendance list");
  await expect(live.getByText("Steve", { exact: true })).toBeVisible();
  await expect(live.getByRole("link", { name: "Invite friends" })).toHaveAttribute(
    "href",
    `/servers/${serverID}#invite-friends`,
  );
  const moderation = page.getByRole("region", { name: "Player moderation" });
  await moderation.getByLabel("Player username", { exact: true }).fill("Steve");
  await moderation.getByLabel("Reason", { exact: true }).fill("Please respect other players");
  await selectValue(page, page.getByLabel("Player action"), "ban");
  await moderation.getByRole("button", { name: "Review player action" }).click();
  expect(calls.some((call) => call.path.endsWith("/players/actions"))).toBe(false);
  await page.getByRole("button", { name: "Confirm player action" }).click();
  await expect(moderation.getByRole("status")).toContainText("Steve is banned");
  await moderation.getByRole("button", { name: "Unban Steve" }).click();
  await page.getByRole("button", { name: "Confirm player action" }).click();
  await expect(moderation.getByRole("status")).toContainText("Steve is no longer banned");
  await selectValue(page, page.getByLabel("Player action"), "kick");
  await moderation.getByRole("button", { name: "Review player action" }).click();
  await page.getByRole("button", { name: "Confirm player action" }).click();
  await expect(moderation.getByRole("status")).toContainText("Kick command delivered");
  const writes = calls.filter((call) => call.path.endsWith("/players/actions"));
  expect(writes).toHaveLength(3);
  expect(
    writes.every(
      (call) =>
        call.body.confirmed && call.headers["idempotency-key"] && call.body.revision.length === 64,
    ),
  ).toBe(true);
});

test("unavailable player readings block moderation and help reflects actual login settings", async ({
  page,
}) => {
  const calls = await mockAPI(page, { initiallyRunning: true });
  await page.route(`http://localhost:8080/servers/${serverID}/usage`, (route) =>
    route.fulfill({ status: 503, json: { error: "Live readings unavailable" } }),
  );
  await openServer(page);
  await page.getByRole("link", { name: "Players", exact: true }).click();
  await expect(page.getByRole("region", { name: "Live players" })).toContainText(
    "Live readings unavailable",
  );
  await page.getByLabel("Player username", { exact: true }).fill("Steve");
  await page.getByLabel("Reason", { exact: true }).fill("Rules");
  await expect(page.getByRole("button", { name: "Review player action" })).toBeDisabled();
  await selectValue(page, page.getByLabel("Connection problem"), "login");
  await expect(page.getByRole("region", { name: "Connection help" })).toContainText(
    "Microsoft verification is on",
  );
  expect(calls.some((call) => call.path.endsWith("/players/actions"))).toBe(false);
  expect(calls.some((call) => call.path.endsWith("/settings") && call.method === "PUT")).toBe(
    false,
  );
});

test("moderation retries retain the original body and stop when Minecraft goes offline", async ({
  page,
}) => {
  await page.clock.install();
  await mockAPI(page, { initiallyRunning: true });
  let running = true;
  let revision = "e".repeat(64);
  const attempts = [];
  await page.route(`http://localhost:8080/servers/${serverID}/usage`, (route) =>
    route.fulfill({
      json: {
        power_state: running ? "running" : "offline",
        cpu_percent: 10,
        memory_bytes: 1000,
        disk_bytes: 2000,
        uptime_ms: 1000,
        players_online: 0,
        players_max: 10,
        players_available: true,
      },
    }),
  );
  await page.route(`http://localhost:8080/servers/${serverID}/players/bans`, (route) =>
    route.fulfill({ json: { players: [], revision } }),
  );
  await page.route(`http://localhost:8080/servers/${serverID}/players/actions`, (route) => {
    attempts.push({
      key: route.request().headers()["idempotency-key"],
      body: route.request().postDataJSON(),
    });
    revision = "f".repeat(64);
    return route.fulfill({ status: 502, json: { error: "Outcome could not be confirmed" } });
  });
  await login(page);
  await page.goto(`/servers/${serverID}/players`);
  await page.getByLabel("Player username", { exact: true }).fill("Steve");
  await page.getByLabel("Reason", { exact: true }).fill("Rules");
  await page.getByRole("button", { name: "Review player action" }).click();
  const confirm = page.getByRole("button", { name: "Confirm player action" });
  await confirm.click();
  await expect(page.getByRole("dialog")).toContainText("Outcome could not be confirmed");
  await expect(confirm).toBeEnabled();
  await confirm.click();
  await expect(confirm).toBeEnabled();
  expect(attempts).toHaveLength(2);
  expect(attempts[1]).toEqual(attempts[0]);
  running = false;
  await page.clock.fastForward(6000);
  await expect(confirm).toBeDisabled();
  expect(attempts).toHaveLength(2);
});

for (const width of [320, 768, 1366]) {
  test(`player-first homepage shows database plans and honest beta access at ${width}px`, async ({
    page,
  }, testInfo) => {
    await page.setViewportSize({ width, height: 900 });
    const calls = await mockAPI(page, { inviteRequired: true, paymentDisabled: true });
    await page.goto("/");
    await expect(
      page.getByRole("heading", { name: /Your world.*Your friends.*Just play/ }),
    ).toBeVisible();
    await expect(
      page.getByRole("link", { name: "Join with an invitation" }).first(),
    ).toHaveAttribute("href", "/register");
    await expect(
      page.getByText("Public checkout is not open during this beta.", { exact: false }),
    ).toBeVisible();
    const plans = page.locator("#hosting-plans");
    await expect(plans).toContainText("Campfire");
    await expect(plans).toContainText("$9.75");
    await expect(plans).toContainText("not paid checkout offers");
    await page.getByText("Will mods download when someone joins?", { exact: true }).click();
    await expect(
      page.getByText("Minecraft does not automatically install executable mods on connection.", {
        exact: false,
      }),
    ).toBeVisible();
    expect(
      calls.some(
        (call) =>
          call.path.startsWith("/servers") ||
          call.path.startsWith("/admin") ||
          call.method === "POST",
      ),
    ).toBe(false);
    const image = await page.request.get("/art/world-night.webp");
    expect(image.status()).toBe(200);
    expect(image.headers()["content-type"]).toContain("image/webp");
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
      true,
    );
    await page.screenshot({ path: testInfo.outputPath(`homepage-${width}.png`), fullPage: true });
  });
}

test("homepage fails safely when plans cannot load and dashboard omits hosting totals", async ({
  page,
}) => {
  await mockAPI(page);
  await page.route("http://localhost:8080/plans", (route) =>
    route.fulfill({ status: 503, json: { error: "Unavailable" } }),
  );
  await page.goto("/");
  await expect(
    page.getByRole("alert").filter({ hasText: "Hosting plans could not be loaded" }),
  ).toBeVisible();
  await expect(page.getByRole("link", { name: "Create your world", exact: true })).toHaveCount(0);
  await expect(page.getByRole("link", { name: "Sign in", exact: true }).first()).toHaveAttribute(
    "href",
    "/login",
  );
  await login(page);
  await expect(page.getByRole("region", { name: "Your next adventure" })).toBeVisible();
  await expect(page.getByText("Allocated memory", { exact: true })).not.toBeVisible();
  await expect(page.locator(".dashboard-hosting-details")).toHaveCount(0);
});

test("dashboard shortcuts use authorized server details and dropdowns keep readable night colors", async ({
  page,
}) => {
  const calls = await mockAPI(page);
  const listed = Object.fromEntries(
    Object.entries(baseServer).filter(
      ([key]) => !["permissions", "role", "management_available"].includes(key),
    ),
  );
  await page.route("http://localhost:8080/servers", (route) =>
    route.fulfill({
      json: [
        listed,
        { ...listed, id: "30000000-0000-4000-8000-000000000099", name: "Second world" },
      ],
    }),
  );
  await login(page);
  const quick = page.getByRole("navigation", { name: "Quick world actions" });
  await expect(quick.getByRole("link", { name: /Game rules/ })).toHaveAttribute(
    "href",
    `/servers/${serverID}/settings`,
  );
  await expect(quick.getByRole("link", { name: /Invite friends/ })).toHaveAttribute(
    "href",
    `/servers/${serverID}#invite-friends`,
  );
  expect(calls.some((call) => call.path === `/servers/${serverID}`)).toBe(true);
  await page.getByRole("combobox", { name: "Server view" }).click();
  await expect(page.getByRole("listbox")).toBeVisible();
  const colors = await page.getByRole("listbox").evaluate((element) => ({
    bg: getComputedStyle(element).backgroundColor,
    fg: getComputedStyle(element).color,
  }));
  const luminance = (color) => {
    const rgb = color
      .match(/[\d.]+/g)
      .slice(0, 3)
      .map(Number)
      .map((n) => n / 255)
      .map((n) => (n <= 0.04045 ? n / 12.92 : ((n + 0.055) / 1.055) ** 2.4));
    return rgb[0] * 0.2126 + rgb[1] * 0.7152 + rgb[2] * 0.0722;
  };
  expect((luminance(colors.fg) + 0.05) / (luminance(colors.bg) + 0.05)).toBeGreaterThan(4.5);
  await page.keyboard.press("Escape");
  await quick.getByRole("link", { name: /Game rules/ }).click();
  await expect(page.getByLabel("Help with settings")).toHaveCount(0);
  await expect(
    page.getByRole("heading", { name: "Minecraft settings", exact: true }),
  ).toBeVisible();
});

test("moderator dashboard does not show owner or admin world shortcuts", async ({ page }) => {
  await mockAPI(page, { role: "MODERATOR" });
  await login(page);
  await expect(page.getByRole("navigation", { name: "Quick world actions" })).toHaveCount(0);
});

test("root-domain homepage links account access to app without relative login URLs", async ({
  request,
}) => {
  const response = await request.get("/", { headers: { host: "playblockyard.com" } });
  expect(response.status()).toBe(200);
  const html = await response.text();
  expect(html).toContain('href="https://app.playblockyard.com/dashboard"');
  expect(html).toContain('href="https://app.playblockyard.com/login"');
  expect(html).not.toContain('href="/login"');
});

test("server overview keeps play focused and opens technical views directly", async ({ page }) => {
  const calls = await mockAPI(page, { initiallyRunning: true });
  await openServer(page);
  await expect(page.getByRole("region", { name: "Play on your server" })).toBeVisible();
  await expect(page.getByRole("region", { name: "Invite your friends" })).toBeVisible();
  await expect(page.getByRole("navigation", { name: "Server pages" })).toBeVisible();
  await expect(page.locator("main details:visible")).toHaveCount(0);
  for (const section of ["joining", "health", "maintenance"]) {
    await expect(page.locator(`[data-section="${section}"]`)).not.toBeVisible();
  }
  await expect(page.getByRole("button", { name: "Delete server", exact: true })).not.toBeVisible();
  await expect(page.getByText("CPU usage", { exact: true })).not.toBeVisible();
  await page.getByRole("button", { name: "Health", exact: true }).click();
  await expect(page.getByText("CPU usage", { exact: true })).toBeVisible();
  expect(
    calls.filter((call) => call.method !== "GET" && call.path.startsWith(`/servers/${serverID}`)),
  ).toHaveLength(0);
});

test("public invitation uses the website theme and a dark accessible launcher popup", async ({
  page,
}) => {
  const calls = await mockAPI(page);
  await page.route(`http://localhost:8080/join/${playerSlug}`, (route) =>
    route.fulfill({ json: playerSetup }),
  );
  await page.goto(`/join/${playerSlug}`);
  await expect(page.getByRole("heading", { name: "Join The Lantern Lodge" })).toBeVisible();
  await expect(page.getByRole("link", { name: "blockyard." })).toHaveAttribute(
    "href",
    "https://playblockyard.com",
  );
  const art = await page
    .locator(".player-hero")
    .evaluate((el) => getComputedStyle(el).backgroundImage);
  expect(art).toContain("world-night.webp");
  await page.getByRole("combobox", { name: "Your Minecraft launcher" }).click();
  await expect(page.getByRole("listbox")).toHaveClass(/select-menu-night/);
  expect(calls.some((call) => call.path.startsWith("/servers"))).toBe(false);
});

test("mobile billing records remain readable on the shared dark theme", async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await mockAPI(page);
  await login(page);
  await page.goto("/billing");
  const row = page.locator(".responsive-table tbody tr").first();
  await expect(row).toBeVisible();
  const contrast = await row
    .locator("td")
    .first()
    .evaluate((element) => {
      const luminance = (color) => {
        const rgb = color
          .match(/[\d.]+/g)
          .slice(0, 3)
          .map(Number)
          .map((n) => n / 255)
          .map((n) => (n <= 0.04045 ? n / 12.92 : ((n + 0.055) / 1.055) ** 2.4));
        return rgb[0] * 0.2126 + rgb[1] * 0.7152 + rgb[2] * 0.0722;
      };
      const fg = luminance(getComputedStyle(element).color);
      const bg = luminance(getComputedStyle(element.closest("tr")).backgroundColor);
      return (Math.max(fg, bg) + 0.05) / (Math.min(fg, bg) + 0.05);
    });
  expect(contrast).toBeGreaterThan(4.5);
});

test("missing pages offer a themed route back to the dashboard", async ({ page }) => {
  await mockAPI(page);
  const response = await page.goto("/this-page-is-missing");
  expect(response.status()).toBe(404);
  await expect(
    page.getByRole("heading", { name: "This path doesn’t lead to a world." }),
  ).toBeVisible();
  await expect(page.getByRole("link", { name: "Back to your worlds" })).toHaveAttribute(
    "href",
    "/dashboard",
  );
});

for (const [name, changes, kind] of [
  ["offline", { powerState: "offline" }, "start"],
  ["online owner", { powerState: "running" }, "invite"],
  ["starting", { powerState: "starting" }, "wait"],
  ["stopping", { powerState: "stopping" }, "wait"],
  ["agreement required", { eula: false }, "agreement"],
  ["agreement not checked", { eula: undefined }, "wait"],
  [
    "usage failed with stale online state",
    { powerState: "running", usageError: new Error("unavailable") },
    "support",
  ],
  ["agreement check failed", { eulaError: new Error("unavailable") }, "support"],
  ["controls unavailable", { ready: false }, "support"],
  ["provisioning", { server: { ...baseServer, status: "PROVISIONING" }, ready: false }, "wait"],
  ["failed setup", { server: { ...baseServer, status: "FAILED" }, ready: false }, "support"],
  ["suspended", { server: { ...baseServer, status: "SUSPENDED" }, ready: false }, "support"],
  ["checkout", { server: { ...baseServer, status: "PENDING_PAYMENT" }, ready: false }, "billing"],
  [
    "deletion pending",
    { server: { ...baseServer, deletion_pending: true }, ready: false },
    "worlds",
  ],
  ["cancelled", { server: { ...baseServer, status: "CANCELLED" }, ready: false }, "worlds"],
  ["no start permission", { server: { ...baseServer, permissions: ["view"] } }, "wait"],
  [
    "moderator online",
    { server: { ...baseServer, permissions: ["view", "start"] }, powerState: "running" },
    "join",
  ],
]) {
  test(`next action respects ${name}`, () => {
    expect(
      nextServerAction({
        server: baseServer,
        ready: true,
        eula: true,
        powerState: "offline",
        ...changes,
      }).kind,
    ).toBe(kind);
  });
}

test("next-action start uses guarded power controls and becomes invite guidance", async ({
  page,
}) => {
  const calls = await mockAPI(page);
  await openServer(page);
  const card = page.getByRole("region", { name: "Play on your server" });
  await expect(card.getByRole("heading", { name: "Start your next adventure." })).toBeVisible();
  await card.getByRole("button", { name: "Start your world", exact: true }).click();
  await expect(card.getByRole("link", { name: "Invite friends", exact: true })).toHaveAttribute(
    "href",
    "#invite-friends",
  );
  await expect(card.getByRole("button", { name: "Start your world", exact: true })).toHaveCount(0);
  expect(calls.filter((call) => call.path === `/servers/${serverID}/start`)).toHaveLength(1);
});

test("next-action agreement directs to explicit acceptance without accepting automatically", async ({
  page,
}) => {
  const calls = await mockAPI(page, { eulaAccepted: false });
  await openServer(page);
  const card = page.getByRole("region", { name: "Play on your server" });
  await card.getByRole("link", { name: "Review Minecraft agreement" }).click();
  await expect(page.getByRole("button", { name: "Accept agreement", exact: true })).toBeDisabled();
  await expect(card.getByRole("button", { name: "Start your world", exact: true })).toHaveCount(0);
  expect(
    calls.filter((call) => call.path === `/servers/${serverID}/eula` && call.method === "POST"),
  ).toHaveLength(0);
});

test("next-action stopping asks players to wait instead of starting again", async ({ page }) => {
  await mockAPI(page, { initiallyRunning: true, holdStop: true });
  await openServer(page);
  await page.getByRole("button", { name: "Stop", exact: true }).click();
  const card = page.getByRole("region", { name: "Play on your server" });
  await expect(card.getByRole("heading", { name: "Saving your adventure." })).toBeVisible();
  await expect(card.getByRole("button", { name: "Start your world", exact: true })).toHaveCount(0);
});

for (const online of [false, true]) {
  test(`installed addons lead discovery and show honest ${online ? "online" : "offline"} load guidance`, async ({
    page,
  }) => {
    await mockAPI(page, { initiallyRunning: online });
    await login(page);
    await page.goto(`/servers/${serverID}/plugins`);
    const installed = page.getByRole("heading", { name: "Your plugins", exact: true });
    await expect(installed).toBeVisible();
    const catalog = page.getByRole("heading", { name: "Find plugins", exact: true });
    expect((await installed.boundingBox()).y).toBeLessThan((await catalog.boundingBox()).y);
    const status = page.getByLabel("Essentials.jar status");
    await expect(status.getByText("Installed", { exact: true })).toBeVisible();
    await expect(
      status.getByText(online ? "Check load status" : "Start to load", { exact: true }),
    ).toBeVisible();
    await expect(status.getByText("Loaded and enabled", { exact: true })).toHaveCount(0);
    await expect(
      page
        .locator(".installed-addon-actions")
        .getByRole("link", { name: "Configuration files" })
        .first(),
    ).toHaveAttribute("href", `/servers/${serverID}/files`);
  });
}

for (const width of [390, 1366]) {
  test(`invite without required client mods keeps joining simple at ${width}px`, async ({
    page,
  }, testInfo) => {
    await page.setViewportSize({ width, height: 900 });
    const calls = await mockAPI(page);
    await page.route(`http://localhost:8080/join/${playerSlug}`, (route) =>
      route.fulfill({
        json: { ...playerSetup, addons: [], pack_ready: false, loader: "", loader_version: "" },
      }),
    );
    await page.goto(`/join/${playerSlug}`);
    await expect(
      page.getByRole("heading", { name: "Open Minecraft. Join your friends." }),
    ).toBeVisible();
    await expect(page.getByRole("button", { name: "Copy address", exact: true })).toBeVisible();
    await expect(page.getByRole("combobox", { name: "Your Minecraft launcher" })).toHaveCount(0);
    await expect(page.getByRole("button", { name: "Download player pack" })).toHaveCount(0);
    expect(calls.some((call) => call.path.startsWith("/servers"))).toBe(false);
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
      true,
    );
    await page.screenshot({
      path: testInfo.outputPath(`simple-invite-${width}.png`),
      fullPage: true,
    });
  });
}

test("optional player mods do not turn the basic join path into mandatory installation", async ({
  page,
}) => {
  await mockAPI(page);
  await page.route(`http://localhost:8080/join/${playerSlug}`, (route) =>
    route.fulfill({
      json: {
        ...playerSetup,
        addons: playerSetup.addons.map((addon) => ({ ...addon, required: false })),
      },
    }),
  );
  await page.goto(`/join/${playerSlug}`);
  await expect(page.getByRole("button", { name: "Download player pack" })).toHaveCount(0);
  await page.getByRole("button", { name: "Set up optional player features" }).click();
  await expect(page.getByRole("button", { name: "Download player pack" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "Add optional player features" })).toBeVisible();
});

test("returning player explicitly confirms setup and can reopen downloads after refresh", async ({
  page,
}) => {
  await mockAPI(page);
  await page.route(`http://localhost:8080/join/${playerSlug}`, (route) =>
    route.fulfill({ json: playerSetup }),
  );
  await page.goto(`/join/${playerSlug}`);
  await selectValue(
    page,
    page.getByRole("combobox", { name: "Your Minecraft launcher" }),
    "sklauncher",
  );
  await expect(page.getByRole("region", { name: "Returning player" })).toHaveCount(0);
  await page.getByRole("button", { name: "My Minecraft setup is ready" }).click();
  await expect(page.getByRole("region", { name: "Returning player" })).toBeVisible();
  await expect(page.getByRole("button", { name: "Download player pack" })).toHaveCount(0);
  await page.reload();
  await expect(page.getByRole("region", { name: "Returning player" })).toBeVisible();
  await page.getByRole("button", { name: "Show setup & downloads" }).click();
  await expect(page.getByRole("combobox", { name: "Your Minecraft launcher" })).toContainText(
    "SKlauncher 4",
  );
  await expect(page.getByRole("button", { name: "Download player pack" })).toBeVisible();
});

test("republished setup resets returning guidance and requires current player setup", async ({
  page,
}) => {
  await mockAPI(page);
  let setup = playerSetup;
  await page.route(`http://localhost:8080/join/${playerSlug}`, (route) =>
    route.fulfill({ json: setup }),
  );
  await page.goto(`/join/${playerSlug}`);
  await page.getByRole("button", { name: "My Minecraft setup is ready" }).click();
  setup = { ...playerSetup, published_at: "2026-10-14T12:00:00Z", minecraft_version: "26.3" };
  await page.reload();
  await expect(
    page.getByRole("status").filter({ hasText: "Your host published a new player setup" }),
  ).toBeVisible();
  await expect(page.getByRole("region", { name: "Returning player" })).toHaveCount(0);
  await expect(page.getByRole("button", { name: "Download player pack" })).toBeVisible();
});

test("open invite rechecks setup and stops downloads after revocation", async ({ page }) => {
  await page.clock.install();
  await mockAPI(page);
  let revoked = false;
  await page.route(`http://localhost:8080/join/${playerSlug}`, (route) =>
    revoked
      ? route.fulfill({ status: 404, json: { error: "resource not found" } })
      : route.fulfill({ json: playerSetup }),
  );
  await page.goto(`/join/${playerSlug}`);
  await expect(page.getByRole("button", { name: "Download player pack" })).toBeVisible();
  revoked = true;
  await page.clock.fastForward(60001);
  await expect(page.getByRole("heading", { name: "This invite is unavailable" })).toBeVisible();
  await expect(page.getByRole("button", { name: "Download player pack" })).toHaveCount(0);
});

test("quick connection help uses published version and login rules without management access", async ({
  page,
}) => {
  const calls = await mockAPI(page);
  await page.route(`http://localhost:8080/join/${playerSlug}`, (route) =>
    route.fulfill({ json: { ...playerSetup, verified_login: true, whitelist: true } }),
  );
  await page.goto(`/join/${playerSlug}`);
  await page.getByText("Can’t join? Find a quick fix", { exact: true }).click();
  const help = page.locator(".player-connection-help");
  const control = help.getByRole("combobox", { name: "What does Minecraft show?" });
  await expect(help.getByText("Open Minecraft Java 1.21.1.", { exact: false })).toBeVisible();
  await selectValue(page, control, "login");
  await expect(help.getByText("An offline username cannot pass", { exact: false })).toBeVisible();
  await selectValue(page, control, "whitelist");
  await expect(
    help.getByText("Send your exact Minecraft username", { exact: false }),
  ).toBeVisible();
  await selectValue(page, control, "mods");
  await expect(
    help.getByText("Downloading the pack alone does not install it.", { exact: false }),
  ).toBeVisible();
  expect(calls.some((call) => call.path.startsWith("/servers"))).toBe(false);
});

test("player readiness works for the current visit when browser storage is blocked", async ({
  page,
}) => {
  await page.addInitScript(() => {
    Storage.prototype.getItem = () => {
      throw Error("blocked");
    };
    Storage.prototype.setItem = () => {
      throw Error("blocked");
    };
  });
  await mockAPI(page);
  await page.route(`http://localhost:8080/join/${playerSlug}`, (route) =>
    route.fulfill({ json: playerSetup }),
  );
  await page.goto(`/join/${playerSlug}`);
  await page.getByRole("button", { name: "My Minecraft setup is ready" }).click();
  await expect(page.getByRole("region", { name: "Returning player" })).toBeVisible();
});

test("downloading does not confirm installation and a refreshed pack clears old download guidance", async ({
  page,
}) => {
  await page.clock.install();
  await mockAPI(page);
  let setup = playerSetup;
  await page.route(`http://localhost:8080/join/${playerSlug}`, (route) =>
    route.fulfill({ json: setup }),
  );
  await page.route(`http://localhost:8080/join/${playerSlug}/pack`, (route) =>
    route.fulfill({ body: Buffer.from("pack"), contentType: "application/x-modrinth-modpack+zip" }),
  );
  await page.goto(`/join/${playerSlug}`);
  const download = page.waitForEvent("download");
  await page.getByRole("button", { name: "Download player pack" }).click();
  await download;
  await expect(page.getByText("Downloaded. Import it in your launcher next.")).toBeVisible();
  await expect(page.getByRole("region", { name: "Returning player" })).toHaveCount(0);
  setup = { ...playerSetup, published_at: "2026-10-14T14:00:00Z" };
  await page.clock.fastForward(60001);
  await expect(page.getByText("Downloaded. Import it in your launcher next.")).toHaveCount(0);
  await expect(page.getByRole("button", { name: "Download player pack" })).toBeVisible();
});

test("invite refresh respects rate-limit retry timing", async ({ page }) => {
  await page.clock.install();
  await mockAPI(page);
  let reads = 0;
  await page.route(`http://localhost:8080/join/${playerSlug}`, (route) => {
    reads++;
    return reads === 1
      ? route.fulfill({
          status: 429,
          headers: {
            "Retry-After": "120",
            "Access-Control-Expose-Headers": "Retry-After, X-Request-ID",
          },
          json: { error: "too many requests" },
        })
      : route.fulfill({ json: playerSetup });
  });
  await page.goto(`/join/${playerSlug}`);
  await expect(
    page.getByRole("heading", { name: "Player setup couldn’t be loaded" }),
  ).toBeVisible();
  await expect(page.getByRole("button", { name: "Try again" })).toHaveCount(0);
  await page.clock.fastForward(60001);
  expect(reads).toBe(1);
  await page.clock.fastForward(60001);
  await expect(page.getByRole("heading", { name: "Join The Lantern Lodge" })).toBeVisible();
  expect(reads).toBe(2);
});

test("returning optional-feature players can reopen the optional download path", async ({
  page,
}) => {
  await mockAPI(page);
  await page.route(`http://localhost:8080/join/${playerSlug}`, (route) =>
    route.fulfill({
      json: {
        ...playerSetup,
        addons: playerSetup.addons.map((addon) => ({ ...addon, required: false })),
      },
    }),
  );
  await page.goto(`/join/${playerSlug}`);
  await page.getByRole("button", { name: "My Minecraft setup is ready" }).click();
  await page.getByRole("button", { name: "Show setup & downloads" }).click();
  await expect(page.getByRole("button", { name: "Download player pack" })).toBeVisible();
});

test("required addons without a ready pack cannot be skipped as normal Minecraft", async ({
  page,
}) => {
  await mockAPI(page);
  await page.route(`http://localhost:8080/join/${playerSlug}`, (route) =>
    route.fulfill({ json: { ...playerSetup, pack_ready: false } }),
  );
  await page.goto(`/join/${playerSlug}`);
  await expect(
    page.getByRole("heading", { name: "Ask your host to finish player setup" }),
  ).toBeVisible();
  await expect(page.getByRole("button", { name: "Download player pack" })).toHaveCount(0);
  await expect(page.getByRole("button", { name: "My Minecraft setup is ready" })).toHaveCount(0);
});

const financeNode = {
  id: "60000000-0000-4000-8000-000000000099",
  name: "Test node",
  enabled: true,
  cpu_capacity: 600,
  ram_capacity_mb: 12288,
  disk_capacity_mb: 102400,
  allocated_cpu: 100,
  allocated_ram_mb: 2048,
  allocated_disk_mb: 10240,
  available_allocations: 10,
};
const financeCosts = {
  node_id: financeNode.id,
  currency: "USD",
  node_monthly_cents: 1800,
  overhead_monthly_cents: 200,
  per_server_monthly_cents: 50,
  fee_bps: 300,
  fee_fixed_cents: 30,
  tax_bps: 0,
  target_margin_bps: 3000,
  occupancy_bps: 5000,
};

test("group recommendations use enabled database plans without currency conversion or upselling", () => {
  expect(PLAYER_GROUPS.map((group) => group.players)).toEqual([4, 8, 16, 30]);
  const small = { ...plan, enabled: true, player_recommendation: 8 };
  const large = { ...small, id: orderID, price_cents: 1500, player_recommendation: 30 };
  expect(recommendGroupPlan([large, small], 4).id).toBe(small.id);
  expect(recommendGroupPlan([small, large], 16).id).toBe(large.id);
  expect(recommendGroupPlan([small], 30)).toBeNull();
  expect(recommendGroupPlan([{ ...large, currency: "EUR" }, small], 16, "USD")).toBeNull();
  expect(recommendGroupPlan([{ ...small, enabled: false }], 4)).toBeNull();
});

test("profit forecast reserves existing allocations and observes every resource limit", () => {
  const p = { ...plan, ram_mb: 2048, disk_mb: 10240, cpu_limit: 100 };
  expect(availableSlots(financeNode, p)).toBe(5);
  expect(availableSlots({ ...financeNode, available_allocations: 2 }, p)).toBe(2);
  expect(availableSlots({ ...financeNode, cpu_capacity: 200 }, p)).toBe(1);
  expect(availableSlots({ ...financeNode, ram_capacity_mb: 4096 }, p)).toBe(1);
  expect(availableSlots({ ...financeNode, disk_capacity_mb: 20480 }, p)).toBe(1);
  expect(availableSlots({ ...financeNode, enabled: false }, p)).toBe(0);
  expect(mixForecast(financeNode, [p], financeCosts, { [p.id]: 6 }).fits).toBe(false);
  expect(mixForecast(financeNode, [p], financeCosts, { [p.id]: 2 }).fits).toBe(true);
  expect(mixForecast(financeNode, [p], financeCosts, {}).profit).toBe(-2000);
});

test("yearly payments charge one fixed fee and price floors cover the target after costs", () => {
  const p = {
    ...plan,
    price_cents: 12000,
    billing_period: "YEARLY",
    currency: "USD",
    ram_mb: 2048,
    disk_mb: 10240,
    cpu_limit: 100,
  };
  const sale = monthlySale(p, financeCosts);
  expect(sale.fees).toBe((360 + 30) / 12);
  const result = planMargin(financeNode, p, financeCosts);
  expect(result.customers).toBe(2);
  expect(result.floorPrice).not.toBeNull();
  const suggested = monthlySale(p, financeCosts, result.floorPrice);
  const profit = result.customers * suggested.contribution - 2000;
  expect(profit / (result.customers * suggested.revenue)).toBeGreaterThanOrEqual(0.3);
  expect(planMargin(financeNode, { ...p, currency: "EUR" }, financeCosts).error).toContain(
    "Currency differs",
  );
  expect(
    mixForecast(financeNode, [{ ...p, currency: "EUR" }], financeCosts, { [p.id]: 1 }).error,
  ).toContain("currencies");
  const taxed = planMargin(
    financeNode,
    { ...p, billing_period: "MONTHLY" },
    { ...financeCosts, tax_bps: 2000 },
  );
  const taxedSale = monthlySale(
    { ...p, billing_period: "MONTHLY" },
    { ...financeCosts, tax_bps: 2000 },
    taxed.floorPrice,
  );
  expect(
    (taxed.customers * taxedSale.contribution - 2000) / (taxed.customers * taxedSale.revenue),
  ).toBeGreaterThanOrEqual(0.3);
});

for (const width of [390, 1366]) {
  test(`four player groups recommend actual plans and preserve backend order terms at ${width}px`, async ({
    page,
  }, testInfo) => {
    await page.setViewportSize({ width, height: 900 });
    const calls = await mockAPI(page);
    const large = {
      ...plan,
      id: "10000000-0000-4000-8000-000000000030",
      name: "Community plan",
      price_cents: 2200,
      player_recommendation: 30,
      ram_mb: 8192,
    };
    await page.route("http://localhost:8080/plans", (route) =>
      route.fulfill({ json: [plan, large] }),
    );
    await login(page);
    await page.goto("/servers/new");
    await page.getByRole("button", { name: "Next", exact: true }).click();
    const groups = page.getByLabel("Player group size");
    await expect(groups.getByRole("button")).toHaveCount(4);
    await groups.getByRole("button", { name: /Community/ }).click();
    await reviewCreation(page);
    await expect(page.locator(".order-summary")).toContainText("Community plan");
    await reviewCreation(page);
    await page.getByLabel("Server name", { exact: true }).fill("Friends world");
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
      true,
    );
    await page.evaluate(() => {
      document.activeElement?.blur();
      window.scrollTo(0, 0);
    });
    await page.screenshot({
      path: testInfo.outputPath(`player-groups-${width}.png`),
      fullPage: true,
    });
    await page.getByRole("button", { name: "Continue to checkout" }).click();
    const order = calls.find((call) => call.path === "/orders" && call.method === "POST");
    expect(order.body.plan_id).toBe(large.id);
    expect(order.body).not.toHaveProperty("price_cents");
    expect(order.body).not.toHaveProperty("ram_mb");
    expect(order.body).not.toHaveProperty("players");
  });
}

test("admin sales keep demo payments separate and save explicit cost assumptions", async ({
  page,
}, testInfo) => {
  await page.setViewportSize({ width: 390, height: 900 });
  const calls = await mockAPI(page, { isAdmin: true });
  let costs = [];
  await page.route("http://localhost:8080/admin/nodes**", (route) =>
    route.fulfill({ json: [financeNode] }),
  );
  await page.route("http://localhost:8080/admin/finances**", (route) =>
    route.fulfill({
      json: {
        month: "2026-10",
        costs,
        sales: [
          {
            provider: "mock",
            currency: "USD",
            paid_count: 2,
            paid_cents: 5000,
            refunded_count: 0,
            refunded_cents: 0,
          },
        ],
      },
    }),
  );
  await page.route("http://localhost:8080/admin/hosting-costs", async (route) => {
    const body = route.request().postDataJSON();
    costs = [body];
    calls.push({ path: "/admin/hosting-costs", method: "PUT", body });
    return route.fulfill({ json: body });
  });
  await login(page);
  await page.goto("/admin");
  await page.getByRole("button", { name: "Finances", exact: true }).click();
  await expect(page.getByText("Demo payments", { exact: true })).toBeVisible();
  await expect(page.getByRole("region", { name: "Profitability forecast" })).toHaveCount(0);
  await page.getByRole("button", { name: "Edit cost assumptions" }).click();
  await page.getByLabel("VPS bill per month", { exact: true }).fill("18");
  await expect(page.getByLabel("VPS bill per month", { exact: true })).toHaveValue("18");
  await expect(page.getByLabel("Expected occupancy (%)", { exact: true })).toHaveValue("50");
  await page.getByLabel("Other monthly costs", { exact: true }).fill("2");
  await page.getByLabel("Support / backup cost per server per month", { exact: true }).fill("0.50");
  await page.getByLabel("Payment fee (%)", { exact: true }).fill("3");
  await page.getByLabel("Fixed payment fee per charge", { exact: true }).fill("0.30");
  await page.getByLabel("Tax included in selling price (%)", { exact: true }).fill("0");
  await page.getByRole("button", { name: "Save costs", exact: true }).click();
  await expect(page.getByRole("status").filter({ hasText: "Costs saved" })).toBeVisible();
  expect(costs[0].node_monthly_cents).toBe(1800);
  expect(costs[0].fee_bps).toBe(300);
  expect(costs[0].fee_fixed_cents).toBe(30);
  await expect(page.getByRole("region", { name: "Profitability forecast" })).toBeVisible();
  expect(calls.some((call) => call.path.startsWith("/admin/plans") && call.method === "PUT")).toBe(
    false,
  );
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await page.evaluate(() => {
    document.activeElement?.blur();
    window.scrollTo(0, 0);
  });
  await page.screenshot({ path: testInfo.outputPath("admin-finances-mobile.png"), fullPage: true });
});

test("installed Chunky offers confirmed terrain controls without typing commands", async ({
  page,
}, testInfo) => {
  await page.setViewportSize({ width: 390, height: 900 });
  await mockAPI(page, { initiallyRunning: true });
  const commands = [];
  await page.route(`http://localhost:8080/servers/${serverID}/plugins`, (route) =>
    route.fulfill({ json: [{ name: "Chunky-Bukkit-1.5.3.jar", size: 1234, is_file: true }] }),
  );
  await page.route(`http://localhost:8080/servers/${serverID}/commands`, (route) => {
    commands.push(route.request().postDataJSON().command);
    return route.fulfill({ status: 204 });
  });
  await login(page);
  await page.goto(`/servers/${serverID}/plugins`);
  await page.getByText("Terrain settings", { exact: true }).click();
  await page.screenshot({ path: testInfo.outputPath("chunky-setup-mobile.png"), fullPage: true });
  const prepare = page.getByRole("button", { name: "Prepare terrain", exact: true });
  await expect(prepare).toBeDisabled();
  expect(commands).toEqual([]);
  await page.getByLabel("I’m ready to generate").check();
  await prepare.click();
  await expect(page.getByRole("status").filter({ hasText: "Request sent" })).toBeVisible();
  expect(commands).toEqual(["chunky start world circle 0 0 500"]);
  await expect(prepare).toBeDisabled();
  await page.getByRole("button", { name: "Pause preparation" }).click();
  await expect.poll(() => commands.length).toBe(2);
  expect(commands[1]).toBe("chunky pause world");
  await page.getByRole("button", { name: "Enter world name manually" }).click();
  await page.getByLabel("World name", { exact: true }).fill("world;stop");
  await expect(page.getByRole("button", { name: "Pause preparation" })).toBeDisabled();
});

test("voice setup keyboard hint has visible contrasting text", async ({ page }) => {
  await mockAPI(page);
  await login(page);
  await page.goto(`/servers/${serverID}/addons/simple-voice-chat`);
  const key = page.locator(".addon-client-guide kbd").filter({ hasText: /^V$/ });
  await expect(key).toBeVisible();
  const colors = await key.evaluate((el) => ({
    foreground: getComputedStyle(el).color,
    background: getComputedStyle(el).backgroundColor,
  }));
  expect(colors.foreground).toBe("rgb(23, 51, 34)");
  expect(colors.background).toBe("rgb(237, 244, 234)");
});

test("Chunky progress parser rejects chat, other plugins and invalid percentages", () => {
  const message =
    "[05:53:40 INFO]: [Chunky] Task running for world. Processed: 4282 chunks (26.55%), ETA: 0:09:06, Rate: 21.7 cps, Current: -49, -51";
  expect(parseChunkyProgress(message)).toEqual({
    world: "world",
    chunks: 4282,
    percent: 26.55,
    eta: "0:09:06",
    rate: 21.7,
  });
  expect(parseChunkyProgress(message.replace("26.55%", "101%"))).toBeNull();
  expect(parseChunkyProgress(message.replace("[Chunky]", "<player> [Chunky]"))).toBeNull();
  expect(parseChunkyProgress(message.replace("[Chunky]", "[OtherPlugin]"))).toBeNull();
  expect(parseChunkyProgress("Task finished! ")).toBeNull();
  expect(parseChunkyProgress(null)).toBeNull();
});

test("installed addon search filters locally and clears without changing files", async ({
  page,
}) => {
  await mockAPI(page);
  await page.route(`http://localhost:8080/servers/${serverID}/plugins`, (route) =>
    route.fulfill({
      json: [
        { name: "Chunky-Bukkit-1.5.3.jar", size: 1234, is_file: true },
        { name: "voicechat-bukkit-2.6.jar", size: 1234, is_file: true },
      ],
    }),
  );
  await login(page);
  await page.goto(`/servers/${serverID}/plugins`);
  await page.getByLabel("Find an installed plugin").fill("CHUNKY");
  await expect(page.locator(".plugin-list").first().locator(":scope > li")).toHaveCount(1);
  await expect(page.locator(".plugin-list").first()).toContainText("Chunky-Bukkit");
  await page.getByLabel("Find an installed plugin").fill("missing");
  await expect(page.getByText("No installed plugins match", { exact: false })).toBeVisible();
  await page.getByRole("button", { name: "Clear search", exact: true }).click();
  await expect(page.locator(".plugin-list").first().locator(":scope > li")).toHaveCount(2);
});

test("Chunky progress is opt-in, world-specific and honest after stream disconnection", async ({
  page,
}) => {
  const calls = await mockAPI(page, { initiallyRunning: true });
  let stream;
  await page.routeWebSocket(`ws://localhost:8080/servers/${serverID}/console`, (ws) => {
    stream = ws;
    ws.send(JSON.stringify({ event: "connected" }));
    ws.send(
      JSON.stringify({
        event: "console output",
        args: [
          "[05:53:40 INFO]: [Chunky] Task running for world. Processed: 4282 chunks (26.55%), ETA: 0:09:06, Rate: 21.7 cps, Current: -49, -51",
        ],
      }),
    );
  });
  await page.route(`http://localhost:8080/servers/${serverID}/plugins`, (route) =>
    route.fulfill({ json: [{ name: "Chunky-Bukkit-1.5.3.jar", size: 1234, is_file: true }] }),
  );
  await login(page);
  await page.clock.install();
  await page.goto(`/servers/${serverID}/plugins`);
  await expect(page.getByText("Terrain settings", { exact: true })).toBeVisible();
  expect(calls.filter((c) => c.path.endsWith("/console-ticket"))).toHaveLength(0);
  await page.getByText("Terrain settings", { exact: true }).click();
  await expect(page.getByText("Last reported: 26.55%")).toBeVisible();
  await expect(page.getByRole("progressbar")).toHaveAttribute("value", "26.55");
  expect(calls.filter((c) => c.path.endsWith("/commands"))).toHaveLength(0);
  await page.clock.fastForward(35000);
  await expect(page.getByText("This report may be out of date.", { exact: false })).toBeVisible();
  stream.close();
  await expect(
    page.getByText("Updates disconnected. Task status cannot be confirmed."),
  ).toBeVisible();
  await expect(page.getByText("This report may be out of date.", { exact: false })).toBeVisible();
  await selectValue(page, page.getByRole("combobox", { name: "Choose a world" }), "world_nether");
  await expect(page.getByRole("progressbar")).toHaveCount(0);
});

test("retrying a failed Chunky request reuses its idempotency key", async ({ page }) => {
  await mockAPI(page, { initiallyRunning: true });
  const keys = [];
  await page.routeWebSocket(`ws://localhost:8080/servers/${serverID}/console`, (ws) =>
    ws.send(JSON.stringify({ event: "connected" })),
  );
  await page.route(`http://localhost:8080/servers/${serverID}/plugins`, (route) =>
    route.fulfill({ json: [{ name: "Chunky-Bukkit-1.5.3.jar", size: 1234, is_file: true }] }),
  );
  await page.route(`http://localhost:8080/servers/${serverID}/commands`, (route) => {
    keys.push(route.request().headers()["idempotency-key"]);
    return keys.length === 1
      ? route.fulfill({ status: 503, json: { error: "Temporary connection failure" } })
      : route.fulfill({ status: 204 });
  });
  await login(page);
  await page.goto(`/servers/${serverID}/plugins`);
  await page.getByText("Terrain settings", { exact: true }).click();
  await page.getByLabel("I’m ready to generate").check();
  const start = page.getByRole("button", { name: "Prepare terrain", exact: true });
  await start.click();
  await expect(start).toBeEnabled();
  await start.click();
  await expect(page.getByRole("status").filter({ hasText: "Request sent" })).toBeVisible();
  expect(keys).toHaveLength(2);
  expect(keys[0]).toBe(keys[1]);
});

test("unsaved voice edits warn on refresh and can be discarded without writing", async ({
  page,
}) => {
  const calls = await mockAPI(page);
  await login(page);
  await page.goto(`/servers/${serverID}/addons/simple-voice-chat`);
  const distance = page.getByLabel("Voice distance", { exact: true });
  await expect(distance).toHaveValue("72");
  await distance.fill("90");
  await expect(
    page.getByRole("status").filter({ hasText: "unsaved voice settings" }),
  ).toBeVisible();
  const prevented = () =>
    page.evaluate(() => {
      const event = new Event("beforeunload", { cancelable: true });
      window.dispatchEvent(event);
      return event.defaultPrevented;
    });
  expect(await prevented()).toBe(true);
  await page.getByRole("button", { name: "Discard changes", exact: true }).click();
  await expect(distance).toHaveValue("72");
  expect(await prevented()).toBe(false);
  expect(
    calls.filter(
      (call) => call.method === "PUT" && call.path.endsWith("/addons/simple-voice-chat"),
    ),
  ).toHaveLength(0);
});

test("voice settings retain edits and reuse the operation key after a failed save", async ({
  page,
}) => {
  await mockAPI(page);
  const saves = [];
  await page.route(
    `http://localhost:8080/servers/${serverID}/addons/simple-voice-chat`,
    async (route) => {
      if (route.request().method() !== "PUT") return route.fallback();
      saves.push({
        key: route.request().headers()["idempotency-key"],
        body: route.request().postDataJSON(),
      });
      return route.fulfill({ status: 503, json: { error: "Temporary connection failure" } });
    },
  );
  await login(page);
  await page.goto(`/servers/${serverID}/addons/simple-voice-chat`);
  const distance = page.getByLabel("Voice distance", { exact: true });
  await distance.fill("90");
  const save = page.getByRole("button", { name: "Save settings", exact: true });
  await save.click();
  await expect(save).toBeEnabled();
  await expect(distance).toHaveValue("90");
  await save.click();
  await expect.poll(() => saves.length).toBe(2);
  expect(saves[0]).toEqual(saves[1]);
  await expect(save).toBeEnabled();
  await distance.fill("100");
  await save.click();
  await expect.poll(() => saves.length).toBe(3);
  expect(saves[2].key).not.toBe(saves[1].key);
  expect(saves[2].body.max_voice_distance).toBe(100);
});

test("dashboard pauses background polling and refreshes once on return", async ({ page }) => {
  await mockAPI(page);
  await login(page);
  await page.clock.install();
  let requests = 0;
  await page.route(
    `http://localhost:8080/servers/${serverID}/addons/simple-voice-chat/network`,
    (route) => {
      requests++;
      return route.fulfill({
        json: { status: "READY", hostname: "play.example.com", port: 24454 },
      });
    },
  );
  await page.goto(`/servers/${serverID}/addons/simple-voice-chat`);
  await expect.poll(() => requests).toBe(1);
  await page.evaluate(() => {
    Object.defineProperty(document, "visibilityState", { configurable: true, value: "hidden" });
    document.dispatchEvent(new Event("visibilitychange"));
  });
  await page.clock.fastForward(20000);
  expect(requests).toBe(1);
  await page.evaluate(() => {
    Object.defineProperty(document, "visibilityState", { configurable: true, value: "visible" });
    document.dispatchEvent(new Event("visibilitychange"));
  });
  await page.clock.fastForward(1);
  await expect.poll(() => requests).toBe(2);
});

test("dashboard polling waits while offline and resumes on reconnect", async ({
  page,
  context,
}) => {
  await mockAPI(page);
  await login(page);
  await page.clock.install();
  let requests = 0;
  await page.route(
    `http://localhost:8080/servers/${serverID}/addons/simple-voice-chat/network`,
    (route) => {
      requests++;
      return route.fulfill({
        json: { status: "READY", hostname: "play.example.com", port: 24454 },
      });
    },
  );
  await page.goto(`/servers/${serverID}/addons/simple-voice-chat`);
  await expect.poll(() => requests).toBe(1);
  await context.setOffline(true);
  await expect.poll(() => page.evaluate(() => navigator.onLine)).toBe(false);
  await page.clock.fastForward(20000);
  expect(requests).toBe(1);
  await context.setOffline(false);
  await expect.poll(() => page.evaluate(() => navigator.onLine)).toBe(true);
  await page.clock.fastForward(1);
  await expect.poll(() => requests).toBe(2);
});

test("Chunky lifecycle reports require explicit plugin messages rather than 100 percent", () => {
  expect(parseChunkyEvent("[05:53:40 INFO]: [Chunky] Task paused for world.")).toEqual({
    world: "world",
    status: "paused",
  });
  expect(
    parseChunkyEvent(
      "[Chunky] Task finished for world. Processed: 16000 chunks (100.00%), Total time: 0:12:34",
    ),
  ).toEqual({
    world: "world",
    chunks: 16000,
    percent: 100,
    elapsed: "0:12:34",
    status: "finished",
  });
  expect(
    parseChunkyEvent(
      "[Chunky] Task running for world. Processed: 16000 chunks (100.00%), ETA: 0:00:00, Rate: 20.0 cps",
    ).status,
  ).toBe("running");
  expect(parseChunkyEvent("[05:53:40 INFO]: <player> [Chunky] Task paused for world.")).toBeNull();
  expect(
    parseChunkyEvent(
      "[Chunky] Task finished for world. Processed: 1 chunks (101%), Total time: 0:00:01",
    ),
  ).toBeNull();
});

test("terrain preparation uses actual custom world choices instead of guessing world", async ({
  page,
}) => {
  await mockAPI(page, { initiallyRunning: true });
  const commands = [];
  await page.route(`http://localhost:8080/servers/${serverID}/addons/chunky/worlds`, (route) =>
    route.fulfill({
      json: [
        { name: "Campfire", label: "Main world" },
        { name: "Campfire_nether", label: "Nether" },
      ],
    }),
  );
  await page.route(`http://localhost:8080/servers/${serverID}/plugins`, (route) =>
    route.fulfill({ json: [{ name: "Chunky-Bukkit-1.5.3.jar", size: 1234, is_file: true }] }),
  );
  await page.routeWebSocket(`ws://localhost:8080/servers/${serverID}/console`, (ws) =>
    ws.send(JSON.stringify({ event: "connected" })),
  );
  await page.route(`http://localhost:8080/servers/${serverID}/commands`, (route) => {
    commands.push(route.request().postDataJSON().command);
    return route.fulfill({ status: 204 });
  });
  await login(page);
  await page.goto(`/servers/${serverID}/plugins`);
  await page.getByText("Terrain settings", { exact: true }).click();
  await expect(page.getByRole("combobox", { name: "Choose a world" })).toContainText(
    "Main world · Campfire",
  );
  await selectValue(
    page,
    page.getByRole("combobox", { name: "Choose a world" }),
    "Campfire_nether",
  );
  await page.getByLabel("I’m ready to generate").check();
  await page.getByRole("button", { name: "Prepare terrain", exact: true }).click();
  await expect.poll(() => commands.length).toBe(1);
  expect(commands[0]).toBe("chunky start Campfire_nether circle 0 0 500");
});

test("unavailable world detection blocks guessed commands and offers explicit manual setup", async ({
  page,
}) => {
  await mockAPI(page, { initiallyRunning: true });
  const commands = [];
  await page.route(`http://localhost:8080/servers/${serverID}/addons/chunky/worlds`, (route) =>
    route.fulfill({ status: 503, json: { error: "World lookup unavailable" } }),
  );
  await page.route(`http://localhost:8080/servers/${serverID}/plugins`, (route) =>
    route.fulfill({ json: [{ name: "Chunky-Bukkit-1.5.3.jar", size: 1234, is_file: true }] }),
  );
  await page.routeWebSocket(`ws://localhost:8080/servers/${serverID}/console`, (ws) =>
    ws.send(JSON.stringify({ event: "connected" })),
  );
  await page.route(`http://localhost:8080/servers/${serverID}/commands`, (route) => {
    commands.push(route.request().postDataJSON().command);
    return route.fulfill({ status: 204 });
  });
  await login(page);
  await page.goto(`/servers/${serverID}/plugins`);
  await page.getByText("Terrain settings", { exact: true }).click();
  await expect(page.getByText("World lookup unavailable", { exact: true })).toBeVisible();
  await page.getByLabel("I’m ready to generate").check();
  await expect(page.getByRole("button", { name: "Prepare terrain", exact: true })).toBeDisabled();
  expect(commands).toEqual([]);
  await page.getByRole("button", { name: "Enter world name manually" }).click();
  await page.getByLabel("World name", { exact: true }).fill("My_world");
  await page.getByLabel("I’m ready to generate").check();
  await page.getByRole("button", { name: "Prepare terrain", exact: true }).click();
  await expect.poll(() => commands.length).toBe(1);
  expect(commands[0]).toBe("chunky start My_world circle 0 0 500");
});

test("Chunky pause and finish reports replace running estimates in the setup panel", async ({
  page,
}) => {
  await mockAPI(page, { initiallyRunning: true });
  let stream;
  await page.routeWebSocket(`ws://localhost:8080/servers/${serverID}/console`, (ws) => {
    stream = ws;
    ws.send(JSON.stringify({ event: "connected" }));
  });
  await page.route(`http://localhost:8080/servers/${serverID}/plugins`, (route) =>
    route.fulfill({ json: [{ name: "Chunky-Bukkit-1.5.3.jar", size: 1234, is_file: true }] }),
  );
  await login(page);
  await page.goto(`/servers/${serverID}/plugins`);
  await page.getByText("Terrain settings", { exact: true }).click();
  await expect.poll(() => !!stream).toBe(true);
  stream.send(
    JSON.stringify({ event: "console output", args: ["[Chunky] Task paused for world."] }),
  );
  const progress = page.getByRole("region", { name: "Terrain preparation progress" });
  await expect(progress).toContainText("Last reported state: Paused");
  await expect(progress.getByRole("progressbar")).toHaveCount(0);
  stream.send(
    JSON.stringify({
      event: "console output",
      args: [
        "[Chunky] Task finished for world. Processed: 16000 chunks (100.00%), Total time: 0:12:34",
      ],
    }),
  );
  await expect(progress).toContainText("Last reported state: Finished");
  await expect(progress.getByRole("progressbar")).toHaveAttribute("value", "100");
  await expect(progress).toContainText("Terrain outside this area may still need generation");
});

test("failed world lookup retries as a read without sending Chunky commands", async ({ page }) => {
  const calls = await mockAPI(page, { initiallyRunning: true });
  let reads = 0;
  await page.route(`http://localhost:8080/servers/${serverID}/addons/chunky/worlds`, (route) => {
    reads++;
    return reads === 1
      ? route.fulfill({ status: 503, json: { error: "World lookup unavailable" } })
      : route.fulfill({ json: [{ name: "Campfire", label: "Main world" }] });
  });
  await page.route(`http://localhost:8080/servers/${serverID}/plugins`, (route) =>
    route.fulfill({ json: [{ name: "Chunky-Bukkit-1.5.3.jar", size: 1234, is_file: true }] }),
  );
  await page.routeWebSocket(`ws://localhost:8080/servers/${serverID}/console`, (ws) =>
    ws.send(JSON.stringify({ event: "connected" })),
  );
  await login(page);
  await page.goto(`/servers/${serverID}/plugins`);
  await page.getByText("Terrain settings", { exact: true }).click();
  await page.getByRole("button", { name: "Retry world lookup" }).click();
  await expect(page.getByRole("combobox", { name: "Choose a world" })).toContainText(
    "Main world · Campfire",
  );
  expect(reads).toBe(2);
  expect(calls.filter((call) => call.path.endsWith("/commands"))).toHaveLength(0);
});

test("dirty file editor confirms closing and keeps edits when cancelled", async ({ page }) => {
  await mockAPI(page);
  await login(page);
  await page.goto(`/servers/${serverID}/files`);
  await page.getByRole("button", { name: "server.properties", exact: true }).click();
  const content = page.getByRole("textbox", { name: "File content" });
  await expect(page.getByRole("button", { name: "Save file", exact: true })).toBeDisabled();
  await content.fill("motd=Keep my work");
  const prevented = () =>
    page.evaluate(() => {
      const event = new Event("beforeunload", { cancelable: true });
      window.dispatchEvent(event);
      return event.defaultPrevented;
    });
  expect(await prevented()).toBe(true);
  await page.getByRole("button", { name: "Close editor", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "Discard unsaved file edits?" });
  await expect(dialog).toBeVisible();
  await dialog.getByRole("button", { name: "Cancel", exact: true }).click();
  await expect(content).toHaveValue("motd=Keep my work");
  await page.getByRole("button", { name: "Close editor", exact: true }).click();
  await dialog.getByRole("button", { name: "Discard and continue", exact: true }).click();
  await expect(content).toHaveCount(0);
  expect(await prevented()).toBe(false);
});

test("dirty file editor guards reload and folder navigation", async ({ page }) => {
  const calls = await mockAPI(page);
  await login(page);
  await page.goto(`/servers/${serverID}/files`);
  await page.getByRole("button", { name: "server.properties", exact: true }).click();
  const content = page.getByRole("textbox", { name: "File content" });
  await content.fill("motd=Keep my work");
  const reads = () => calls.filter((call) => call.path.endsWith("/files/text")).length;
  expect(reads()).toBe(1);
  await page.getByRole("button", { name: "Reload file", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "Discard unsaved file edits?" });
  await expect(dialog).toBeVisible();
  expect(reads()).toBe(1);
  await dialog.getByRole("button", { name: "Discard and continue", exact: true }).click();
  await expect(content).toHaveValue("motd=Welcome friends\n");
  await expect(page.getByRole("button", { name: "Save file", exact: true })).toBeDisabled();
  await content.fill("motd=Another change");
  await page.getByRole("button", { name: "world", exact: true }).click();
  await expect(dialog).toBeVisible();
  await expect(page.locator(".file-path code")).toHaveText("/");
  await dialog.getByRole("button", { name: "Cancel", exact: true }).click();
  await expect(content).toHaveValue("motd=Another change");
  await page.getByRole("button", { name: "world", exact: true }).click();
  await dialog.getByRole("button", { name: "Discard and continue", exact: true }).click();
  await expect(page.locator(".file-path code")).toHaveText("/world");
  await expect(content).toHaveCount(0);
});

test("file save retry retains content and key and locks edits during the request", async ({
  page,
}) => {
  await mockAPI(page);
  const saves = [];
  let release;
  await page.route(`http://localhost:8080/servers/${serverID}/files`, async (route) => {
    if (route.request().method() !== "POST") return route.fallback();
    saves.push({
      key: route.request().headers()["idempotency-key"],
      body: route.request().postDataJSON(),
    });
    if (saves.length === 1)
      return route.fulfill({ status: 503, json: { error: "Save temporarily unavailable" } });
    await new Promise((resolve) => {
      release = resolve;
    });
    return route.fulfill({ json: { status: "saved", restart_required: true } });
  });
  await login(page);
  await page.goto(`/servers/${serverID}/files`);
  await page.getByRole("button", { name: "server.properties", exact: true }).click();
  const content = page.getByRole("textbox", { name: "File content" });
  await content.fill("motd=Keep my work");
  await page.getByRole("button", { name: "Save file", exact: true }).click();
  await expect(page.getByText("Save temporarily unavailable", { exact: true })).toBeVisible();
  await expect(content).toHaveValue("motd=Keep my work");
  await page.getByRole("button", { name: "Save file", exact: true }).click();
  await expect.poll(() => !!release).toBe(true);
  await expect(content).toHaveAttribute("readonly", "");
  await expect(page.getByRole("button", { name: "Close editor", exact: true })).toBeDisabled();
  expect(saves[0]).toEqual(saves[1]);
  release();
  await expect(content).toHaveCount(0);
  await expect(page.getByRole("status")).toContainText("File saved");
});

test("Minecraft settings retry preserves edits and disables the form while saving", async ({
  page,
}) => {
  await mockAPI(page);
  const saves = [];
  let release;
  await page.route(`http://localhost:8080/servers/${serverID}/settings`, async (route) => {
    if (route.request().method() !== "PUT") return route.fallback();
    saves.push({
      key: route.request().headers()["idempotency-key"],
      body: route.request().postDataJSON(),
    });
    await new Promise((resolve) => {
      release = resolve;
    });
    return route.fulfill({ status: 503, json: { error: "Settings temporarily unavailable" } });
  });
  await login(page);
  await page.goto(`/servers/${serverID}/settings`);
  const motd = page.getByRole("textbox", { name: "Server message (MOTD)" });
  await motd.fill("Our next adventure");
  await page.getByRole("button", { name: "Save settings", exact: true }).click();
  await expect.poll(() => !!release).toBe(true);
  await expect(motd).toBeDisabled();
  release();
  await expect(motd).toBeEnabled();
  await expect(motd).toHaveValue("Our next adventure");
  release = null;
  await page.getByRole("button", { name: "Save settings", exact: true }).click();
  await expect.poll(() => saves.length).toBe(2);
  expect(saves[0]).toEqual(saves[1]);
  await expect.poll(() => !!release).toBe(true);
  release();
  await expect(motd).toBeEnabled();
});

test("dashboard links offer a themed choice before leaving unsaved file edits", async ({
  page,
}) => {
  await mockAPI(page);
  await login(page);
  await page.goto(`/servers/${serverID}/files`);
  await page.getByRole("button", { name: "server.properties", exact: true }).click();
  const content = page.getByRole("textbox", { name: "File content" });
  await content.fill("motd=Keep my work");
  await page.getByRole("link", { name: "Dashboard", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "Leave without saving?" });
  await expect(dialog).toBeVisible();
  await expect(page).toHaveURL(new RegExp(`/servers/${serverID}/files$`));
  await dialog.getByRole("button", { name: "Stay and keep editing", exact: true }).click();
  await expect(content).toHaveValue("motd=Keep my work");
  await page.getByRole("link", { name: "Dashboard", exact: true }).click();
  await dialog.getByRole("button", { name: "Leave without saving", exact: true }).click();
  await expect(page).toHaveURL(/\/dashboard$/);
});

test("unsaved settings link guard supports Escape and leaves new-tab documentation alone", async ({
  page,
}) => {
  const calls = await mockAPI(page);
  await login(page);
  await page.goto(`/servers/${serverID}/addons/simple-voice-chat`);
  await page.getByLabel("Voice distance", { exact: true }).fill("90");
  await page.getByRole("link", { name: "Dashboard", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "Leave without saving?" });
  await expect(dialog).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(dialog).toHaveCount(0);
  await expect(page.getByLabel("Voice distance", { exact: true })).toHaveValue("90");
  await page.evaluate(() => {
    const anchor = document.createElement("a");
    anchor.href = "https://example.com/docs";
    anchor.target = "_blank";
    anchor.download = "guide";
    // Prevent actual downloads while still exercising the page's click listener.
    anchor.addEventListener("click", (event) => event.preventDefault());
    document.body.appendChild(anchor);
    anchor.click();
    anchor.remove();
  });
  await expect(dialog).toHaveCount(0);
  expect(
    calls.filter(
      (call) => call.method === "PUT" && call.path.endsWith("/addons/simple-voice-chat"),
    ),
  ).toHaveLength(0);
  await page.getByRole("button", { name: "Discard changes", exact: true }).click();
  await page.getByRole("link", { name: "Dashboard", exact: true }).click();
  await expect(page).toHaveURL(/\/dashboard$/);
});

test("installed addon labels and guided filters preserve filenames and permissions", () => {
  expect(installedAddonInfo("voicechat-fabric-2.6.jar")).toEqual({
    name: "Simple Voice Chat",
    guide: "voice",
  });
  expect(installedAddonInfo("Chunky-Bukkit-1.5.3.jar")).toEqual({
    name: "Chunky",
    guide: "chunky",
  });
  expect(installedAddonInfo("my-custom-addon.jar").name).toBe("my-custom-addon.jar");
  expect(hasGuidedSetup("voicechat.jar", ["console"])).toBe(false);
  expect(hasGuidedSetup("Chunky.jar", ["console"])).toBe(true);
  const files = [{ name: "voicechat-fabric-2.6.jar" }, { name: "other.jar" }];
  expect(filterInstalledAddons(files, "simple voice", false, ["settings"])).toEqual([files[0]]);
  expect(filterInstalledAddons(files, "", true, ["console"])).toEqual([]);
});

for (const width of [390, 1366])
  test(`installed addon names and guided setup filters stay clear at ${width}px`, async ({
    page,
  }, testInfo) => {
    const calls = await mockAPI(page);
    await page.setViewportSize({ width, height: 900 });
    await page.route(`http://localhost:8080/servers/${serverID}/plugins`, (route) =>
      route.fulfill({
        json: [
          { name: "voicechat-bukkit-2.6.jar", size: 1234, is_file: true },
          { name: "Chunky-Bukkit-1.5.3.jar", size: 1234, is_file: true },
          { name: "my-custom-addon.jar", size: 1234, is_file: true },
        ],
      }),
    );
    await login(page);
    await page.goto(`/servers/${serverID}/plugins`);
    const rows = page.locator(".plugin-list").first();
    await expect(rows.locator(":scope > li")).toHaveCount(3);
    await expect(rows.getByText("Simple Voice Chat", { exact: true })).toBeVisible();
    await expect(rows.getByText("voicechat-bukkit-2.6.jar", { exact: true })).toBeVisible();
    await page.getByRole("button", { name: "With guided setup (2)", exact: true }).click();
    await expect(rows.locator(":scope > li")).toHaveCount(2);
    await expect(rows.getByText("my-custom-addon.jar", { exact: true })).toHaveCount(0);
    await page.getByLabel("Find an installed plugin").fill("simple voice");
    await expect(rows.locator(":scope > li")).toHaveCount(1);
    await expect(rows).toContainText("voicechat-bukkit-2.6.jar");
    await page.getByLabel("Find an installed plugin").fill("missing");
    await expect(
      page.getByRole("button", { name: "Show all installed", exact: true }),
    ).toBeVisible();
    await page.getByRole("button", { name: "Show all installed", exact: true }).click();
    await expect(rows.locator(":scope > li")).toHaveCount(3);
    const chunky = rows.locator(":scope > li").filter({ hasText: "Chunky-Bukkit-1.5.3.jar" });
    await chunky.getByText("Terrain settings", { exact: true }).click();
    await expect(
      chunky.getByRole("link", { name: "Configuration files", exact: true }),
    ).toHaveCount(1);
    expect(
      calls.filter(
        (call) =>
          ["POST", "PUT", "DELETE"].includes(call.method) &&
          call.path.startsWith(`/servers/${serverID}/`) &&
          !call.path.endsWith("/console-ticket"),
      ),
    ).toHaveLength(0);
    expect(
      await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
    ).toBe(true);
    await page.screenshot({
      path: testInfo.outputPath(`installed-setup-${width}.png`),
      fullPage: true,
    });
  });

for (const width of [390, 1366])
  test(`file search and sorting keep folders first without changing files at ${width}px`, async ({
    page,
  }, testInfo) => {
    const calls = await mockAPI(page);
    await page.setViewportSize({ width, height: 900 });
    let reads = 0;
    await page.route(`http://localhost:8080/servers/${serverID}/files?**`, (route) => {
      reads++;
      return route.fulfill({
        json: [
          { name: "server.properties", is_file: true, size: 100 },
          { name: "world", is_file: false, size: 0 },
          { name: "backup10.zip", is_file: true, size: 2000 },
          { name: "backup2.zip", is_file: true, size: 500 },
          { name: "config", is_file: false, size: 0 },
        ],
      });
    });
    await login(page);
    await page.goto(`/servers/${serverID}/files`);
    const names = page.locator(".management-table .file-name");
    await expect(names).toHaveText([
      "config",
      "world",
      "backup2.zip",
      "backup10.zip",
      "server.properties",
    ]);
    await selectValue(page, page.getByRole("combobox", { name: "Sort files" }), "size");
    await expect(names).toHaveText([
      "config",
      "world",
      "backup10.zip",
      "backup2.zip",
      "server.properties",
    ]);
    await page.getByLabel("Find in this folder").fill("BACKUP");
    await expect(names).toHaveText(["backup10.zip", "backup2.zip"]);
    await page.getByLabel("Find in this folder").fill("absent");
    await expect(page.getByRole("button", { name: "Clear file search" })).toBeVisible();
    await page.getByRole("button", { name: "Clear file search" }).click();
    await expect(names).toHaveCount(5);
    expect(reads).toBe(1);
    expect(
      calls.filter((call) => call.path.endsWith("/files") && call.method !== "GET"),
    ).toHaveLength(0);
    expect(
      await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
    ).toBe(true);
    await page.screenshot({
      path: testInfo.outputPath(`file-browser-${width}.png`),
      fullPage: true,
    });
  });

test("folder breadcrumbs return to ancestors only after reviewing unsaved edits", async ({
  page,
}) => {
  await mockAPI(page);
  await page.route(`http://localhost:8080/servers/${serverID}/files?**`, (route) => {
    const dir = new URL(route.request().url()).searchParams.get("path");
    return route.fulfill({
      json:
        dir === "/"
          ? [{ name: "config", is_file: false, size: 0 }]
          : dir === "/config"
            ? [{ name: "voicechat", is_file: false, size: 0 }]
            : [{ name: "voicechat-server.properties", is_file: true, size: 24 }],
    });
  });
  await page.route(`http://localhost:8080/servers/${serverID}/files/text?**`, (route) =>
    route.fulfill({
      json: {
        path: "/config/voicechat/voicechat-server.properties",
        content: "port=24454\\n",
        revision: "b".repeat(64),
      },
    }),
  );
  await login(page);
  await page.goto(`/servers/${serverID}/files`);
  await page.getByRole("button", { name: "config", exact: true }).click();
  await page.getByRole("button", { name: "voicechat", exact: true }).click();
  const trail = page.getByRole("navigation", { name: "Folder location" });
  await expect(trail.locator('[aria-current="page"]')).toHaveText("voicechat");
  await page.getByRole("button", { name: "voicechat-server.properties", exact: true }).click();
  const text = page.getByRole("textbox", { name: "File content" });
  await text.fill("port=24455");
  await trail.getByRole("button", { name: "Server files", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "Discard unsaved file edits?" });
  await expect(dialog).toBeVisible();
  await dialog.getByRole("button", { name: "Cancel", exact: true }).click();
  await expect(page.locator(".file-path code")).toHaveText("/config/voicechat");
  await expect(text).toHaveValue("port=24455");
  await trail.getByRole("button", { name: "Server files", exact: true }).click();
  await dialog.getByRole("button", { name: "Discard and continue", exact: true }).click();
  await expect(page.locator(".file-path code")).toHaveText("/");
  await expect(text).toHaveCount(0);
});

test("failed file listing offers refresh without claiming an empty folder", async ({ page }) => {
  await mockAPI(page);
  let reads = 0;
  await page.route(`http://localhost:8080/servers/${serverID}/files?**`, (route) => {
    reads++;
    return reads === 1
      ? route.fulfill({ status: 503, json: { error: "File listing unavailable" } })
      : route.fulfill({ json: [] });
  });
  await login(page);
  await page.goto(`/servers/${serverID}/files`);
  await expect(
    page.getByText("Couldn’t load this folder. Use Refresh to try again."),
  ).toBeVisible();
  await expect(page.getByText("This folder is empty.", { exact: true })).toHaveCount(0);
  await page.getByRole("button", { name: "Refresh", exact: true }).click();
  await expect(page.getByText("This folder is empty.", { exact: true })).toBeVisible();
  expect(reads).toBe(2);
});

test("file sizes distinguish small files while preserving memory and disk units", () => {
  expect(formatBytes(0)).toBe("0 B");
  expect(formatBytes(100)).toBe("100 B");
  expect(formatBytes(2048)).toBe("2.0 KB");
  expect(formatBytes(1048576)).toBe("1.0 MB");
  expect(formatBytes(1073741824)).toBe("1.0 GB");
  for (const unknown of [null, undefined, NaN, Infinity, -1])
    expect(formatBytes(unknown)).toBe("—");
});

test("find-in-file treats patterns literally, preserves Unicode offsets and bounds results", () => {
  expect(findInFile("voice.host voiceXhost VOICE.HOST", "voice.host").matches).toEqual([
    { start: 0, end: 10 },
    { start: 22, end: 32 },
  ]);
  expect(findInFile("x [a] x", "[a]").matches).toEqual([{ start: 2, end: 5 }]);
  expect(findInFile("İ😀 setting=yes", "setting").matches).toEqual([{ start: 4, end: 11 }]);
  const result = findInFile("a".repeat(1001), "a");
  expect(result.matches).toHaveLength(1000);
  expect(result.truncated).toBe(true);
  expect(findInFile("a".repeat(1000), "a").truncated).toBe(false);
  expect(findInFile("abc", "").matches).toEqual([]);
});

for (const width of [390, 1366])
  test(`file editor search highlights settings without editing the file at ${width}px`, async ({
    page,
  }, testInfo) => {
    const calls = await mockAPI(page);
    await page.setViewportSize({ width, height: 900 });
    const content =
      "# Voice settings\nvoice_host=play.example.com\nother=voiceXhost\nvoice_host=backup.example.com\n";
    await page.route(`http://localhost:8080/servers/${serverID}/files/text?**`, (route) =>
      route.fulfill({ json: { path: "/server.properties", content, revision: "b".repeat(64) } }),
    );
    await login(page);
    await page.goto(`/servers/${serverID}/files`);
    await page.getByRole("button", { name: "server.properties", exact: true }).click();
    const editor = page.getByRole("textbox", { name: "File content" });
    await page.getByRole("button", { name: "Find in file", exact: true }).click();
    const query = page.getByRole("searchbox", { name: "Find text in this file" });
    await expect(query).toBeFocused();
    await query.fill("VOICE_HOST");
    await expect(page.getByText("2 matches", { exact: true })).toBeVisible();
    await query.press("Enter");
    await expect(editor).toBeFocused();
    expect(await editor.evaluate((el) => el.selectionStart)).toBe(content.indexOf("voice_host"));
    await page.getByRole("button", { name: "Next match", exact: true }).click();
    expect(await editor.evaluate((el) => el.selectionStart)).toBe(
      content.lastIndexOf("voice_host"),
    );
    await page.getByRole("button", { name: "Next match", exact: true }).click();
    expect(await editor.evaluate((el) => el.selectionStart)).toBe(content.indexOf("voice_host"));
    await page.getByRole("button", { name: "Previous match", exact: true }).click();
    expect(await editor.evaluate((el) => el.selectionStart)).toBe(
      content.lastIndexOf("voice_host"),
    );
    await expect(editor).toHaveValue(content);
    await expect(page.getByRole("button", { name: "Save file", exact: true })).toBeDisabled();
    expect(
      await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
    ).toBe(true);
    await page.screenshot({
      path: testInfo.outputPath(`find-in-file-${width}.png`),
      fullPage: true,
    });
    await query.fill("not_here");
    await expect(page.getByText("No matches in this file.", { exact: true })).toBeVisible();
    await expect(page.getByRole("button", { name: "Next match", exact: true })).toBeDisabled();
    await query.press("Escape");
    await expect(query).toHaveCount(0);
    await expect(editor).toBeFocused();
    await page.getByRole("button", { name: "Close editor", exact: true }).click();
    await expect(page.getByRole("dialog")).toHaveCount(0);
    expect(
      calls.filter((call) => call.path.endsWith("/files") && call.method !== "GET"),
    ).toHaveLength(0);
  });

test("file editor shortcut opens search and editing recalculates matches safely", async ({
  page,
}) => {
  await mockAPI(page);
  await login(page);
  await page.goto(`/servers/${serverID}/files`);
  await page.getByRole("button", { name: "server.properties", exact: true }).click();
  const editor = page.getByRole("textbox", { name: "File content" });
  await editor.press("Control+f");
  const query = page.getByRole("searchbox", { name: "Find text in this file" });
  await expect(query).toBeFocused();
  await query.fill("motd");
  await page.getByRole("button", { name: "Next match", exact: true }).click();
  await editor.fill("difficulty=normal\n");
  await expect(page.getByText("No matches in this file.", { exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: "Next match", exact: true })).toBeDisabled();
  await expect(page.getByRole("button", { name: "Save file", exact: true })).toBeEnabled();
});

test("backup history failure stays distinct from empty and refresh recovers", async ({ page }) => {
  const calls = await mockAPI(page);
  let fail = true;
  await page.route(`http://localhost:8080/servers/${serverID}/backups`, (route) =>
    fail
      ? route.fulfill({ status: 503, json: { error: "History unavailable" } })
      : route.fulfill({ json: [] }),
  );
  await login(page);
  await page.goto(`/servers/${serverID}/backups`);
  await expect(page.getByRole("heading", { name: "Backup status unavailable" })).toBeVisible();
  await expect(page.getByText(/Couldn’t refresh backup history/)).toBeVisible();
  await expect(page.getByText(/No backups yet\. Save a copy/)).toHaveCount(0);
  await page.getByLabel("Backup name", { exact: true }).fill("Before our adventure");
  await expect(page.getByRole("button", { name: "Create backup", exact: true })).toBeDisabled();
  fail = false;
  await page.getByRole("button", { name: "Refresh", exact: true }).click();
  await expect(page.getByText(/No backups yet\. Save a copy/)).toBeVisible();
  await expect(page.getByRole("button", { name: "Create backup", exact: true })).toBeEnabled();
  expect(
    calls.filter((call) => call.path.includes("/backups") && call.method !== "GET"),
  ).toHaveLength(0);
});

test("stale backup history disables deletion confirmation until a successful refresh", async ({
  page,
}) => {
  const calls = await mockAPI(page);
  await page.clock.install();
  let fail = false;
  await page.route(`http://localhost:8080/servers/${serverID}/backups`, (route) =>
    fail
      ? route.fulfill({ status: 503, json: { error: "History unavailable" } })
      : route.fallback(),
  );
  await login(page);
  await page.goto(`/servers/${serverID}/backups`);
  await expect(page.getByText("Before changes", { exact: true })).toBeVisible();
  await page.getByRole("button", { name: "Delete", exact: true }).click();
  const dialog = page.getByRole("dialog");
  const confirm = dialog.getByRole("button", { name: "Delete backup", exact: true });
  await expect(confirm).toBeEnabled();
  fail = true;
  await page.clock.fastForward(10001);
  await expect(page.getByText(/Entries below are from the last successful check/)).toBeVisible();
  await expect(confirm).toBeDisabled();
  await expect(dialog).toContainText("This backup could not be verified");
  await expect(page.getByRole("button", { name: "Download", exact: true })).toBeDisabled();
  await expect(page.getByRole("button", { name: "Restore", exact: true })).toBeDisabled();
  await expect(page.getByRole("button", { name: "Delete", exact: true })).toBeDisabled();
  await dialog.getByRole("button", { name: "Cancel", exact: true }).click();
  fail = false;
  await page.getByRole("button", { name: "Refresh", exact: true }).click();
  await expect(page.getByText(/Entries below are from the last successful check/)).toHaveCount(0);
  await expect(page.getByRole("button", { name: "Delete", exact: true })).toBeEnabled();
  expect(
    calls.filter((call) => call.path.includes("/backups") && call.method !== "GET"),
  ).toHaveLength(0);
});

test("diagnostic report excludes private fields, invalid numbers and stale readings", () => {
  const input = {
    server: {
      id: serverID,
      server_type: "PAPER",
      cpu_limit: 200,
      ram_mb: 4096,
      disk_mb: 20480,
      name: "PRIVATE-NAME",
      hostname: "PRIVATE-HOST",
      password: "PRIVATE-SECRET",
    },
    ready: true,
    powerState: "running",
    symptom: "blocks",
    capturedAt: "2026-10-15T10:00:00.000Z",
    usage: {
      cpu_percent: 90,
      memory_bytes: 1048576,
      disk_bytes: 2097152,
      uptime_ms: 10000,
      logs: "PRIVATE-LOG",
      players: ["PRIVATE-PLAYER"],
    },
  };
  const report = diagnosticReport(input);
  expect(report).toContain("CPU use: 90%");
  expect(report).toContain("Memory use: 1 MB");
  expect(report).toContain("Process uptime: 10 seconds");
  expect(report).toContain("Minecraft tick rate: Not measured");
  expect(report).not.toContain("PRIVATE-");
  const unavailable = diagnosticReport({ ...input, usageError: new Error("PRIVATE-ERROR") });
  expect(unavailable).toContain("no stale readings included");
  expect(unavailable).toContain("CPU use: Unavailable");
  expect(unavailable).toContain("Observed process state: unavailable");
  expect(unavailable).not.toContain("PRIVATE-");
  for (const value of [null, undefined, NaN, Infinity, -1, "99"]) {
    expect(
      diagnosticReport({ ...input, usage: { cpu_percent: value, memory_bytes: value } }),
    ).toContain("CPU use: Unavailable");
    expect(
      diagnosticReport({ ...input, usage: { cpu_percent: value, memory_bytes: value } }),
    ).toContain("Memory use: Unavailable");
  }
});

for (const width of [390, 1366]) {
  test(`lag guidance and diagnostic report work at ${width}px`, async ({ page }) => {
    const calls = await mockAPI(page, { initiallyRunning: true });
    await page.setViewportSize({ width, height: 900 });
    await page.addInitScript(() => {
      Object.defineProperty(navigator, "clipboard", {
        configurable: true,
        value: {
          writeText: async (text) => {
            window.copiedDiagnostics = text;
          },
        },
      });
    });
    await openServer(page);
    await page.getByRole("button", { name: "Health", exact: true }).click();
    const card = page.getByRole("region", { name: "Server health" });
    await expect(card).toContainText("Everyone lagging?");
    await selectValue(page, card.getByLabel("What do players notice?"), "fps");
    await expect(card).toContainText("Lower client render distance");
    await expect(card.getByRole("link", { name: "Open Minecraft settings →" })).toHaveCount(0);
    await selectValue(page, card.getByLabel("What do players notice?"), "chunks");
    await expect(card).toContainText("Pause terrain generation");
    await card.getByRole("button", { name: "Copy diagnostic report" }).click();
    await expect(card.getByRole("status")).toContainText("Report copied.");
    const preview = card.getByLabel("Diagnostic report preview");
    const report = await preview.inputValue();
    expect(report).toContain("Player-reported symptom: Slow chunk loading");
    expect(report).toContain("Player ping / packet loss: Not measured");
    expect(await page.evaluate(() => window.copiedDiagnostics)).toBe(report);
    expect(await preview.getAttribute("readonly")).not.toBeNull();
    await expect(card.getByRole("link", { name: "Open Minecraft settings →" })).toBeVisible();
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
      true,
    );
    await page.screenshot({
      path: test.info().outputPath(`lag-help-${width}.png`),
      fullPage: true,
    });
    expect(
      calls.filter(
        (call) =>
          call.method !== "GET" &&
          !call.path.startsWith("/auth/") &&
          !call.path.endsWith("/console-ticket"),
      ),
    ).toHaveLength(0);
  });
}

test("diagnostic copy fallback shows a selectable report without stale usage", async ({ page }) => {
  await mockAPI(page, { initiallyRunning: true });
  await page.addInitScript(() => {
    Object.defineProperty(navigator, "clipboard", {
      configurable: true,
      value: {
        writeText: async () => {
          throw new Error("Denied");
        },
      },
    });
  });
  await page.route(`http://localhost:8080/servers/${serverID}/usage`, (route) =>
    route.fulfill({ status: 503, json: { error: "Readings unavailable" } }),
  );
  await openServer(page);
  await page.getByRole("button", { name: "Health", exact: true }).click();
  const card = page.getByRole("region", { name: "Server health" });
  await card.getByRole("button", { name: "Copy diagnostic report" }).click();
  await expect(card.getByRole("status")).toContainText("Select and copy the report below manually");
  await expect(card.getByLabel("Diagnostic report preview")).toHaveValue(/CPU use: Unavailable/);
});

for (const width of [390, 1366]) {
  test(`creation wizard keeps one step visible and preserves choices at ${width}px`, async ({
    page,
  }, testInfo) => {
    const calls = await mockAPI(page);
    await page.setViewportSize({ width, height: 900 });
    await login(page);
    await page.goto("/servers/new");
    await expect(page.getByRole("heading", { name: "How do you want to play?" })).toBeVisible();
    await expect(page.getByLabel("Server name", { exact: true })).toHaveCount(0);
    await expect(page.locator(".group-picker")).toHaveCount(0);
    await page.getByRole("button", { name: /Minecraft with mods/ }).click();
    await page.locator("form").dispatchEvent("submit");
    expect(calls.filter((call) => call.path === "/orders" && call.method === "POST")).toHaveLength(
      0,
    );
    await page.screenshot({
      path: testInfo.outputPath(`create-style-${width}.png`),
      fullPage: true,
    });
    await page.getByRole("button", { name: "Next", exact: true }).click();
    await expect(page.getByRole("heading", { name: "Who’s joining?" })).toBeFocused();
    await expect(page.locator(".play-style-grid")).toHaveCount(0);
    await page.getByRole("button", { name: /Game night/ }).click();
    await page.screenshot({
      path: testInfo.outputPath(`create-group-${width}.png`),
      fullPage: true,
    });
    await page.getByRole("button", { name: "Next", exact: true }).click();
    await page.getByLabel("Server name", { exact: true }).fill("Our modded world");
    await expect(page.locator(".order-summary")).toContainText("Fabric · Minecraft 1.21.1");
    await expect(page.locator(".creation-advanced")).not.toHaveAttribute("open", "");
    await page.getByRole("button", { name: "Back", exact: true }).click();
    await expect(page.getByRole("button", { name: /Game night/ })).toHaveAttribute(
      "aria-pressed",
      "true",
    );
    await page.getByRole("button", { name: "Next", exact: true }).click();
    await expect(page.getByLabel("Server name", { exact: true })).toHaveValue("Our modded world");
    await page.screenshot({
      path: testInfo.outputPath(`create-review-${width}.png`),
      fullPage: true,
    });
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
      true,
    );
    expect(calls.filter((call) => call.path === "/orders" && call.method === "POST")).toHaveLength(
      0,
    );
    await page.getByRole("button", { name: "Continue to checkout" }).click();
    await expect(page).toHaveURL(/checkout/);
    expect(calls.find((call) => call.path === "/orders" && call.method === "POST").body).toEqual({
      plan_id: planID,
      name: "Our modded world",
      minecraft_version: "1.21.1",
      server_type: "FABRIC",
    });
  });
}

test("wizard uses approved available software and rejects disabled plan overrides", async ({
  page,
}) => {
  await mockAPI(page);
  await page.route("http://localhost:8080/minecraft/options", (route) =>
    route.fulfill({ json: [{ server_type: "FORGE", minecraft_version: "1.21.1" }] }),
  );
  await page.route("http://localhost:8080/plans", (route) =>
    route.fulfill({
      json: [
        plan,
        {
          ...plan,
          id: "10000000-0000-4000-8000-000000000099",
          name: "Disabled cheap plan",
          enabled: false,
          price_cents: 1,
        },
      ],
    }),
  );
  await login(page);
  await page.goto("/servers/new?plan=10000000-0000-4000-8000-000000000099");
  await expect(page.getByRole("button", { name: /Regular Minecraft/ })).toBeDisabled();
  await expect(page.getByRole("button", { name: /Minecraft with mods/ })).toHaveAttribute(
    "aria-pressed",
    "true",
  );
  await reviewCreation(page);
  await expect(page.locator(".order-summary")).toContainText("Forge · Minecraft 1.21.1");
  await page.getByText("Advanced settings", { exact: true }).click();
  await page.getByRole("combobox", { name: "Hosting plan", exact: true }).click();
  await expect(page.getByRole("option")).toHaveCount(1);
  await expect(page.getByRole("option", { name: /Disabled cheap plan/ })).toHaveCount(0);
});

test("API deadlines cover headers, JSON bodies and downloads without retrying", async () => {
  const original = globalThis.fetch;
  const waitForAbort = (signal) =>
    new Promise((resolve, reject) => {
      signal.addEventListener("abort", () => reject(new DOMException("Aborted", "AbortError")), {
        once: true,
      });
    });
  try {
    for (const phase of ["headers", "json", "download"]) {
      let attempts = 0;
      globalThis.fetch = async (url, { signal }) => {
        attempts++;
        if (phase === "headers") return waitForAbort(signal);
        return {
          ok: true,
          status: 200,
          headers: new Headers({ "X-Request-ID": "safe-reference" }),
          json: () => waitForAbort(signal),
          blob: () => waitForAbort(signal),
        };
      };
      await expect(
        apiRequest("/slow", { timeoutMs: 20, method: "POST", download: phase === "download" }),
      ).rejects.toMatchObject({
        status: 0,
        code: "REQUEST_TIMEOUT",
        message: "Request timed out. Check the result before retrying.",
      });
      expect(attempts).toBe(1);
    }
  } finally {
    globalThis.fetch = original;
  }
});

test("API cancellation stays distinct from timeout and malformed success is rejected", async () => {
  const original = globalThis.fetch;
  try {
    const cancelled = new AbortController();
    cancelled.abort();
    let attempts = 0;
    globalThis.fetch = async () => {
      attempts++;
      return new Response("not json", { status: 200 });
    };
    await expect(apiRequest("/cancelled", { signal: cancelled.signal })).rejects.toMatchObject({
      name: "AbortError",
    });
    expect(attempts).toBe(0);
    await expect(apiRequest("/invalid")).rejects.toMatchObject({
      code: "INVALID_RESPONSE",
      status: 502,
    });
    globalThis.fetch = async () =>
      new Response(JSON.stringify({ error: "Slow down" }), {
        status: 429,
        headers: { "Retry-After": "120", "X-Request-ID": "request-reference" },
      });
    await expect(apiRequest("/limited")).rejects.toMatchObject({
      status: 429,
      retryAfterMs: 120000,
      requestId: "request-reference",
    });
  } finally {
    globalThis.fetch = original;
  }
});

test("offline dashboard preserves settings and reconnect does not replay saves or reload successful forms", async ({
  page,
  context,
}) => {
  const calls = await mockAPI(page);
  await login(page);
  await page.goto(`/servers/${serverID}/settings`);
  const message = page.getByRole("textbox", { name: /Server message/ });
  await message.fill("Keep this draft");
  const reads = calls.filter(
    (call) => call.path.endsWith("/settings") && call.method === "GET",
  ).length;
  await context.setOffline(true);
  await expect(
    page.getByText("You’re offline. Reconnect to continue.", { exact: true }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Save settings", exact: true }).click();
  await expect(
    page.getByText("You’re offline. Reconnect and try again.", { exact: true }),
  ).toBeVisible();
  await expect(message).toHaveValue("Keep this draft");
  await expect(page.getByRole("button", { name: "Save settings", exact: true })).toBeEnabled();
  expect(
    calls.filter((call) => call.path.endsWith("/settings") && call.method === "PUT"),
  ).toHaveLength(0);
  await context.setOffline(false);
  await expect(
    page.getByText("You’re offline. Reconnect to continue.", { exact: true }),
  ).toHaveCount(0);
  await expect(message).toHaveValue("Keep this draft");
  expect(
    calls.filter((call) => call.path.endsWith("/settings") && call.method === "GET"),
  ).toHaveLength(reads);
  expect(
    calls.filter((call) => call.path.endsWith("/settings") && call.method === "PUT"),
  ).toHaveLength(0);
  await page.getByRole("button", { name: "Save settings", exact: true }).click();
  await expect(page.getByRole("status")).toContainText("Settings saved");
  expect(
    calls.filter((call) => call.path.endsWith("/settings") && call.method === "PUT"),
  ).toHaveLength(1);
});

test("failed one-time setup read recovers on reconnect without repeating writes", async ({
  page,
}) => {
  const calls = await mockAPI(page);
  let reads = 0;
  await page.route("http://localhost:8080/minecraft/options", (route) => {
    reads++;
    return reads === 1
      ? route.fulfill({ status: 503, json: { error: "Setup temporarily unavailable" } })
      : route.fulfill({ json: [{ server_type: "PAPER", minecraft_version: "26.3" }] });
  });
  await login(page);
  await page.goto("/servers/new");
  await expect(page.getByText("Setup temporarily unavailable", { exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: "Next", exact: true })).toBeDisabled();
  await page.evaluate(() => window.dispatchEvent(new Event("online")));
  await expect(page.getByRole("button", { name: "Next", exact: true })).toBeEnabled();
  expect(reads).toBe(2);
  expect(calls.filter((call) => call.path === "/orders" && call.method === "POST")).toHaveLength(0);
});

test("timed-out file save releases the button and retains its draft and retry key", async ({
  page,
}) => {
  await mockAPI(page);
  const saves = [];
  let pending;
  await page.route(`http://localhost:8080/servers/${serverID}/files`, (route) => {
    if (route.request().method() !== "POST") return route.fallback();
    saves.push({
      key: route.request().headers()["idempotency-key"],
      body: route.request().postDataJSON(),
    });
    if (saves.length === 1) {
      pending = route;
      return;
    }
    return route.fulfill({ json: { status: "saved", restart_required: true } });
  });
  await login(page);
  await page.goto(`/servers/${serverID}/files`);
  await page.getByRole("button", { name: "server.properties", exact: true }).click();
  await page.clock.install();
  const text = page.getByRole("textbox", { name: "File content" });
  await text.fill("motd=Reliable draft");
  await page.getByRole("button", { name: "Save file", exact: true }).click();
  await expect.poll(() => saves.length).toBe(1);
  await page.clock.fastForward(30001);
  await expect(
    page.getByText("Request timed out. Check the result before retrying.", { exact: true }),
  ).toBeVisible();
  await expect(text).toHaveValue("motd=Reliable draft");
  await expect(text).not.toHaveAttribute("readonly", "");
  await pending.abort().catch(() => {});
  await page.getByRole("button", { name: "Save file", exact: true }).click();
  await expect(page.getByRole("status")).toContainText("File saved");
  expect(saves).toHaveLength(2);
  expect(saves[1]).toEqual(saves[0]);
});

test("reconnect during an unfinished read recovers once if that read fails", async ({ page }) => {
  await mockAPI(page);
  let pending;
  let reads = 0;
  await page.route("http://localhost:8080/minecraft/options", (route) => {
    reads++;
    if (reads === 1) {
      pending = route;
      return;
    }
    return route.fulfill({ json: [{ server_type: "PAPER", minecraft_version: "26.3" }] });
  });
  await login(page);
  await page.goto("/servers/new");
  await expect.poll(() => !!pending).toBe(true);
  await page.evaluate(() => window.dispatchEvent(new Event("online")));
  await pending.abort();
  await expect(page.getByRole("button", { name: "Next", exact: true })).toBeEnabled();
  expect(reads).toBe(2);
});

test("returning dashboard omits empty panels and does not fetch the plan catalogue", async ({
  page,
}) => {
  const calls = await mockAPI(page);
  await login(page);
  await expect(page.getByRole("heading", { name: "Continue playing", exact: true })).toBeVisible();
  await expect(page.getByRole("region", { name: "Notifications", exact: true })).toHaveCount(0);
  await expect(page.getByText("Hosting details", { exact: true })).toHaveCount(0);
  await expect(page.getByText("Private beta help", { exact: true })).toHaveCount(0);
  await expect(page.locator(".plans-section")).toHaveCount(0);
  await expect(page.getByRole("link", { name: "Open my world", exact: false })).toHaveAttribute(
    "href",
    `/servers/${serverID}`,
  );
  expect(calls.filter((call) => call.path === "/plans")).toHaveLength(0);
});

test("notification dismissal preserves other items and only hides a confirmed write", async ({
  page,
}) => {
  await mockAPI(page);
  const first = "60000000-0000-4000-8000-000000000001";
  const second = "60000000-0000-4000-8000-000000000002";
  let attempts = 0;
  let reads = 0;
  await page.route("http://localhost:8080/notifications", (route) => {
    reads++;
    return route.fulfill({
      json: [
        {
          id: first,
          server_id: serverID,
          server_name: baseServer.name,
          title: "First alert",
          created_at: "2026-10-15T10:00:00Z",
        },
        {
          id: second,
          server_id: serverID,
          server_name: baseServer.name,
          title: "Second alert",
          created_at: "2026-10-15T11:00:00Z",
        },
      ],
    });
  });
  await page.route(`http://localhost:8080/notifications/${first}/read`, (route) => {
    attempts++;
    return attempts === 1
      ? route.fulfill({ status: 503, json: { error: "Could not mark this notification read" } })
      : route.fulfill({ json: { read: true } });
  });
  await login(page);
  const panel = page.getByRole("region", { name: "Notifications", exact: true });
  const row = panel.locator("li").filter({ hasText: "First alert" });
  await row.getByRole("button", { name: "Mark read", exact: true }).click();
  await expect(panel.getByRole("alert")).toContainText("Could not mark this notification read");
  await expect(row).toBeVisible();
  await row.getByRole("button", { name: "Mark read", exact: true }).click();
  await expect(row).toHaveCount(0);
  await expect(panel.getByText("Second alert", { exact: true })).toBeVisible();
  await expect(panel.getByText("1 new", { exact: true })).toBeVisible();
  expect(attempts).toBe(2);
  expect(reads).toBe(1);
});

test("notification history errors stay distinct from an empty inbox and can be refreshed", async ({
  page,
}) => {
  await mockAPI(page);
  let failed = true;
  await page.route("http://localhost:8080/notifications", (route) =>
    failed
      ? route.fulfill({ status: 503, json: { error: "Notifications unavailable" } })
      : route.fulfill({ json: [] }),
  );
  await login(page);
  const panel = page.getByRole("region", { name: "Notifications", exact: true });
  await expect(panel.getByRole("alert")).toContainText("Notifications unavailable");
  await expect(panel.getByText("You’re all caught up.")).toHaveCount(0);
  failed = false;
  await panel.getByRole("button", { name: "Refresh notifications", exact: true }).click();
  await expect(panel).toHaveCount(0);
});

test("offline server cards never describe a player count as currently playing", async ({
  page,
}) => {
  await mockAPI(page);
  await login(page);
  const card = page
    .getByRole("link")
    .filter({ has: page.getByRole("heading", { name: baseServer.name, exact: true }) });
  await expect(card.getByText("Ready to start", { exact: true })).toBeVisible();
  await expect(card).toContainText("Manage");
  await expect(card).not.toContainText("0 playing");
});

for (const width of [390, 1366]) {
  test(`single-server dashboard stays compact while the server directory keeps filters at ${width}px`, async ({
    page,
  }, testInfo) => {
    await mockAPI(page);
    await page.setViewportSize({ width, height: 900 });
    await login(page);
    await expect(page.getByRole("heading", { name: baseServer.name, exact: true })).toBeVisible();
    await expect(page.getByLabel("Search servers", { exact: true })).toHaveCount(0);
    await expect(page.getByRole("combobox", { name: "Server view", exact: true })).toHaveCount(0);
    await page.screenshot({
      path: testInfo.outputPath(`compact-dashboard-${width}.png`),
      fullPage: true,
    });
    await page.goto("/servers");
    await expect(page.getByLabel("Search servers", { exact: true })).toBeVisible();
    await expect(page.getByRole("combobox", { name: "Server view", exact: true })).toBeVisible();
  });
}

test("owner invitation clipboard fallback stays selectable and excludes management credentials", async ({
  page,
}) => {
  await mockAPI(page);
  await page.addInitScript(() =>
    Object.defineProperty(navigator, "clipboard", {
      configurable: true,
      value: {
        writeText: async () => {
          throw new Error("Denied");
        },
      },
    }),
  );
  await page.route(`http://localhost:8080/servers/${serverID}/player-invite`, (route) =>
    route.fulfill({
      json: { slug: playerSlug, expires_at: "2077-11-13T12:00:00Z", setup: playerSetup },
    }),
  );
  await openServer(page);
  const panel = page.getByRole("region", { name: "Invite your friends" });
  await panel.getByRole("button", { name: "Copy invite link", exact: true }).click();
  const fallback = panel.getByLabel("Invite link", { exact: true });
  await expect(fallback).toHaveValue(new RegExp(`/join/${playerSlug}$`));
  await expect(fallback).toHaveAttribute("readonly", "");
  await fallback.focus();
  expect(await fallback.evaluate((input) => input.selectionEnd - input.selectionStart)).toBe(
    (await fallback.inputValue()).length,
  );
  expect(await fallback.inputValue()).not.toContain("csrf");
  await expect(panel.getByRole("alert")).toHaveCount(0);
});

test("invitation read failure offers a read-only retry without replacing the link", async ({
  page,
}) => {
  const calls = await mockAPI(page);
  let reads = 0;
  await page.route(`http://localhost:8080/servers/${serverID}/player-invite`, (route) => {
    reads++;
    return reads === 1
      ? route.fulfill({ status: 503, json: { error: "Invitation unavailable" } })
      : route.fulfill({ json: null });
  });
  await openServer(page);
  const panel = page.getByRole("region", { name: "Invite your friends" });
  await expect(
    panel.getByRole("button", { name: "Prepare invite link", exact: true }),
  ).toBeDisabled();
  await panel.getByRole("button", { name: "Retry invitation", exact: true }).click();
  await expect(
    panel.getByRole("button", { name: "Prepare invite link", exact: true }),
  ).toBeEnabled();
  expect(reads).toBe(2);
  expect(
    calls.filter((call) => call.path.endsWith("/player-invite") && call.method !== "GET"),
  ).toHaveLength(0);
});

test("cancelled download choices never publish and an unready setup cannot be copied", async ({
  page,
}) => {
  await mockAPI(page);
  let writes = 0;
  await page.route(`http://localhost:8080/servers/${serverID}/player-invite`, (route) => {
    if (route.request().method() !== "GET") writes++;
    return route.fulfill({
      json: {
        slug: playerSlug,
        expires_at: "2077-11-13T12:00:00Z",
        setup: { ...playerSetup, pack_ready: true, pack_reason: "Refresh the changed setup" },
      },
    });
  });
  await openServer(page);
  const panel = page.getByRole("region", { name: "Invite your friends" });
  await expect(panel.getByText("Player setup needs a check", { exact: true })).toBeVisible();
  await expect(panel.getByRole("button", { name: "Copy invite link", exact: true })).toBeDisabled();
  await panel.getByRole("button", { name: "Download options", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "Player download options", exact: true });
  await dialog.getByLabel("Include beta client builds").check();
  await dialog.getByRole("button", { name: "Cancel", exact: true }).click();
  await panel.getByRole("button", { name: "Download options", exact: true }).click();
  await expect(dialog.getByLabel("Include beta client builds")).not.toBeChecked();
  expect(writes).toBe(0);
});

test("simple invite preparation defaults to stable required-only downloads", async ({ page }) => {
  await mockAPI(page);
  let invite = null;
  let published;
  await page.route(`http://localhost:8080/servers/${serverID}/player-invite`, (route) => {
    if (route.request().method() === "POST") {
      published = route.request().postDataJSON();
      invite = {
        slug: playerSlug,
        expires_at: "2077-11-13T12:00:00Z",
        setup: { ...playerSetup, ...published },
      };
    }
    return route.fulfill({ json: invite });
  });
  await openServer(page);
  const panel = page.getByRole("region", { name: "Invite your friends" });
  await expect(panel.getByLabel("Include beta client builds")).toHaveCount(0);
  await panel.getByRole("button", { name: "Prepare invite link", exact: true }).click();
  await expect(panel.getByRole("button", { name: "Copy invite link", exact: true })).toBeEnabled();
  expect(published).toEqual({ include_beta: false, include_optional: false });
});

test("player address copying falls back to a selectable field without a page error", async ({
  page,
}) => {
  await mockAPI(page);
  await page.addInitScript(() =>
    Object.defineProperty(navigator, "clipboard", {
      configurable: true,
      value: {
        writeText: async () => {
          throw new Error("Denied");
        },
      },
    }),
  );
  await page.route(`http://localhost:8080/join/${playerSlug}`, (route) =>
    route.fulfill({ json: { ...playerSetup, addons: [], pack_ready: false } }),
  );
  await page.goto(`/join/${playerSlug}`);
  await page.getByRole("button", { name: "Copy address", exact: true }).click();
  const address = page.getByRole("textbox", { name: "Server address to copy", exact: true });
  await expect(address).toHaveValue(playerSetup.address);
  await address.focus();
  expect(await address.evaluate((input) => input.selectionEnd - input.selectionStart)).toBe(
    playerSetup.address.length,
  );
  await expect(page.locator(".public-player-page").getByRole("alert")).toHaveCount(0);
});

test("public invite pauses refresh in the background and refreshes once on return", async ({
  page,
}) => {
  await mockAPI(page);
  await page.clock.install();
  let reads = 0;
  await page.route(`http://localhost:8080/join/${playerSlug}`, (route) => {
    reads++;
    return route.fulfill({ json: playerSetup });
  });
  await page.goto(`/join/${playerSlug}`);
  await expect(
    page.getByRole("heading", { name: "Join The Lantern Lodge", exact: true }),
  ).toBeVisible();
  await page.evaluate(() => {
    Object.defineProperty(document, "visibilityState", { configurable: true, value: "hidden" });
    document.dispatchEvent(new Event("visibilitychange"));
  });
  await page.clock.fastForward(180001);
  expect(reads).toBe(1);
  await page.evaluate(() => {
    Object.defineProperty(document, "visibilityState", { configurable: true, value: "visible" });
    document.dispatchEvent(new Event("visibilitychange"));
  });
  await page.clock.fastForward(1);
  await expect.poll(() => reads).toBe(2);
});

test("public invite keeps rate-limit cooldown when reconnecting", async ({ page }) => {
  await mockAPI(page);
  await page.clock.install();
  let reads = 0;
  await page.route(`http://localhost:8080/join/${playerSlug}`, (route) => {
    reads++;
    return reads === 1
      ? route.fulfill({
          status: 429,
          headers: { "Retry-After": "120", "Access-Control-Expose-Headers": "Retry-After" },
          json: { error: "Wait before retrying" },
        })
      : route.fulfill({ json: playerSetup });
  });
  await page.goto(`/join/${playerSlug}`);
  await expect(
    page.getByRole("heading", { name: "Player setup couldn’t be loaded" }),
  ).toBeVisible();
  await page.evaluate(() => window.dispatchEvent(new Event("online")));
  await page.clock.fastForward(60001);
  expect(reads).toBe(1);
  await page.clock.fastForward(60001);
  await expect(
    page.getByRole("heading", { name: "Join The Lantern Lodge", exact: true }),
  ).toBeVisible();
  expect(reads).toBe(2);
});

test("public joining details open on demand and close with Escape", async ({ page }) => {
  await mockAPI(page);
  await page.route(`http://localhost:8080/join/${playerSlug}`, (route) =>
    route.fulfill({ json: { ...playerSetup, addons: [], pack_ready: false } }),
  );
  await page.goto(`/join/${playerSlug}`);
  await expect(page.locator(".public-player-page details")).toHaveCount(0);
  await page.getByRole("button", { name: "Can’t join? Find a quick fix", exact: true }).click();
  await expect(page.getByRole("dialog", { name: "Joining help", exact: true })).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(page.getByRole("dialog")).toHaveCount(0);
  await page.getByRole("button", { name: "About this player setup", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "Player requirements", exact: true });
  await expect(dialog).toBeVisible();
  await expect(dialog.getByRole("button")).toHaveCount(1);
  await dialog.getByRole("button", { name: "Close", exact: true }).click();
  await expect(dialog).toHaveCount(0);
});

test("management stop stays pending until offline and prevents repeated requests", async ({
  page,
}) => {
  const calls = await mockAPI(page, { initiallyRunning: true, holdStop: true });
  await login(page);
  await page.goto(`/servers/${serverID}/settings`);
  await page.getByRole("textbox", { name: /Server message/ }).fill("My pending draft");
  await page.getByRole("button", { name: "Stop", exact: true }).click();
  const stopping = page.getByRole("button", { name: "Stopping…", exact: true });
  await expect(stopping).toBeDisabled();
  await expect(page.getByText("Saving and stopping…", { exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: "Save settings", exact: true })).toBeDisabled();
  await stopping.evaluate((button) => button.click());
  expect(
    calls.filter((call) => call.path.endsWith("/stop") && call.method === "POST"),
  ).toHaveLength(1);
  await page.route(`http://localhost:8080/servers/${serverID}/usage`, (route) =>
    route.fulfill({
      json: {
        power_state: "offline",
        cpu_percent: 0,
        memory_bytes: 0,
        disk_bytes: 0,
        uptime_ms: 0,
      },
    }),
  );
  await page.getByRole("button", { name: "Check status", exact: true }).click();
  await expect(page.getByRole("button", { name: "Start", exact: true })).toBeEnabled();
  await expect(page.getByRole("button", { name: "Save settings", exact: true })).toBeEnabled();
  await expect(page.getByRole("textbox", { name: /Server message/ })).toHaveValue(
    "My pending draft",
  );
});

test("management controls follow moderator permissions without requesting restricted files", async ({
  page,
}) => {
  const calls = await mockAPI(page, { role: "MODERATOR", initiallyRunning: true });
  await login(page);
  await page.goto(`/servers/${serverID}/files`);
  await expect(page.getByRole("button", { name: "Restart", exact: true })).toBeEnabled();
  await expect(page.getByRole("button", { name: "Stop", exact: true })).toHaveCount(0);
  await expect(
    page.getByText("Your server role does not allow this page.", { exact: true }),
  ).toBeVisible();
  expect(calls.filter((call) => call.path.includes("/files"))).toHaveLength(0);
});

test("unknown management status blocks power and saving until a read succeeds", async ({
  page,
}) => {
  await mockAPI(page);
  let failed = true;
  await page.route(`http://localhost:8080/servers/${serverID}/usage`, (route) =>
    failed
      ? route.fulfill({ status: 503, json: { error: "Live status unavailable" } })
      : route.fulfill({
          json: {
            power_state: "offline",
            cpu_percent: 0,
            memory_bytes: 0,
            disk_bytes: 0,
            uptime_ms: 0,
          },
        }),
  );
  await login(page);
  await page.goto(`/servers/${serverID}/files`);
  await page.getByRole("button", { name: "server.properties", exact: true }).click();
  await page.getByRole("textbox", { name: "File content", exact: true }).fill("motd=Keep my draft");
  for (const name of ["Start", "Stop", "Restart", "Save file"])
    await expect(page.getByRole("button", { name, exact: true })).toBeDisabled();
  failed = false;
  await page.getByRole("button", { name: "Check status", exact: true }).click();
  await expect(page.getByRole("button", { name: "Save file", exact: true })).toBeEnabled();
  await expect(page.getByRole("textbox", { name: "File content", exact: true })).toHaveValue(
    "motd=Keep my draft",
  );
});

test("failed management power retry retains its key and accepted restart waits for a fresh read", async ({
  page,
}) => {
  await mockAPI(page, { initiallyRunning: true });
  const attempts = [];
  await page.route(`http://localhost:8080/servers/${serverID}/restart`, (route) => {
    attempts.push({
      key: route.request().headers()["idempotency-key"],
      body: route.request().postDataJSON(),
    });
    return attempts.length === 1
      ? route.fulfill({ status: 503, json: { error: "Request outcome uncertain" } })
      : route.fulfill({ json: { status: "accepted" } });
  });
  await login(page);
  await page.goto(`/servers/${serverID}/settings`);
  await page.getByRole("button", { name: "Restart", exact: true }).click();
  await expect(page.getByText("Request outcome uncertain", { exact: true })).toBeVisible();
  let pendingRead;
  await page.route(`http://localhost:8080/servers/${serverID}/usage`, (route) => {
    pendingRead = route;
  });
  await page.getByRole("button", { name: "Restart", exact: true }).click();
  await expect(page.getByRole("button", { name: "Restarting…", exact: true })).toBeDisabled();
  await expect.poll(() => !!pendingRead).toBe(true);
  expect(attempts).toHaveLength(2);
  expect(attempts[1]).toEqual(attempts[0]);
  await pendingRead.fulfill({
    json: {
      power_state: "running",
      cpu_percent: 20,
      memory_bytes: 1000,
      disk_bytes: 1000,
      uptime_ms: 1000,
    },
  });
  await expect(page.getByRole("button", { name: "Restart", exact: true })).toBeEnabled();
  await expect(page.getByText("Restart requested.", { exact: true })).toBeVisible();
});

test("overview power controls do not trust a retained running reading after polling fails", async ({
  page,
}) => {
  await mockAPI(page, { initiallyRunning: true });
  let failed = false;
  await page.route(`http://localhost:8080/servers/${serverID}/usage`, (route) =>
    failed
      ? route.fulfill({ status: 503, json: { error: "Metrics temporarily unavailable" } })
      : route.fulfill({
          json: {
            power_state: "running",
            cpu_percent: 20,
            memory_bytes: 1000,
            disk_bytes: 1000,
            uptime_ms: 420000,
          },
        }),
  );
  await page.clock.install();
  await openServer(page);
  await expect(page.getByRole("button", { name: "Stop", exact: true })).toBeEnabled();
  failed = true;
  await page.clock.fastForward(5001);
  await expect(page.getByText("Metrics temporarily unavailable", { exact: true })).toBeVisible();
  for (const name of ["Start", "Stop", "Restart"])
    await expect(page.getByRole("button", { name, exact: true })).toBeDisabled();
  failed = false;
  await page.clock.fastForward(5001);
  await expect(page.getByRole("button", { name: "Stop", exact: true })).toBeEnabled();
});

test("server-details polling failure keeps an open file draft while disabling saves", async ({
  page,
}) => {
  await mockAPI(page);
  let failed = false;
  await page.route(`http://localhost:8080/servers/${serverID}`, (route) =>
    failed
      ? route.fulfill({ status: 503, json: { error: "Server details temporarily unavailable" } })
      : route.fallback(),
  );
  await login(page);
  await page.clock.install();
  await page.goto(`/servers/${serverID}/files`);
  await page.getByRole("button", { name: "server.properties", exact: true }).click();
  const text = page.getByRole("textbox", { name: "File content", exact: true });
  await text.fill("motd=Keep this through an outage");
  await expect(page.getByRole("button", { name: "Save file", exact: true })).toBeEnabled();
  failed = true;
  await page.clock.fastForward(10001);
  await expect(
    page.getByText("Server details temporarily unavailable", { exact: true }),
  ).toBeVisible();
  await expect(text).toHaveValue("motd=Keep this through an outage");
  await expect(page.getByRole("button", { name: "Save file", exact: true })).toBeDisabled();
  failed = false;
  await page.clock.fastForward(10001);
  await expect(page.getByRole("button", { name: "Save file", exact: true })).toBeEnabled();
  await expect(text).toHaveValue("motd=Keep this through an outage");
});

test("billing distinguishes unavailable history and recovers without a write", async ({ page }) => {
  const calls = await mockAPI(page);
  let failed = true;
  await page.route("http://localhost:8080/orders", async (route) => {
    if (!failed) return route.fallback();
    await route.fulfill({
      status: 503,
      contentType: "application/json",
      body: JSON.stringify({ error: "Temporarily unavailable" }),
    });
  });
  await login(page);
  await page.getByRole("link", { name: "Billing", exact: true }).click();
  await expect(page.getByText("Order history unavailable.", { exact: true })).toBeVisible();
  await expect(page.getByText("Your orders will appear here.", { exact: true })).toHaveCount(0);
  failed = false;
  await page.getByRole("button", { name: "Refresh billing" }).click();
  await expect(page.getByRole("link", { name: "Finish checkout" })).toBeVisible();
  expect(calls.some((c) => c.path.endsWith("/renewals"))).toBe(false);
});

test("billing renewal retry retains its request key and blocks duplicate clicks", async ({
  page,
}) => {
  await mockAPI(page);
  const keys = [];
  let release;
  await page.route(`http://localhost:8080/servers/${serverID}/renewals`, async (route) => {
    keys.push(route.request().headers()["idempotency-key"]);
    if (keys.length === 1) {
      await new Promise((resolve) => {
        release = resolve;
      });
      return route.fulfill({
        status: 503,
        contentType: "application/json",
        body: JSON.stringify({ error: "Try again" }),
      });
    }
    return route.fallback();
  });
  await login(page);
  await page.getByRole("link", { name: "Billing", exact: true }).click();
  await page.getByRole("button", { name: "Renew", exact: true }).click();
  await expect(page.getByRole("button", { name: "Opening checkout…" })).toBeDisabled();
  await expect.poll(() => keys.length).toBe(1);
  release();
  await expect(page.getByRole("button", { name: "Renew", exact: true })).toBeEnabled();
  await page.getByRole("button", { name: "Renew", exact: true }).click();
  await expect(page).toHaveURL(new RegExp(`/checkout/${orderID}$`));
  expect(keys).toHaveLength(2);
  expect(keys[0]).toMatch(/^[0-9a-f-]{36}$/);
  expect(keys[1]).toBe(keys[0]);
});

test("billing keeps stale subscriptions visible but blocks changes until refreshed", async ({
  page,
}) => {
  await page.clock.install();
  await mockAPI(page);
  let fail = false;
  await page.route("http://localhost:8080/subscriptions", async (route) => {
    if (!fail) return route.fallback();
    return route.fulfill({
      status: 503,
      contentType: "application/json",
      body: JSON.stringify({ error: "Subscriptions unavailable" }),
    });
  });
  await login(page);
  await page.getByRole("link", { name: "Billing", exact: true }).click();
  await expect(page.getByRole("button", { name: "Renew", exact: true })).toBeEnabled();
  fail = true;
  await page.clock.runFor(10001);
  await expect(page.getByRole("button", { name: "Renew", exact: true })).toBeDisabled();
  await expect(page.getByRole("button", { name: "End after this period" })).toBeDisabled();
  fail = false;
  await page.getByRole("button", { name: "Refresh billing" }).click();
  await expect(page.getByRole("button", { name: "Renew", exact: true })).toBeEnabled();
});

test("account recovers a failed profile read without changing account data", async ({ page }) => {
  const calls = await mockAPI(page);
  let failed = true;
  await page.route("http://localhost:8080/account", async (route) => {
    if (!failed) return route.fallback();
    return route.fulfill({
      status: 503,
      contentType: "application/json",
      body: JSON.stringify({ error: "Account unavailable" }),
    });
  });
  await login(page);
  await page.getByRole("link", { name: "Account", exact: true }).click();
  await expect(page.getByRole("button", { name: "Refresh account" })).toBeVisible();
  failed = false;
  await page.getByRole("button", { name: "Refresh account" }).click();
  await expect(page.getByLabel("Current password")).toBeVisible();
  await expect(page.getByText("Account ID", { exact: true })).toHaveCount(0);
  expect(calls.some((c) => c.path === "/account/password")).toBe(false);
});

test("account password submission locks fields and preserves them after failure", async ({
  page,
}) => {
  await mockAPI(page);
  let release;
  let writes = 0;
  await page.route("http://localhost:8080/account/password", async (route) => {
    writes++;
    await new Promise((resolve) => {
      release = resolve;
    });
    return route.fulfill({
      status: 503,
      contentType: "application/json",
      body: JSON.stringify({ error: "Please retry" }),
    });
  });
  await login(page);
  await page.getByRole("link", { name: "Account", exact: true }).click();
  await page.getByLabel("Current password").fill("original-password");
  await page.getByLabel("New password", { exact: true }).fill("replacement-password");
  await page.getByLabel("Confirm new password").fill("replacement-password");
  await page.getByRole("button", { name: "Change password and sign out" }).click();
  await expect(page.getByLabel("Current password")).toBeDisabled();
  await expect(page.getByLabel("New password", { exact: true })).toBeDisabled();
  await expect(page.getByRole("button", { name: "Changing…" })).toBeDisabled();
  await expect.poll(() => writes).toBe(1);
  release();
  await expect(page.getByLabel("Current password")).toBeEnabled();
  await expect(page.getByLabel("New password", { exact: true })).toHaveValue(
    "replacement-password",
  );
  await expect(page.getByLabel("Confirm new password")).toHaveValue("replacement-password");
  expect(writes).toBe(1);
});

test("account password visibility is optional and the mobile page fits", async ({
  page,
}, testInfo) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await mockAPI(page);
  await login(page);
  await page.goto("/account");
  const password = page.getByLabel("New password", { exact: true });
  await expect(password).toHaveAttribute("type", "password");
  await page.getByLabel("Show passwords").check();
  await expect(password).toHaveAttribute("type", "text");
  await page.getByLabel("Show passwords").uncheck();
  await expect(password).toHaveAttribute("type", "password");
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(
    true,
  );
  await page.screenshot({ path: testInfo.outputPath("account-mobile.png"), fullPage: true });
});

test("players access list recovers from unavailable reads without sending commands", async ({
  page,
}) => {
  const calls = await mockAPI(page, { initiallyRunning: true });
  let failed = true;
  await page.route(`http://localhost:8080/servers/${serverID}/whitelist`, async (route) => {
    if (!failed) return route.fallback();
    return route.fulfill({ status: 503, json: { error: "Access list unavailable" } });
  });
  await openServer(page);
  await page.getByRole("link", { name: "Players", exact: true }).click();
  await expect(page.getByRole("button", { name: "Refresh access list" })).toBeVisible();
  await expect(page.getByText("No players added yet.", { exact: true })).toHaveCount(0);
  failed = false;
  await page.getByRole("button", { name: "Refresh access list" }).click();
  await expect(page.getByText("milkaxq", { exact: true })).toBeVisible();
  expect(calls.filter((c) => c.path.endsWith("/whitelist") && c.method === "POST")).toHaveLength(0);
});

test("players retry preserves the original command after the access revision changes", async ({
  page,
}) => {
  await mockAPI(page, { initiallyRunning: true });
  let revision = "a".repeat(64);
  const writes = [];
  await page.route(`http://localhost:8080/servers/${serverID}/whitelist`, async (route) => {
    if (route.request().method() === "GET")
      return route.fulfill({ json: { enabled: true, players: [], revision } });
    writes.push({
      key: route.request().headers()["idempotency-key"],
      body: route.request().postDataJSON(),
    });
    if (writes.length === 1) {
      revision = "b".repeat(64);
      return route.fulfill({ status: 502, json: { error: "Response lost" } });
    }
    return route.fulfill({ json: { confirmed: true } });
  });
  await openServer(page);
  await page.getByRole("link", { name: "Players", exact: true }).click();
  await page.getByLabel("Minecraft username", { exact: true }).fill("Friend_123");
  await page.getByRole("button", { name: "Add player", exact: true }).click();
  await expect(page.getByRole("alert").filter({ hasText: "Response lost" })).toBeVisible();
  await expect(page.getByRole("button", { name: "Add player", exact: true })).toBeEnabled();
  await page.getByRole("button", { name: "Add player", exact: true }).click();
  await expect(page.getByRole("status")).toContainText("Friend_123 can now join");
  expect(writes).toHaveLength(2);
  expect(writes[1]).toEqual(writes[0]);
  expect(writes[0].body.revision).toBe("a".repeat(64));
});

test("world controls block stale backup data and recover together", async ({ page }) => {
  await page.clock.install();
  await mockAPI(page);
  let failed = false;
  await page.route(`http://localhost:8080/servers/${serverID}/backups`, async (route) => {
    if (!failed) return route.fallback();
    return route.fulfill({ status: 503, json: { error: "Backup history unavailable" } });
  });
  await openServer(page);
  await page.getByRole("link", { name: "World", exact: true }).click();
  await page.getByRole("button", { name: "Create safety backup" }).click();
  await expect(page.getByRole("button", { name: "Reset world", exact: true })).toBeEnabled();
  failed = true;
  await page.clock.runFor(10001);
  await expect(page.getByRole("button", { name: "Reset world", exact: true })).toBeDisabled();
  await expect(page.getByRole("button", { name: "Upload world .zip" })).toBeDisabled();
  await expect(page.getByRole("button", { name: "Create safety backup" })).toBeDisabled();
  failed = false;
  await page.getByRole("button", { name: "Refresh world" }).click();
  await expect(page.getByRole("button", { name: "Reset world", exact: true })).toBeEnabled();
});

test("world reset retry retains its request and typed confirmation after failure", async ({
  page,
}) => {
  await mockAPI(page);
  const writes = [];
  await page.route(`http://localhost:8080/servers/${serverID}/world/reset`, async (route) => {
    writes.push({
      key: route.request().headers()["idempotency-key"],
      body: route.request().postDataJSON(),
    });
    if (writes.length === 1)
      return route.fulfill({ status: 502, json: { error: "Response lost" } });
    return route.fallback();
  });
  await openServer(page);
  await page.getByRole("link", { name: "World", exact: true }).click();
  await page.getByRole("button", { name: "Create safety backup" }).click();
  await page.getByRole("button", { name: "Reset world", exact: true }).click();
  const dialog = page.getByRole("dialog");
  await dialog.getByRole("textbox").fill("world");
  await dialog.getByRole("button", { name: "Reset world", exact: true }).click();
  await expect(dialog.getByRole("alert")).toContainText("Response lost");
  await expect(dialog.getByRole("textbox")).toHaveValue("world");
  await dialog.getByRole("button", { name: "Reset world", exact: true }).click();
  await expect(dialog).not.toBeVisible();
  expect(writes).toHaveLength(2);
  expect(writes[1]).toEqual(writes[0]);
});

test("member access failure blocks changes and refresh restores the list", async ({ page }) => {
  await page.clock.install();
  await mockAPI(page);
  let failed = false;
  await page.route(`http://localhost:8080/servers/${serverID}/members`, async (route) => {
    if (!failed || route.request().method() !== "GET") return route.fallback();
    return route.fulfill({ status: 503, json: { error: "Members unavailable" } });
  });
  await openServer(page);
  await page.getByRole("link", { name: "Members", exact: true }).click();
  await page.getByLabel("Member email").fill("sam@example.com");
  await page.getByRole("button", { name: "Add member", exact: true }).click();
  await expect(page.getByLabel("Role for Sam")).toBeEnabled();
  failed = true;
  await page.clock.runFor(10001);
  await expect(page.getByLabel("Role for Sam")).toBeDisabled();
  await expect(page.getByRole("button", { name: "Remove member", exact: true })).toBeDisabled();
  await expect(page.getByRole("button", { name: "Add member", exact: true })).toBeDisabled();
  failed = false;
  await page.getByRole("button", { name: "Refresh members" }).click();
  await expect(page.getByLabel("Role for Sam")).toBeEnabled();
});

test("member add retry retains its key and email after a failed response", async ({ page }) => {
  await mockAPI(page);
  const writes = [];
  await page.route(`http://localhost:8080/servers/${serverID}/members`, async (route) => {
    if (route.request().method() !== "POST") return route.fallback();
    writes.push({
      key: route.request().headers()["idempotency-key"],
      body: route.request().postDataJSON(),
    });
    if (writes.length === 1)
      return route.fulfill({ status: 502, json: { error: "Response lost" } });
    return route.fallback();
  });
  await openServer(page);
  await page.getByRole("link", { name: "Members", exact: true }).click();
  await page.getByLabel("Member email").fill("sam@example.com");
  await page.getByRole("button", { name: "Add member", exact: true }).click();
  await expect(page.getByRole("alert").filter({ hasText: "Response lost" })).toBeVisible();
  await expect(page.getByLabel("Member email")).toHaveValue("sam@example.com");
  await page.getByRole("button", { name: "Add member", exact: true }).click();
  await expect(page.getByText("sam@example.com", { exact: true })).toBeVisible();
  expect(writes).toHaveLength(2);
  expect(writes[1]).toEqual(writes[0]);
});

test("member cards and role choices fit small screens", async ({ page }, testInfo) => {
  await page.setViewportSize({ width: 320, height: 900 });
  await mockAPI(page);
  await openServer(page);
  await page.getByRole("link", { name: "Members", exact: true }).click();
  await page.getByLabel("Member email").fill("sam@example.com");
  await page.getByRole("button", { name: "Add member", exact: true }).click();
  await expect(page.getByRole("list", { name: "Server members" })).toContainText("sam@example.com");
  await expect(page.getByLabel("Role for Sam")).toBeVisible();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await page.evaluate(() => {
    document.activeElement?.blur();
    window.scrollTo(0, 0);
  });
  await page.screenshot({ path: testInfo.outputPath("members-mobile.png"), fullPage: true });
});
