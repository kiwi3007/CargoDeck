# Lógica de Escaneo y Detección de Plataformas

Este documento describe cómo Playerr detecta automáticamente la plataforma de un juego basándose en la extensión de sus archivos.

**Ubicación del Código:** `MediaScannerService.cs` -> `GetPlatformFromExtension`

## Reglas Actuales de Detección

El sistema analiza la extensión del archivo (`.ext`) para asignar una "Platform Key".

### 1. Nintendo Switch
*   **Extensiones:** `.nsp`, `.xci`, `.nsz`, `.xcz`
*   **Key:** `nintendo_switch`

### 2. PlayStation 4
*   **Extensiones:** `.pkg`
*   **Key:** `ps4`
*   *Nota: Actualmente asume que todo .pkg es PS4, aunque PS3 también lo usa.*

### 3. MacOS
*   **Extensiones:** `.dmg`, `.app`
*   **Key:** `macos`

### 4. Retro Emulation (Consolas Clásicas)
*   **Key:** `retro_emulation`
*   **Nintendo 64:** `.z64`, `.n64`, `.v64`
*   **SNES:** `.sfc`, `.smc`
*   **NES:** `.nes`
*   **GameBoy / Color / Advance:** `.gb`, `.gbc`, `.gba`
*   **Sega Genesis / MegaDrive:** `.md`, `.gen`, `.smd`
*   **Sega Master System / Game Gear:** `.sms`, `.gg`
*   **PC Engine:** `.pce`

### 5. "Default" (PC y Resto)
Si la extensión **no coincide** con ninguna de las anteriores, se asigna la key `default`.
Esto incluye:
*   `.exe` (PC Windows)
*   `.iso` (PC, PS1, PS2, PS3, PSP, Wii, GameCube...)
*   `.bin` / `.cue` (PS1, Saturn...)
*   `.zip`, `.rar`, `.7z`

## Limitaciones Detectadas
*   **Ambigüedad de ISO:** Un archivo `.iso` se trata como "Default" (PC). No distingue automáticamente entre una ISO de PS2 y una de PC.
*   **PlayStation:**
    *   **PS1/PS2:** No tienen reglas específicas. Caen en "Default".
    *   **PS3:** No detecta estructura de carpetas (BLUS/BLES). Solo detecta `.pkg` como PS4.
    *   **PS5:** No soportado actualmente.
