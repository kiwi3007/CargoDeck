# Playerr v0.3.0: The "Install Anywhere" Update 🚀

We've improved the core engine to be smarter, faster, and more compatible with your messy game libraries.

### 🌟 New Features
*   **Smart Installer Detection:** The scanner now digs deeper! Support added for nested installers (Depth-1) and fuzzy naming patterns (`setup_*.exe`, `install*.exe`). If it's there, Playerr will find it.
*   **Visual Status Indicators:** New **Green "Install Ready" Button** in the UI instantly tells you which games have valid installers detected.
*   **Universal macOS Support:** Official builds now available for both **Apple Silicon (M1/M2/M3)** and **Intel** Macs.
*   **Internationalization:** Full documentation available in **Spanish** (`README.es.md`).
*   **Security Hardening:** Credential management moved to secure external configuration files.

### 🐛 Bug Fixes
*   **Critical Installation Fix:** Resolved an issue where valid installers in subfolders (common with GOG) were detected but failed to launch.
*   **Database Stability:** Fixed SQLite schema issues ensuring robust metadata persistence.
*   **Cross-Platform Paths:** Improved path resolution logic for mixed-OS environments.

### 📦 Updating
*   **Docker:** `docker-compose pull && docker-compose up -d`
*   **Desktop:** Download the latest installer for your OS below.

---

# Playerr v0.3.0: La Actualización "Instala Donde Sea" 🚀

Hemos mejorado el motor principal para que sea más inteligente, rápido y compatible con tus caóticas bibliotecas de juegos.

### 🌟 Nuevas Características
*   **Detección Inteligente de Instaladores:** ¡El escáner ahora busca más profundo! Añadido soporte para instaladores anidados (Profundidad-1) y patrones de nombres difusos (`setup_*.exe`, `install*.exe`). Si está ahí, Playerr lo encontrará.
*   **Visual Status Indicators:** Nuevo **Botón Verde "Listo para Instalar"** en la interfaz que te dice al instante qué juegos tienen instaladores válidos detectados.
*   **Soporte Universal macOS:** Binarios oficiales ahora disponibles tanto para Macs con **Apple Silicon (M1/M2/M3)** como **Intel**.
*   **Internacionalización:** Documentación completa disponible en **Español** (`README.es.md`).
*   **Seguridad Reforzada:** Gestión de credenciales movida a archivos de configuración externos seguros.

### 🐛 Correcciones de Errores
*   **Arreglo Crítico de Instalación:** Resuelto un problema donde instaladores válidos en subcarpetas (común en GOG) eran detectados pero fallaban al lanzarse.
*   **Estabilidad de Base de Datos:** Arreglados problemas de esquema SQLite asegurando una persistencia de metadatos robusta.
*   **Rutas Multiplataforma:** Lógica de resolución de rutas mejorada para entornos con sistemas operativos mixtos.

### 📦 Actualización
*   **Docker:** `docker-compose pull && docker-compose up -d`
*   **Escritorio:** Descarga el instalador más reciente para tu SO abajo.
