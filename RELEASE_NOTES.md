# Playerr v0.3.7: Hotfix - Media Scanner 🔍

### 🐛 Bug Fixes
*   **Media Scanner (Hotfix):** Implemented **dynamic platform ID lookup** to solve the crash when adding games. This ensures the scanner works correctly on both new installations (PC ID 6) and legacy databases (PC ID 1).
*   **PSP Support:** Added official support and database seeding for PlayStation Portable (PSP) games (ID 38).
*   **Library:** Fixed the "Clear Library" trash button which was previously inactive. Added a new "Clean Library" option in Settings for easier management.

---

# Playerr v0.3.6: Hotfix - Add Game UI 🩹

### 🐛 Bug Fixes
*   **Frontend Library:** Fixed an issue where adding a game without a selected platform would try to use the legacy ID (1) instead of the new standard PC ID (6), causing database errors on new installations.

---

# Playerr v0.3.5: Hotfix - Fresh Install Fix 🚑

###  Critical Bug Fixes
*   **Database Seeding:** Fixed a critical issue where fresh installations would fail to add games due to incorrect default platform IDs (Foreign Key Error 19). New databases now initialize correctly with standard IDs for PC, Mac, and Linux.
