import { formatBytes } from "../lib/api";

const failures = {
  node_capacity_unavailable: [
    "Not enough available capacity",
    "Check Nodes for a machine with enough RAM, disk and CPU. Free capacity or enable another node before retrying.",
  ],
  panel_installation_failed: [
    "Minecraft installation failed",
    "Check the installation log in Panel and fix the download or installer error before retrying.",
  ],
  panel_installation_pending: [
    "Installation is still in progress",
    "Wait for the installation to finish. Check Panel if it stays pending.",
  ],
  software_configuration_unavailable: [
    "Server software is not configured",
    "Check the selected Minecraft version, egg and loader configuration before retrying.",
  ],
  panel_create_outcome_unknown: [
    "Server creation needs inspection",
    "The request may have reached Panel. Inspect Panel before retrying to avoid creating a duplicate server.",
  ],
  panel_resource_mismatch: [
    "Server resources do not match",
    "Compare the Panel server with its purchased plan and reserved node. Resolve the mismatch before retrying.",
  ],
  reservation_inconsistent: [
    "Capacity reservation needs inspection",
    "Check the server’s node reservation before retrying. Do not change allocations without reconciling the server.",
  ],
  panel_request_rejected: [
    "Panel rejected the request",
    "Check Panel credentials, permissions and configuration before retrying.",
  ],
  operation_retry_required: [
    "Hosting operation was interrupted",
    "Check Panel and worker health. Retry once the connection or service has recovered.",
  ],
};
export function JobHelp({ server }) {
  const message = failures[server.job_error];
  if (!message && server.status !== "FAILED") return null;
  return (
    <div className="job-help">
      <strong>{message?.[0] || "Setup needs attention"}</strong>
      <p>
        {message?.[1] ||
          "Inspect the provisioning job logs before retrying. Your server request has been retained."}
      </p>
      {["PENDING", "RUNNING"].includes(server.job_status) && (
        <p>A job is already queued or running. Wait for its result before retrying.</p>
      )}
      <details>
        <summary>Technical details</summary>
        <code>{server.job_error || "No error code recorded"}</code>
        {server.job_id && <p>Job: {server.job_id}</p>}
      </details>
    </div>
  );
}
export function NodeCapacity({ nodes, plans }) {
  return (
    <section className="capacity-overview" aria-label="Available hosting capacity">
      <h2>Available hosting capacity</h2>
      <p>
        Remaining resources after existing reservations. Plan counts are alternatives, not totals;
        final allocation is checked when provisioning.
      </p>
      <div className="capacity-grid">
        {nodes.map((node) => {
          const ram = Math.max(0, node.ram_capacity_mb - node.allocated_ram_mb);
          const disk = Math.max(0, node.disk_capacity_mb - node.allocated_disk_mb);
          const cpu = Math.max(0, node.cpu_capacity - node.allocated_cpu);
          return (
            <article className="capacity-card" key={node.id}>
              <h3>{node.name}</h3>
              <p>{node.enabled ? "Available for provisioning" : "Disabled · no new servers"}</p>
              {[
                ["RAM", ram, node.ram_capacity_mb, formatBytes(ram * 1048576)],
                ["Disk", disk, node.disk_capacity_mb, formatBytes(disk * 1048576)],
                ["CPU", cpu, node.cpu_capacity, `${cpu}%`],
              ].map(([label, free, total, display]) => (
                <div className="capacity-resource" key={label}>
                  <span>
                    {label} free <strong>{display}</strong>
                  </span>
                  <progress
                    aria-label={`${node.name} ${label} allocated`}
                    max={Math.max(1, total)}
                    value={Math.min(total, Math.max(0, total - free))}
                  />
                </div>
              ))}
              <ul>
                {plans
                  ?.filter((p) => p.enabled)
                  .map((plan) => {
                    const fits =
                      node.enabled && plan.ram_mb > 0 && plan.disk_mb > 0 && plan.cpu_limit > 0
                        ? Math.max(
                            0,
                            Math.min(
                              Math.floor(ram / plan.ram_mb),
                              Math.floor(disk / plan.disk_mb),
                              Math.floor(cpu / plan.cpu_limit),
                            ),
                          )
                        : 0;
                    return (
                      <li key={plan.id}>
                        {plan.name}:{" "}
                        <strong>
                          {fits} more {fits === 1 ? "server" : "servers"}
                        </strong>
                      </li>
                    );
                  })}
              </ul>
            </article>
          );
        })}
      </div>
    </section>
  );
}
