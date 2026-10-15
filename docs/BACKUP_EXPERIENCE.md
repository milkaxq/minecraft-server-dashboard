# Backup and recovery experience

The Backups page starts with recovery status and history, followed by daily backup
settings. A usable recovery copy must have both successful and completed flags.
Failed or unfinished copies do not count. The summary shows the latest successful
completion time and reminds owners that later changes need a newer copy.

Backups contain server files (worlds, settings and add-on files), subject to backup
exclusions. A completed backup is not proof of a tested restore. The website does
not claim that backup status alone guarantees recovery.

Restore confirmation identifies the selected copy and its creation time. It
explains replacement of current server files and loss of later changes, suggests
preserving the current files first, and requires an explicit acknowledgement.
Restore is disabled if the server starts, the backup disappears/becomes unusable,
or backup history cannot be checked while the dialog is open. Backend ownership,
offline checks and confirmation enforcement still apply.

Deleting the only completed recovery copy shows a specific warning. Full plans
explain how to free a slot without implying that this page automatically removes
backups. Plans with zero slots explain that backup access needs the host's help.

Validation includes the full 142-test browser suite and targeted checks after
moving daily settings below recovery status. Tests cover failed/unfinished
backups, acknowledgement before restore, server startup during confirmation,
mobile/laptop layout and daily-backup access for suspended owners/moderators.
Frontend lint, production build and formatting checks passed. Live verification
for this change is read-only: no customer backup is created, restored or deleted.

Deployment checks: production readiness and the backup page returned 200. The
backup API returned 401 without a session, confirming that backup data still
requires authentication. No live backup mutations were submitted.

## Choosing a recovery copy

The newest usable copy is selected by backup creation time, rather than completion
time: a slow older backup should not outrank a newer snapshot. It is identified in
history and has download/restore shortcuts in the protection summary. Restore
still opens the acknowledgement dialog and respects the offline check.

If the newest usable copy started more than 48 hours ago, the summary recommends
making a fresh copy before changes. This is a reminder, not a claim that an older
backup is corrupt or cannot be restored. The age display updates once per minute.

Six targeted browser checks passed, including the slow-old/new-success/failed-new
selection case, restore acknowledgement, changing power state and mobile/laptop
visual checks. Frontend lint and production build passed.

## Unavailable history

An unsuccessful history read is distinct from an empty account: the page shows a
retry message instead of “No backups yet”. Entries retained after a failed poll
are explicitly identified as coming from the last successful check, and the slot
count is unavailable until history loads successfully.

Create, download, restore and delete controls pause while history is unavailable.
An open deletion dialog also verifies that the selected backup still exists and
is completed; its handler repeats this check before submitting. Restore retains
its additional successful-backup, stopped-server and acknowledgement checks.
Refresh recovers the controls after a successful read without submitting any
backup mutation. Server-side authorization and validation remain authoritative.

Validation: 25 related browser checks passed, including initial history failure,
empty-history recovery, failure during deletion confirmation, existing restore
and schedule flows, and mobile/laptop visual audits. Lint, formatting and the
production build passed. All backup mutations in these checks use API fixtures.
