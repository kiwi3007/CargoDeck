# Playerr v0.4.5 - The Connected Update 🔌
This release introduces advanced hardware connectivity features and expands platform support.

### ⚡ Advanced USB Protocol
*   **Direct Hardware Transfer**: Implemented a robust USB transfer system allowing direct installation to connected devices.
*   **Smart Permissions**: Improved USB handling on macOS to gracefully handle permission denied errors without requiring root access.
*   **Protocol Integrity**: Enhanced handshake and error handling to ensure data integrity during transfers.
*   **Credits**: Implementation based on the excellent work by `rashevskyv/dbi`.

### 🛠️ Improvements
*   **UI/UX**: Added "Close Anyway" option for modal dialogs to prevent UI lockups.
*   **Platform Detection**: Refined logic for hiding irrelevant actions (Play/Install) on specific platforms.

# Playerr v0.4.4 - The Cleanup & Scanner Update 🧹
This release focuses on scanning accuracy, Wine/Whisky integration, and security improvements for Linux.

### 🔍 Scanner Intelligence
*   **Smart Parent Lookup**: Fixed false positives (e.g., "Alien Planet") where generic folders like `x64`, `bin`, or `gog` were mistaken for game titles. The scanner now intelligently looks up to the parent folder.
*   **Linux Magic Bytes**: Added binary verification (ELF/Shebang) for extensionless files on Linux to prevent text files (LICENSE, README) from being detected as games.
*   **Offline Fallback (PC)**: PC games are now added to the library even if metadata lookup fails, preventing valid games from being skipped.
*   **Z-Drive Protection**: Blacklisted Wine `z:` and `d:` drives to prevent scanning host system files or empty mounts inside bottles.

### 🍷 Wine / Whisky Integration
*   **Dedicated Scan Button**: Added a specific "Scan" button in Settings next to the Wine Prefix path for targeted scanning.
*   **Junk Filtering**: Auto-blacklisted common junk like "Windows Media Player" and "MD5" checkers found in bottles.

### 🛠️ Fixes & Improvements
*   **White Screen Fix**: Resolved frontend build issues causing white screens on startup by ensuring clean webpack builds.
*   **Streets of Rage 4 Fix**: Specific regression fix ensuring this title is correctly identified despite GOG folder structures.

# Playerr v0.4.3 - The Universal Update 🚀

This release marks a major milestone in scanner reliability, metadata accuracy, and system stability. 

### Smart Detection & Universal Metadata
*   **Universal Detector (v0.4.3)**: Robust game identification that handles complex versioned folders and messy release titles by stripping metadata and version junk before processing.
*   **Sequel & Roman Numeral Protection**: Improved heuristics to distinguish between version numbers and actual sequels (e.g. games with digits or Roman numerals in the title).
*   **Metadata Reliability**: Added fallback search mechanisms to ensure games are added even when primary metadata sources are incomplete.

###  High-Performance Scanner
*   **Fast Hierarchical Discovery**: Replaced flat enumeration with a smart, branch-skipping system (up to 10x faster).
*   **Winner-Takes-All Clustering**: Groups executables by folder and picks the best candidate, preventing duplicates.
*   **Global Smart Blacklist**: Automatically filters junk branches (shadercache, compatdata) and system subfolders.
*   **Linux Support Improvements**: Added support for `AppRun` and extensionless executables.

###  Disk & System Stability
*   **Disk Safe Protection**: Implemented log rotation (capped at 10MB) and automatic post-import cleanup to prevent Docker containers from exhausting disk space.
*   **Windows Stability Fix**: Resolved the `AccessViolationException` crash during library scans.
*   **Improved Threading**: Enforced synchronous startup to ensure Photino stays on the main thread (fixes launch issues on Mac).
*   **Real Alive-Check**: The window now waits for the backend to be fully responsive before showing the UI.
*   **Failsafe Logging**: New `playerr_startup.log` in TEMP directory to help diagnose early crashes.

