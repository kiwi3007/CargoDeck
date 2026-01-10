# 🧠 MEMORIAS.md: Mecánicas y Configuraciones Probadas

Este documento recoge las soluciones, configuraciones y flujos de trabajo que **sabemos que funcionan** (Proven Mechanics). Úsalo para recuperar el contexto si la sesión se reinicia.

---

## 1. 🐳 Docker y Base de Datos (Critical)

### El Problema de la Persistencia
Los contenedores Docker con volúmenes persistentes (`-v config:/config`) no ejecutan la inicialización por defecto si la base de datos `playerr.db` ya existe. Esto causaba fallos al añadir juegos nuevos si faltaban Platform IDs (ej: ID 6 para PC).

### La Solución Probada (Upsert Seeding)
En `Program.cs`, la lógica de inicialización **no debe usar `!context.Platforms.Any()`**.
**Lógica Correcta:** Iterar sobre la lista de plataformas requeridas y verificar existencia **una por una**.
```csharp
// Program.cs - Lógica Robusta
foreach (var platform in defaultPlatforms) {
    if (!context.Platforms.Any(p => p.Id == platform.Id)) {
        context.Platforms.Add(platform); // Upsert: Solo añade si falta
    }
}
```
*Estado:* ✅ **Implementado y Verificado** en v0.3.7.

---

## 2. 🧹 Limpieza de Librería (Trash Button)

### Mecánica Backend
La funcionalidad "Vaciar Librería" requiere dos partes:
1.  **Endpoint:** `DELETE /api/v3/media/clean` (o `game/all`).
2.  **Repo:** `SqliteGameRepository.DeleteAllAsync()` debe implementar el borrado real.
    *   *Nota:* Inicialmente estaba vacío. Ahora usa `context.Games.RemoveRange(allGames)`.

### Mecánica Frontend
El botón de papelera en `Library.tsx` y el botón "Clean Library" en `Settings.tsx` llaman a estos endpoints y **requieren** confirmación de usuario (`window.confirm`).

---

## 3. 🚀 Proceso de Release (The Ritual)

Para publicar una nueva versión, seguir estrictamente estos pasos:

1.  **Bump de Versión (Checklist Crítica):**
    *   [ ] `package.json` -> `version`
    *   [ ] `build_all.sh` -> `VERSION`
    *   [ ] `create_mac_app.sh` -> `CFBundleVersion`
    *   [ ] `src/Playerr.Host/Playerr.Host.csproj` -> `<Version>`
    *   [ ] `frontend/src/pages/About.tsx` -> Texto `Playerr vX.X.X`

2.  **Documentación:**
    *   Actualizar `RELEASE_NOTES.md` con los cambios.
    *   (Opcional) Hacer commit: `git commit -am "vX.X.X Release"`

2.  **Compilar (Build Artifacts):**
    *   Ejecutar: `./build_all.sh`
    *   Esto genera cruzadamente para:
        *   Windows x64 (`.zip`)
        *   Linux x64 (`.tar.gz`)
        *   macOS Intel (`.dmg`)
        *   macOS Silicon (`.dmg`)

3.  **Ubicación:** Los binarios finales están SIEMPRE en `build_artifacts/`.

4.  **Docker (Si aplica):**
    *   Si usas `docker-compose` local: Ejecutar `docker-compose build --no-cache` para regenerar la imagen con la nueva versión.
    *   Si usas CasaOS/DockerHub: Push de la nueva imagen `maikboarder/playerr:latest`.

---

## 4. 🎮 Soporte de Plataformas

### IDs Estándar (Hardcoded & Seeded)
*   **PC (Windows/Linux):** ID 6
*   **Mac:** ID 14
*   **PlayStation Portable (PSP):** ID 38 (Nuevo en v0.3.7)
*   **Switch:** ID 130
*   **PS1/2/3/4/5:** 7, 8, 9, 48, 167

### Escáner de Medios (MediaScanner)
*   Usa `IGameMetadataService` con fallback dinámico.
*   Si no encuentra plataforma por nombre, busca por **folder keywords** o extensiones.
*   **Importante:** Siempre intenta asignar un Platform ID válido. Si falla, **ID 6 (PC)** es el fallback seguro para evitar crashes.

---

## 5. 🛠 Configuración de Entorno (Environment)

### Credenciales ("La Bóveda")
*   **Ubicación:** `config/*.json` (NUNCA en código).
*   **Perfiles:**
    *   "Orb" (Casa/Potente)
    *   "Raspberry" (Ligero)
*   **Carga:** El sistema carga automáticamente desde `config/`. Si el usuario dice "Carga Orb", solo verificamos que los archivos estén ahí.

### Desarrollo Local
*   **Backend:** `dotnet run --project src/Playerr.Host/Playerr.Host.csproj`
*   **Frontend:** `npx webpack serve --config ./frontend/build/webpack.config.js`
*   **Puerto Backend:** 5002 (para evitar conflictos en macOS).

---
**Última Actualización:** v0.3.7 Hotfix 2
