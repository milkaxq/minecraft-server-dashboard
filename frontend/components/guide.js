"use client";
import Link from "next/link";
import { useEffect, useState } from "react";
import { Shell, PageHeading } from "./ui";
import { SOFTWARE } from "../lib/software";

const goals = {
  classic: [
    "Original Minecraft",
    "Choose Vanilla. Everyone joins with the same Minecraft Java version; no extra loader is needed.",
  ],
  plugins: [
    "Survival with plugins",
    "Choose Paper as a starting point. Plugins add permissions, homes and administration tools on the server. Players usually join with normal Minecraft.",
  ],
  mods: [
    "New blocks, machines or a modpack",
    "Read the mod or modpack’s requirements first. Choose its exact loader: Fabric, Forge or NeoForge. They are separate ecosystems, and players often need the same pack installed.",
  ],
};
export function Guide() {
  const [goal, setGoal] = useState("plugins");
  useEffect(() => {
    function revealTopic() {
      const target = document.getElementById(window.location.hash.slice(1));
      if (target?.tagName === "DETAILS") {
        target.open = true;
        target.querySelector("summary")?.focus({ preventScroll: true });
        target.scrollIntoView({ block: "start" });
      }
    }
    revealTopic();
    window.addEventListener("hashchange", revealTopic);
    return () => window.removeEventListener("hashchange", revealTopic);
  }, []);
  return (
    <Shell>
      <div className="guide-content">
        <PageHeading
          eyebrow="MINECRAFT MADE SIMPLE"
          title="Your first server, step by step."
          description="Choose the right software, install extras, and invite your friends."
        />
        <nav className="guide-topics" aria-label="Guide topics">
          <a href="#choose-software">Choose software</a>
          <a href="#start-and-join">Start and join</a>
          <a href="#server-login">Server login</a>
          <a href="#add-extras">Plugins and mods</a>
          <a href="#worlds-and-backups">Worlds and backups</a>
          <a href="#server-errors">Fix startup problems</a>
          <a href="#custom-code">Write your own code</a>
        </nav>
        <section id="choose-software" className="panel management-panel">
          <h2>1. What do you want to play?</h2>
          <div className="power-controls" role="group" aria-label="What do you want to play?">
            {Object.entries(goals).map(([key, [label]]) => (
              <button
                key={key}
                className={`button ${goal === key ? "primary" : "secondary"}`}
                aria-pressed={goal === key}
                onClick={() => setGoal(key)}
              >
                {label}
              </button>
            ))}
          </div>
          <p className="notice">{goals[goal][1]}</p>
          <div className="table-scroll">
            <table className="management-table guide-table">
              <thead>
                <tr>
                  <th>Software</th>
                  <th>What it does</th>
                  <th>Extras</th>
                </tr>
              </thead>
              <tbody>
                {Object.entries(SOFTWARE).map(([key, value]) => (
                  <tr key={key}>
                    <td>
                      <strong>{value.name}</strong>
                    </td>
                    <td>{value.description}</td>
                    <td>
                      {value.category === "classic"
                        ? "None"
                        : value.category === "mods"
                          ? "Matching loader mods"
                          : "Bukkit/Spigot plugins"}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          <p className="field-help">
            Paper-specific plugins may require Paper or Purpur and will not work on Spigot. Forge
            mods do not automatically work on NeoForge. Availability and Minecraft versions are
            shown when creating a server.
          </p>
          <Link className="button primary" href="/servers/new">
            Choose a plan and create a server
          </Link>
        </section>
        <section id="start-and-join" className="panel management-panel">
          <h2>2. Set up and start</h2>
          <ol>
            <li>
              Choose the Minecraft version your friends or modpack use. For mods, check the required
              loader version too. Large modpacks often need more memory than a small vanilla server;
              follow the pack author’s recommendation.
            </li>
            <li>
              Complete checkout and wait for setup. Open your server and read and accept the
              Minecraft EULA before its first start.
            </li>
            <li>
              Open Settings to choose difficulty, game mode, PvP and player limits. Keep Minecraft
              account verification enabled for a normal public server.
            </li>
            <li>
              Click Start. Wait for the console to report that startup has finished, then copy the
              address from Overview.
            </li>
            <li>
              In Minecraft Java Edition, open Multiplayer → Add Server and paste the address,
              including the port. Use the matching Minecraft version.
            </li>
          </ol>
        </section>
        <section id="server-login" className="panel management-panel">
          <h2>Signing in to Minecraft servers</h2>
          <p>
            Most servers require a Microsoft account that owns Minecraft Java Edition. In
            SKlauncher, choose Microsoft login for those servers. An offline username cannot join a
            server with Minecraft account verification enabled. If Microsoft login reports an
            invalid session, sign out of the launcher account and sign back in before launching the
            game again.
          </p>
          <p>
            Some Paper servers use AuthMe for separate server accounts. Both Microsoft and offline
            players can connect when the host has configured this mode. After joining in Minecraft,
            follow the registration dialog on your first visit, or use{" "}
            <code>/register yourPassword yourPassword</code>. On later visits, use the login dialog
            or <code>/login yourPassword</code>.
          </p>
          <p>
            Choose a unique password of at least 10 characters. Do not reuse your Microsoft or
            Blockyard password. Registration belongs to that Minecraft server; your website login is
            separate. A whitelist may still require your host to allow your exact Minecraft
            username.
          </p>
          <p>
            Hosts: install and verify the login plugin before disabling Minecraft account
            verification. Keep backups before switching modes: Minecraft player identities can
            change, which affects inventories, permissions and whitelists. AuthMe is a Paper plugin;
            modded servers need a login solution compatible with their loader.
          </p>
          <a href="https://docs.skmedix.pl/getting-started/login" target="_blank" rel="noreferrer">
            SKlauncher login instructions
          </a>
        </section>
        <section id="add-extras" className="panel management-panel">
          <h2>3. Add plugins or mods</h2>
          <p className="field-help">
            ViaVersion lets newer Java clients connect to an older Paper server; it does not add
            newer blocks or gameplay. Search for ViaVersion in Plugins, install it while stopped,
            then start the server. Complete modpacks need their author’s server pack and matching
            client setup; the catalog currently installs individual projects and required
            dependencies.
          </p>
          <ol>
            <li>Create a backup before making changes, then stop the server.</li>
            <li>
              Download a trusted release made for your Minecraft version and chosen software. Read
              its dependencies and installation instructions. A .jar is executable code: only
              install files you trust.
            </li>
            <li>
              For Paper, Spigot or Purpur, open Plugins. For Fabric, Forge or NeoForge, open Mods.
              Search Modrinth, choose Review install, check the files and required dependencies,
              then click Install now. The browser matches your server’s version and software. You
              can still upload trusted .jar files manually.
            </li>
            <li>
              Start the server once so it can generate configuration files. Stop it, open Advanced →
              Files, edit the generated text configuration, save, and start again.
            </li>
            <li>
              For gameplay mods, install the matching loader and modpack on each player’s computer.
              Client-only mods belong on the player’s computer, not the server. Server-only mods
              usually do not need a client installation.
            </li>
          </ol>
          <p className="notice">
            Upload checks identify the loader, but cannot guarantee every mod’s Minecraft version or
            dependencies. Read the release requirements and console errors. We do not install packs
            automatically.
          </p>
          <p>
            A modpack ZIP is not a plugin or mod .jar. Use the pack’s dedicated server pack and
            follow its instructions. Upload its compatible mods and config files using Mods and
            Files; do not run a downloaded host installation script. If its Minecraft or loader
            version is unavailable, contact hosting support before buying.
          </p>
        </section>
        <section id="worlds-and-backups" className="panel management-panel">
          <h2>4. Worlds, backups and troubleshooting</h2>
          <details id="move-world">
            <summary>How do I move my existing world?</summary>
            <p>
              Stop the server and open World. Download your current world or upload a ZIP with
              level.dat inside the world folder. Match the original Minecraft version and any mods
              that created the world. Upload and reset actions require confirmation. Backups include
              all server files; restoring one replaces the current files.
            </p>
          </details>
          <details id="server-errors">
            <summary>My server will not start after adding a mod.</summary>
            <p>
              Open Advanced → Console and find the first error. Check the Minecraft version, loader,
              required dependencies, and whether the mod is client-only. Stop the server, remove the
              last added file, and start again. Add a few mods at a time to find the cause. Restore
              your backup if necessary.
            </p>
          </details>
          <details id="custom-code">
            <summary>Can I write my own plugin or mod?</summary>
            <p>
              Yes. For a plugin, use the Bukkit/Spigot or Paper API and build a .jar with the
              correct plugin descriptor. For a mod, develop against your chosen loader and Minecraft
              version. Test locally, upload the built .jar through Plugins or Mods, and restart. The
              Console accepts Minecraft commands; it is not a system shell.
            </p>
          </details>
          <details id="change-software">
            <summary>Can I switch an existing server to a different loader?</summary>
            <p>
              Create a new server with the required software and test a copy of your world first.
              Removing mods can remove their blocks, items and dimensions. Keep a backup before
              changing versions or software.
            </p>
          </details>
          <p className="field-help">
            Official installation guides:{" "}
            <a
              href="https://docs.papermc.io/paper/adding-plugins/"
              target="_blank"
              rel="noreferrer"
            >
              Paper plugins
            </a>{" "}
            ·{" "}
            <a
              href="https://docs.fabricmc.net/players/installing-mods/"
              target="_blank"
              rel="noreferrer"
            >
              Fabric mods
            </a>{" "}
            ·{" "}
            <a href="https://docs.neoforged.net/user/docs/server/" target="_blank" rel="noreferrer">
              NeoForge servers
            </a>
            .
          </p>
        </section>
      </div>
    </Shell>
  );
}
