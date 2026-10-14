"use client";
import { useState } from "react";
import { useAuth } from "./auth";
import { useResource } from "./data";
import { Select } from "./select";
import { ErrorMessage, Loading, Table } from "./ui";
import { price } from "../lib/api";
import { planMargin, mixForecast } from "../lib/margins";
const money = (amount, currency) =>
  new Intl.NumberFormat(undefined, { style: "currency", currency }).format(amount / 100);
const fields = [
  ["node_monthly_cents", "VPS bill per month", 0, 100000000],
  ["overhead_monthly_cents", "Other monthly costs", 0, 100000000],
  ["per_server_monthly_cents", "Support / backup cost per server per month", 0, 100000000],
  ["fee_bps", "Payment fee (%)", 0, 5000],
  ["fee_fixed_cents", "Fixed payment fee per charge", 0, 1000000],
  ["tax_bps", "Tax included in selling price (%)", 0, 5000],
  ["occupancy_bps", "Expected occupancy (%)", 1, 10000],
  ["target_margin_bps", "Target margin (%)", 0, 9500],
];
export function AdminFinances() {
  const { api } = useAuth();
  const [month, setMonth] = useState(() => new Date().toISOString().slice(0, 7));
  const report = useResource(`/admin/finances?month=${encodeURIComponent(month)}`, 30000);
  const nodes = useResource("/admin/nodes", 30000);
  const plans = useResource("/admin/plans", 30000);
  const [nodeChoice, setNode] = useState("");
  const [draft, setDraft] = useState(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState(null);
  const [notice, setNotice] = useState("");
  const [quantities, setQuantities] = useState({});
  const node = nodes.data?.find((n) => n.id === nodeChoice) || nodes.data?.[0];
  const saved = report.data?.costs.find((c) => c.node_id === node?.id);
  const costs = draft?.node_id === node?.id ? draft : saved;
  const enabled = plans.data?.filter((p) => p.enabled !== false) || [];
  async function save(e) {
    e.preventDefault();
    setBusy(true);
    setError(null);
    try {
      await api("/admin/hosting-costs", { method: "PUT", body: costs });
      setDraft(null);
      report.refresh();
      setNotice("Costs saved. Selling prices have not been changed.");
    } catch (err) {
      setError(err);
    } finally {
      setBusy(false);
    }
  }
  function edit() {
    setDraft(
      saved || {
        node_id: node.id,
        currency: enabled[0]?.currency || "USD",
        node_monthly_cents: "",
        overhead_monthly_cents: "",
        per_server_monthly_cents: "",
        fee_bps: "",
        fee_fixed_cents: "",
        tax_bps: "",
        occupancy_bps: 5000,
        target_margin_bps: 3000,
      },
    );
  }
  const forecast =
    costs &&
    !nodes.error &&
    !plans.error &&
    !report.error &&
    fields.every(
      ([key, , min, max]) => Number.isInteger(costs[key]) && costs[key] >= min && costs[key] <= max,
    )
      ? mixForecast(node, enabled, costs, quantities)
      : null;
  return (
    <>
      <ErrorMessage error={report.error || nodes.error || plans.error || error} />
      {notice && (
        <p className="notice success" role="status">
          {notice}
        </p>
      )}
      <section className="panel management-panel">
        <h2>Sales & margins</h2>
        <p className="field-help">
          Recorded order sales and a launch forecast. Demo payments are not real income. This report
          is not a cash ledger: processor payouts and the timing of refunds are not tracked.
        </p>
        <label className="field">
          Sales month (UTC)
          <input
            type="month"
            required
            value={month}
            onChange={(e) => {
              if (e.target.value) setMonth(e.target.value);
            }}
          />
        </label>
        {report.loading ? (
          <Loading />
        ) : report.data?.sales.length ? (
          <FinanceTable
            headings={[
              "Provider",
              "Currency",
              "Paid orders",
              "Paid value",
              "Refunded orders",
              "Refunded value",
            ]}
            rows={report.data.sales.map((s) => [
              s.provider === "mock" ? "Demo payments" : s.provider || "Unclassified provider",
              s.currency,
              s.paid_count,
              money(s.paid_cents, s.currency),
              s.refunded_count,
              money(s.refunded_cents, s.currency),
            ])}
          />
        ) : (
          <p>No paid orders recorded for this month.</p>
        )}
        <p className="field-help">
          Values use each order’s purchased price, not today’s plan price. Refund totals refer to
          orders originally paid in this selected month. Currencies and providers are kept separate.
        </p>
      </section>
      <section className="panel management-panel">
        <h2>Cost assumptions</h2>
        <p>
          Enter your real bill and allowances before using the forecast. Include support time,
          backups, email and other costs. Payment fees vary by provider and country; use your actual
          rate.{" "}
          <a
            className="text-link"
            href="https://stripe.com/pricing"
            target="_blank"
            rel="noreferrer"
          >
            Stripe pricing ↗
          </a>
        </p>
        {nodes.loading ? (
          <Loading />
        ) : node ? (
          <>
            <label className="field">
              Hosting node
              <Select
                value={node.id}
                onChange={(e) => {
                  setNode(e.target.value);
                  setDraft(null);
                  setQuantities({});
                  setNotice("");
                }}
              >
                {nodes.data.map((n) => (
                  <option key={n.id} value={n.id}>
                    {n.name}
                  </option>
                ))}
              </Select>
            </label>
            {!costs && (
              <p className="notice">
                No costs saved yet. We cannot estimate profitability without your bill.
              </p>
            )}
            <button className="button secondary" onClick={edit} disabled={busy}>
              Edit cost assumptions
            </button>
            {draft && costs && (
              <form onSubmit={save} className="finance-cost-form">
                <label className="field">
                  Cost currency
                  <input
                    value={costs.currency}
                    required
                    pattern="[A-Z]{3}"
                    maxLength={3}
                    onChange={(e) => setDraft({ ...costs, currency: e.target.value.toUpperCase() })}
                  />
                </label>
                <div className="field-row">
                  {fields.map(([key, label, min, max]) => (
                    <label className="field" key={key}>
                      {label}
                      <input
                        type="number"
                        required
                        min={min / 100}
                        max={max / 100}
                        step="0.01"
                        value={costs[key] === "" ? "" : costs[key] / 100}
                        onChange={(e) =>
                          setDraft({
                            ...costs,
                            [key]:
                              e.target.value === "" ? "" : Math.round(Number(e.target.value) * 100),
                          })
                        }
                      />
                    </label>
                  ))}
                </div>
                <p className="field-help">
                  Occupancy and target margin are modelling assumptions; adjust the example 50% /
                  30% values. Enter every cost explicitly, including zero where appropriate. Use one
                  currency; no exchange conversion is assumed.
                </p>
                <button className="button primary" disabled={busy}>
                  {busy ? "Saving…" : "Save costs"}
                </button>
              </form>
            )}
          </>
        ) : (
          <p>Add a hosting node first.</p>
        )}
      </section>
      {node && costs && forecast && (
        <section className="panel management-panel" aria-label="Profitability forecast">
          <h2>Can these plans cover your costs?</h2>
          <p className="field-help">
            Additional sales only. Existing allocations stay reserved; existing income is not
            included. The forecast charges the full node bill and overhead to new sales. CPU, RAM,
            disk and free addresses limit the number you can add. Resource fit is not a guarantee of
            smooth Minecraft play.
          </p>
          <FinanceTable
            headings={[
              "Plan",
              "New slots",
              "Expected sales",
              "Monthly profit",
              "Margin",
              "Break-even servers",
              "Price floor for target",
            ]}
            rows={enabled.map((p) => {
              const result = planMargin(node, p, costs);
              return [
                p.name,
                result.slots ?? "—",
                result.customers ?? "—",
                result.error || (
                  <span className={result.profit < 0 ? "finance-loss" : "finance-profit"}>
                    {money(result.profit, costs.currency)}
                  </span>
                ),
                result.margin == null ? "—" : `${(result.margin * 100).toFixed(1)}%`,
                result.breakEven == null
                  ? "Not reachable"
                  : `${result.breakEven}${result.canBreakEven ? "" : " · exceeds capacity"}`,
                result.floorPrice == null
                  ? result.error
                    ? "—"
                    : "Target not achievable within price limits"
                  : `${money(result.floorPrice, costs.currency)}/${p.billing_period === "YEARLY" ? "year" : "month"}`,
              ];
            })}
          />
          <h3>Try a mix of plans</h3>
          <div className="finance-mix-grid">
            {enabled.map((p) => (
              <label className="field" key={p.id}>
                {p.name} · {price(p)}
                <input
                  type="number"
                  min="0"
                  max="10000"
                  step="1"
                  value={quantities[p.id] ?? 0}
                  onChange={(e) => setQuantities({ ...quantities, [p.id]: Number(e.target.value) })}
                />
              </label>
            ))}
          </div>
          {forecast.error ? (
            <p className="error-message">{forecast.error}</p>
          ) : (
            <p
              className={`notice ${forecast.fits && forecast.profit >= 0 ? "success" : "finance-warning"}`}
              role="status"
            >
              {forecast.fits
                ? "Fits currently available resources"
                : "This mix exceeds available resources or addresses"}{" "}
              · Estimated monthly {forecast.profit < 0 ? "loss" : "profit"}:{" "}
              {money(Math.abs(forecast.profit), costs.currency)}
              {forecast.margin != null ? ` · ${(forecast.margin * 100).toFixed(1)}% margin` : ""}.
            </p>
          )}
          <p className="field-help">
            Price floors include a cent-rounding allowance and assume the entered occupancy and
            costs. They do not change plans automatically or guarantee profit. Update selling prices
            in Plans after reviewing real costs and testing server performance.
          </p>
        </section>
      )}
    </>
  );
}

function FinanceTable({ headings, rows }) {
  return (
    <div className="table-scroll">
      <Table className="management-table">
        <thead>
          <tr>
            {headings.map((heading) => (
              <th key={heading}>{heading}</th>
            ))}
          </tr>
        </thead>
        <tbody>
          {rows.map((row, index) => (
            <tr key={`${row[0]}:${index}`}>
              {row.map((value, column) => (
                <td key={column}>{value}</td>
              ))}
            </tr>
          ))}
        </tbody>
      </Table>
    </div>
  );
}
