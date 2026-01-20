# Lógica de Escaneo y Detección (v0.4.6+)

Este documento describe la arquitectura del escáner de medios de Playerr, enfocado en la precisión, velocidad y eliminación de duplicados.

## 1. Fase de Descubrimiento Jerárquica

A partir de la versión **v0.4.2**, Playerr utiliza un sistema de descubrimiento jerárquico recursivo en lugar de una enumeración plana.

### Salto Prematuro de Ramas (Branch Skipping)
Para optimizar el rendimiento, el escáner ignora **ramas completas** del árbol de directorios si el nombre de la carpeta está en la `_folderBlacklist`.
- [x] **Carpetas Ignoradas:** `shadercache`, `compatdata`, `node_modules`, `temp`, `Redist`, `DirectX`, `PPPwnGo`, `GoldHEN`, `Python!+Npcap` etc.
- [x] **Permitidas:** `steamapps` y `common` ya no están bloqueadas para permitir el escaneo de bibliotecas de Steam.
- **Resultado:** El escáner puede profundizar en carpetas de sistema de aplicaciones como Steam manteniendo una velocidad óptima al ignorar caché y datos pesados no relevantes.

## 2. Limpieza "Nuclear" de Títulos (v0.4.6)

Para garantizar coincidencias precisas en IGDB (especialmente para Switch y PS4), se aplica una estrategia de limpieza agresiva antes de la tokenización:

1.  **Extracción de Seriales:** Se extraen identificadores de Switch (16-hex) y PlayStation (CUSA, SLPS, etc.) *antes* de limpiar, preservando la identidad del juego.
2.  **Eliminación de Corchetes:** Se elimina todo el contenido dentro de `[]`, `()`, `{}` y `［］` para borrar etiquetas de escena, versiones y metadatos.
3.  **Filtrado de Patrones de Tamaño:** Se eliminan cadenas como "2.90GB", "100MB".
4.  **Filtrado de Ruido Regional:** Se eliminan explícitamente etiquetas regionales de 2 letras (`US`, `EU`, `JP`, etc.) y palabras de la lista de ruido (`repack`, `fitgirl`, `opoisso893`).
5.  **Anti-Artefactos de Versión:** Se eliminan patrones como `v1.00`, `A0100` y la palabra "00" para evitar falsos positivos (e.g., "00 Dilly").
6.  **Preservación de Secuelas:** Se ha relajado la regla numérica para permitir títulos como "Streets of Rage 4".

## 3. Agrupación por Carpeta (Clustering)

El escáner aplica una lógica de **"Winner Takes All"** (El Ganador se lo lleva todo).
1.  Se agrupan todos los archivos válidos encontrados por su **Carpeta Padre**.
2.  Se evalúan todos los candidatos de esa carpeta mediante el sistema de puntuación.
3.  **Solo se añade un juego por carpeta** (Winner Takes All), evitando entradas duplicadas.
4.  **EXCEPCIÓN (v0.4.1+):** Esta lógica se desactiva (No-Clustering) para extensiones de consola/retro (`.iso`, `.nsp`, `.pkg`, etc.). Cada archivo se trata como único.

## 4. Sistema de Puntuación (Scoring)

Cada archivo candidato recibe una puntuación para determinar si es el ejecutable principal:

| Puntos | Criterio |
| :--- | :--- |
| **+100** | El nombre del archivo coincide exactamente con el nombre de la carpeta. |
| **+90** | Coincidencia parcial o sin espacios/guiones con el nombre de la carpeta. |
| **+50** | Nombres prioritarios: `AppRun`, `Start.sh`. |
| **+25** | Ubicación en carpetas estándar: `binaries`, `win64`, `shipping`, `retail`. |
| **+20** | Archivo más pesado de la carpeta (Desempate por tamaño). |
| **-50** | Palabras clave de penalización: `setup`, `install`, `launcher`, `config`, `settings`. |

## 5. Listas Negras Globales

### Palabras Clave (Keywords)
Si el nombre del archivo contiene alguno de estos términos, es ignorado inmediatamente:
`steam_api`, `crashpad`, `unitycrash`, `vcredist`, `bios`, `firmware`, `updater`, `unins000`.

### Archivos Ocultos / Basura
- Se ignoran explícitamente archivos que comienzan con `._` (metadatos macOS).
- Se ignoran carpetas de herramientas de exploit conocidas (`PPPwnGo`, `GoldHEN`).

### Extensiones Prohibidas en Carpetas
En modo de escaneo de carpetas (PC), se ignoran archivos que suelen ser librerías o datos:
`.dll`, `.so`, `.lib`, `.a`, `.bin` (excepto en plataformas retro conocidas).

## 6. Soporte Específico de Linux

*   **Binarios sin extensión:** En sistemas Linux, los archivos sin extensión se consideran candidatos válidos.
    *   **Verificación de Cabecera (Security):** Se leen los primeros 4 bytes del archivo para confirmar que contiene una cabecera **ELF** (`0x7F 'E' 'L' 'F'`) o un shebang (`#!`), descartando así archivos de texto plano como `LICENSE` o `README` que no tengan extensión.
*   **Prioridad AppRun:** Soporte nativo para ejecutar AppImages extraídas o volcados de juegos de Linux que usan el estándar `AppRun`.

## 7. Detección de Plataformas por Extensión y Serial

El sistema analiza tanto la extensión como el contenido del nombre (serial) para asignar una plataforma precisa:

### Consolas Modernas
*   **Nintendo Switch:** `.nsp`, `.xci`, `.nsz`, `.xcz` -> `nintendo_switch`
*   **PlayStation 4/5:** `.pkg` -> `ps4` / `ps5`
    *   **Nota:** Se eliminó el soporte para `.bin` en PS4 para evitar falsos positivos con payloads de exploits.
    *   **Seriales:** Detecta `CUSA`, `PPSA`, `PLJS`, `ELJS`, etc.

### PlayStation Global (PS1-PS5)
El escáner reconoce seriales de **todas las regiones** (USA, EUR, JP, Asia) para identificar la consola correcta:
*   **PS1/PS2:** `SLES`, `SLUS`, `SCES`, `SCUS`, `SLPS`, `SLPM`, `SCCS`, `SLKA`
*   **PS3:** `BLES`, `BLUS`, `BCES`, `BCUS`, `NPEB`, `NPUB`, `BLJM`, `BCAS`, etc.

### MacOS
*   **Extensiones:** `.dmg`, `.app` -> `macos`

### Retro Emulation (No-Cluster)
*   **Nintendo 64:** `.z64`, `.n64`, `.v64` -> `nintendo_64`
*   **SNES:** `.sfc`, `.smc` -> `snes`
*   **NES:** `.nes` -> `nes`
*   **GameBoy:** `.gb`, `.gbc`, `.gba` -> `gameboy_advance`
*   **Sega:** `.md`, `.gen`, `.smd`, `.sms`, `.gg` -> `sega_genesis`
*   **PC Engine:** `.pce` -> `pc_engine`

### PC / Default
*   **Clustering:** Solo se activa para `.exe`, `.bat`, `.sh`.
*   **ISO:** Las imágenes `.iso` se tratan como juegos individuales (One File = One Game).
