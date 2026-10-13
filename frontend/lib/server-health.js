// Resource readings describe pressure, not Minecraft tick rate or a player's ping.
export function serverHealth({ server, ready, powerState, usage, usageError, eula }) {
  if (!ready) {
    const states = {
      PENDING_PAYMENT: ["Waiting for payment", "Finish checkout in Billing to begin setup."],
      PAYMENT_CONFIRMED: [
        "Waiting for setup",
        "Your request is queued. This page updates automatically.",
      ],
      PROVISIONING: [
        "Installing Minecraft",
        "Wait for installation, then review the agreement and start your server.",
      ],
      FAILED: [
        "Setup needs attention",
        "Your host can inspect and retry setup. Report the problem below.",
      ],
      SUSPENDED: [
        "Server suspended",
        "Contact your host to find out what is needed to reactivate it.",
      ],
      RUNNING: [
        "Controls unavailable",
        "We cannot reach live management right now. Try again or contact your host.",
      ],
    };
    const [title, help] = states[server.status] || [
      "Status unavailable",
      "Check the server overview or contact your host.",
    ];
    return { title, help };
  }
  if (usageError)
    return {
      title: "Live status unavailable",
      help: "The latest reading could not be fetched. We cannot confirm whether Minecraft is online right now.",
    };
  if (powerState === "starting")
    return {
      title: "Starting Minecraft",
      help: "The world and add-ons are loading. Wait for Online; repeated start requests will not make it faster.",
    };
  if (powerState === "stopping")
    return {
      title: "Saving world and stopping",
      help: "Wait for Offline before starting again or changing files.",
    };
  if (eula === false)
    return {
      title: "Agreement needed",
      help: "The owner must review and accept the Minecraft agreement above before the first start.",
    };
  if (powerState === "offline")
    return {
      title: "Minecraft is offline",
      help: "Use Start above when you are ready to play. If it stopped unexpectedly, review Startup help below.",
    };
  if (powerState !== "running" || !usage)
    return {
      title: "Checking server status",
      help: "Waiting for a current reading. No connection or performance diagnosis is available yet.",
    };
  if (server.disk_mb > 0 && usage.disk_bytes / (server.disk_mb * 1048576) >= 0.85)
    return {
      title: "Storage is nearly full",
      help: "A full disk can interrupt world saves. Review your files and preserve important data before removing anything.",
      action: "storage",
    };
  if (server.cpu_limit > 0 && usage.cpu_percent / server.cpu_limit >= 0.9)
    return {
      title: "CPU use is near its allowance",
      help: "Watch whether this stays high while players notice delays. One reading does not prove that the server is lagging.",
    };
  if (server.ram_mb > 0 && usage.memory_bytes / (server.ram_mb * 1048576) >= 0.9)
    return {
      title: "Memory use is near its allowance",
      help: "Minecraft can reserve much of its available memory. This reading alone does not prove a problem; review add-ons and view distance if play is slow.",
    };
  return {
    title: "Minecraft is online",
    help: "The game process is running. Joining still depends on the player’s client, login and access settings.",
  };
}
