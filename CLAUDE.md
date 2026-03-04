# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Architecture Overview

Playerr is a **self-hosted game library manager & PVR** inspired by Radarr/Sonarr. It consists of:

### Backend (.NET 8 / ASP.NET Core)
Located in `src/`:
- **Playerr.Host** - Main ASP.NET Core application with Photino.NET desktop window
- **Playerr.Api.V3** - API controllers (Games, Search, Indexers, Steam, Switch, etc.)
- **Playerr.Core** - Core business logic (Games, MediaScanner, Indexers, Metadata, Download clients)
- **Playerr.Common** - Shared types and interfaces
- **Playerr.Http** - HTTP client utilities
- **Playerr.SignalR** - Real-time communication (library updates)
- **Playerr.Console** - Console application entry point
- **Playerr.UsbHelper** - USB helper for Switch consoles

Key architecture patterns:
- **Dependency Injection**: Heavy use of interfaces (`IGameRepository`, `ILaunchStrategy`, `IDownloadClient`)
- **SQLite**: Persistent storage via `PlayerrDbContext`
- **SignalR**: Real-time updates for library scanning
- **Multi-platform**: Targets win-x64/win-x86/osx-x64/osx-arm64/linux-x64/linux-arm64

### Frontend (React/TypeScript)
Located in `frontend/src/`:
- Built with **Webpack** (not Create React App)
- Uses **React Router**, **TanStack Query**, **SignalR client**
- Outputs to `_output/UI/` (next to the backend executable)
- Dark-themed UI with FontAwesome icons

## Common Commands

### Backend (C#/.NET)
```bash
# Build backend
dotnet build src/Playerr.Host/Playerr.Host.csproj

# Run backend (desktop window mode)
dotnet run --project src/Playerr.Host/Playerr.Host.csproj

# Run backend headless (Docker/server mode)
dotnet run --project src/Playerr.Host/Playerr.Host.csproj -- --headless

# Build for specific platform
dotnet publish src/Playerr.Host/Playerr.Host.csproj -c Release -r linux-x64 --self-contained true -p:PublishSingleFile=true

# Run Swagger UI (only in development)
dotnet run --project src/Playerr.Host/Playerr.Host.csproj
```

### Frontend (React/TypeScript)
```bash
# Install dependencies
npm install

# Build frontend
npm run build

# Watch mode (development)
npm run start

# Lint
npm run lint
npm run lint-fix
```

### Docker
```bash
# Build from source
docker build -t playerr:local .

# Using docker-compose
docker-compose up -d

# Build without cache
docker-compose build --no-cache
```

### Full Build (Release Artifacts)
```bash
./build_all.sh
# Outputs to build_artifacts/ directory
```

## Key Files to Know

| File | Purpose |
|------|---------|
| `src/Playerr.Host/Program.cs` | Application entry point, service registration, config handling |
| `src/Playerr.Core/Games/MediaScannerService.cs` | Library scanning logic |
| `frontend/build/webpack.config.js` | Webpack configuration |
| `Dockerfile` | Multi-stage build (frontend -> backend -> runtime) |
| `package.json` | Frontend dependencies and scripts |
| `build_all.sh` | Multi-platform build script |
| `config.example/` | Configuration templates |
| `indexers.json` | Default indexer configuration |

## Database Schema

- **SQLite** at `config/playerr.db`
- Main tables: `Games`, `Platforms`, `GameFiles`
- Schema migrations are applied manually via raw SQL in `Program.cs`
- Default platforms are seeded on startup (PC, Mac, Switch, PS1-5, Xbox Series X, PSP)

## Configuration

Config files stored in `config/` directory:
- `appsettings.json` - Server settings (port, IP)
- `hydra.json` - Indexer configuration
- `prowlarr.json` / `jackett.json` - Download manager settings
- `igdb.json` - IGDB API credentials
- `media.json` - Media paths (folder, download, destination)

## Important Development Notes

1. **Frontend testing**: User handles UI testing; use curl for backend verification
2. **Platform IDs**: PC=6, Mac=14, Switch=130, PSP=38, PS1-5=7,8,9,48,167
3. **Port handling**: Backend tries ports 5002-5005, then falls back to dynamic
4. **Headless mode**: Use `--headless` flag or `DOTNET_RUNNING_IN_CONTAINER=true` for Docker
5. **UI serving**: Frontend built to `_output/UI/` and served from backend

## API Endpoints

Base URL: `http://localhost:5002/api/v3/`

Main controllers:
- `GamesController` - Game CRUD operations
- `SearchController` - Indexer search (Torrents, NZB, Jackett, Prowlarr)
- `Metadata/GameLookupController` - IGDB/Steam metadata
- `Settings/*` - Various settings endpoints
- `MediaController` - Media scanning and management

## Build Output Structure

```
_output/
  UI/          # Frontend build (served by backend)
  win-x64/     # Windows executable
  linux-x64/   # Linux executable
  osx-x64/     # macOS Intel
  osx-arm64/   # macOS Apple Silicon
_temp/
  obj/         # Intermediate build artifacts
  bin/         # Compiled binaries
```

## Testing

Test projects follow the pattern `*Test.csproj` and use NUnit:
```bash
dotnet test src/Playerr.Host.Test/Playerr.Host.Test.csproj
```

## Release Process

1. Bump version in: `package.json`, `build_all.sh`, `src/Directory.Build.props`, `frontend/src/About.tsx`
2. Update `RELEASE_NOTES.md` (no emojis in official docs)
3. Run `./build_all.sh` to create artifacts
4. Test releases or push to Docker Hub