# Group choices, sales and margin planning

The customer chooses a play style and one of four group sizes: Close friends
(up to 4 players), Game night (8), Growing group (16), Community (30). These
are intent categories, not four invented resource packages or enforced player
limits. `recommendGroupPlan` selects the least expensive monthly-equivalent
enabled database plan with a sufficient player recommendation in one currency.
Two group sizes can share a price when the same plan fits. Mods and larger worlds
may require more memory; players can expand the plan comparison and choose a
suitable plan explicitly. Only the purchased plan UUID is sent for checkout.
The Go backend continues resolving every price and resource limit from PostgreSQL.

No selling prices are changed automatically. Admin → Finances records actual
monthly VPS cost, other overhead, support/backup allowance per server, processor
fees, tax included in displayed prices, expected occupancy and target margin.
Enter actual invoices and provider fees. The example 50% occupancy and 30% margin
are editable planning assumptions, not evidence about demand or achieved profit.
Costs are stored in `hosting_cost_profiles`, administrator access is required,
and changes create audit records. Existing plans, orders and subscriptions retain
all their purchased terms. Unknown costs do not produce a profit forecast.

For an additional-sales forecast:

- Available slots are bounded by unallocated CPU, RAM, disk and game allocations.
  Allocations reserved for addon ports are excluded.
- The whole node bill and entered overhead are charged to these new sales;
  existing allocations stay reserved and existing income is not included.
- Expected customer count is floored from available slots × expected occupancy.
- Revenue excludes the entered included-tax portion. Payment fees and per-server
  costs are deducted before fixed hosting costs. Margin is profit divided by
  revenue excluding that tax portion.
- Annual prices and payment fees are averaged over twelve months; the fixed
  processing fee is charged once per annual payment, not twelve times.
- The suggested price floor includes a conservative cent-rounding allowance.
  A target with insufficient revenue after taxes/fees, or a price above the
  permitted plan price, is reported as unreachable. This does not guarantee
  demand, smooth game performance or complete coverage of unentered expenses.
- A mixed-plan forecast checks every resource total and free addresses. It does
  not reserve resources; actual provisioning retains its transaction locks.

Operators must configure usable hosting capacity after reserving resources for
OS, services and backup storage. The forecast reads configured capacities, not
advertised VPS specifications. A currently unavailable capacity/plan/report read
suppresses the forecast. Capacity and financial snapshots refresh every 30 seconds.

The monthly order-sales report groups providers and currencies separately, using
the order's purchased price and UTC `paid_at` month. Mock payments are clearly
labelled as demo sales. Refunded totals refer to orders paid in that selected month;
the actual refund timing, processor balances, payouts and business tax filings are
not a cash ledger in this MVP. Do not describe this report as audited profit.

Run backend finance, HTTP authorization, billing and capacity tests against a
disposable PostgreSQL database via `TEST_DATABASE_URL`; never a live database.
Frontend tests cover group selection, purchased order fields, occupancy/fees,
annual billing, tax rounding, resource constraints and the private admin form.
