// Forecasts only. Billing continues to resolve purchased terms on the Go backend.
export function availableSlots(node, plan) {
  if (
    !node?.enabled ||
    !plan ||
    ![plan.ram_mb, plan.disk_mb, plan.cpu_limit].every((n) => Number.isFinite(n) && n > 0)
  )
    return 0;
  return Math.max(
    0,
    Math.min(
      Math.floor((node.ram_capacity_mb - node.allocated_ram_mb) / plan.ram_mb),
      Math.floor((node.disk_capacity_mb - node.allocated_disk_mb) / plan.disk_mb),
      Math.floor((node.cpu_capacity - node.allocated_cpu) / plan.cpu_limit),
      Number.isInteger(node.available_allocations) ? node.available_allocations : 0,
    ),
  );
}
export function monthlySale(plan, costs, gross = plan.price_cents) {
  const months = plan.billing_period === "YEARLY" ? 12 : 1;
  const revenue = Math.floor((gross * 10000) / (10000 + costs.tax_bps)) / months;
  const fees = (Math.ceil((gross * costs.fee_bps) / 10000) + costs.fee_fixed_cents) / months;
  return { revenue, fees, contribution: revenue - fees - costs.per_server_monthly_cents };
}
export function planMargin(node, plan, costs) {
  if (plan.currency !== costs.currency)
    return { error: "Currency differs; enter costs in the plan’s currency." };
  const slots = availableSlots(node, plan);
  if (!slots)
    return {
      slots: 0,
      error: "No additional server of this plan fits the current capacity and free addresses.",
    };
  const customers = Math.floor((slots * costs.occupancy_bps) / 10000);
  if (!customers)
    return {
      slots,
      error:
        "Expected occupancy is less than one customer. Increase the assumption to model sales.",
    };
  const fixed = costs.node_monthly_cents + costs.overhead_monthly_cents;
  const sale = monthlySale(plan, costs);
  const profit = customers * sale.contribution - fixed;
  const margin = sale.revenue > 0 ? profit / (customers * sale.revenue) : null;
  const breakEven = sale.contribution > 0 ? Math.ceil(fixed / sale.contribution) : null;
  // Use an affine price floor with an allowance for cent rounding, rather than
  // binary-searching a non-monotonic staircase of tax/fee rounding.
  const months = plan.billing_period === "YEARLY" ? 12 : 1;
  const retention = 1 - costs.target_margin_bps / 10000;
  const coefficient = retention / (1 + costs.tax_bps / 10000) - costs.fee_bps / 10000;
  const rounding = (costs.tax_bps ? retention : 0) + (costs.fee_bps ? 1 : 0);
  const suggested =
    coefficient > 0
      ? Math.ceil(
          (months * (fixed / customers + costs.per_server_monthly_cents) +
            costs.fee_fixed_cents +
            rounding) /
            coefficient,
        )
      : null;
  const floorPrice = suggested !== null && suggested <= 100000000 ? suggested : null;
  return {
    slots,
    customers,
    profit,
    margin,
    breakEven,
    floorPrice,
    canBreakEven: breakEven !== null && breakEven <= slots,
  };
}
export function mixForecast(node, plans, costs, quantities) {
  let cpu = 0,
    ram = 0,
    disk = 0,
    count = 0,
    revenue = 0,
    fees = 0;
  for (const plan of plans) {
    const qty = quantities[plan.id] || 0;
    if (!Number.isInteger(qty) || qty < 0 || qty > 10000)
      return { error: "Use whole server counts between 0 and 10,000." };
    if (qty && plan.currency !== costs.currency)
      return { error: "Do not combine currencies in one forecast." };
    const sale = monthlySale(plan, costs);
    count += qty;
    cpu += plan.cpu_limit * qty;
    ram += plan.ram_mb * qty;
    disk += plan.disk_mb * qty;
    revenue += sale.revenue * qty;
    fees += sale.fees * qty;
  }
  const fits =
    node.enabled &&
    cpu <= node.cpu_capacity - node.allocated_cpu &&
    ram <= node.ram_capacity_mb - node.allocated_ram_mb &&
    disk <= node.disk_capacity_mb - node.allocated_disk_mb &&
    count <= node.available_allocations;
  const profit =
    revenue -
    fees -
    count * costs.per_server_monthly_cents -
    costs.node_monthly_cents -
    costs.overhead_monthly_cents;
  return { fits, count, revenue, fees, profit, margin: revenue > 0 ? profit / revenue : null };
}
