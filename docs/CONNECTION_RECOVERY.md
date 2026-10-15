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
