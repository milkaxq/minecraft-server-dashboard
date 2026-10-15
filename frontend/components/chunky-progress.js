"use client";
import { useEffect, useState } from "react";
import { useAuth } from "./auth";
import { consoleURL } from "../lib/api";
import { parseChunkyProgress } from "../lib/chunky-progress";

export function ChunkyProgress({ id, world, running, allowed }) {
  const { api } = useAuth();
  const [result, setResult] = useState(null);
  const [connection, setConnection] = useState("connecting");
  const [attempt, setAttempt] = useState(0);
  const [now, setNow] = useState(Date.now);
  useEffect(() => {
    if (!running || !allowed) return;
    let alive = true;
    let socket;
    const controller = new AbortController();
    const clock = setInterval(() => setNow(Date.now()), 5000);
    async function connect() {
      setConnection("connecting");
      setResult(null);
      try {
        const ticket = await api(`/servers/${id}/console-ticket`, {
          method: "POST",
          signal: controller.signal,
        });
        if (!alive) return;
        socket = new WebSocket(consoleURL(id), ["hosting.console.v1", `ticket.${ticket.ticket}`]);
        socket.onmessage = (event) => {
          if (!alive || typeof event.data !== "string" || event.data.length > 65536) return;
          let message;
          try {
            message = JSON.parse(event.data);
          } catch {
            return;
          }
          if (message.event === "connected") setConnection("connected");
          if (message.event !== "console output" || typeof message.args?.[0] !== "string") return;
          for (const line of message.args[0].split("\n")) {
            const progress = parseChunkyProgress(line);
            if (progress?.world === world) {
              const received = Date.now();
              setResult({ ...progress, received, id });
              setNow(received);
            }
          }
        };
        socket.onerror = () => {
          if (alive) setConnection("disconnected");
        };
        socket.onclose = () => {
          if (alive) setConnection("disconnected");
        };
      } catch (error) {
        if (alive && error.name !== "AbortError") setConnection("disconnected");
      }
    }
    connect();
    return () => {
      alive = false;
      clearInterval(clock);
      controller.abort();
      socket?.close();
    };
  }, [api, id, world, running, allowed, attempt]);
  if (!allowed || !running) return null;
  const report = result?.id === id && result.world === world ? result : null;
  const fresh = report && now - report.received < 30000 && connection === "connected";
  return (
    <section className="chunky-progress" aria-label="Terrain preparation progress">
      <h3>Preparation progress</h3>
      <p className="field-help">
        {connection === "connecting"
          ? "Connecting to server updates…"
          : connection === "disconnected"
            ? "Updates disconnected. Task status cannot be confirmed."
            : "Listening for Chunky updates. No commands are run by opening this panel."}
      </p>
      {report ? (
        <>
          <label>
            Last reported: {report.percent}%<progress value={report.percent} max="100" />
          </label>
          <p>
            {report.chunks.toLocaleString()} chunks · {report.rate} chunks/second · Estimated
            remaining {report.eta}
          </p>
          <p className="field-help">
            {fresh
              ? "Received recently. Console history can include older reports; this does not confirm the task is still running."
              : "This report may be out of date. It does not confirm the task is still running or has finished."}
          </p>
        </>
      ) : (
        <p>
          No progress received for this world yet. A task may be paused, finished, not started, or
          using a different message format. Check View progress for the full response.
        </p>
      )}
      {connection === "disconnected" && (
        <button
          type="button"
          className="button secondary"
          onClick={() => setAttempt((value) => value + 1)}
        >
          Reconnect updates
        </button>
      )}
    </section>
  );
}
