import "./globals.css";
import { headers } from "next/headers";
import { isMarketingHost } from "../lib/site";
import "./dashboard-theme.css";
import { AuthProvider } from "../components/auth";
export const metadata = {
  title: "Blockyard — your Minecraft worlds",
  description: "A simpler home for your Minecraft servers.",
};
export default async function Layout({ children }) {
  const marketing = isMarketingHost((await headers()).get("host") || "");
  return (
    <html lang="en">
      <body>
        <AuthProvider marketing={marketing}>{children}</AuthProvider>
      </body>
    </html>
  );
}
