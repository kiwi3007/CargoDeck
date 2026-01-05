# Playerr v0.1.2-beta
### **Self-Hosted Game Library Manager & PVR**

[![Go to Website](https://img.shields.io/badge/Website-playerr.app-6366f1?style=for-the-badge)](https://playerr.app)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg?style=for-the-badge)](https://opensource.org/licenses/MIT)
[![Docker Support](https://img.shields.io/badge/Docker-amd64%20%2F%20arm64-2496ed?style=for-the-badge&logo=docker)](https://hub.docker.com/r/maikboarder/playerr)

### Downloads (v0.1.2-beta)
[![Windows](https://img.shields.io/badge/Windows-0078D4?style=for-the-badge&logo=windows&logoColor=white)](https://github.com/Maikboarder/Playerr/releases/download/v0.1.2-beta/Playerr-Windows-x64.zip)
[![macOS ARM64](https://img.shields.io/badge/macOS_Apple_Silicon-000000?style=for-the-badge&logo=apple&logoColor=white)](https://github.com/Maikboarder/Playerr/releases/download/v0.1.2-beta/Playerr.dmg)
[![macOS Intel](https://img.shields.io/badge/macOS_Intel-000000?style=for-the-badge&logo=apple&logoColor=white)](https://github.com/Maikboarder/Playerr/releases/download/v0.1.2-beta/Playerr-Intel.dmg)
[![Linux](https://img.shields.io/badge/Linux-FCC624?style=for-the-badge&logo=linux&logoColor=black)](https://github.com/Maikboarder/Playerr/releases/download/v0.1.2-beta/Playerr-Linux-x64.tar.gz)

Inspired by the workflow of Radarr and Sonarr, Playerr is designed to be the definitive solution for video game enthusiasts who self-host their libraries. It bridges the gap between your local digital assets and the vast world of gaming metadata.

## Main Features

*   **Intelligent Library Scanning:** Recursive and smart recognition engine that identifies video game platforms across your storage, mapping local files to their respective titles.
*   **Rich Metadata Integration:** Native hooks into IGDB and Steam APIs to fetch high-quality artwork, descriptions, ratings, and release dates.
*   **Seamless PVR Workflow:** Support for Prowlarr and Jackett for automated indexer management and advanced searching.
*   **NZB Protocol Support:** Native integration for Usenet downloads via NZB files, automatically handling protocol associations. Compatible with **SABnzbd** and **NZBGet**.
*   **Integrated Download Management:** Native control for industry-standard clients like qBittorrent and Transmission.
*   **Modern Web GUI:** A vibrant, dark-themed responsive interface designed for both desktop and containerized environments.
*   **Unified Library View:** Display your entire gaming collection in one place, including native support for syncing and viewing your **Steam Library**.

## Screenshots

| Library | Game Details |
|:---:|:---:|
| ![Library](https://raw.githubusercontent.com/Maikboarder/Playerr/master/screenshots/Library.png) | ![Game Details](https://raw.githubusercontent.com/Maikboarder/Playerr/master/screenshots/Details.png) |

| Settings (Indexers) | Library Grid |
|:---:|:---:|
| ![Settings](https://raw.githubusercontent.com/Maikboarder/Playerr/master/screenshots/Indexers%3ATorrents.png) | ![Library Grid](https://raw.githubusercontent.com/Maikboarder/Playerr/master/screenshots/Library%20Games.png) |

<p align="center">
  <img src="https://raw.githubusercontent.com/Maikboarder/Playerr/master/screenshots/SteamProfile.png" alt="Steam Profile Integration" width="600">
  <br>
  <em>Steam Profile Integration</em>
</p>

## Supported Platforms

Playerr is architected for maximum reach, offering multi-platform binaries and containerized solutions:

*   **Docker:** Universal support for amd64 and arm64 (Raspberry Pi, CasaOS, Synology, etc.).
*   **Windows:** Native 64-bit performance. [Download .zip](https://github.com/Maikboarder/Playerr/releases/download/v0.1.2-beta/Playerr-Windows-x64.zip)
*   **macOS:** Optimized for Apple Silicon ([Download .dmg](https://github.com/Maikboarder/Playerr/releases/download/v0.1.2-beta/Playerr.dmg)) and Intel ([Download .dmg](https://github.com/Maikboarder/Playerr/releases/download/v0.1.2-beta/Playerr-Intel.dmg)).
*   **Linux:** Generic 64-bit binary distributions. [Download .tar.gz](https://github.com/Maikboarder/Playerr/releases/download/v0.1.2-beta/Playerr-Linux-x64.tar.gz)

## Installation & Setup

### Docker (Recommended)
The easiest way to run Playerr is using Docker. It includes everything you need in a single container. Access the UI at `http://your-ip:2727`.

#### Standard Desktop / Server
Create a `docker-compose.yml` file and run `docker-compose up -d`:
```yaml
services:
  playerr:
    image: maikboarder/playerr:latest
    container_name: playerr
    ports:
      - "2727:2727"
    volumes:
      - ./config:/app/config
      - /your/games/path:/media
    restart: unless-stopped
```

#### CasaOS
1. Go to **App Store** -> **Custom Install**.
2. Click on **Import** (top right) and paste this specific code (includes the icon):
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
       en_us: Playerr
   ```
3. Click **Install**.

#### Synology / NAS
1. Open **Container Manager** (or Docker).
2. Go to **Project** -> **Create**.
3. Paste the Docker Compose code and configure your local folders.
4. Click **Done**.

### Build from Source (For Developers)

If you want to modify the code or build the image locally instead of pulling it from Docker Hub:

1. Clone the repository:
   ```bash
   git clone https://github.com/maikboarder/playerr.git
   cd playerr
   ```

2. Use the build command:
   ```bash
   docker build -t playerr:local .
   ```

3. Or use a `docker-compose.override.yml` to force a local build:
   ```yaml
   services:
     playerr:
       build: .
       image: playerr:local
       # ... rest of your config
   ```

---

## Roadmap

- [x] **v0.1.2 Beta:** NZB Protocol support and Ko-fi Widget.
    - [x] **Feature:** NZB Support (Search & Download).
    - [x] **Feature:** Ko-fi Global Widget.
    - [x] **Release:** Builds (Win/Lin/Mac-Arm), Docs updated, Tag pushed.
    - **Optimization:** IGDB Rate Limit handling (batching + delays).
    - **Feature:** Hardlink Support (Atomic Move) with fallback to Copy.
    - **Integration:** qBittorrent UrlBase support for Reverse Proxies.
- [ ] **Bazzite Support:** Researching compatibility with Lutris and Proton.
- [ ] **DBI Protocol Integration:** Advanced USB file transfer and management for Portable Consoles environments.
- [ ] **CasaOS Official App:** Direct integration into the CasaOS App Store.
- [ ] **Legacy Support:** Extended optimization for Intel-based macOS systems.
- [ ] **Extensibility:** Support for community-driven scripts and metadata plugins.

## Community & Support

I'm building Playerr with the community in mind. Your feedback is the engine that drives our development.

*   **Contribute:** Found a bug? Have a killer feature idea? Open an issue or a PR!
*   **Support:** If Playerr brings value to your setup, consider supporting the project. Your contributions enable more focused development, better stability, and faster implementation of the roadmap.

[<img src="https://storage.ko-fi.com/cdn/cup-border.png" width="200" alt="Buy Me a Coffee at ko-fi.com" />](https://ko-fi.com/maikboarder)
[![Sponsor on GitHub](https://img.shields.io/static/v1?label=Sponsor&message=%E2%9D%A4&logo=GitHub&color=%23fe8e86&style=for-the-badge)](https://github.com/sponsors/Maikboarder)

## License

Distributed under the MIT License. See `LICENSE` for more information.

## Legal Disclaimer

Playerr is an open-source project for educational and personal library management. It is **not affiliated** with any third-party game platforms or metadata providers. The developers do not condone piracy; users are responsible for complying with their local laws regarding copyright and content usage. See `DISCLAIMER.md` for the full legal notice.

---
*Developed by Maikboarder*

---

# Playerr v0.1.2-beta
### Gestor de Biblioteca de Videojuegos & PVR (Self-Hosted)

[![Ir a la web](https://img.shields.io/badge/Website-playerr.app-6366f1?style=for-the-badge)](https://playerr.app)
[![Licencia: MIT](https://img.shields.io/badge/License-MIT-yellow.svg?style=for-the-badge)](https://opensource.org/licenses/MIT)
[![Soporte Docker](https://img.shields.io/badge/Docker-amd64%20%2F%20arm64-2496ed?style=for-the-badge&logo=docker)](https://hub.docker.com/r/maikboarder/playerr)

### Descargas (v0.1.2-beta)
[![Windows](https://img.shields.io/badge/Windows-0078D4?style=for-the-badge&logo=windows&logoColor=white)](https://github.com/Maikboarder/Playerr/releases/download/v0.1.2-beta/Playerr-Windows-x64.zip)
[![macOS ARM64](https://img.shields.io/badge/macOS_Apple_Silicon-000000?style=for-the-badge&logo=apple&logoColor=white)](https://github.com/Maikboarder/Playerr/releases/download/v0.1.2-beta/Playerr.dmg)
[![macOS Intel](https://img.shields.io/badge/macOS_Intel-000000?style=for-the-badge&logo=apple&logoColor=white)](https://github.com/Maikboarder/Playerr/releases/download/v0.1.2-beta/Playerr-Intel.dmg)
[![Linux](https://img.shields.io/badge/Linux-FCC624?style=for-the-badge&logo=linux&logoColor=black)](https://github.com/Maikboarder/Playerr/releases/download/v0.1.2-beta/Playerr-Linux-x64.tar.gz)

Inspirado en el flujo de trabajo de Radarr y Sonarr, Playerr está diseñado para ser la solución definitiva para los entusiastas de los videojuegos que gestionan sus bibliotecas en local. Playerr conecta tus archivos digitales con el mundo del metadato gamer.

---

## Características principales

*   **Escaneo inteligente de biblioteca:** Reconocimiento recursivo y automático de plataformas de videojuegos en tu almacenamiento, mapeando archivos locales a sus títulos correspondientes.
*   **Integración de metadatos:** Conexión nativa con IGDB y Steam para obtener imágenes, descripciones, valoraciones y fechas de lanzamiento.
*   **Flujo PVR automatizado:** Soporte para Prowlarr y Jackett para gestión avanzada de indexadores y búsquedas.
*   **Soporte Protocolo NZB:** Integración nativa para descargas Usenet mediante archivos NZB, gestionando automáticamente la asociación de protocolos. Compatible con **SABnzbd** y **NZBGet**.
*   **Gestión de descargas integrada:** Control nativo de clientes como qBittorrent y Transmission.
*   **Interfaz web moderna:** GUI oscura, responsiva y pensada para escritorio y contenedores.
*   **Vista unificada:** Muestra toda tu colección en un solo lugar, incluyendo sincronización y visualización de tu biblioteca de Steam.

## Plataformas soportadas

Playerr está diseñado para máxima compatibilidad, ofreciendo binarios multiplataforma y soluciones en contenedor:

*   **Docker:** Soporte universal para amd64 y arm64 (Raspberry Pi, CasaOS, Synology, etc.).
*   **Windows:** Rendimiento nativo 64-bit. [Descargar .zip](https://github.com/Maikboarder/Playerr/releases/download/v0.1.2-beta/Playerr-Windows-x64.zip)
*   **macOS:** Optimizado para Apple Silicon ([Descargar .dmg](https://github.com/Maikboarder/Playerr/releases/download/v0.1.2-beta/Playerr.dmg)) y sistemas Intel ([Descargar .dmg](https://github.com/Maikboarder/Playerr/releases/download/v0.1.2-beta/Playerr-Intel.dmg)).
*   **Linux:** Distribuciones genéricas 64-bit. [Descargar .tar.gz](https://github.com/Maikboarder/Playerr/releases/download/v0.1.2-beta/Playerr-Linux-x64.tar.gz)

## Instalación y configuración

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
3. Pega el código y configura tus carpetas locales.
4. Haz clic en **Listo**.

### Compilar desde el código (Desarrolladores)

Si quieres modificar el código o construir la imagen localmente:

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

