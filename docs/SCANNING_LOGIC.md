# Lógica de Escaneo y Detección (v0.4.2+)

Este documento describe la arquitectura del escáner de medios de Playerr, enfocado en la precisión, velocidad y eliminación de duplicados.

## 1. Fase de Descubrimiento Jerárquica

A partir de la versión **v0.4.2**, Playerr utiliza un sistema de descubrimiento jerárquico recursivo en lugar de una enumeración plana.

### Salto Prematuro de Ramas (Branch Skipping)
Para optimizar el rendimiento, el escáner ignora **ramas completas** del árbol de directorios si el nombre de la carpeta está en la `_folderBlacklist`.
*   **Carpetas Ignoradas:** `shadercache`, `compatdata`, `steamapps`, `.steam`, `.local`, `node_modules`, `temp`, `Redist`, `DirectX`, etc.
*   **Resultado:** En servidores con librerías de Steam masivas, el tiempo de escaneo se reduce de horas a segundos.

## 2. Agrupación por Carpeta (Clustering)

El escáner aplica una lógica de **"Winner Takes All"** (El Ganador se lo lleva todo).
1.  Se agrupan todos los archivos válidos encontrados por su **Carpeta Padre**.
2.  Se evalúan todos los candidatos de esa carpeta mediante el sistema de puntuación.
3.  **Solo se añade un juego por carpeta**, evitando que aparezcan entradas duplicadas para el juego, su lanzador, su configurador o su desinstalador.

## 3. Sistema de Puntuación (Scoring)

Cada archivo candidato recibe una puntuación para determinar si es el ejecutable principal:

| Puntos | Criterio |
| :--- | :--- |
| **+100** | El nombre del archivo coincide exactamente con el nombre de la carpeta. |
| **+90** | Coincidencia parcial o sin espacios/guiones con el nombre de la carpeta. |
| **+50** | Nombres prioritarios: `AppRun`, `Start.sh`. |
| **+25** | Ubicación en carpetas estándar: `binaries`, `win64`, `shipping`, `retail`. |
| **+20** | Archivo más pesado de la carpeta (Desempate por tamaño). |
| **-50** | Palabras clave de penalización: `setup`, `install`, `launcher`, `config`, `settings`. |

## 4. Listas Negras Globales

### Palabras Clave (Keywords)
Si el nombre del archivo contiene alguno de estos términos, es ignorado inmediatamente:
`steam_api`, `crashpad`, `unitycrash`, `vcredist`, `bios`, `firmware`, `updater`, `unins000`.

### Extensiones Prohibidas en Carpetas
En modo de escaneo de carpetas (PC), se ignoran archivos que suelen ser librerías o datos:
`.dll`, `.so`, `.lib`, `.a`, `.bin` (excepto en plataformas de consola conocidas).

## 5. Soporte Específico de Linux

*   **Binarios sin extensión:** En sistemas Linux, los archivos sin extensión se consideran candidatos válidos (siempre que no sean carpetas o archivos ocultos).
*   **Prioridad AppRun:** Soporte nativo para ejecutar AppImages extraídas o volcados de juegos de Linux que usan el estándar `AppRun`.

## 6. Detección de Plataformas por Extensión

El sistema analiza la extensión del archivo para asignar una "Platform Key":

### Nintendo Switch
*   **Extensiones:** `.nsp`, `.xci`, `.nsz`, `.xcz`
*   **Key:** `nintendo_switch`

### PlayStation 4
*   **Extensiones:** `.pkg`
*   **Key:** `ps4`

### MacOS
*   **Extensiones:** `.dmg`, `.app` (identifica bundles)
*   **Key:** `macos`

### Retro Emulation
*   **Nintendo 64:** `.z64`, `.n64`, `.v64`
*   **SNES:** `.sfc`, `.smc`
*   **NES:** `.nes`
*   **Handhelds:** `.gb`, `.gbc`, `.gba`
*   **Sega:** `.md`, `.gen`, `.smd`, `.sms`, `.gg`

### PC / Default
Cualquier ejecutable `.exe` o archivo no clasificado cae en `pc_windows` o `default`.
