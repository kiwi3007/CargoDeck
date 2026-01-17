using System;
using Microsoft.AspNetCore.Mvc;
using Playerr.Core.Games;
using Playerr.Core.MetadataSource;
using Playerr.Core.Launcher;
using System.Collections.Generic;
using System.Threading.Tasks;
using System.Linq;
using System.IO;
using System.Runtime.InteropServices;
using System.Text.RegularExpressions;
using Playerr.Core.Configuration;

namespace Playerr.Api.V3.Games
{
    [ApiController]
    [Route("api/v3/[controller]")]
    public class GameController : ControllerBase
    {
        private readonly IGameRepository _repository;
        private readonly IGameMetadataServiceFactory _metadataServiceFactory;
        private readonly Playerr.Core.IO.IArchiveService _archiveService;
        private readonly ILauncherService _launcherService;
        private readonly ConfigurationService _configService;

        public GameController(IGameRepository repository, IGameMetadataServiceFactory metadataServiceFactory, Playerr.Core.IO.IArchiveService archiveService, ILauncherService launcherService, ConfigurationService configService)
        {
            _repository = repository;
            _metadataServiceFactory = metadataServiceFactory;
            _archiveService = archiveService;
            _launcherService = launcherService;
            _configService = configService;
        }

        [HttpGet]
        public async Task<IEnumerable<Game>> GetAll([FromQuery] string lang = "es")
        {
            System.Console.WriteLine("[API] GetAll Games Request Received");
            try 
            {
                var games = await _repository.GetAllAsync();
                System.Console.WriteLine($"[API] Retrieved {games.Count()} games from DB");
                return games;
            }
            catch (Exception ex)
            {
                System.Console.WriteLine($"[API] Error in GetAll: {ex.Message} - {ex.StackTrace}");
                throw;
            }
        }

        [HttpGet("{id}")]
        public async Task<ActionResult<Game>> GetById(int id, [FromQuery] string? lang = null)
        {
            var game = await _repository.GetByIdAsync(id);
            if (game == null)
            {
                return NotFound();
            }

            // If a language is requested and the game has an IgdbId, fetch localized metadata
            if (!string.IsNullOrEmpty(lang) && game.IgdbId.HasValue)
            {
                try
                {
                    var metadataService = _metadataServiceFactory.CreateService();
                    var localizedGame = await metadataService.GetGameMetadataAsync(game.IgdbId.Value, lang);
                    
                    if (localizedGame != null)
                    {
                        // Override localized fields for the display
                        game.Title = localizedGame.Title;
                        game.Overview = localizedGame.Overview;
                        game.Storyline = localizedGame.Storyline;
                        game.Genres = localizedGame.Genres;
                        if (game.Platform != null)
                        {
                            game.Platform.Name = metadataService.LocalizePlatform(game.Platform.Name, lang);
                        }
                    }
                }
                catch
                {
                    // Fallback to stored metadata if IGDB fetch fails
                }
            }

            game.IsInstallable = IsPathInstallable(game.Path);

            var uninstallerPath = FindUninstaller(game.Path);
            var downloadPathHint = FindDownloadFolder(game.Title, game.Path);

            var isInstaller = game.Status == GameStatus.InstallerDetected || 
                              (!string.IsNullOrEmpty(game.ExecutablePath) && 
                               (game.ExecutablePath.EndsWith("setup.exe", System.StringComparison.OrdinalIgnoreCase) || 
                                game.ExecutablePath.EndsWith("install.exe", System.StringComparison.OrdinalIgnoreCase)));

            bool canPlay = (game.SteamId.HasValue && game.SteamId.Value > 0) || 
                           (!string.IsNullOrEmpty(game.ExecutablePath) && 
                            System.IO.File.Exists(game.ExecutablePath) && 
                            !isInstaller);

            System.Console.WriteLine($"[API] Game {id} GetById - canPlay: {canPlay} (Path: {game.ExecutablePath}, SteamId: {game.SteamId}, Status: {game.Status})");

            return Ok(new
            {
                game.Id,
                game.Title,
                game.AlternativeTitle,
                game.Year,
                game.Overview,
                game.Storyline,
                game.PlatformId,
                game.Platform,
                game.Added,
                game.Images,
                game.Genres,
                game.AvailablePlatforms,
                game.Developer,
                game.Publisher,
                game.ReleaseDate,
                game.Rating,
                game.RatingCount,
                game.Status,
                game.Monitored,
                game.Path,
                game.SizeOnDisk,
                game.IgdbId,
                game.SteamId,
                game.GogId,
                game.InstallPath,
                game.IsInstallable,
                game.ExecutablePath,
                game.IsExternal,
                uninstallerPath,
                downloadPath = downloadPathHint,
                canPlay = canPlay // Explicit property name
            });
        }

        [HttpPost]
        public async Task<ActionResult<Game>> Create([FromBody] Game game)
        {
            System.Console.WriteLine($"[GameController] [Create] Attempting to add game: '{game.Title}' (IGDB: {game.IgdbId})");
            try 
            {
                var created = await _repository.AddAsync(game);
                System.Console.WriteLine($"[GameController] [Create] Success. Game ID: {created.Id}");
                return CreatedAtAction(nameof(GetById), new { id = created.Id }, created);
            }
            catch (Exception ex)
            {
                System.Console.WriteLine($"[GameController] [Create] FAILURE: {ex}");
                // Return 500 so frontend sees the error
                return StatusCode(500, $"Internal Server Error: {ex.Message}");
            }
        }

        [HttpPut("{id}")]
        public async Task<ActionResult<Game>> Update(int id, [FromBody] Game gameUpdate)
        {
            var existingGame = await _repository.GetByIdAsync(id);
            if (existingGame == null)
            {
                return NotFound();
            }

            // Check if IGDB ID has changed
            bool igdbIdChanged = gameUpdate.IgdbId.HasValue && gameUpdate.IgdbId != existingGame.IgdbId;

            // Apply updates
            if (gameUpdate.IgdbId.HasValue) existingGame.IgdbId = gameUpdate.IgdbId;
            if (!string.IsNullOrEmpty(gameUpdate.Title)) existingGame.Title = gameUpdate.Title;
            if (!string.IsNullOrEmpty(gameUpdate.InstallPath)) existingGame.InstallPath = gameUpdate.InstallPath;
            if (!string.IsNullOrEmpty(gameUpdate.ExecutablePath)) existingGame.ExecutablePath = gameUpdate.ExecutablePath;
            // Add other fields as necessary if the frontend sends them

            // If IGDB ID changed, fetch fresh metadata
            if (igdbIdChanged)
            {
                try
                {
                    var metadataService = _metadataServiceFactory.CreateService();
                    // Fetch in English (or default) to store in DB. Localization happens on GetById.
                    var freshMetadata = await metadataService.GetGameMetadataAsync(existingGame.IgdbId.Value, "en");
                    
                    if (freshMetadata != null) {
                       // Update core metadata
                       existingGame.Title = freshMetadata.Title; 
                       existingGame.Overview = freshMetadata.Overview;
                       existingGame.Storyline = freshMetadata.Storyline;
                       existingGame.Year = freshMetadata.Year;
                       existingGame.ReleaseDate = freshMetadata.ReleaseDate;
                       existingGame.Rating = freshMetadata.Rating;
                       existingGame.Genres = freshMetadata.Genres;
                       
                       // IMAGES are critical!
                       if (freshMetadata.Images != null) {
                           existingGame.Images = freshMetadata.Images;
                       }
                    }
                }
                catch (System.Exception ex)
                {
                    // Log error but proceed with saving the ID change at least?
                    System.Console.WriteLine($"Error refreshing metadata: {ex.Message}");
                }
            }

            var updated = await _repository.UpdateAsync(id, existingGame);
            return Ok(updated);
        }

        [HttpDelete("{id}")]
        public async Task<ActionResult> Delete(int id, [FromQuery] bool deleteFiles = false, [FromQuery] string? targetPath = null, [FromQuery] bool deleteDownloadFiles = false, [FromQuery] string? downloadPath = null)
        {
            var game = await _repository.GetByIdAsync(id);
            if (game == null) return NotFound();

            if (deleteFiles && !string.IsNullOrEmpty(game.Path))
            {
                // Determine what to delete: targetPath override or game.Path default
                string pathToDelete = !string.IsNullOrEmpty(targetPath) ? targetPath : game.Path;
                
                // Security/Safety Check:
                // 1. If targetPath is provided, it MUST contain the game.Path (i.e. be a parent or the same path)
                //    Wait, checking "Contains" might be tricky with normalization. 
                //    A parent path P contains child C? No, C starts with P.
                //    game.Path (Child) starts with pathToDelete (Parent).
                
                bool isSafe = false;

                if (string.IsNullOrEmpty(targetPath) || targetPath == game.Path)
                {
                    isSafe = true; // Default behavior is safe-ish (deletes what we know)
                }
                else
                {
                    // Validate relationship
                    var normalizedGamePath = System.IO.Path.GetFullPath(game.Path).TrimEnd(System.IO.Path.DirectorySeparatorChar);
                    var normalizedTarget = System.IO.Path.GetFullPath(pathToDelete).TrimEnd(System.IO.Path.DirectorySeparatorChar);
                    
                    if (normalizedGamePath.StartsWith(normalizedTarget, StringComparison.OrdinalIgnoreCase))
                    {
                        isSafe = true;
                    }
                }

                // Global Safety Blocklist to prevent deleting roots or critical folders
                if (IsCriticalPath(pathToDelete))
                {
                    System.Console.WriteLine($"[Delete] BLOCKED deletion of critical path: {pathToDelete}");
                    isSafe = false;
                }

                if (isSafe)
                {
                    try
                    {
                        if (System.IO.File.Exists(pathToDelete))
                        {
                            System.IO.File.Delete(pathToDelete);
                            System.Console.WriteLine($"[Delete] Deleted file: {pathToDelete}");
                        }
                        else if (System.IO.Directory.Exists(pathToDelete))
                        {
                            System.IO.Directory.Delete(pathToDelete, true);
                            System.Console.WriteLine($"[Delete] Deleted directory: {pathToDelete}");
                        }
                    }
                    catch (Exception ex)
                    {
                        System.Console.WriteLine($"[Delete] Error deleting library files at {pathToDelete}: {ex.Message}");
                    }
                }
                else
                {
                    System.Console.WriteLine($"[Delete] Safety check failed for library path: {pathToDelete}");
                    // We don't abort the metadata delete, but we warn? 
                    // Or we shout abort? Ideally abort if user explicitly requested file delete and it failed safety.
                    // But for now, let's proceed to delete metadata so the "broken" game is gone.
                }
            }

            // --- Download Folder Deletion Logic ---
            if (deleteDownloadFiles && !string.IsNullOrEmpty(downloadPath))
            {
                bool isDownloadSafe = !IsCriticalPath(downloadPath);
                
                if (isDownloadSafe && System.IO.Directory.Exists(downloadPath))
                {
                    try
                    {
                        System.IO.Directory.Delete(downloadPath, true);
                        System.Console.WriteLine($"[Delete] Deleted download directory: {downloadPath}");
                    }
                    catch (Exception ex)
                    {
                        System.Console.WriteLine($"[Delete] Error deleting download folder at {downloadPath}: {ex.Message}");
                    }
                }
                else if (!isDownloadSafe)
                {
                    System.Console.WriteLine($"[Delete] BLOCKED deletion of critical download path: {downloadPath}");
                }
            }

            var removed = await _repository.DeleteAsync(id);
            if (!removed)
            {
                return NotFound();
            }

            return NoContent();
        }

        private bool IsCriticalPath(string path)
        {
            if (string.IsNullOrEmpty(path)) return true;
            var full = System.IO.Path.GetFullPath(path).TrimEnd(System.IO.Path.DirectorySeparatorChar);
            var root = System.IO.Path.GetPathRoot(full);
            
            // 1. Root
            if (full.Equals(root, StringComparison.OrdinalIgnoreCase)) return true;
            
            // 2. Common System Folders (Linux/Mac/Win)
            var sensitive = new[] { 
                "/", "/bin", "/boot", "/dev", "/etc", "/home", "/lib", "/proc", "/root", "/run", "/sbin", "/sys", "/tmp", "/usr", "/var",
                "/Users", "/Users/imaik", "/Users/imaik/Desktop", "/Users/imaik/Documents", "/Users/imaik/Downloads",
                "C:\\", "C:\\Windows", "C:\\Program Files", "C:\\Users"
            };

            foreach (var s in sensitive)
            {
                 // Exact match blocking
                 if (full.Equals(s, StringComparison.OrdinalIgnoreCase)) return true;
                 
                 // Also block if it's a DIRECT child of a very sensitive root? 
                 // e.g. /Users/imaik/Desktop/Juegos is OK. 
                 // /Users/imaik/Desktop is BLOCKED (Safe).
            }

            return false;
        }


        [HttpPost("{id}/uninstall")]
        public async Task<ActionResult> Uninstall(int id)
        {
            var game = await _repository.GetByIdAsync(id);
            if (game == null) return NotFound("Game not found");
            
            if (string.IsNullOrEmpty(game.Path) || !Directory.Exists(game.Path))
                return BadRequest("Game path not found or invalid.");

            var uninstaller = FindUninstaller(game.Path);
            if (!string.IsNullOrEmpty(uninstaller))
            {
                // Reuse LaunchInstaller logic but for uninstaller
                return LaunchInstaller(uninstaller);
            }

            return NotFound("No uninstaller found.");
        }


        [HttpPost("{id}/install")]
        public async Task<ActionResult> Install(int id)
        {
            var game = await _repository.GetByIdAsync(id);
            if (game == null) return NotFound("Game not found in repository");

            if (string.IsNullOrEmpty(game.Path)) return BadRequest("Game path is not set.");
            
            string targetPath = game.Path;
            System.Console.WriteLine($"[Install] Target Path: {targetPath}");

            // Case 0.1: Archive (Zip, Rar, 7z)
            if (_archiveService.IsArchive(targetPath))
            {
                var extractDir = Path.Combine(Path.GetDirectoryName(targetPath), Path.GetFileNameWithoutExtension(targetPath));
                if (_archiveService.Extract(targetPath, extractDir))
                {
                     // Update the game path to the new directory so subsequent scans/installs work
                     game.Path = extractDir;
                     await _repository.UpdateAsync(id, game);
                     
                     return Ok(new { message = $"Archive extracted to {extractDir}. Please Scan or Install again from the new folder.", path = extractDir });
                }
                else
                {
                    return BadRequest("Failed to extract archive.");
                }
            }

            // Case 0.2: ISO Image (MacOS and Windows supported)
            if (System.IO.File.Exists(targetPath) && 
                System.IO.Path.GetExtension(targetPath).Equals(".iso", System.StringComparison.OrdinalIgnoreCase))
            {
                if (RuntimeInformation.IsOSPlatform(OSPlatform.OSX))
                {
                    var mountPoint = await MountIsoMacOS(targetPath);
                    if (!string.IsNullOrEmpty(mountPoint))
                    {
                        System.Console.WriteLine($"[Install] ISO Mounted at: {mountPoint}");
                        targetPath = mountPoint; 
                    }
                    else
                    {
                        return BadRequest("Failed to mount ISO image on macOS.");
                    }
                }
                else if (RuntimeInformation.IsOSPlatform(OSPlatform.Windows))
                {
                    var mountPoint = await MountIsoWindows(targetPath);
                    if (!string.IsNullOrEmpty(mountPoint))
                    {
                        System.Console.WriteLine($"[Install] ISO Mounted at: {mountPoint}");
                        targetPath = mountPoint;
                    }
                    else
                    {
                        return BadRequest("Failed to mount ISO image on Windows.");
                    }
                }
                else
                {
                    return BadRequest("ISO mounting and installation is not supported in Docker/Headless mode. Please install manually.");
                }
            }

            // Common Installer Discovery (Fuzzy + Depth 1)
            var installerPath = FindInstaller(targetPath, game.Title);
            if (installerPath != null)
            {
                return LaunchInstaller(installerPath);
            }

            return BadRequest($"No valid installer found in: {targetPath}");
        }

        [HttpPost("{id}/play")]
        public async Task<ActionResult> Play(int id)
        {
            System.Console.WriteLine($"[API] Play Request Received for Game ID: {id}");
            var game = await _repository.GetByIdAsync(id);
            if (game == null) 
            {
                System.Console.WriteLine($"[API] Game ID {id} not found.");
                return NotFound("Game not found");
            }

            System.Console.WriteLine($"[API] Launching Game: {game.Title} (SteamID: {game.SteamId})");

            try
            {
                await _launcherService.LaunchGameAsync(game);
                return Ok(new { message = $"Launching {game.Title}..." });
            }
            catch (System.Exception ex)
            {
                System.Console.WriteLine($"[Play] Error: {ex.Message}");
                return BadRequest(new { error = ex.Message });
            }
        }

        private string? FindInstaller(string rootPath, string? gameTitleHint = null)
        {
            if (string.IsNullOrEmpty(rootPath)) return null;

            // 1. If path is already an .exe, use it
            if (System.IO.File.Exists(rootPath) && System.IO.Path.GetExtension(rootPath).Equals(".exe", StringComparison.OrdinalIgnoreCase))
            {
                return rootPath;
            }

            // 2. If directory, look for patterns
            if (System.IO.Directory.Exists(rootPath))
            {
                try
                {
                    var patterns = new[] { "setup*.exe", "install*.exe", "installer.exe", "game.exe" };
                    var candidates = new List<string>();

                    // Depth 0: Root
                    foreach (var pattern in patterns)
                        candidates.AddRange(System.IO.Directory.GetFiles(rootPath, pattern, System.IO.SearchOption.TopDirectoryOnly));

                    // Depth 1: Immediate subdirs
                    var subDirs = System.IO.Directory.GetDirectories(rootPath);
                    foreach (var subDir in subDirs)
                    {
                        foreach (var pattern in patterns)
                            candidates.AddRange(System.IO.Directory.GetFiles(subDir, pattern, System.IO.SearchOption.TopDirectoryOnly));
                    }

                    if (!candidates.Any()) return null;

                    // Prioritization logic:
                    // 1. Exact match if possible (or containing game title)
                    if (!string.IsNullOrEmpty(gameTitleHint))
                    {
                        var bestMatch = candidates.FirstOrDefault(c => 
                            System.IO.Path.GetFileNameWithoutExtension(c).Contains(gameTitleHint, StringComparison.OrdinalIgnoreCase));
                        if (bestMatch != null) return bestMatch;
                    }

                    // 2. Smart default prioritized names
                    var defaults = new[] { "setup.exe", "install.exe", "installer.exe" };
                    foreach (var def in defaults)
                    {
                        var match = candidates.FirstOrDefault(c => System.IO.Path.GetFileName(c).Equals(def, StringComparison.OrdinalIgnoreCase));
                        if (match != null) return match;
                    }

                    // 3. Fallback: Heaviest file (usually the main installer)
                    return candidates.OrderByDescending(c => new System.IO.FileInfo(c).Length).FirstOrDefault();
                }
                catch { return null; }
            }

            return null;
        }



        private ActionResult LaunchInstaller(string path)
        {
            try 
            {
                var startInfo = new System.Diagnostics.ProcessStartInfo();
                
                // GOG / Inno Setup Detection
                var fileName = System.IO.Path.GetFileName(path).ToLower();
                var isGog = fileName.StartsWith("setup_") || fileName.StartsWith("setup.exe");
                var silentArgs = "/VERYSILENT /SUPPRESSMSGBOXES /NORESTART /SP-";

                if (RuntimeInformation.IsOSPlatform(OSPlatform.Windows))
                {
                    startInfo.FileName = path;
                    startInfo.WorkingDirectory = System.IO.Path.GetDirectoryName(path);
                    startInfo.UseShellExecute = true; // Use shell for .exe on Windows
                    
                    if (isGog) 
                    {
                        System.Console.WriteLine("[Install] Detected likely GOG/Inno Installer. Applying Silent Flags.");
                        startInfo.Arguments = silentArgs;
                    }
                }
                else if (RuntimeInformation.IsOSPlatform(OSPlatform.OSX))
                {
                    // macOS -> Use 'open' command which delegates to system association (Crossover, Wine, etc.)
                    System.Console.WriteLine($"[Install] macOS detected. Delegating to 'open': {path}");
                    startInfo.FileName = "open";
                    startInfo.WorkingDirectory = System.IO.Path.GetDirectoryName(path);
                    
                    // Arguments for open: just the file path.
                    // Note: 'open' doesn't easily accept args for the target executable unless using --args (and complex escaping)
                    // For now, we launch the installer. Silent flags might propagate if configured in 'open', but standard 'open file.exe' is safest.
                    startInfo.Arguments = $"\"{path}\"";
                    startInfo.UseShellExecute = false;
                }
                else
                {
                    // Linux/Docker -> Try Wine
                    System.Console.WriteLine($"[Install] Linux/Docker detected. Attempting to launch via Wine: {path}");
                    
                    startInfo.FileName = "wine";
                    startInfo.WorkingDirectory = System.IO.Path.GetDirectoryName(path);
                    
                    var wineArgs = $"\"{path}\"";
                    if (isGog) wineArgs += $" {silentArgs}";
                    
                    startInfo.Arguments = wineArgs;
                    startInfo.UseShellExecute = false; 
                }

                System.Diagnostics.Process.Start(startInfo);
                return Ok(new { message = $"Installer launched: {System.IO.Path.GetFileName(path)}" });
            }
            catch (System.Exception ex)
            {
                System.Console.WriteLine($"[Install] Launch error: {ex.Message}");
                return StatusCode(500, $"Error launching installer: {ex.Message}");
            }
        }

        [HttpDelete("all")]
        public async Task<ActionResult> DeleteAll()
        {
            await _repository.DeleteAllAsync();
            return NoContent();
        }

        private bool IsPathInstallable(string? path)
        {
            if (string.IsNullOrEmpty(path)) return false;

            // 1. Handle file directly (Archive or ISO or EXE)
            if (System.IO.File.Exists(path))
            {
                var ext = System.IO.Path.GetExtension(path).ToLower();
                if (ext == ".exe" || ext == ".iso") return true;
                if (_archiveService.IsArchive(path)) return true;
                return false;
            }

            // 2. Handle directory via FindInstaller
            return FindInstaller(path) != null;
        }

        private async Task<string?> MountIsoMacOS(string isoPath)
        {
            try
            {
                var process = new System.Diagnostics.Process
                {
                    StartInfo = new System.Diagnostics.ProcessStartInfo
                    {
                        FileName = "hdiutil",
                        Arguments = $"mount \"{isoPath}\"",
                        RedirectStandardOutput = true,
                        RedirectStandardError = true,
                        UseShellExecute = false,
                        CreateNoWindow = true
                    }
                };

                process.Start();
                string output = await process.StandardOutput.ReadToEndAsync();
                await process.WaitForExitAsync();

                if (process.ExitCode == 0)
                {
                    // Output format: /dev/diskXsY   Apple_HFS   /Volumes/VolumeName
                    // We need to capture the /Volumes/... part
                    var match = Regex.Match(output, @"(/Volumes/.+)");
                    if (match.Success)
                    {
                        return match.Groups[1].Value.Trim();
                    }
                }
                else
                {
                     string error = await process.StandardError.ReadToEndAsync();
                     System.Console.WriteLine($"[Mount] Error: {error}");
                }
            }
            catch (System.Exception ex)
            {
                System.Console.WriteLine($"[Mount] Exception: {ex.Message}");
            }
            return null;
        }

        private async Task<string?> MountIsoWindows(string isoPath)
        {
            try
            {
                // PowerShell command to mount and get the drive letter
                // Mount-DiskImage -ImagePath "C:\path\to.iso" -PassThru | Get-Volume | Select-Object -ExpandProperty DriveLetter
                var psCommand = $"Mount-DiskImage -ImagePath \"{isoPath}\" -PassThru | Get-Volume | Select-Object -ExpandProperty DriveLetter";
                
                var process = new System.Diagnostics.Process
                {
                    StartInfo = new System.Diagnostics.ProcessStartInfo
                    {
                        FileName = "powershell",
                        Arguments = $"-Command \"{psCommand}\"",
                        RedirectStandardOutput = true,
                        RedirectStandardError = true,
                        UseShellExecute = false,
                        CreateNoWindow = true
                    }
                };

                process.Start();
                string output = await process.StandardOutput.ReadToEndAsync();
                await process.WaitForExitAsync();

                if (process.ExitCode == 0 && !string.IsNullOrWhiteSpace(output))
                {
                    var driveLetter = output.Trim().Substring(0, 1);
                    return $"{driveLetter}:\\";
                }
                else
                {
                     string error = await process.StandardError.ReadToEndAsync();
                     System.Console.WriteLine($"[Mount-Win] Error: {error}");
                }
            }
            catch (System.Exception ex)
            {
                System.Console.WriteLine($"[Mount-Win] Exception: {ex.Message}");
            }
            return null;
        }

        private string? FindUninstaller(string? rootPath)
        {
            if (string.IsNullOrEmpty(rootPath) || !System.IO.Directory.Exists(rootPath)) return null;

            try
            {
                var patterns = new[] { "unins*.exe", "uninstall.exe", "*uninstall*.exe", "setup.exe" }; // setup.exe is sometimes also the uninstaller
                var candidates = new List<string>();

                foreach (var pattern in patterns)
                {
                    candidates.AddRange(System.IO.Directory.GetFiles(rootPath, pattern, System.IO.SearchOption.TopDirectoryOnly));
                }

                // Look in common subfolders
                var subDirs = new[] { "bin", "bin64", "tools" };
                foreach (var sub in subDirs)
                {
                    var subPath = System.IO.Path.Combine(rootPath, sub);
                    if (System.IO.Directory.Exists(subPath))
                    {
                        foreach (var pattern in patterns)
                            candidates.AddRange(System.IO.Directory.GetFiles(subPath, pattern, System.IO.SearchOption.TopDirectoryOnly));
                    }
                }

                if (!candidates.Any()) return null;

                // Prioritize "unins" followed by "uninstall"
                var prioritized = candidates
                    .OrderBy(c => {
                        var name = System.IO.Path.GetFileName(c).ToLower();
                        if (name.StartsWith("unins")) return 0;
                        if (name.Contains("uninstall")) return 1;
                        return 2;
                    })
                    .ThenByDescending(c => new System.IO.FileInfo(c).Length)
                    .FirstOrDefault();

                return prioritized;
            }
            catch { return null; }
        }

        private string? FindDownloadFolder(string gameTitle, string? gamePath)
        {
            try
            {
                var settings = _configService.LoadMediaSettings();
                var downloadRoot = settings.DownloadPath;

                if (string.IsNullOrEmpty(downloadRoot) || !System.IO.Directory.Exists(downloadRoot)) return null;

                // Look for directories in downloadRoot (Level 1 and Level 2)
                var level1Dirs = System.IO.Directory.GetDirectories(downloadRoot);
                var allDirs = new List<string>(level1Dirs);
                
                foreach (var l1 in level1Dirs)
                {
                    try { allDirs.AddRange(System.IO.Directory.GetDirectories(l1)); } catch { }
                }

                // Strategy 1: Match by immediate parent folder name of game.Path
                if (!string.IsNullOrEmpty(gamePath))
                {
                    var parentDir = System.IO.Path.GetDirectoryName(gamePath);
                    if (!string.IsNullOrEmpty(parentDir))
                    {
                        var folderName = new System.IO.DirectoryInfo(parentDir).Name;
                        var match = allDirs.FirstOrDefault(d => 
                            string.Equals(System.IO.Path.GetFileName(d), folderName, StringComparison.OrdinalIgnoreCase));
                        
                        if (match != null) return match;
                    }
                }

                // Strategy 2: Match by game title
                var titleMatch = allDirs.FirstOrDefault(d => 
                    System.IO.Path.GetFileName(d).Contains(gameTitle, StringComparison.OrdinalIgnoreCase));
                
                if (titleMatch != null) return titleMatch;

                // Strategy 3: Fuzzy match (alphanumeric only)
                var cleanTitle = System.Text.RegularExpressions.Regex.Replace(gameTitle, @"[^a-zA-Z0-9]", "");
                if (cleanTitle.Length > 2)
                {
                     var fuzzyMatch = allDirs.FirstOrDefault(d => {
                         var cleanDirName = System.Text.RegularExpressions.Regex.Replace(System.IO.Path.GetFileName(d), @"[^a-zA-Z0-9]", "");
                         return cleanDirName.Contains(cleanTitle, StringComparison.OrdinalIgnoreCase);
                     });
                     
                     if (fuzzyMatch != null) return fuzzyMatch;
                }

                return null;
            }
            catch { return null; }
        }
    }
}
