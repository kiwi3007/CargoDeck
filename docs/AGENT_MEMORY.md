# 🧠 AGENT MEMORY: Playerr Project Context

Este documento sirve como "memoria persistente" para garantizar la continuidad entre diferentes sesiones de agentes. **Léelo antes de empezar cualquier tarea.**

---

## 1. 🌟 Contexto del Proyecto (The Mission)
**Playerr** es un gestor y lanzador de juegos multiplataforma (macOS, Windows, Linux).
*   **Filosofía:** "Native First but Flexible". Priorizamos la integración nativa con el SO, pero damos soporte a herramientas de terceros (Wine, Crossover) mediante delegación, no micro-gestión.
*   **Stack:** Backend (.NET 8 / ASP.NET Core) + Frontend (React/TypeScript).

---

## 2. 📜 Las "Reglas Sagradas" (The Sacred Texts)
Estas reglas son inmutables a menos que el usuario lo autorice explícitamente. Consultar siempre los documentos en `docs/`.

### 2.1 Lógica de Instalación (`docs/INSTALLER_LOGIC.md`)
*   **Detección:** Fuzzy Matching (`setup*.exe`, `install*.exe`) + Profundidad 1 (Raíz + 1 Nivel de subcarpetas).
*   **Priorización:** Si hay ambigüedad, gana el archivo que contiene el nombre del juego o el más pesado.
*   **Implementación:** Usar siempre el helper `FindInstaller` en `GameController.cs` y `MediaScannerService.cs`. **Nunca volver a la lógica estricta de "solo setup.exe".**

### 2.2 Lógica de Lanzamiento (`docs/TECHNICAL_SPECS_LAUNCHER.md`) - v2.0 Stable
*   **macOS:** Estrategia `MacNativeStrategy`. Usamos el comando `/usr/bin/open "{exePath}"`.
    *   **Prohibido:** Intentar integrar Whisky/Crossover vía CLI. Es inestable.
    *   **Delegación:** Confiamos en la asociación de archivos del usuario (Finder).

---

## 3. ✅ Logros y Estado Actual (The Checkpoint)
*   **Base de Datos:** SQLite (`playerr.db`). Recientemente arreglado un error de esquema (faltaba columna `IsInstallable`).
*   **Feature: Botón Verde (Install Ready):**
    *   El frontend muestra un botón verde brillante si `game.IsInstallable == true`.
    *   Esto se calcula dinámicamente en `GameController.GetById` usando la lógica de detección.
*   **Fix Crítico:** Armonización de detección vs. ejecución. Ahora ambos usan `FindInstaller`, permitiendo instalar juegos de GOG anidados en subcarpetas.

---

## 4. 🗣️ Estilo de Comunicación
*   **Idioma:** Responder en el idioma que use el usuario (principalmente Español).
*   **Tono:** Profesional, técnico pero colaborador ("Pair Programmer").
*   **Proactividad:** Proponer soluciones robustas (como `FindInstaller`) en lugar de parches rápidos. Documentar cambios importantes.

---

## 5. 🛠️ Comandos Frecuentes
*   **Build Backend:** `dotnet build src/Playerr.Host/Playerr.Host.csproj`
*   **Run Backend:** `dotnet run --project src/Playerr.Host/Playerr.Host.csproj`
*   **Frontend Dev:** `npx webpack serve --config ./frontend/build/webpack.config.js`
*   **Debug Output:** `curl -v http://127.0.0.1:5002/api/v3/...`

---

## 6. 🔑 Gestión de Credenciales (The Vault)
**Importante:** Todas las credenciales sensibles residen en el directorio `config/` y **NUNCA** deben ser hardcodeadas.

*   **Fuentes de Verdad:**
    *   **Orb (Casa):** Configuración para uso local/potente.
    *   **Raspberry Pi:** Configuración para entorno ligero.
*   **Ubicación:** Ambos perfiles ya están configurados en archivos JSON dentro de `config/` (ej: `jackett.json`, `prowlarr.json`).
*   **Protocolo de Activación (User Command):**
    *   **El Trigger:** El usuario dirá explícitamente comandos como **"Carga Orb"** o **"Carga Raspberry"**.
    *   **Tu Respuesta:** NUNCA pidas claves o APIs. Tu acción es verificar/asegurar que el backend esté usando los archivos correctos de `config/`.
    *   **Recuperación:** Si pierdes el contexto y el usuario dice "Carga Orb", no preguntes "¿Qué es Orb?". Ve a `config/`, busca los archivos y confirma la carga.
    *   **Persistencia (Regla de Oro):** Si el usuario dice "Lanza la app" tras haber pedido "Orb" (o si te quejas de que está vacío), **SIEMPRE asegúrate de que los archivos `jackett.orb.json` y `prowlarr.orb.json` están copiados en `jackett.json` y `prowlarr.json` ANTES de lanzar.** No lances la app "de fábrica" si el usuario ya te ha dado contexto de su entorno.
