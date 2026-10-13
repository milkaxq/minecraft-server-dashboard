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
