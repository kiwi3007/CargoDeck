# Playerr v0.4.3 - The Universal Update 🚀

This release marks a major milestone in scanner reliability, metadata accuracy, and system stability. 

### 🔍 Smart Detection & Universal Metadata
*   **Universal Detector (v0.4.9)**: Robust game identification that handles complex versioned folders and messy release titles by stripping metadata and version junk before processing.
*   **Sequel & Roman Numeral Protection**: Improved heuristics to distinguish between version numbers and actual sequels (e.g. games with digits or Roman numerals in the title).
*   **Metadata Reliability**: Added fallback search mechanisms to ensure games are added even when primary metadata sources are incomplete.

### ⚡ High-Performance Scanner
*   **Fast Hierarchical Discovery**: Replaced flat enumeration with a smart, branch-skipping system (up to 10x faster).
*   **Winner-Takes-All Clustering**: Groups executables by folder and picks the best candidate, preventing duplicates.
*   **Global Smart Blacklist**: Automatically filters junk branches (shadercache, compatdata) and system subfolders.
*   **Linux Support Improvements**: Added support for `AppRun` and extensionless executables.

### 🛡️ Disk & System Stability
*   **Disk Safe Protection**: Implemented log rotation (capped at 10MB) and automatic post-import cleanup to prevent Docker containers from exhausting disk space.
*   **Windows Stability Fix**: Resolved the `AccessViolationException` crash during library scans.
*   **Improved Threading**: Enforced synchronous startup to ensure Photino stays on the main thread (fixes launch issues on Mac).
*   **Real Alive-Check**: The window now waits for the backend to be fully responsive before showing the UI.
*   **Failsafe Logging**: New `playerr_startup.log` in TEMP directory to help diagnose early crashes.

