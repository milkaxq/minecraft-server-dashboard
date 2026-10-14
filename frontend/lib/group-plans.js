// Group sizes describe intent. Prices, allowances and player recommendations come from the API.
export const PLAYER_GROUPS = [
  {
    id: "friends",
    players: 4,
    title: "Close friends",
    description: "A small world for you and a few friends.",
    icon: "🌱",
  },
  {
    id: "night",
    players: 8,
    title: "Game night",
    description: "Bring the whole friend group together.",
    icon: "🏕️",
  },
  {
    id: "growing",
    players: 16,
    title: "Growing group",
    description: "More friends, more shared adventures.",
    icon: "🏘️",
  },
  {
    id: "community",
    players: 30,
    title: "Community",
    description: "A bigger gathering with room to grow.",
    icon: "🏰",
  },
];
export function recommendGroupPlan(
  plans,
  players,
  currency = plans?.find((plan) => plan.enabled !== false)?.currency,
) {
  return (
    plans
      ?.filter(
        (p) => p.enabled !== false && p.currency === currency && p.player_recommendation >= players,
      )
      .sort(
        (a, b) =>
          a.price_cents / (a.billing_period === "YEARLY" ? 12 : 1) -
            b.price_cents / (b.billing_period === "YEARLY" ? 12 : 1) ||
          a.ram_mb - b.ram_mb ||
          a.id.localeCompare(b.id),
      )[0] || null
  );
}
