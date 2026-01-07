# Lógica de Instaladores de Playerr

Este documento define la lógica "sagrada" para el manejo de diferentes tipos de juegos e instaladores en Playerr.

## 1. Las Imágenes de Disco (La "Scene" Clásica) 💿

Son copias 1:1 de los discos físicos o digitales originales. Son el estándar de grupos como CODEX, RUNE, PLAZA.

*   **Archivo Contenedor:** `.iso` (99%), a veces `.bin` + `.cue`.
*   **Estructura Interna:**
    ```text
    [Juego.iso]
    ├── setup.exe        <-- El instalador
    ├── autorun.inf      <-- Script antiguo de Windows
    ├── data.bin         <-- Archivos comprimidos del juego
    └── CODEX/           <-- (O PLAZA/RUNE) Carpeta con el crack
        ├── steam_api.dll
        └── steam_emu.ini
    ```

### ¿Cómo lo maneja Playerr?
1.  **Montar:** Ejecutar el comando de montaje nativo (Windows 10/11, Linux, Mac) para que aparezca como unidad virtual (ej: `E:\`).
2.  **Instalar:** Ejecutar instalador desde la unidad montada (ej: `E:\setup.exe`).
3.  **Medicina (Opcional):** Copiar contenido de la carpeta `CODEX` (o equivalente) a la carpeta de instalación final.

---

## 2. Los "Repacks" (La Pesadilla de la CPU) 📦

Populares (FitGirl, DODI, ElAmigos) por pesar poco, aunque tardan en instalar.

*   **Archivo Contenedor:** Carpeta con archivos sueltos o `.rar` que se extrae primero.
*   **Estructura Interna:**
    ```text
    [Carpeta Descargada]
    ├── setup.exe        <-- Instalador personalizado (Inno Setup)
    ├── fg-01.bin        <-- Archivos hiper-comprimidos
    ├── fg-02.bin
    ├── Verify.bat       <-- Script para comprobar integridad
    └── MD5/             <-- Hashes de comprobación
    ```

### ¿Cómo lo maneja Playerr?
1.  **No hace falta montar.**
2.  **Instalar:** Ejecutar `setup.exe` directamente desde la carpeta.
3.  **Automatización:** Intentar pasar argumentos `/SILENT` o `/VERYSILENT` (común en Inno Setup) para instalación desatendida, aunque no siempre funciona.

---

## 3. Los "Portables" o "Pre-instalados" (SteamRIP) 📂

Los más fáciles. Ya instalados y comprimidos.

*   **Archivo Contenedor:** `.zip`, `.7z`, `.rar`.
*   **Estructura Interna:**
    ```text
    [Juego.zip]
    └── NombreDelJuego/
        ├── Game.exe     <-- El ejecutable final
        ├── Data/
        └── Engine/
    ```

### ¿Cómo lo maneja Playerr?
1.  **Descomprimir:** Usar librería (SevenZipSharp, SharpCompress) para extraer en `/Library`.
2.  **Jugar:** No hay instalación. Escanear la carpeta para encontrar el `.exe` y jugar.
