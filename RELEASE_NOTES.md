# Playerr v0.3.5: Hotfix - Fresh Install Fix 🚑

### 🐛 Critical Bug Fixes
*   **Database Seeding:** Fixed a critical issue where fresh installations would fail to add games due to incorrect default platform IDs (Foreign Key Error 19). New databases now initialize correctly with standard IDs for PC, Mac, and Linux.

---

# Playerr v0.3.4: The "Sponsor & Sync" Update 🔧

We've improved the core engine to be smarter, faster, and more compatible with your messy game libraries.

### 🌟 New Features
*   **Smart Installer Detection:** The scanner now digs deeper! Support added for nested installers (Depth-1) and fuzzy naming patterns (`setup_*.exe`, `install*.exe`). If it's there, Playerr will find it.
*   **Visual Status Indicators:** New **Green "Install Ready" Button** in the UI instantly tells you which games have valid installers detected.
*   **Universal macOS Support:** Official builds now available for both **Apple Silicon (M1/M2/M3)** and **Intel** Macs.
*   **Internationalization:** Full documentation available in **Spanish** (`README.es.md`).
*   **Security Hardening:** Credential management moved to secure external configuration files.
*   **Platform Metadata:** Search results now display platform badges and search limit increased to 100.

### 🐛 Bug Fixes
*   **Critical Installation Fix:** Resolved an issue where valid installers in subfolders were detected but failed to launch.
*   **Database Stability:** Fixed SQLite schema issues ensuring robust metadata persistence.
*   **Cross-Platform Paths:** Improved path resolution logic for mixed-OS environments.

### 📦 Updating
*   **Docker:** `docker-compose pull && docker-compose up -d`
*   **Desktop:** Download the latest installer for your OS below.