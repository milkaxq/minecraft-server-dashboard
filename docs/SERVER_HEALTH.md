# Simple server health

The Simple overview now wraps the existing startup check in one health card.
It uses the overview's existing usage readings rather than another metrics poll.
Lifecycle, starting/stopping, agreement, offline and unavailable states explain
what the owner should do next. A running game process does not guarantee that a
particular player can join.

Resource notices compare readings with purchased allocations: disk at 85%, CPU
at 90% of its allowance, and memory at 90%. These are advisory pressure readings,
not tick-rate or ping measurements. Memory use can be normal Java reservation.
The lag help explains the measurement limits and suggests comparing whether all
players have delays or only one. No game commands or tuning run automatically.

Startup findings remain visible with their existing safe explanations and console
fallback. Findings from a log do not by themselves prove the game is offline.
No raw logs or environment secrets are attached to reports.

Report a problem opens a reviewable form with the server, source page, request
reference, displayed state and startup-check timestamp. Feedback accepts only
allowlisted state values and bounded valid UTC check timestamps from the URL.
The context is marked as reported by the browser and appended to the user's
reviewed message; its length is reserved within the existing message limit.
The failing usage request's reference takes priority when live metrics fail.
Reports still require the user to press Send report.

Validation: the full 145-test browser regression run passed, followed by seven
focused checks including unavailable metrics, safe support context and rejecting
arbitrary query values. Frontend lint, production build and formatting passed.
No Minecraft client or gameplay test was performed for this dashboard change.

Deployment verification: production readiness and the overview route returned
200. Usage and startup-check API routes returned 401 without a session. Checks
were read-only; no live support report or Minecraft command was submitted.

## Guided lag troubleshooting and diagnostic copy

The existing health card now offers three symptom choices: delayed blocks or
rubber-banding, slow chunk loading, and low FPS/stuttering. Guidance distinguishes
shared server workload from a single player's connection or graphics performance.
It suggests comparing explored/new terrain, pausing terrain generation while
playing, and changing view distance one step at a time. It never runs commands,
changes settings or recommends buying resources based on one reading. Settings
links appear only for members with settings permission.

Copy diagnostic report prepares a preview and attempts to copy the same snapshot.
Clipboard denial leaves a read-only text area for manual selection. The snapshot
contains an application server UUID, allowlisted software name, selected symptom,
resource values/allowances and process state/uptime. It does not serialize API
objects or include player identities, hostnames, logs, file contents, request error
text or credentials. Non-finite, negative and nonnumeric readings are unavailable.
When usage is unavailable, retained readings are omitted. Capture time describes
report creation, not measurement time; the report uses the last dashboard reading.
Tick rate and ping/packet loss are explicitly unmeasured. No report is sent or
persisted automatically, and no new network requests are added for this feature.

Nine focused checks cover report fields/privacy, invalid values, unavailable usage,
clipboard fallback, symptom navigation, existing support reporting and mobile and
laptop layouts. Lint, formatting and production build also passed. Tests use
mocked APIs; no gameplay performance diagnosis was performed.
