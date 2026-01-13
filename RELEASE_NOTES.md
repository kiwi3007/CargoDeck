# Playerr v0.3.13: UI Refinement & Stability
*   **UI/UX Polishing:**
    *   **Visual Consistency:** Standardized checkboxes and action buttons across Settings.
    *   **Media Folder:** Restored the missing "Scan Now" button and improved layout.
*   **Stability:**
    *   **Persistent Modals:** implemented custom, persistent delete confirmation modals for the Status and Library pages, replacing flaky browser alerts.
    *   **Bug Fixes:** Fixed JSX syntax errors and added missing translation keys to ensure reliable builds.

---

# Playerr v0.3.12: Hotfix - Steam Sync
*   **Steam Sync:** Fixed critical `FOREIGN KEY constraint failed` error when syncing Steam games.
    *   Explicitly assigned `PlatformId = 6` (PC) to all imported Steam games to ensure database integrity.


