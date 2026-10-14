// Choose guidance from observed state, never from the existence of an address.
export function nextServerAction({ server, ready, powerState, eula, eulaError, usageError }) {
  const can = (permission) => server.permissions?.includes(permission);
  if (server.deletion_pending || server.status === "CANCELLED")
    return {
      title: server.deletion_pending ? "Your world is being removed." : "This world has ended.",
      help: "Return to your worlds to choose another adventure.",
      kind: "worlds",
    };
  if (server.status === "PENDING_PAYMENT")
    return {
      title: "Finish checkout to create your world.",
      help: "Setup begins after payment is confirmed.",
      kind: server.role === "OWNER" ? "billing" : "wait",
    };
  if (["PAYMENT_CONFIRMED", "PROVISIONING"].includes(server.status))
    return {
      title: "We’re preparing your world.",
      help: "This page updates automatically. You can start once installation is finished.",
      kind: "wait",
    };
  if (server.status === "SUSPENDED")
    return {
      title: "Your world is paused.",
      help: "Contact your host to find out what is needed to resume playing.",
      kind: "support",
    };
  if (server.status === "FAILED")
    return {
      title: "Setup needs a little help.",
      help: "Your host can investigate and retry this request. You don’t need to create another server.",
      kind: "support",
    };
  if (!ready || usageError || eulaError)
    return {
      title: "We can’t confirm your next step yet.",
      help: "Live controls or setup checks are unavailable. Try refreshing, or contact your host if this continues.",
      kind: "support",
    };
  if (eula === false)
    return {
      title: "One agreement before your first adventure.",
      help: "The server owner must review Minecraft’s agreement before starting.",
      kind: "agreement",
    };
  if (powerState === "starting")
    return {
      title: "Your world is waking up.",
      help: "Minecraft is loading the world and addons. Wait for Online before joining.",
      kind: "wait",
    };
  if (powerState === "stopping")
    return {
      title: "Saving your adventure.",
      help: "Minecraft is shutting down safely. Wait for Offline before starting again.",
      kind: "wait",
    };
  if (powerState === "running")
    return {
      title: "Your world is ready. Bring your friends.",
      help: "Copy the address to join, or prepare an invitation with player setup instructions.",
      kind: can("members") ? "invite" : "join",
    };
  if (eula === true && powerState === "offline")
    return {
      title: can("start") ? "Start your next adventure." : "Ask your host to start the world.",
      help: "Once Minecraft is Online, join with the address below.",
      kind: can("start") ? "start" : "wait",
    };
  return {
    title: "Checking your world…",
    help: "Waiting for the latest status and Minecraft agreement check.",
    kind: "wait",
  };
}
