# Plugins, mods, and custom gameplay

The platform currently provisions **Vanilla, Paper, Spigot, Purpur, Fabric, Forge and NeoForge**. The local demo is
Paper 1.21.1 on Java 21. Plugin and mod compatibility must match the selected
Minecraft/server-software version.

## Install a plugin today

1. Choose a Paper-compatible plugin for your Minecraft version and download its
   `.jar` and any required dependency plugins.
2. Make a backup, then stop your server.
3. Open **Plugins**, upload the `.jar` (up to 64 MiB), then start the server.
4. Check **Advanced → Console** for successful loading or errors.
5. If configuration files were created in `/plugins/<plugin-name>/`, stop the
   server again, edit them through **Advanced → Files**, and start it again.

Plugin uploads work on Paper, Spigot and Purpur; Paper-specific descriptors require Paper/Purpur. The file is kept in the server's `/plugins`
folder and loaded by Paper inside its existing isolated container. Upload Java
bytecode only from a source you trust. Updating a plugin requires explicitly
removing the old jar and uploading the replacement while stopped; keep backups
of its configuration/data. In-game operator/permission-plugin permissions are
separate from dashboard Admin/Moderator membership.

Official guide: https://docs.papermc.io/paper/adding-plugins/

## Actual mods

Fabric, Forge and NeoForge mods require their matching loader/server software.
Putting a Fabric mod into Paper's `plugins` folder does not install that mod.
Loader support uses reviewed Pterodactyl eggs, per-release loader pins, and a
Mods page for uploading/listing/deleting jars. Only operator-approved releases
appear during purchase. See [software setup](MINECRAFT_SOFTWARE.md). Switching an existing Paper world also needs
an explicit backup and compatibility check.

With a supported loader, the general workflow is: stop, upload compatible mod
jars and their dependencies into `/mods`, configure them, then start and inspect
the console. Some mods are server-only; others also require each player to have
matching client mods. A modpack must match both Minecraft and the loader version.
Do not put client-only mods on a server.

Official Fabric guides: https://docs.fabricmc.net/players/installing-mods and
https://wiki.fabricmc.net/player:tutorials:server:windows

## Write your own gameplay code

A Paper plugin is normally Java (Kotlin is another JVM option). Your hosting
backend remains Go. A plugin can add commands, react to player joins, implement
minigames, or change gameplay. Build a jar locally, then upload it through the
same Plugins page. There is no host-shell access or on-server source compiler.

For example, a command handler inside your `JavaPlugin` class could be:

```java
@Override
public boolean onCommand(CommandSender sender, Command command,
                         String label, String[] args) {
    if (!command.getName().equalsIgnoreCase("hello")) {
        return false;
    }
    sender.sendMessage("Hello from your custom server!");
    return true;
}
```

Declare that command in `src/main/resources/plugin.yml`, alongside the plugin's
name, version, main class and compatible API version:

```yaml
name: BlockyardHello
version: 1.0.0
main: dev.blockyard.hello.HelloPlugin
api-version: '1.21'
commands:
  hello:
    description: Say hello
    usage: /hello
```

The Java class must be `dev.blockyard.hello.HelloPlugin`, extend `JavaPlugin`, and
import `org.bukkit.command.Command`, `org.bukkit.command.CommandSender` and
`org.bukkit.plugin.java.JavaPlugin`. This is an illustrative handler and descriptor,
not a bundled/tested plugin project. Follow Paper's project setup guide for Gradle,
its API dependency, Java toolchain and jar build:
https://docs.papermc.io/paper/dev/project-setup/

For the current 1.21.1 demo, target Java 21 and a compatible 1.21.1 Paper API rather
than copying the latest-version dependency from the documentation unchanged.
Use the built jar from `build/libs/`, upload it, start the server and type `/hello`
in-game or `hello` in its Minecraft console.

If you want gameplay backed by Go, a small Java plugin can call a separate Go
HTTP service. Keep credentials scoped to that service, avoid blocking the game
thread with network requests, and never give the plugin the hosting database or
Panel application API credentials. This bridge is a future extension, not an
implemented integration in this MVP.

The website’s **Guide** page explains software choices, first start, modpacks,
custom code, worlds and troubleshooting. Mod uploads check the loader descriptor
and reject Fabric client-only jars; exact Minecraft versions and dependencies
still need to match the release documentation.
