# Playerr
[🇺🇸 Read in English](README.md)

### **Gestor de Biblioteca de Videojuegos & PVR (Self-Hosted)**

[![Ir a la web](https://img.shields.io/badge/Website-playerr.app-6366f1?style=for-the-badge)](https://playerr.app)
[![Licencia: MIT](https://img.shields.io/badge/License-MIT-yellow.svg?style=for-the-badge)](https://opensource.org/licenses/MIT)
[![Soporte Docker](https://img.shields.io/badge/Docker-amd64%20%2F%20arm64-2496ed?style=for-the-badge&logo=docker)](https://hub.docker.com/r/maikboarder/playerr)

### Descargas (Última versión)
[![Windows](https://img.shields.io/badge/Windows-0078D4?style=for-the-badge&logo=windows&logoColor=white)](https://github.com/Maikboarder/Playerr/releases/latest/download/Playerr-Windows-x64.zip)
[![Playerr.exe](https://img.shields.io/badge/Playerr.exe-Installer-0078D4?style=for-the-badge&logo=windows&logoColor=white)](https://github.com/Maikboarder/Playerr/releases/latest/download/Playerr-Windows-Setup-x64.exe)
[![Playerr.app](https://img.shields.io/badge/Playerr.app-macOS-000000?style=for-the-badge&logo=apple&logoColor=white)](https://github.com/Maikboarder/Playerr/releases/latest/download/Playerr.dmg)
[![macOS ARM64](https://img.shields.io/badge/macOS_Apple_Silicon-000000?style=for-the-badge&logo=apple&logoColor=white)](https://github.com/Maikboarder/Playerr/releases/latest/download/Playerr.dmg)
[![macOS Intel](https://img.shields.io/badge/macOS_Intel-000000?style=for-the-badge&logo=apple&logoColor=white)](https://github.com/Maikboarder/Playerr/releases/latest/download/Playerr-Intel.dmg)
[![Linux](https://img.shields.io/badge/Linux-FCC624?style=for-the-badge&logo=linux&logoColor=black)](https://github.com/Maikboarder/Playerr/releases/latest/download/Playerr-Linux-x64.tar.gz)

Inspirado en el flujo de trabajo de Radarr y Sonarr, Playerr está diseñado para ser la solución definitiva para los entusiastas de los videojuegos que gestionan sus bibliotecas en local. Playerr conecta tus archivos digitales con el mundo del metadato gamer, cerrando la brecha entre tus activos locales y la vasta información disponible en la red.

## Características principales

*   **Escaneo inteligente de biblioteca:** Motor de reconocimiento recursivo e inteligente que identifica plataformas de videojuegos en tu almacenamiento, mapeando archivos locales a sus títulos correspondientes.
*   **Integración de metadatos robusta:** Conexión nativa con IGDB y Steam para obtener imágenes de alta calidad, descripciones, valoraciones y fechas de lanzamiento.
*   **Flujo PVR automatizado:** Soporte para Prowlarr y Jackett para gestión automática de indexadores y búsquedas avanzadas.
*   **Soporte Protocolo NZB:** Integración nativa para descargas Usenet mediante archivos NZB, gestionando automáticamente la asociación de protocolos. Compatible con **SABnzbd** y **NZBGet**.
*   **Conectividad con Clientes de Descarga:** Integración nativa mediante API para gestionar transferencias en los estándares de la industria (qBittorrent, Transmission, SABnzbd).
*   **Interfaz web moderna:** GUI visualmente atractiva, oscura y responsiva, diseñada tanto para escritorio como para entornos Docker.
*   **Gestión automática de rutas y archivos:** Renombrado automático de carpetas basado en títulos de IGDB sanitizados, preservando la estructura del lanzamiento original mientras se mantiene la biblioteca limpia y organizada.
*   **Herramienta de Despliegue Automatizado:** Procesa eficientemente paquetes de instalación locales e identifica los ejecutables primarios para optimizar la organización de la biblioteca.
*   **Vista unificada:** Muestra toda tu colección en un solo lugar, incluyendo soporte nativo para sincronización y visualización de tu **Biblioteca de Steam**.

## Screenshots

| Vista de Juego | Detalles del Juego |
|:---:|:---:|
| ![Vista de Juego](https://raw.githubusercontent.com/Maikboarder/Playerr/master/screenshots/Game%20View.png) | ![Detalles del Juego](https://raw.githubusercontent.com/Maikboarder/Playerr/master/screenshots/Details.png) |

| Configuración (Indexers) | Cuadrícula de Biblioteca |
|:---:|:---:|
| ![Configuración](https://raw.githubusercontent.com/Maikboarder/Playerr/master/screenshots/Indexers%3ATorrents.png) | ![Cuadrícula de Biblioteca](https://raw.githubusercontent.com/Maikboarder/Playerr/master/screenshots/Library%20Games.png) |

<p align="center">
  <img src="https://raw.githubusercontent.com/Maikboarder/Playerr/master/screenshots/Search%20Manager.png" alt="Gestor de Búsqueda" width="600">
  <br>
  <em>Gestor de Búsqueda</em>
</p>

## Plataformas soportadas

Playerr está diseñado para un alcance máximo, ofreciendo binarios multiplataforma y soluciones en contenedor:

*   **Docker:** Soporte universal para amd64 y arm64 (Raspberry Pi, CasaOS, Synology, etc.).
*   **Windows:** Rendimiento nativo 64-bit. [Descargar .exe (Instalador)](https://github.com/Maikboarder/Playerr/releases/latest/download/Playerr-Windows-Setup-x64.exe) o [Descargar .zip (Portable)](https://github.com/Maikboarder/Playerr/releases/latest/download/Playerr-Windows-x64.zip)
*   **macOS:** Optimizado para Apple Silicon ([Descargar .dmg](https://github.com/Maikboarder/Playerr/releases/latest/download/Playerr.dmg)) y sistemas Intel ([Descargar .dmg](https://github.com/Maikboarder/Playerr/releases/latest/download/Playerr-Intel.dmg)).
*   **Linux:** Distribuciones genéricas 64-bit. [Descargar .tar.gz](https://github.com/Maikboarder/Playerr/releases/latest/download/Playerr-Linux-x64.tar.gz)

### 🎮 Compatibilidad y Ejecución
Playerr gestiona tu biblioteca, pero para ejecutar títulos nativos de Windows en macOS o Linux, recomendamos las siguientes capas de compatibilidad:

* **macOS:** [Whisky](https://getwhisky.app/) (Gratis/Open Source) o [CrossOver](https://www.codeweavers.com/crossover) (Pago/Soporte Oficial).
* **Linux / Steam Deck:** Soporte nativo vía **Proton** (Steam), [Lutris](https://lutris.net/) o [Bottles](https://usebottles.com/).

## Instalación y configuración

> **Nota:** Requiere claves API de IGDB válidas (gratuitas) para la obtención de metadatos.

### Docker (Recomendado)
La forma más sencilla de ejecutar Playerr es con Docker. Incluye todo lo necesario en un solo contenedor. Accede a la interfaz en `http://tu-ip:2727`.

#### Escritorio / Servidor estándar
Crea un archivo `docker-compose.yml` y ejecuta `docker-compose up -d`:
```yaml
services:
  playerr:
    image: maikboarder/playerr:latest
    container_name: playerr
    ports:
      - "2727:2727"
    volumes:
      - ./config:/app/config
      - /ruta/a/tus/juegos:/media
    restart: unless-stopped
```

#### CasaOS
1. Ve a **App Store** -> **Instalación personalizada**.
2. Haz clic en **Importar** (arriba a la derecha) y pega este código (incluye el icono):
   ```yaml
   services:
     playerr:
       image: maikboarder/playerr:latest
       container_name: playerr
       ports:
         - "2727:2727"
       volumes:
         - /DATA/AppData/playerr/config:/app/config
         - /DATA/Media/Games:/media
       restart: unless-stopped
   
   x-casaos:
     architectures:
       - amd64
       - arm64
     main: playerr
     icon: https://raw.githubusercontent.com/Maikboarder/Playerr/master/frontend/src/assets/app_logo.png
     title:
       es_es: Playerr
   ```
3. Haz clic en **Instalar**.

#### Synology / NAS
1. Abre **Container Manager** (o Docker).
2. Ve a **Proyecto** -> **Crear**.
3. Pega el código Docker Compose y configura tus carpetas locales.
4. Haz clic en **Listo**.

### Compilar desde el código (Desarrolladores)

Si quieres modificar el código o construir la imagen localmente en lugar de descargarla desde Docker Hub:

1. Clona el repositorio:
   ```bash
   git clone https://github.com/maikboarder/playerr.git
   cd playerr
   ```

2. Usa el comando de compilación:
   ```bash
   docker build -t playerr:local .
   ```

3. O usa un `docker-compose.override.yml` para forzar compilación local:
   ```yaml
   services:
     playerr:
       build: .
       image: playerr:local
       # ... resto de la config
   ```

---

## 🗺️ Roadmap (Hoja de Ruta)

### Fase 1: Cimientos (v0.1.0 - v0.1.2)
- [x] **Funcionalidad PVR Core:** Motor de búsqueda y categorización automática.
- [x] **Soporte de Protocolo NZB:** Integración nativa con SABnzbd y NZBGet.
- [x] **Despliegue Multiplataforma:** Binarios oficiales para Windows, macOS (Apple e Intel) y Linux.
- [x] **Instalador de Windows:** Instalador profesional NSIS para una experiencia de configuración fluida.
- [x] **Persistencia de Datos:** Integración con SQLite para asegurar la longevidad de tu biblioteca y metadatos.

### Fase 2: Funciones Avanzadas (Enfoque Actual)
- [x] **Optimización de Infraestructura y Almacenamiento:**
  - [x] **Hardlinks (Atomic Move):** Gestión instantánea de archivos sin fragmentación de datos.
  - [x] **Integración con Unraid:** Soporte mediante plantilla XML comunitaria (`_unraid/playerr.xml`).
  - [x] **Gestión Inteligente de API:** Control de límites y procesamiento por lotes para proveedores de metadatos.
- [ ] **Integración "One-Click Launch":** Soporte de ejecución directa para juegos instalados con detección automática de rutas.
- [x] **Refinamiento de UI/UX:** Iconografía premium (FontAwesome) y diseño consistente basado en el tema Nord.

### Fase 3: Ecosistema y Visión de Futuro
- [ ] **Bazzite y Gaming en Linux:** Hooks de compatibilidad especializados para Lutris, Proton y Steam Deck.
- [ ] **Protocolo DBI:** Transferencia avanzada de archivos por USB para la gestión de dispositivos de hardware portátiles.
- [ ] **Tiendas Oficiales:** Integración en los catálogos oficiales de aplicaciones de Unraid, CasaOS y Synology.
- [ ] **Motor de Extensibilidad:** Soporte para scripts de la comunidad y plugins de metadatos.

## Comunidad y soporte

Estoy construyendo Playerr con la comunidad en mente. Tu feedback es el motor que impulsa nuestro desarrollo.

*   **Contribuye:** ¿Encontraste un bug? ¿Tienes una idea brillante? ¡Abre un issue o un PR!
*   **Soporte:** Si Playerr aporta valor a tu setup, considera apoyar el proyecto. Tus contribuciones permiten un desarrollo más enfocado, mejor estabilidad y una implementación más rápida del roadmap.


[![Sponsor on GitHub](https://img.shields.io/static/v1?label=Sponsor&message=%E2%9D%A4&logo=GitHub&color=%23fe8e86&style=for-the-badge)](https://github.com/sponsors/Maikboarder)

## Licencia

Distribuido bajo la Licencia MIT. Consulta `LICENSE` para más información.

## Aviso Legal

Playerr es un proyecto de código abierto para la gestión educativa y personal de bibliotecas. **No está afiliado** con ninguna plataforma de juegos o proveedor de metadatos de terceros. Los desarrolladores no aprueban la piratería; los usuarios son responsables de cumplir con las leyes locales de derechos de autor. Consulta `DISCLAIMER.md` para el aviso legal completo.

---
*Desarrollado por Maikboarder*
