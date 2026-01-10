# Playerr v0.3.9: Maintenance Release

### Changes
*   **Documentation:** Cleaned up documentation, removing non-text elements to improve professionalism.
*   **Internal:** Updated build workflows and memory persistence for release management.

---

# Playerr v0.3.8: Stable Release

### Changes
*   **Docker:** Consolidated fix for database initialization on persistent volumes.
*   **Maintenance:** Version bump and stability improvements.

---

# Playerr v0.3.7: Hotfix - Media Scanner

### Bug Fixes
*   **Media Scanner (Hotfix):** Implemented **dynamic platform ID lookup** to solve the crash when adding games. This ensures the scanner works correctly on both new installations (PC ID 6) and legacy databases (PC ID 1).
*   **PSP Support:** Added official support and database seeding for PlayStation Portable (PSP) games (ID 38).
*   **Library:** Fixed the "Clear Library" trash button which was previously inactive. Added a new "Clean Library" option in Settings for easier management.
*   **Docker/Database:** Fixed a critical issue where existing Docker installations (with persistent volumes) would fail to add games because new Platform IDs were not being seeded. The database migration is now robust.
