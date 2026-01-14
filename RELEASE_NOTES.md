# Playerr v0.4.1 - Windows Stability Fix
*   **AccessViolationException Fix**: Resolved the "Fatal error" crash on Windows during library scans.
*   **Improved Threading**: Enforced synchronous startup to ensure Photino stays on the main thread (fixes launch issues on Mac).
*   **Real Alive-Check**: The window now waits for the backend to be fully responsive before showing the UI.
*   **Failsafe Logging**: New `playerr_startup.log` in TEMP directory to help diagnose early crashes.

# Playerr v0.4.0 - Important Update!
*   **Steam Launch Support:** You can now launch your Steam games directly from the app (local machine)!
*   **Installed Games:** Initial implementation of the execution system for all your already installed games (Early stage).
*   **Wine/Whisky Integration:** You can now specify the path where Wine/Whisky games are installed to run them conveniently from the application.
*   **Uninstall & Cleanup:** You can now uninstall and delete games from their installation paths directly from the app.





