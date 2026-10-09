"use client";
import Link from "next/link";
import { Icon } from "./ui";
import { softwareName, supportsMods } from "../lib/software";
export function FirstServerChecklist({
  server,
  ready,
  eula,
  powerState,
  address,
  addressCopied,
  onCopy,
}) {
  const steps = [
    {
      done: ready,
      title: "Finish server setup",
      text: ready
        ? "Your server is installed."
        : "Wait for installation to finish. If setup needs attention, contact your host.",
    },
    {
      done: eula === true,
      title: "Review the Minecraft agreement",
      text:
        eula === true
          ? "The owner has accepted the EULA."
          : "The server owner must read and accept the Minecraft EULA before the first start.",
    },
    {
      done: powerState === "running",
      title: "Start your server",
      text:
        powerState === "running"
          ? "Minecraft is online."
          : "Use Start above, then wait for Online. Startup can take a little while.",
    },
    {
      done: addressCopied,
      title: "Copy the address and join",
      text: `In Minecraft Java ${server.minecraft_version}, choose Multiplayer → Add Server. ${supportsMods(server.server_type) ? `Use ${softwareName(server.server_type)} with the matching loader and required mods.` : "A normal Minecraft Java client is enough."}`,
    },
  ];
  return (
    <details
      className="first-server-checklist"
      open={!ready || eula === false || powerState === "offline"}
    >
      <summary>
        Your first session{" "}
        <span>
          {steps.filter((s) => s.done).length} of {steps.length} steps complete
        </span>
      </summary>
      <ol>
        {steps.map((step, index) => (
          <li key={step.title}>
            <span
              className={`checklist-step ${step.done ? "complete" : ""}`}
              aria-label={step.done ? "Complete" : "Not complete"}
            >
              {step.done ? <Icon name="check" size={16} /> : index + 1}
            </span>
            <div>
              <strong>{step.title}</strong>
              <p>{step.text}</p>
              {index === 3 && address && (
                <button className="button secondary" onClick={onCopy}>
                  Copy address
                </button>
              )}
            </div>
          </li>
        ))}
      </ol>
      <Link className="text-link" href="/guide#start-and-join">
        Open the setup guide →
      </Link>
    </details>
  );
}
export function BetaGettingStarted({ servers }) {
  const current = servers?.find((s) => s.status !== "CANCELLED");
  return (
    <section className="panel beta-getting-started" aria-labelledby="beta-start-title">
      <h2 id="beta-start-title">Welcome to the private beta</h2>
      <p>
        Your account is ready. Your host will provide server access; payments are disabled for this
        beta.
      </p>
      <ol>
        <li>
          Create and verify your account <strong>Complete</strong>
        </li>
        <li>
          {current ? (
            <Link href={`/servers/${current.id}`}>
              Open your server and follow the first-session checklist →
            </Link>
          ) : (
            "Ask your host for server access. It will appear here when available."
          )}
        </li>
        <li>Accept the agreement, start Minecraft, then copy the address and join.</li>
      </ol>
      <Link href="/guide#start-and-join" className="text-link">
        Read the joining guide →
      </Link>
    </section>
  );
}
