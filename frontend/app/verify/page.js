import { AuthScreen } from "../../components/auth-screen";
export const metadata = { referrer: "no-referrer", robots: { index: false, follow: false } };
export default function Page() {
  return <AuthScreen mode="verify" />;
}
