import "./globals.css";
import { AuthProvider } from "../components/auth";
export const metadata = {
  title: "Blockyard — your Minecraft worlds",
  description: "A simpler home for your Minecraft servers.",
};
export default function Layout({ children }) {
  return (
    <html lang="en">
      <body>
        <AuthProvider>{children}</AuthProvider>
      </body>
    </html>
  );
}
