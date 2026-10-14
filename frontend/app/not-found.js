import Link from "next/link";
import { Brand } from "../components/ui";

export default function NotFound() {
  return (
    <main className="public-player-page">
      <header>
        <Brand href="https://playblockyard.com" />
      </header>
      <section className="player-hero">
        <span className="eyebrow">A LITTLE OFF THE MAP</span>
        <h1>This path doesn’t lead to a world.</h1>
        <p>The page may have moved. Your worlds are waiting in the dashboard.</p>
        <div className="player-address">
          <Link className="button primary" href="/dashboard">
            Back to your worlds
          </Link>
          <Link className="button secondary" href="https://playblockyard.com">
            Visit website
          </Link>
        </div>
      </section>
    </main>
  );
}
