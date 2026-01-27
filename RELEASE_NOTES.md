# Playerr v0.4.6 - The Transmission Update 🔄
This release brings full, robust support for Transmission, fixing critical issues with Prowlarr indexers and improving stability.

### 🌐 Transmission Integration Fixed
*   **Redirect Handling**: Fixed issue where Transmission failed to add downloads from Prowlarr due to HTTP 301 redirects. Playerr now intelligently follows redirects and extracts the final destination.
*   **Magnet Link Support**: Added native support for Prowlarr's magnet link redirects.
*   **Smart Naming**: Fixed issue where magnet links showed as hashes in Transmission. Playerr now extracts the correct filename from the original URL and injects it into the magnet link.
*   **Control Fixes**: Resolved bug where Pause/Resume/Delete buttons in the Status tab were unresponsive for Transmission clients.

### 🧹 Improvements
*   **Debug Cleanup**: Removed extensive debug logging to ensure cleaner operation logs.
*   **Performance**: Optimized URL handling for faster addition of downloads.

