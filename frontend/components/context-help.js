import Link from "next/link";

const topics = {
  software: {
    title: "Which software should I choose?",
    text: "Vanilla is original Minecraft. Paper, Spigot and Purpur use plugins. Fabric, Forge and NeoForge use mods made for that exact loader.",
    steps: [
      "Check your modpack’s Minecraft and loader requirements first.",
      "Choose only a version offered here. Larger modpacks may need a larger plan.",
    ],
    anchor: "choose-software",
  },
  overview: {
    title: "How do I start and join?",
    text: "Ready means setup has finished. Online means Minecraft is actually running.",
    steps: [
      "Accept the Minecraft EULA before the first start.",
      "Click Start and wait for the console’s startup-complete message.",
      "In Minecraft Java Edition, add the address and port shown here.",
    ],
    anchor: "start-and-join",
  },
  mods: {
    title: "How do mods work?",
    text: "Mods must match this server’s Minecraft version and loader. Players often need the same modpack installed on their computers.",
    steps: [
      "Back up your server, then stop it.",
      "Upload compatible server .jar files and all required dependencies.",
      "Start again and check Console for missing dependencies. Client-only mods belong on your computer.",
    ],
    anchor: "add-extras",
  },
  plugins: {
    title: "How do plugins work?",
    text: "Plugins add server features such as homes and permissions. Players usually join with normal Minecraft. Paper-only plugins need Paper or Purpur.",
    steps: [
      "Back up and stop the server before uploading a trusted plugin .jar.",
      "Start once to generate its configuration, then stop before editing it in Files.",
    ],
    anchor: "add-extras",
  },
  worlds: {
    title: "How do I move or reset a world?",
    text: "A world contains your builds and player progress. Uploading or resetting replaces the current world.",
    steps: [
      "Create a safety backup and stop the server first.",
      "Use a world ZIP containing level.dat; match the Minecraft version and mods that created it.",
      "Read the confirmation carefully before replacing anything.",
    ],
    anchor: "worlds-and-backups",
  },
  backups: {
    title: "What does a backup protect?",
    text: "Backups include all server files: worlds, settings, mods and plugins. A world download contains only the world.",
    steps: [
      "Stop the server for a consistent backup and wait until the backup completes.",
      "Download a copy before major changes. Restoring replaces current files and requires confirmation.",
    ],
    anchor: "worlds-and-backups",
  },
  settings: {
    title: "When do settings take effect?",
    text: "These controls edit common Minecraft settings. Save while the server is stopped, then start again to apply them.",
    steps: [
      "Keep Minecraft account verification enabled for a normal public server.",
      "View distance affects memory and CPU use; lower it if the server struggles.",
    ],
    anchor: "start-and-join",
  },
  files: {
    title: "What can I do with Files?",
    text: "Browse your Minecraft files and edit text configuration. These files belong to this game server.",
    steps: [
      "Back up before editing and stop the server before saving changes.",
      "Use Mods or Plugins for .jar uploads. Use Files for generated configuration and server-pack files.",
    ],
    anchor: "add-extras",
  },
  console: {
    title: "How do I use the console?",
    text: "The console shows Minecraft logs and accepts Minecraft commands. You can also read startup errors here.",
    steps: [
      "Wait for the console connection before sending a command.",
      "Enter Minecraft commands without the chat’s leading slash. Try help for available commands.",
      "If startup fails, look for the first error and check the last mod or plugin you added.",
    ],
    anchor: "server-errors",
  },
};

export function ContextHelp({ topic }) {
  const help = topics[topic];
  if (!help) return null;
  return (
    <aside className="context-help" aria-label={`Help with ${topic}`}>
      <details>
        <summary>{help.title}</summary>
        <p>{help.text}</p>
        <ol>
          {help.steps.map((step) => (
            <li key={step}>{step}</li>
          ))}
        </ol>
        <Link href={`/guide#${help.anchor}`}>Open the step-by-step guide →</Link>
      </details>
    </aside>
  );
}
