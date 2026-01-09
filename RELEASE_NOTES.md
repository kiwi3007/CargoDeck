# Playerr v0.3.7: Hotfix - Media Scanner 🔍

### 🐛 Bug Fixes
*   **Media Scanner:** Fixed extensive platform ID mismatches that caused the scanner to fail when adding found games (PC IDs were updated to match the v0.3.5 database schema).
*   **PSP Support:** Added official support and database seeding for PlayStation Portable (PSP) games (ID 38).

---

# Playerr v0.3.6: Hotfix - Add Game UI 🩹

### 🐛 Bug Fixes
*   **Frontend Library:** Fixed an issue where adding a game without a selected platform would try to use the legacy ID (1) instead of the new standard PC ID (6), causing database errors on new installations.

---

# Playerr v0.3.5: Hotfix - Fresh Install Fix 🚑

###  Critical Bug Fixes
*   **Database Seeding:** Fixed a critical issue where fresh installations would fail to add games due to incorrect default platform IDs (Foreign Key Error 19). New databases now initialize correctly with standard IDs for PC, Mac, and Linux.
