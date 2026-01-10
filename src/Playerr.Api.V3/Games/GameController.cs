using System;
using Microsoft.AspNetCore.Mvc;
using Playerr.Core.Games;
using Playerr.Core.MetadataSource;
using System.Collections.Generic;
using System.Threading.Tasks;
using System.Linq;
using System.IO;
using System.Runtime.InteropServices;
using System.Text.RegularExpressions;

namespace Playerr.Api.V3.Games
{
    [ApiController]
    [Route("api/v3/[controller]")]
    public class GameController : ControllerBase
    {
        private readonly IGameRepository _repository;
        private readonly IGameMetadataServiceFactory _metadataServiceFactory;
        private readonly Playerr.Core.IO.IArchiveService _archiveService;

        public GameController(IGameRepository repository, IGameMetadataServiceFactory metadataServiceFactory, Playerr.Core.IO.IArchiveService archiveService)
        {
            _repository = repository;
            _metadataServiceFactory = metadataServiceFactory;
            _archiveService = archiveService;
        }

        [HttpGet]
        public async Task<ActionResult<List<Game>>> GetAll()
        {
            var games = await _repository.GetAllAsync();
            return Ok(games);
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

            return Ok(game);
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
        public async Task<ActionResult> Delete(int id)
        {
            var removed = await _repository.DeleteAsync(id);
            if (!removed)
            {
                return NotFound();
            }

            return NoContent();
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
    }
}
