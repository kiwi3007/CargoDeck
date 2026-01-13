# PLAYERR CORE: Game Launcher & Execution Architecture (v2.0 Stable)

**Contexto:** Este documento define la lógica del `LauncherService`. Su responsabilidad es determinar el ejecutable correcto y lanzarlo delegando en el SO anfitrión.

---

## 🏗️ 1. Arquitectura de Diseño (The Strategy Pattern)

Se mantiene el patrón **Strategy** para la escalabilidad.

### 1.1 Interfaces
```csharp
public interface ILaunchStrategy {
    bool IsSupported();
    Task LaunchAsync(Game game, string executablePath);
}
```

### 1.2 Factory de Selección
*   **Windows:** `WindowsNativeStrategy`
*   **macOS:** `MacNativeStrategy` (Anteriormente Whisky, ahora simplificada)
*   **Linux:** `LinuxWineStrategy`

---

## 🧠 2. Algoritmo de Descubrimiento (Executable Discovery)

Se mantiene la heurística de puntuación para encontrar el `.exe` o binario correcto automáticamente.

### 2.1 Sistema de Puntuación (Scoring)
*   **Blacklist (Ignorar):** `unins`, `setup`, `config`, `dxsetup`, `vcredist`, `crashhandler`.
*   **Puntuación Positiva:**
    *   **+50 pts:** Nombre archivo == Nombre carpeta padre.
    *   **+30 pts:** Archivo más pesado (Max File Size).
    *   **+20 pts:** Palabras clave: `Shipping`, `Client`, `Game`, `Launcher`.
    *   **+10 pts:** Subcarpetas: `Binaries`, `Win64`.

### 2.2 Patrones de Estructura de Directorios (Folder Structure Patterns)
Para mejorar la precisión del Scoring, el sistema debe identificar primero ante qué tipo de estructura de carpetas nos encontramos.

#### A. The "Deep Nested" Pattern (Unreal/Unity Games)
Es el estándar moderno (AAA y muchos Indies). El ejecutable en la raíz suele ser solo un launcher falso o un bootstrap, mientras que el ejecutable real está oculto en subdirectorios.

*   **Lógica de Búsqueda:** Si no se encuentra un candidato claro en la raíz, descender recursivamente buscando carpetas clave:
    *   `Binaries/Win64` (Standard Unreal Engine)
    *   `GameData` (Unity)
    *   `Bin`
*   **Regla de Oro:** Si existen dos ejecutables con el mismo nombre (uno en raíz y otro en subcarpeta), priorizar el de mayor tamaño en la subcarpeta (suele ser el binario real). El de la raíz se considera válido solo si su tamaño es < 5MB (típico wrapper de Steam) y no hay otro mejor.

#### B. The "Scene Release" Pattern (Carpetas Basura)
Las descargas de fuentes no oficiales (torrent/repacks) suelen incluir carpetas extras que **DEBEN** ser ignoradas durante el escaneo para evitar falsos positivos y ruido.

*   **Blacklist de Carpetas (Recursive Skip):**
    *   `_CommonRedist`
    *   `Support`
    *   `DirectX`
    *   `Crack`, `CODEX`, `RUNE`, `SKIDROW`, `TENOKE` (Carpetas de crack sin aplicar).
    *   `BonusContent`, `Soundtrack`, `Artbook`.

#### C. The "Installer" Trap (Detección de No-Juego)
Antes de asignar un `ExecutablePath`, el sistema debe verificar si lo que ha encontrado es en realidad el instalador del juego y no el juego en sí.

*   **Heurística de Alerta:** El candidato ganador tiene nombres como `setup.exe`, `install.exe` o `unins000.exe`.
*   **Acción:**
    1.  Marcar `GameStatus.InstallerDetected`.
    2.  **UI:** Cambiar el botón "Jugar" a "Instalar / Setup".
    3.  **Lógica:** No intentar lanzar como juego nativo silencioso; requiere interacción del usuario.

---

## 🚀 3. Implementación de Estrategias por SO

### 3.1 Windows Native Strategy
*   **Target:** Windows 10/11.
*   **Lógica:** `Process.Start` directo.
*   **Requisito:** Establecer `WorkingDirectory` a la carpeta del ejecutable.

### 3.2 macOS Native Strategy (Generic) 🍎
*   **Target:** macOS.
*   **Filosofía:** "File Association Delegate". No intentamos forzar una herramienta específica. Dejamos que macOS decida cómo abrir el archivo.
*   **Lógica:**
    *   **Comando:** `/usr/bin/open`
    *   **Argumentos:** `"{executablePath}"`
*   **Comportamiento Esperado:**
    *   Si es `.app`: Se abre nativamente.
    *   Si es `.exe`: Se abrirá con la aplicación que el usuario tenga asociada por defecto (Whisky, Crossover, Wine) o mostrará un error de sistema si no hay ninguna. Playerr no gestiona la compatibilidad, solo invoca el archivo.

### 3.3 Linux/Docker Strategy 🐧
*   **Target:** Linux Desktop / Steam Deck.
*   **Lógica:**
    *   Detección de binario nativo (ELF) vs Windows (.exe).
    *   Si es `.exe`, intentar usar `wine` si está en el PATH.
    *   **Headless Guard:** Bloquear ejecución GUI si estamos en contenedor Docker sin display server.

---

## 🔄 4. Flujo de UI
1.  **Jugar:** Verificar ruta -> Lanzar estrategia.
2.  **Configurar (⚙️):** Permitir selección manual de ejecutable.

---

### 💡 ¿Qué cambia con esto?
Al usar el comando `open` en Mac (`MacNativeStrategy`):
* Es **a prueba de fallos**: Si falla, es culpa de macOS, no de tu código.
* Si tú (como usuario) ya tienes Whisky instalado y los `.exe` se abren con Whisky al hacer doble clic en el Finder, **Playerr funcionará automáticamente** sin tener que escribir ni una línea de código compleja sobre "botellas" o rutas "Z:".
