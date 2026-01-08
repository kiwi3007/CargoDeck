# Playerr v0.3.0: The "Install Anywhere" Update 🚀

We've improved the core engine to be smarter, faster, and more compatible with your messy game libraries.

### 🌟 New Features
*   **Smart Installer Detection:** The scanner now digs deeper! Support added for nested installers (Depth-1) and fuzzy naming patterns (`setup_*.exe`, `install*.exe`). If it's there, Playerr will find it.
*   **Visual Status Indicators:** New **Green "Install Ready" Button** in the UI instantly tells you which games have valid installers detected.
*   **Universal macOS Support:** Official builds now available for both **Apple Silicon (M1/M2/M3)** and **Intel** Macs.
*   **Internationalization:** Full documentation available in **Spanish** (`README.es.md`).
*   **Security Hardening:** Credential management moved to secure external configuration files.

### 🐛 Bug Fixes
*   **Critical Installation Fix:** Resolved an issue where valid installers in subfolders (common with GOG) were detected but failed to launch.
*   **Database Stability:** Fixed SQLite schema issues ensuring robust metadata persistence.
*   **Cross-Platform Paths:** Improved path resolution logic for mixed-OS environments.

### 📦 Updating
*   **Docker:** `docker-compose pull && docker-compose up -d`
*   **Desktop:** Download the latest installer for your OS below.
