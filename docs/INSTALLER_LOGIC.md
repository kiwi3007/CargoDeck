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

---

## 4. Los "Scenedest" / Extracción Directa de Archivos RAR 📚

A diferencia de los Portables (que vienen en un solo `.zip`), muchos grupos de la Scene (FLT, SKIDROW) no usan ISOs, sino que suben el juego en múltiples archivos RAR partidos que, al extraerse, dejan el juego listo para usar.

*   **Archivo Contenedor:** `.rar`, `.r00`, `.r01`... (50 o 100 archivos de 500MB).
*   **Estructura Interna:** Al descomprimir el primero, se genera la carpeta del juego directamente.

### ¿Cómo lo maneja Playerr?
1.  **Unión:** Detectar la secuencia de archivos (`part01`, `r00`).
2.  **Extracción en cadena:** Descomprimir el volumen principal.
3.  **Post-procesado:** A menudo requieren aplicar un crack que viene en una carpeta `Crack` o `Scientific` dentro de la extracción.

---

## 5. Scripts de Instalación / GOG Installers 🎮

GOG (Good Old Games) usa instaladores propios que son muy limpios, pero a veces vienen divididos en un `.exe` y varios archivos `.bin` de 4GB.

*   **Archivo Contenedor:** `setup_juego_version.exe` + `setup_juego_version-1.bin`.
*   **Estructura Interna:** Es un instalador Inno Setup modificado.

### ¿Cómo lo maneja Playerr?
1.  **Detección de dependencias:** Asegurarse de que todos los archivos `.bin` están en la misma carpeta antes de lanzar el `.exe`.
2.  **Modo Silencioso:** Estos son los más agradecidos para la automatización usando los parámetros `/SP-`, `/VERYSILENT`, `/SUPPRESSMSGBOXES`.

---

## 6. Los Juegos "WINE-Prefix" o Botellas (Linux/Mac) 🍷

Esto es vital si quieres que Playerr brille en Docker, Steam Deck o macOS. Muchos juegos no se "instalan" en el sistema, sino que se preparan dentro de un "prefijo" de Wine o una "botella".

*   **Archivo Contenedor:** Carpetas de Windows emuladas.
*   **Estructura Interna:** `drive_c/Program Files/Juego...`.

### ¿Cómo lo maneja Playerr?
1.  **Creación del entorno:** Playerr debe crear la carpeta del prefijo.
2.  **Lanzamiento:** No ejecuta el `.exe` directamente, sino que llama a `wine` o `proton` pasando la ruta del binario como argumento.

---

## 7. Descompresión de "Dump" de Steam ⚙️

Existen herramientas (como Steam-Store-Front) que descargan los archivos crudos de los servidores de Steam (Manifiestos). No hay instalador, solo miles de archivos pequeños.

*   **Archivo Contenedor:** Carpetas con nombres de ID numéricos (AppID).

### ¿Cómo lo maneja Playerr?
1.  **Mapeo:** Usar el AppID para consultar los metadatos de Steam y renombrar la carpeta de `1245620` a `Elden Ring`.
2.  **Emulador de Steam:** Casi siempre requieren inyectar una DLL (`steam_api64.dll`) modificada (como Goldberg Emulator) para que el juego arranque sin el cliente de Steam abierto.

---

# 📋 Resumen de Lógica Extendida para el Agente

Para que el backend de Playerr sea infalible, la jerarquía de detección debería ser:

| Prioridad | Si encuentra... | Acción de Playerr |
| :--- | :--- | :--- |
| 1 | `.iso` | **Montar** -> Buscar `setup.exe` |
| 2 | `setup.exe` + `.bin` | **Ejecutar** (Modo Silent) |
| 3 | `.rar` / `.zip` / `.7z` | **Extraer** -> Buscar `.exe` |
| 4 | Carpeta con `steam_api.dll` | **Portable** -> Aplicar Steam Emulator |
| 5 | Solo carpeta | **Escanear** -> Identificar ejecutable principal |
