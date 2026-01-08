# Lógica de Instaladores de Playerr

Este documento define la lógica "sagrada" para el manejo de diferentes tipos de juegos e instaladores en Playerr.

## ⚠️ Regla General de Escaneo (Discovery Rules)
Para todas las prioridades abajo descritas, el `MediaScannerService` debe aplicar estas reglas de búsqueda:
1.  **Patrones Flexibles (Fuzzy Name):** No buscar strings exactos. Usar patrones: `setup*.exe`, `install*.exe`, `installer.exe`.
2.  **Profundidad (Recursividad Limitada):** Escanear la carpeta raíz de la descarga Y el primer nivel de subdirectorios (`Depth = 1`). Esto es vital para descargas que vienen dentro de una carpeta contenedora (ej: `/Downloads/Juego/Setup/setup.exe`).

---

## 1. Las Imágenes de Disco (La "Scene" Clásica) 💿

Son copias 1:1 de los discos físicos o digitales originales. Son el estándar de grupos como CODEX, RUNE, PLAZA.

* **Archivo Contenedor:** `.iso` (99%), a veces `.bin` + `.cue`, `.mdf`, `.nrg`.
* **Estructura Interna:**
    ```text
    [Juego.iso]
    ├── setup.exe        <-- (O setup_game.exe)
    ├── autorun.inf
    ├── data.bin
    └── CODEX/           <-- Carpeta con el crack
    ```

### ¿Cómo lo maneja Playerr?
1.  **Montar:** Ejecutar comando de montaje nativo.
2.  **Instalar:** Buscar ejecutable que cumpla el patrón `setup*.exe` o `install*.exe` en la unidad montada.
3.  **Medicina:** Copiar carpeta `CODEX`/`RUNE`/`PLAZA` si existe.

---

## 2. Los "Repacks" e Instaladores Nativos 📦

Populares (FitGirl, DODI, GOG Offline Installers).

* **Archivo Contenedor:** Carpeta con archivos sueltos.
* **Estructura Interna:**
    ```text
    [Carpeta Descargada]
    ├── setup.exe                     <-- Scene standard
    ├── setup_hollow_knight_v1.5.exe  <-- GOG standard
    ├── installer.exe                 <-- Generic standard
    ├── data-01.bin
    └── MD5/
    ```

### ¿Cómo lo maneja Playerr?
1.  **Búsqueda:** Escanear Raíz y Subcarpetas (Nivel 1) buscando `setup*.exe` o `install*.exe`.
2.  **Validación:** Si hay varios, priorizar el que contenga el nombre del juego o sea el más pesado.
3.  **Instalar:** Ejecutar con argumentos de automatización (`/SILENT`, `/VERYSILENT`, `/SP-`, `/SUPPRESSMSGBOXES`, `/NOCANCEL`).

---

## 3. Los "Portables" o "Pre-instalados" (SteamRIP) 📂

Los más fáciles. Ya instalados y comprimidos.

* **Archivo Contenedor:** `.zip`, `.7z`, `.rar` (archivo único).
* **Estructura Interna:**
    ```text
    [Juego.zip]
    └── NombreDelJuego/
        ├── Game.exe
        └── Data/
    ```

### ¿Cómo lo maneja Playerr?
1.  **Descomprimir:** Extraer en `/Library`.
2.  **Jugar:** Ejecutar lógica de "Executable Discovery".

---

## 4. Los "Scenedest" / Extracción Directa de Archivos RAR 📚

Juegos divididos en múltiples volúmenes RAR.

* **Archivo Contenedor:** `.rar`, `.r00`, `.r01`, `part1.rar`...
* **Estructura Interna:** Al descomprimir el primero, se genera la carpeta del juego.

### ¿Cómo lo maneja Playerr?
1.  **Unión:** Detectar secuencia.
2.  **Extracción:** Descomprimir volumen principal.
3.  **Crack:** Buscar carpeta `Crack` o `Scientific` tras la extracción.

---

## 5. Scripts de Instalación Complejos (GOG Multipart) 🎮

Instaladores de GOG divididos en binarios.

* **Archivo Contenedor:** `setup_juego.exe` + `setup_juego-1.bin`.

### ¿Cómo lo maneja Playerr?
1.  **Detección:** Asegurarse de que el `.exe` y los `.bin` están en la misma carpeta.
2.  **Ejecución:** Tratar igual que la Prioridad 2 (Native Installer) con argumentos silenciosos.

---

## 6. Los Juegos "WINE-Prefix" o Botellas (Linux/Mac) 🍷

Carpetas pre-configuradas para Wine.

* **Archivo Contenedor:** Carpetas `drive_c` o estructuras tipo Botella.

### ¿Cómo lo maneja Playerr?
1.  **Lanzamiento:** No ejecuta el `.exe` directamente, sino que llama a `wine` o `proton`.

---

## 7. Descompresión de "Dump" de Steam ⚙️

Archivos crudos de Steam (Depots).

* **Archivo Contenedor:** Carpetas numéricas (AppID).

### ¿Cómo lo maneja Playerr?
1.  **Mapeo:** Usar AppID para renombrar carpeta.
2.  **Emulador:** Inyectar `steam_api64.dll` (Goldberg/SSE).

---

# 📋 Resumen de Lógica Extendida

Jerarquía de detección actualizada:

| Prioridad | Si encuentra... | Acción de Playerr |
| :--- | :--- | :--- |
| 1 | `.iso` / `.bin` | **Montar** -> Buscar `setup*.exe` |
| 2 | `setup*.exe` / `install*.exe` | **Ejecutar** (Modo Silent) |
| 3 | `.rar` / `.zip` / `.7z` | **Extraer** -> Buscar Ejecutable |
| 4 | Carpeta con `steam_api.dll` | **Portable** -> Aplicar Emu |
| 5 | Solo carpeta | **Escanear** -> Identificar ejecutable principal |
