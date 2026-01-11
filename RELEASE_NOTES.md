# Playerr v0.3.12: Hotfix - Steam Sync
*   **Steam Sync:** Fixed critical `FOREIGN KEY constraint failed` error when syncing Steam games.
    *   Explicitly assigned `PlatformId = 6` (PC) to all imported Steam games to ensure database integrity.

---

# Playerr v0.3.11: Hotfix - Schema Stability
*   **Database:** Added missing `IsInstallable` column to the automatic schema patcher. This resolves the `SQLite Error 1` crash for users upgrading from older versions.
