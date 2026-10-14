import { headers } from "next/headers";
import { isMarketingHost } from "../lib/site";
import { Landing } from "../components/landing";
export default async function Page() {
  const marketing = isMarketingHost((await headers()).get("host") || "");
  return <Landing appOrigin={marketing ? "https://app.playblockyard.com" : ""} />;
}
