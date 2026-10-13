import { PublicPlayerJoin } from "../../../components/player-invite";
export const metadata = {
  title: "Join your friends — Blockyard",
  robots: { index: false, follow: false },
  referrer: "no-referrer",
};
export default async function Page({ params }) {
  const { slug } = await params;
  return <PublicPlayerJoin key={slug} slug={slug} />;
}
