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

**Resultado:** Guardar en `Game.ExecutablePath`.

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
