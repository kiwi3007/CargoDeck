# 🧠 AGENT MEMORY: Playerr Project Context

Este documento sirve como "memoria persistente" para garantizar la continuidad entre diferentes sesiones de agentes. **Léelo antes de empezar cualquier tarea.**

---

## 1. 🌟 Contexto del Proyecto (The Mission)
**Playerr** es un gestor y lanzador de juegos multiplataforma (macOS, Windows, Linux).
*   **Filosofía:** "Native First but Flexible". Priorizamos la integración nativa con el SO, pero damos soporte a herramientas de terceros (Wine, Crossover) mediante delegación, no micro-gestión.
*   **Stack:** Backend (.NET 8 / ASP.NET Core) + Frontend (React/TypeScript).

---

## 2. 📜 Las "Reglas Sagradas" (The Sacred Texts)
Estas reglas son inmutables a menos que el usuario lo autorice explícitamente.

### 2.1 Reglas de Trabajo
*   **User Testing Rule:** El usuario se encarga de todo el testeo frontend. **No usar la herramienta de navegador para verificación de UI**, consume mucho tiempo. Centrarnos en código y verificación backend (curl, logs).
*   **Documentación:** PROHIBIDO usar emojis en documentos oficiales (`README.md`, `RELEASE_NOTES.md`, `MEMORIAS.md`). Solo texto profesional.

### 2.2 Lógica de Instalación (`docs/INSTALLER_LOGIC.md`)
*   **Detección:** Fuzzy Matching (`setup*.exe`, `install*.exe`) + Profundidad 1. Prioridad: Nombre del juego o archivo más pesado.
*   **Implementación:** Usar siempre `FindInstaller` en `GameController.cs`. Nunca "solo setup.exe".

### 2.3 Lógica de Lanzamiento (`docs/TECHNICAL_SPECS_LAUNCHER.md`)
*   **macOS:** Estrategia `MacNativeStrategy` (`/usr/bin/open "{exePath}"`).
*   **Prohibido:** Integrar Whisky/Crossover vía CLI. Delegar en Finder.

---

## 3. 🛠️ Soluciones y Mecánicas Probadas (Proven Mechanics)

### 3.1 Docker y Base de Datos (Upsert Seeding)
Los volúmenes persistentes pueden saltarse la inicialización.
*   **Solución:** En `Program.cs`, no usar `!Any()`. Iterar y añadir plataformas una por una si faltan (Upsert).

### 3.2 Limpieza de Librería
*   **Backend:** `DELETE /api/v3/media/clean` -> `SqliteGameRepository.DeleteAllAsync()` (borrado real).
*   **Frontend:** Botones con confirmación (`window.confirm`).

### 3.3 Soporte de Plataformas (IDs Estándar)
*   **PC:** 6 | **Mac:** 14 | **Switch:** 130 | **PSP:** 38 | **PS1-5:** 7, 8, 9, 48, 167.
*   **MediaScanner:** Si falla la detección, fallback a **ID 6 (PC)** para evitar crashes.

---

## 4. � Proceso de Release (The Ritual)
1.  **Bump Versión:** `package.json`, `build_all.sh`, `csproj`, `About.tsx`.
2.  **Docs:** Actualizar `RELEASE_NOTES.md` (Sin emojis).
3.  **Compilar:** `./build_all.sh` (Genera artifactos en `build_artifacts/`).
4.  **Docker:** `docker-compose build --no-cache` o push a Hub.

---

## 5. 🔑 Gestión de Credenciales (The Vault)
**Ubicación:** `config/*.json`. NUNCA hardcoded.

*   **Perfiles:** "Orb" (Casa/Potente) vs "Raspberry" (Ligero).
*   **Protocolo:** Si el usuario dice "Carga Orb", verificar existencia en `config/`.
*   **Regla de Oro:** Antes de lanzar, asegurar que se usan los JSON correctos si el usuario ha dado contexto.

---

## 6. 🗣️ Estilo de Comunicación y Comandos
*   **Idioma:** Español.
*   **Tono:** Pair Programmer profesional.
*   **Comandos Frecuentes:**
    *   Backend Build: `dotnet build src/Playerr.Host/Playerr.Host.csproj`
    *   Backend Run: `dotnet run --project src/Playerr.Host/Playerr.Host.csproj` (Puerto 5002 o 5000)
    *   Frontend: `npx webpack serve --config ./frontend/build/webpack.config.js`
    *   Debug API: `curl -v http://127.0.0.1:5002/api/v3/...`

---

## 7. 🚧 Lecciones Aprendidas (Log)
*   **Feature Hydra (Enero 2026):** Añadido soporte para índices JSON como feature aditiva en `hydra.json`. Se arregló persistencia registrando explícitamente el Controller.
*   **Revert Unified Indexers:** No hacer refactors masivos. Cambios atómicos.
*   **Edición de Código:** Reemplazar, no añadir al final.
*   **Recuperación:** `git reset --hard HEAD` es tu amigo si se rompe la estabilidad.


- **Windows Blank Screen / Mac Launch Failure**: Resolved by forcing total synchronous initialization in `Program.Main` to ensure Photino stays on the STA thread. Added an HTTP "Alive-check" that waits for the backend to serve 'index.html' before calling `.Load()`.
- **Professional Startup**: Implemented smart port selection (trying 5002-5005 before dynamic) and normalized internal addresses to `localhost` for better WebView2/macOS compatibility.
- **Logging**: Added `playerr.log` in the config directory and a failsafe `playerr_startup.log` in the system TEMP directory for early crash capture.
