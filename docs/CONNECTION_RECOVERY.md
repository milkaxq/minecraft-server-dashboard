# Connection recovery

Dashboard requests have a 30-second deadline through response-body consumption.
Raw uploads and downloads allow 120 seconds. Timeout aborts the browser request;
it cannot undo an operation already accepted by the backend. Mutation errors ask
the customer to check the result before retrying. Existing idempotency keys remain
in their owning action flows. No write is automatically retried or queued.

Caller cancellation remains AbortError, distinct from a request timeout. Timers
and abort listeners are cleaned up. Malformed successful JSON is an error rather
than a false success with null data. HTTP errors keep status, request reference,
and rate-limit retry timing. HEAD and 204 responses do not require JSON bodies.

When the browser reports offline, a single-line notice appears and new API calls
fail locally. This indicator reflects browser connectivity, not the reachability
or latency of a particular Minecraft server. It adds no probes or network polls.

Periodic reads still pause while hidden/offline and respect Retry-After. Failed
one-time reads can recover on reconnect or returning to a visible page, including
when the old request fails just after reconnection. Successful one-time forms are
not refreshed on reconnect, preserving edits. Recovery neither overlaps a pending
read nor replays writes. Reconnection during a pending read schedules at most one
recovery if that read fails; it does not create an endless retry loop.

Validation uses mocked API/browser connections and covers header/body/download
timeouts, cancellation, malformed responses, rate limits, preserved settings,
failed setup recovery, save timeout/retry keys and existing customer flows. Live
checks are read-only; web connectivity does not measure Minecraft ping or ticks.

## Management power controls

Settings, Files and the other management pages hide power actions outside the
member's permissions. Requests use an immediate in-flight guard and reuse their
key for an identical failed action. The backend remains authoritative for
permission, agreement and state validation.

An accepted Start/Stop keeps its button pending until a running/offline reading
arrives. Restart waits for a fresh read and respects any reported starting/stopping
state; “Restart requested” describes acceptance, not proof of a completed restart.
Check status only refreshes the read and never resends the action.

Unavailable status, transitions and pending actions block file/settings saves.
Drafts stay available to edit and copy. Controls also pause if server details are
unavailable or deletion is pending. Per-server keyed content prevents pending
state and retry keys from carrying into another server's page. The overview's
normal power buttons likewise reject stale readings after a poll fails; a failed
agreement read still leaves a confirmed-running server's Stop available.
