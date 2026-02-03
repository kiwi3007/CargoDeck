using System;
using System.Collections.Generic;
using System.IO;
using System.Linq;
using System.Threading.Tasks;
using System.Text.RegularExpressions;
using Playerr.Core.Configuration;
using Playerr.Core.MetadataSource;
using System.Diagnostics.CodeAnalysis;
using System.Globalization;

namespace Playerr.Core.Games
{
    [SuppressMessage("Microsoft.Performance", "CA1822:MarkMembersAsStatic")]
    [SuppressMessage("Microsoft.Globalization", "CA1303:Do not pass literals as localized parameters")]
    [SuppressMessage("Microsoft.Globalization", "CA1304:SpecifyCultureInfo")]
    [SuppressMessage("Microsoft.Globalization", "CA1307:SpecifyStringComparison")]
    [SuppressMessage("Microsoft.Globalization", "CA1311:SpecifyCultureForToLowerAndToUpper")]
    [SuppressMessage("Microsoft.Design", "CA1001:TypesThatOwnDisposableFieldsShouldBeDisposable")]
    [SuppressMessage("Microsoft.Design", "CA1031:DoNotCatchGeneralExceptionTypes")]
    [SuppressMessage("Microsoft.Performance", "CA1860:AvoidUsingAnyWhenUseCount")]
    [SuppressMessage("Microsoft.Reliability", "CA2007:DoNotDirectlyAwaitATask")]
    [SuppressMessage("Microsoft.Design", "CA1003:UseGenericEventHandlerInstances")]
    public class MediaScannerService
    {
        private readonly ConfigurationService _configService;
        private readonly IGameMetadataServiceFactory _metadataServiceFactory;
        private readonly IGameRepository _gameRepository;
        
        // Scan State Tracking
        public bool IsScanning { get; private set; }
        public string? LastGameFound { get; private set; }
        public int GamesAddedCount { get; private set; }
        private System.Threading.CancellationTokenSource? _scanCts;

        public delegate void GameAddedHandler(Game game);
        public event GameAddedHandler? OnGameAdded;
        public event Action? OnScanStarted;

        private static readonly HashSet<string> _noiseWords = new HashSet<string>(StringComparer.OrdinalIgnoreCase) {
            "setup", "install", "installer", "gog", "repack", "fitgirl", "dodi", "cracked", 
            "unpacked", "steamrip", "portable", "multi10", "multi5", "multi2", "v1", "v2",
            "xatab", "codex", "skidrow", "reloaded", "razor1911", "plaza", "cpy", "dlpsgame",
            "nsw2u", "egold", "quacked", "venom", "inc", "rpgonly", "gamesfull", "bitsearch",
            "www", "app", "com", "net", "org", "iso", "bin", "decepticon", "empress", 
            "tenoke", "rune", "goldberg", "ali213", "p2p", "fairlight", "flt", "prophet", "kaos", "elamigos",
            "xyz", "dot", "v0", "v196608", "v65536", "v131072", "dlc", "update", "upd", "collection", "anniversary", "edition",
            "us", "eu", "es", "uk", "asia", "cn", "ru", "gb", "mb", "kb", "romslab", "madloader", "usa", "eur", "jp", "region",
            "eng", "english", "spa", "spanish", "fra", "french", "ger", "german", "ita", "italian", "kor", "korean", "chi", "chinese", "tw", "hk",
            "rpgarchive", "gamesmega", "nxdump", "nx", "switch", "game",
            "opoisso893", "cyb1k", "pppwn", "pppwngo", "goldhen", "ps4", "ps5", "playstation", "sony",
            "definitive", "remastered", "remake",
            "nsp", "xci", "nsz", "xcz", "vpk", "pkg", "nla", "zip", "rar", "7z"
        };
        public event Action<int>? OnScanFinished;
        public event Action? OnBatchFinished;

        // 1. Valid Extensions (Whitelist)
        private static readonly Dictionary<string, PlatformRule> _platformRules = new(StringComparer.OrdinalIgnoreCase)
        {
            ["nintendo_switch"] = new() { Extensions = new[] { ".nsp", ".xci", ".nsz", ".xcz" } },
            ["ps4"] = new() { Extensions = new[] { ".pkg" } }, // Removed .bin to avoid exploits/payloads
            ["pc_windows"] = new() { Extensions = new[] { ".iso", ".exe", ".setup" }, IsFolderMode = true },
            ["ps3"] = new() { Extensions = new[] { ".iso", ".pkg", ".bin" }, IsFolderMode = true },
            ["retro_emulation"] = new() { Extensions = new[] { 
                ".iso", ".bin", ".cue", ".chd", ".rvz", ".wbfs", // Disk based
                ".z64", ".n64", ".v64",                         // N64
                ".sfc", ".smc",                                 // SNES
                ".nes",                                         // NES
                ".gb", ".gba", ".gbc",                          // GameBoy
                ".md", ".gen", ".smd",                          // Genesis/MegaDrive
                ".sms", ".gg",                                  // Master System / Game Gear
                ".pce",                                         // PC Engine
                ".zip", ".7z", ".rar"                           // Compressed ROMs
            } },
            ["macos"] = new() { Extensions = new[] { ".dmg", ".pkg", ".app" } },
            ["default"] = new() { Extensions = null, IsFolderMode = false } // Special case: All extensions, File Mode (v0.1.0 behavior)
        };

        private static string[] _allExtensions = _platformRules.Values
            .Where(r => r.Extensions != null)
            .SelectMany(r => r.Extensions)
            .Distinct()
            .Where(ext => !ext.Equals(".bin", StringComparison.OrdinalIgnoreCase) && // Exclude .bin
                          !ext.Equals(".rar", StringComparison.OrdinalIgnoreCase) && // Exclude archives from Auto-Scan (v0.4.7 User Feedback)
                          !ext.Equals(".zip", StringComparison.OrdinalIgnoreCase) &&
                          !ext.Equals(".7z", StringComparison.OrdinalIgnoreCase))
            .ToArray();

        // 2. Exclusion Rules (Global Blacklist)
        private static readonly HashSet<string> _globalBlacklist = new(StringComparer.OrdinalIgnoreCase)
        {
            ".nfo", ".txt", ".url", ".website", ".html", ".md",
            ".sfv", ".md5", ".sha1",
            ".jpg", ".png", ".jpeg", ".bmp",
            ".ds_store", ".db",
            ".dll", ".so", ".lib", ".a", ".bin" // Strictly ignore these in folder scans
        };

        private static readonly HashSet<string> _keywordBlacklist = new(StringComparer.OrdinalIgnoreCase)
        {
            "steam_api", "crashpad", "unitycrash", "unins000", "uninstall", "update", "config", "dxsetup", "redist", "vcredist", "fna", "mono", "bios", "firmware", "retroarch", "overlay", "shdr", "slang", "glsl", "cg", "dlc", "update"
        };

        private readonly HashSet<string> _filenameBlacklist = new(StringComparer.OrdinalIgnoreCase)
        {
            "steam_api.dll", "steam_api64.dll", "openvr_api.dll", "nvapi.dll", "nvapi64.dll",
            "d3dcompiler_47.dll", "d3dcompiler_43.dll", "xinput1_3.dll", "xinput9_1_0.dll",
            "msvcp140.dll", "vcruntime140.dll", "unityplayer.dll", "crashpad_handler.exe", "unitycrashhandler64.exe",
            "unins000.exe", "uninstall.exe", "update.exe", "updater.exe", "config.exe", "settings.exe",
            "wmplayer.exe"
        };

        private static readonly HashSet<string> _folderBlacklist = new(StringComparer.OrdinalIgnoreCase)
        {
            "_CommonRedist", "CommonRedist", "Redist", "DirectX", "Support", 
            "Prerequisites", "Launcher", "Ship", "Shipping", 
            "Retail", "x64", "x86", "System", "Binaries", "Engine", "Content", "Asset", "Resource",
            "shadercache", "compatdata", "depotcache", ".steam", ".local", ".cache", "temp", "tmp", "node_modules",
            "windows", "system32", "syswow64", "Microsoft.NET", "Framework", "Framework64", "Internet Explorer", "Accessories", "Windows NT", "INF", "WinSxS", "SysARM32", "Sysnative", "command",
            "retroarch", "autoconfig", "assets", "overlays", "database", "cursors", "cheats", "filters", "libretro", "thumbnails", "config", "remaps", "playlists", "cores", "screenshots",
            "retroarch", "autoconfig", "assets", "overlays", "database", "cursors", "cheats", "filters", "libretro", "thumbnails", "config", "remaps", "playlists", "cores", "screenshots",
            "z:", "d:",
            "Bonus", "Extras", "Soundtrack", "Artbook", "Manuals" // Explicit user requested blacklist for extra content
        };

        private static readonly HashSet<string> _containerNames = new(StringComparer.OrdinalIgnoreCase)
        {
            "common", "steamapps", "games", "juegos", "roms", "emulators", "others", "downloads", "library", "biblioteca", "collection"
        };

        private static readonly HashSet<string> _noClusterExtensions = new(StringComparer.OrdinalIgnoreCase)
        {
            ".iso", ".bin", ".cue", ".pkg",
            ".nsp", ".xci", ".nsz", ".xcz",
            ".z64", ".n64", ".v64",
            ".sfc", ".smc", ".nes",
            ".gb", ".gbc", ".gba",
            ".md", ".gen", ".smd", ".sms", ".gg",
            ".pce"
        };

        [SuppressMessage("Microsoft.Performance", "CA1852:SealInternalTypes")]
        private class PlatformRule
        {
            public string[] Extensions { get; set; } = Array.Empty<string>();
            public bool IsFolderMode { get; set; }
        }

        [SuppressMessage("Microsoft.Performance", "CA1852:SealInternalTypes")]
        private class GameCandidate
        {
            public string Title { get; set; } = string.Empty;
            public string Path { get; set; } = string.Empty;
            public string? PlatformKey { get; set; }
            public string? Serial { get; set; }
            public string? ExecutablePath { get; set; }
            public bool IsInstaller { get; set; }
            public bool IsExternal { get; set; }
        }

        public MediaScannerService(
            ConfigurationService configService, 
            IGameMetadataServiceFactory metadataServiceFactory,
            IGameRepository gameRepository)
        {
            _configService = configService;
            _metadataServiceFactory = metadataServiceFactory;
            _gameRepository = gameRepository;
        }

        public void StopScan()
        {
            if (IsScanning && _scanCts != null)
            {
                Log("Cancellation requested by user.");
                _scanCts.Cancel();
            }
        }

        public async Task CleanLibraryAsync()
        {
            Log("Cleaning library...");
            await _gameRepository.DeleteAllAsync();
            GamesAddedCount = 0;
            LastGameFound = null;
            Log("Library cleaned.");
        }

        public async Task<int> ScanAsync(string? overridePath = null, string? overridePlatform = null)
        {
            if (IsScanning)
            {
                Console.WriteLine("Scan skipped: Another scan is already in progress.");
                return 0;
            }

            var settings = _configService.LoadMediaSettings();
            var folderPath = overridePath ?? settings.FolderPath;
            var platformKey = overridePlatform ?? (string.IsNullOrWhiteSpace(settings.Platform) ? "default" : settings.Platform);

            if (string.IsNullOrEmpty(folderPath) || !Directory.Exists(folderPath))
            {
                var skipMsg = $"Media scanner skip: Path not configured or doesn't exist: '{folderPath}'";
                if (folderPath != null && folderPath.StartsWith("smb://", StringComparison.OrdinalIgnoreCase))
                {
                    skipMsg = "The address starts with 'smb://'. This is a protocol, not a path. Please mount the drive in Finder and use the path in '/Volumes/'.";
                }
                Console.WriteLine(skipMsg);
                Log(skipMsg);
                return 0;
            }

            if (!_platformRules.TryGetValue(platformKey, out var rule))
            {
                Console.WriteLine($"Unknown platform '{platformKey}', defaulting to standard rules.");
                rule = _platformRules["default"];
            }

            var logMsg = $"Starting scan. Platform: {platformKey}, FolderMode: {rule.IsFolderMode}, Path: {folderPath}";
            Console.WriteLine(logMsg);
            Log(logMsg);

            int gamesAdded = 0;
            var existingGames = await _gameRepository.GetAllAsync();
            var metadataService = _metadataServiceFactory.CreateService();

            OnScanStarted?.Invoke();
            IsScanning = true;
            GamesAddedCount = 0;
            LastGameFound = null;
            _scanCts = new System.Threading.CancellationTokenSource();

            try
            {
                if (rule.IsFolderMode)
                {
                    gamesAdded = await ScanFolderModeAsync(folderPath, rule, existingGames, platformKey, metadataService, _scanCts.Token);
                    
                    // FALLBACK: If folder mode found 0 games, try file mode just in case
                    if (gamesAdded == 0 && !_scanCts.Token.IsCancellationRequested)
                    {
                        Log($"ScanFolderMode found 0 games. Falling back to ScanFileMode for exhaustive search.");
                        gamesAdded = await ScanFileModeAsync(folderPath, rule, existingGames, platformKey, metadataService, _scanCts.Token);
                    }
                }
                else
                {
                    gamesAdded = await ScanFileModeAsync(folderPath, rule, existingGames, platformKey, metadataService, _scanCts.Token);
                }

                // NEW: Wine/Whisky Integration (External Library)
                // We scan this separately if configured.
                // NEW: Wine/Whisky Integration (External Library)
                // We scan this separately if configured.
                var winePath = settings.WinePrefixPath;
                // Only scan external libraries if we are doing a FULL library scan (no override path)
                if (string.IsNullOrEmpty(overridePath) && !string.IsNullOrEmpty(winePath) && Directory.Exists(winePath))
                {
                    Log($"Scanning Wine/Whisky External Path: {winePath}");
                    var externalGames = await ScanExternalLibraryAsync(winePath, existingGames, metadataService, _scanCts.Token);
                    gamesAdded += externalGames;
                }
            }
            catch (OperationCanceledException)
            {
                Log("Scan was cancelled by user.");
            }
            catch (Exception ex)
            {
                Log($"Error during scan: {ex.Message}");
            }
            finally
            {
                IsScanning = false;
                _scanCts?.Dispose();
                _scanCts = null;
                GamesAddedCount = gamesAdded;
                Log($"Scan Finished/Stopped. Added: {gamesAdded}");
                OnScanFinished?.Invoke(gamesAdded);
            }

            return gamesAdded;
        }

        private async Task<int> ScanFolderModeAsync(string rootPath, PlatformRule rule, List<Game> existingGames, string platformKey, GameMetadataService metadataService, System.Threading.CancellationToken ct)
        {
            var candidates = new List<GameCandidate>();
            string[] directories;
            try
            {
                directories = Directory.GetDirectories(rootPath);
            }
            catch (Exception ex)
            {
                Log($"Error accessing root path: {ex.Message}");
                return 0;
            }

            foreach (var dir in directories)
            {
                ct.ThrowIfCancellationRequested();
                var folderName = Path.GetFileName(dir);
                
                // Advanced Scanner Logic V2: Find the best executable in the folder structure
                var (bestExePath, isInstaller) = FindBestExecutable(dir, rule.Extensions, isExternal: false);
                
                if (!string.IsNullOrEmpty(bestExePath))
                {
                    var (cleanName, serial) = CleanGameTitle(folderName);
                    
                    // Check if game exists
                    if (!existingGames.Any(g => g.Title.Equals(cleanName, StringComparison.OrdinalIgnoreCase)))
                    {
                        var candidate = new GameCandidate 
                        { 
                            Title = cleanName, 
                            Path = dir, // Main Path is still the folder
                            PlatformKey = platformKey, 
                            Serial = serial,
                            ExecutablePath = bestExePath,
                            IsInstaller = isInstaller
                        };
                        
                        // New in V2: Store extra metadata
                        // We store these in a temporary way or pass them to ProcessPotentialGame
                        // Since GameCandidate is internal, we can extend it or use a Dictionary/Tuple
                        // For now, let's just make sure ProcessPotentialGame can handle it.
                        // Wait, GameCandidate doesn't have ExecutablePath. Let's add it to the internal class first?
                        // Or just modify ProcessPotentialGame signature.
                        
                        // Let's modify the candidate list processing to handle this.
                        // Actually, I can't easily modify GameCandidate definition without another tool call. 
                        // I will update ProcessPotentialGame to take optional executablePath and status.
                        
                       candidates.Add(candidate);
                       // Quick hack: Store the detected executable path in a temporary lookup if needed, 
                       // but better design is to update GameCandidate. 
                       // Since I cannot change GameCandidate class in this block easily without hitting the whole file,
                       // I will assume I can update the ProcessCandidatesBatchAsync to re-scan or pass data.
                       // Use a distinct request for GameCandidate update? 
                       // No, I can't.
                       // I will just use a Tuple list for this method instead of GameCandidate class, OR just rely on ProcessPotentialGame to "re-find" it?
                       // "Re-finding" is expensive.
                       // I will change candidates to `List<(GameCandidate Candidate, string ExePath, bool IsInstaller)>` locally?
                       // No, that breaks `ProcessCandidatesBatchAsync`.
                       
                       // OPTION: I will inject the data into the object via reflection or just use the Path property intelligently?
                       // No. Let's look at ScanFileModeAsync.
                       
                       // CORRECT APPROACH: I must assume I can modify GameCandidate in the loop/scope if I had access to it.
                       // But I only replaced specific methods. GameCandidate is defined upstream.
                       // I will use a parallel dictionary to store the `ExecutablePath` and `IsInstaller` for this batch.
                       // _candidateMetadata[candidate] = (bestExePath, isInstaller);
                    }
                    else
                    {
                        Log($"Game already exists in DB (Skipping collect): {cleanName}");
                    }
                }
            }

            // We need to pass the executable info to the processor. 
            // Since I cannot easily change the signature of `ProcessCandidatesBatchAsync` in this partial edit without risking breaking other calls,
            // I will do the actual "FindBestExecutable" INSIDE `ProcessPotentialGame` or just before calling it?
            // Doing it inside `ProcessPotentialGame` is cleaner for the architecture if we consider "Path" as the source of truth.
            // BUT `ProcessPotentialGame` is called for both File and Folder mode.
            // In File Mode, Path IS the executable. In Folder Mode, Path is the folder.
            
            // Let's UPDATE `ProcessCandidatesBatchAsync` to re-evaluate or accept metadata.
            // Actually, I am replacing the whole block of methods. I can change `ProcessCandidatesBatchAsync` signature!
            
            return await ProcessCandidatesBatchAsync(candidates, existingGames, metadataService, ct);
        }

        // Updated Helper for V2 Scanner
        // Returns: (Path to best executable, IsInstaller)
        private (string? Path, bool IsInstaller) FindBestExecutable(string folderPath, string[]? allowedExtensions, bool isExternal = false)
        {
            try
            {
                var root = new DirectoryInfo(folderPath);
                if (!root.Exists) return (null, false);

                var candidates = new List<(FileInfo File, int Score, bool IsInstaller)>();
                
                // Recursive search with depth limit (e.g. 3-4 levels) to avoid scanning too deep
                // And explicitly skipping blacklist folders
                var allFiles = GetFilesSafe(root, 0, isExternal ? 5 : 3); 

                foreach (var file in allFiles)
                {
                    if (allowedExtensions != null && !allowedExtensions.Contains(file.Extension, StringComparer.OrdinalIgnoreCase)) continue;
                    if (_globalBlacklist.Contains(file.Extension)) continue;

                    // SAFETY: Never treat archives as executables in File Mode or FindBestExecutable unless assumed Retro?
                    // Actually, for Retro, the extension MUST be in allowedExtensions.
                    // If allowedExtensions is NULL (Default mode), we MUST exclude archives to prevent "bonus.rar" from becoming the game exe.
                    if (allowedExtensions == null && (file.Extension.Equals(".rar", StringComparison.OrdinalIgnoreCase) || 
                                                      file.Extension.Equals(".zip", StringComparison.OrdinalIgnoreCase) || 
                                                      file.Extension.Equals(".7z", StringComparison.OrdinalIgnoreCase)))
                    {
                         continue;
                    }

                    int score = 0;
                    bool isInstaller = false;
                    string name = Path.GetFileNameWithoutExtension(file.Name).ToLowerInvariant();
                    if (string.IsNullOrEmpty(name)) name = file.Name.ToLowerInvariant(); // Support extensionless files (v0.4.2)
                    string folderName = file.Directory?.Name.ToLowerInvariant() ?? "";
                    
                    // --- SCORING RULES (v0.4.2 Winner Takes All) ---
                    
                    // 1. Blacklist Filtering (TAREA 1)
                    if (IsBlacklistedFile(file.Name, isExternal)) continue;

                    // 2. Name Match (+100) (TAREA 3)
                    var rootFolderName = root.Name.ToLowerInvariant();
                    if (name == rootFolderName) score += 100;
                    else if (name.Replace(" ", "").Replace("-", "") == rootFolderName.Replace(" ", "").Replace("-", "")) score += 90;
                    else if (rootFolderName.Contains(name) && name.Length > 4) score += 30;

                    // 3. Priority Names (+50) (TAREA 2 & 3)
                    if (file.Name.Equals("AppRun", StringComparison.OrdinalIgnoreCase) || 
                        file.Name.Equals("Start.sh", StringComparison.OrdinalIgnoreCase))
                    {
                        score += 50;
                    }

                    // 4. Installer/Config Penalty (-50) OR Bonus (+100) (TAREA 3 refined)
                    // Check if current folder or parent indicates a Repack/Installer source
                    bool isInstallerFriendlyContext = rootFolderName.Contains("repack") || 
                                                      rootFolderName.Contains("installer") || 
                                                      rootFolderName.Contains("setup") ||
                                                      rootFolderName.Contains("codex") || 
                                                      rootFolderName.Contains("plaza") || 
                                                      rootFolderName.Contains("rune") || 
                                                      rootFolderName.Contains("kaos") || 
                                                      rootFolderName.Contains("fitgirl") || 
                                                      rootFolderName.Contains("dodi") || 
                                                      rootFolderName.Contains("elamigos") || 
                                                      rootFolderName.Contains("gog") ||
                                                      rootFolderName.Contains("flt") ||
                                                      rootFolderName.Contains("tenoke") ||
                                                      rootFolderName.Contains("skidrow");

                    if (name.Contains("launch") || name.Contains("settings") || name.Contains("server") || 
                        name.Contains("config") || name.Contains("setup") || name.Contains("install") || name.Contains("update"))
                    {
                        if ((name.Contains("setup") || name.Contains("install") || name.Contains("update")) && isInstallerFriendlyContext)
                        {
                            // In a Repack context, the installer IS the game
                            score += 100;
                            isInstaller = true;
                        }
                        else
                        {
                            score -= 50;
                            if (name.Contains("setup") || name.Contains("install")) isInstaller = true;
                        }
                    }

                    // 5. Folder Location Bonus
            if (folderName == "binaries" || folderName == "win64" || folderName == "release" || folderName == "shipping" || folderName == "retail") score += 25;
            
            // 6. Native Linux executable bonus (on Linux systems)
            if (System.Runtime.InteropServices.RuntimeInformation.IsOSPlatform(System.Runtime.InteropServices.OSPlatform.Linux) && string.IsNullOrEmpty(file.Extension))
            {
                score += 10;
            }

            // 7. Archive Penalty (v0.4.7 Fix): Penalize archives massively ALWAYS
            // This prevents "bonus.rar" from winning against "setup.exe"
            if (file.Extension.Equals(".rar", StringComparison.OrdinalIgnoreCase) || 
                file.Extension.Equals(".zip", StringComparison.OrdinalIgnoreCase) || 
                file.Extension.Equals(".7z", StringComparison.OrdinalIgnoreCase))
            {
                score -= 5000; // Nuclear Option: Never select an archive in Folder Mode unless it's the ONLY thing there (and score > 0?? No, score will be negative)
            }
            
            candidates.Add((file, score, isInstaller));
                }

                if (!candidates.Any()) return (null, false);

                // 7. Largest File Bonus (+20) (TAREA 3)
                var largestFile = candidates.OrderByDescending(x => x.File.Length).FirstOrDefault();
                for (int i = 0; i < candidates.Count; i++)
                {
                    if (candidates[i].File.FullName == largestFile.File.FullName)
                    {
                        candidates[i] = (candidates[i].File, candidates[i].Score + 20, candidates[i].IsInstaller);
                    }
                }

                // Final Selection
                var winner = candidates.OrderByDescending(x => x.Score).FirstOrDefault();
                
                // --- Linux Executable Intelligence (v0.4.4) ---
                if (System.Runtime.InteropServices.RuntimeInformation.IsOSPlatform(System.Runtime.InteropServices.OSPlatform.Linux) && 
                    winner.File != null && string.IsNullOrEmpty(winner.File.Extension))
                {
                    if (!IsBinaryExecutable(winner.File.FullName))
                    {
                        Log($"[Scanner] Skipping non-binary Linux candidate: {winner.File.Name}");
                        return (null, false);
                    }
                }

                return (winner.File.FullName, winner.IsInstaller);
            }
            catch (Exception ex)
            {
                Log($"Error discovering executable in {folderPath}: {ex.Message}");
                return (null, false);
            }
        }

        private bool IsBinaryExecutable(string filePath)
        {
            try
            {
                using (var fs = new FileStream(filePath, FileMode.Open, FileAccess.Read))
                {
                    if (fs.Length < 4) return false;
                    var buffer = new byte[4];
                    fs.Read(buffer, 0, 4);

                    // 1. ELF Header: 0x7F 'E' 'L' 'F'
                    if (buffer[0] == 0x7F && buffer[1] == 0x45 && buffer[2] == 0x4C && buffer[3] == 0x46) return true;

                    // 2. Shebang: #!
                    if (buffer[0] == 0x23 && buffer[1] == 0x21) return true;
                }
            }
            catch { }
            return false;
        }

        private List<FileInfo> GetFilesSafe(DirectoryInfo root, int currentDepth, int maxDepth)
        {
            var results = new List<FileInfo>();
            
            // Intelligence: If we are in a "Bridge" folder (common, SteamApps), reset depth to allow finding games inside
            if (root.Name.Equals("common", StringComparison.OrdinalIgnoreCase) || root.Name.Equals("SteamApps", StringComparison.OrdinalIgnoreCase))
            {
                maxDepth += 2; // Allow 2 more levels for Steam nested games
            }

            if (currentDepth > maxDepth) return results;

            // blacklist folders
            if (root.Name.StartsWith(".") || _folderBlacklist.Contains(root.Name) || IsMetadataSubfolder(root.Name))
            {
                return results;
            }

            try
            {
                results.AddRange(root.GetFiles());
                foreach (var dir in root.GetDirectories())
                {
                    results.AddRange(GetFilesSafe(dir, currentDepth + 1, maxDepth));
                }
            }
            catch { } // Ignore permission errors

            return results;
        }

        private async Task<int> ScanFileModeAsync(string rootPath, PlatformRule rule, List<Game> existingGames, string platformKey, GameMetadataService metadataService, System.Threading.CancellationToken ct)
        {
            var candidates = new List<GameCandidate>();
            var extensionsToUse = platformKey == "default" ? _allExtensions : rule.Extensions;
            Log($"Scanning (Fast FileMode) Root: {rootPath}. Valid Extensions: {(extensionsToUse != null ? string.Join(", ", extensionsToUse) : "ALL")}");

            try
            {
                var validFilesByFolder = new Dictionary<string, List<string>>();
                
                // Fast Hierarchical Discovery (v0.4.2)
                // Instead of SearchOption.AllDirectories (Slow), we recurse manually and skip blacklisted branches
                DiscoverFilesHierarchical(new DirectoryInfo(rootPath), extensionsToUse, validFilesByFolder, ct, 0, 4); 

                Log($"[FileMode] Discovery phase finished. Found {validFilesByFolder.Count} candidate folders items. Applying clustering...");

                foreach (var folderEntry in validFilesByFolder)
                {
                    ct.ThrowIfCancellationRequested();
                    var folderPath = folderEntry.Key;
                    var filePaths = folderEntry.Value;

                    var folderName = new DirectoryInfo(folderPath).Name;
                    bool isContainer = _containerNames.Contains(folderName);
                    bool isConsole = platformKey != "pc_windows" && platformKey != "macos" && platformKey != "default";

                    // TAREA 2: Skip clustering for specific extensions (ROMs, ISOs, PKG, etc.)
                    // Check if ANY file in the folder has an extension that skips clustering
                    bool hasNoClusterExtension = filePaths.Any(f => _noClusterExtensions.Contains(Path.GetExtension(f)));

                    // LOGIC SWITCH: If it's a container, a console platform, or has "No-Cluster" extensions, DO NOT cluster.
                    if (isContainer || isConsole || hasNoClusterExtension)
                    {
                        foreach (var filePath in filePaths)
                        {
                            var rawFileName = Path.GetFileNameWithoutExtension(filePath);
                            if (string.IsNullOrEmpty(rawFileName)) rawFileName = Path.GetFileName(filePath);
                            
                            var (cleanTitle, serial) = CleanGameTitle(rawFileName);

                            if (!existingGames.Any(g => g.Title.Equals(cleanTitle, StringComparison.OrdinalIgnoreCase)))
                            {
                                var candidate = new GameCandidate 
                                { 
                                    Title = cleanTitle, 
                                    Path = filePath, // For ROMs/ISOs, Path is the file itself
                                    PlatformKey = platformKey == "default" ? GetPlatformFromExtension(Path.GetExtension(filePath)) : platformKey, 
                                    Serial = serial,
                                    ExecutablePath = filePath,
                                    IsInstaller = false
                                };

                                // TAREA: Ambiguity Resolution for ISO/BIN/PKG in console mode too
                                if (candidate.PlatformKey == "default" || candidate.PlatformKey == "ps4")
                                {
                                    var refined = ResolvePlatformFromSerial(serial ?? "");
                                    if (refined != "default") candidate.PlatformKey = refined;
                                }

                                candidates.Add(candidate);
                            }
                        }
                        continue;
                    }

                    // TAREA 3: PC/Desktop Logic: Only apply clustering if we have executable scripts/binaries
                    // Otherwise treat them as one-file-one-game if they are unique enough (or let the filter handle it)
                    var (bestExePath, isInstaller) = FindBestExecutableInList(folderPath, filePaths);

                    if (!string.IsNullOrEmpty(bestExePath))
                    {
                        var rawFileName = Path.GetFileNameWithoutExtension(bestExePath);
                        if (string.IsNullOrEmpty(rawFileName)) rawFileName = Path.GetFileName(bestExePath);
                        
                        var dirInfo = new DirectoryInfo(folderPath);
                        var rawFolderName = dirInfo.Name;

                        // Intelligence: If folder name is generic (e.g. "bin", "x64", "x64.gog"), climb up
                        var genericFolders = new HashSet<string>(StringComparer.OrdinalIgnoreCase) 
                        { 
                            "bin", "binaries", "data", "system", "system32", "x64", "x86", "win64", "win32", 
                            "release", "retail", "debug", "shipping", "gog", "game", "games"
                        };

                        if ((genericFolders.Contains(rawFolderName) || rawFolderName.EndsWith(".gog", StringComparison.OrdinalIgnoreCase)) && dirInfo.Parent != null)
                        {
                            // Check if parent is also generic (could happen in nested structures like bin/x64)
                             if (genericFolders.Contains(dirInfo.Parent.Name))
                             {
                                 if (dirInfo.Parent.Parent != null) rawFolderName = dirInfo.Parent.Parent.Name;
                             }
                             else
                             {
                                 rawFolderName = dirInfo.Parent.Name;
                             }
                        }
                        
                        // Clean both to compare their "quality"
                        var (cleanFile, _) = CleanGameTitle(rawFileName);
                        var (cleanFolder, _) = CleanGameTitle(rawFolderName);

                        // Selection Heuristic
                        bool isGenericFile = rawFileName.Equals("setup", StringComparison.OrdinalIgnoreCase) || 
                                             rawFileName.Equals("install", StringComparison.OrdinalIgnoreCase) || 
                                             rawFileName.Equals("game", StringComparison.OrdinalIgnoreCase);

                        string selectedName;
                        string source;

                        if (isGenericFile)
                        {
                            selectedName = cleanFolder;
                            source = "Folder (Generic File)";
                        }
                        else if (cleanFolder.Equals(cleanFile, StringComparison.OrdinalIgnoreCase))
                        {
                            selectedName = cleanFolder;
                            source = "Folder (Exact Match)";
                        }
                        else if (cleanFolder.Contains(cleanFile, StringComparison.OrdinalIgnoreCase) && !_noiseWords.Any(nw => cleanFolder.Contains(nw, StringComparison.OrdinalIgnoreCase)))
                        {
                            selectedName = cleanFolder;
                            source = "Folder (Subset Match)";
                        }
                        else if (cleanFile.Contains(cleanFolder, StringComparison.OrdinalIgnoreCase))
                        {
                            // If filename contains folder name, usually filename has MORE noise (e.g. "Streets of Rage 4 g" vs "Streets of Rage 4")
                            // Prefer folder if it's cleaner/shorter
                            if (cleanFile.Length > cleanFolder.Length + 3)
                            {
                                selectedName = cleanFolder;
                                source = "Folder (Cleaner Substring)";
                            }
                            else
                            {
                                selectedName = cleanFile;
                                source = "Filename (Subset Match)";
                            }
                        }
                        else if (cleanFolder.Length > 3 && cleanFile.StartsWith(cleanFolder, StringComparison.OrdinalIgnoreCase))
                        {
                            selectedName = cleanFolder;
                            source = "Folder (Cleaner Prefix)";
                        }
                        else if (cleanFile.Length > 3 && cleanFolder.StartsWith(cleanFile, StringComparison.OrdinalIgnoreCase))
                        {
                            selectedName = cleanFile;
                            source = "Filename (Cleaner Prefix)";
                        }
                        else
                        {
                            selectedName = cleanFolder.Length > 0 ? cleanFolder : cleanFile;
                            source = cleanFolder.Length > 0 ? "Folder (Context Default)" : "Filename (Fallback)";
                        }

                        var finalTitle = selectedName;
                        var (_, serial) = CleanGameTitle(rawFileName); 
                        
                        Log($"[Scanner] Title Resolution: File('{cleanFile}') vs Folder('{cleanFolder}') -> Selected: '{finalTitle}' (Source: {source})");

                        if (!existingGames.Any(g => g.Title.Equals(finalTitle, StringComparison.OrdinalIgnoreCase)))
                        {
                            string finalPlatformKey = platformKey;
                            if (platformKey == "default") 
                            {
                                finalPlatformKey = GetPlatformFromExtension(Path.GetExtension(bestExePath));
                                
                                if (finalPlatformKey == "default" || finalPlatformKey == "ps4")
                                {
                                    var refined = ResolvePlatformFromSerial(serial ?? "");
                                    if (refined != "default") finalPlatformKey = refined;
                                }
                            }

                            candidates.Add(new GameCandidate 
                            { 
                                Title = finalTitle, 
                                Path = folderPath,
                                PlatformKey = finalPlatformKey, 
                                Serial = serial,
                                ExecutablePath = bestExePath,
                                IsInstaller = isInstaller
                            });
                        }
                    }
                }
            }
            catch (Exception ex)
            {
                Log($"Error during Fast FileMode scan: {ex.Message}");
            }

            return await ProcessCandidatesBatchAsync(candidates, existingGames, metadataService, ct);
        }

        private void DiscoverFilesHierarchical(DirectoryInfo root, string[]? allowedExtensions, Dictionary<string, List<string>> results, System.Threading.CancellationToken ct, int currentDepth, int maxDepth)
        {
            ct.ThrowIfCancellationRequested();

            if (!root.Exists) return;

            bool isContainer = _containerNames.Contains(root.Name);

            // Intelligence: If we are in a "Bridge" (Container) folder, allow deeper scan
            if (isContainer)
            {
                maxDepth += 2;
            }

            if (currentDepth > maxDepth) return;

            if (root.Name.StartsWith(".") || _folderBlacklist.Contains(root.Name) || IsMetadataSubfolder(root.Name)) return;

            try
            {
                // If it's a container, we still look for files (e.g., ROMs in 'Juegos')
                // but we give them a chance to be clustered differently.
                var files = root.EnumerateFiles();
                foreach (var file in files)
                {
                    if (IsBlacklistedFile(file.Name, isExternal: false)) continue;

                    if (IsValidFile(file.FullName, allowedExtensions))
                    {
                        if (!results.ContainsKey(root.FullName))
                            results[root.FullName] = new List<string>();

                        results[root.FullName].Add(file.FullName);
                    }
                }

                foreach (var subDir in root.EnumerateDirectories())
                {
                    // TAREA: PS3 Folder Detection
                    if (subDir.Name.Equals("PS3_GAME", StringComparison.OrdinalIgnoreCase))
                    {
                        // If we find PS3_GAME, the root folder is a PS3 game
                        if (!results.ContainsKey(root.FullName))
                            results[root.FullName] = new List<string>();
                        
                        // We mark it by adding the directory itself or a dummy marker if needed
                        // For now, let's treat the folder as the "file" so ScanFileMode treats it as 1-game.
                        if (!results[root.FullName].Contains(subDir.FullName))
                            results[root.FullName].Add(subDir.FullName); 
                        
                        continue; // No need to recurse into PS3_GAME for files
                    }

                    DiscoverFilesHierarchical(subDir, allowedExtensions, results, ct, currentDepth + 1, maxDepth);
                }
            }
            catch { /* Skip permission errors */ }
        }

        // New Helper for clustering in File Mode
        private (string? Path, bool IsInstaller) FindBestExecutableInList(string folderPath, List<string> filePaths)
        {
            var candidates = new List<(string FilePath, int Score, bool IsInstaller)>();
            var root = new DirectoryInfo(folderPath);

            foreach (var filePath in filePaths)
            {
                var file = new FileInfo(filePath);
                int score = 0;
                bool isInstaller = false;
                string name = Path.GetFileNameWithoutExtension(file.Name).ToLowerInvariant();
                if (string.IsNullOrEmpty(name)) name = file.Name.ToLowerInvariant();
                
                // Scoring (Same logic as FindBestExecutable but for a specific list)
                var rootFolderName = root.Name.ToLowerInvariant();
                if (name == rootFolderName) score += 100;
                else if (name.Replace(" ", "").Replace("-", "") == rootFolderName.Replace(" ", "").Replace("-", "")) score += 90;
                else if (rootFolderName.Contains(name) && name.Length > 4) score += 30;

                if (file.Name.Equals("AppRun", StringComparison.OrdinalIgnoreCase) || 
                    file.Name.Equals("Start.sh", StringComparison.OrdinalIgnoreCase)) score += 50;

                if (name.Contains("launch") || name.Contains("settings") || name.Contains("server") || 
                    name.Contains("config") || name.Contains("setup") || name.Contains("install"))
                {
                    score -= 50;
                    if (name.Contains("setup") || name.Contains("install")) isInstaller = true;
                }

                candidates.Add((filePath, score, isInstaller));
            }

            if (!candidates.Any()) return (null, false);

            // TAREA 3: Only cluster if we found typical PC executables/scripts
            // If we only have data files or other extensions, the selection might be weaker
            var winner = candidates.OrderByDescending(c => c.Score).First();
            
            // Check if winner extension is PC-specific for clustering
            var winnerExt = Path.GetExtension(winner.FilePath).ToLowerInvariant();
            bool isPcExec = winnerExt == ".exe" || winnerExt == ".bat" || winnerExt == ".sh" || string.IsNullOrEmpty(winnerExt);
            
            if (!isPcExec)
            {
                // If the "best" file isn't an executable, we might be mis-clustering.
                // However, FindBestExecutableInList is only called if clustering IS ALLOWED.
                // We already filtered No-Cluster extensions in ScanFileModeAsync.
            }

            // Tie-break with size if scores are equal
            var topScorers = candidates.Where(c => c.Score == winner.Score).ToList();
            if (topScorers.Count > 1)
            {
                 winner = topScorers.OrderByDescending(c => new FileInfo(c.FilePath).Length).First();
            }

            return (winner.FilePath, winner.IsInstaller);
        }

        private async Task<int> ScanExternalLibraryAsync(string rootPath, List<Game> existingGames, GameMetadataService metadataService, System.Threading.CancellationToken ct)
        {
            var candidates = new List<GameCandidate>();
            Log($"Starting Hierarchical External Scan: {rootPath}");
            
            await ScanDirectoryHierarchicalAsync(new DirectoryInfo(rootPath), candidates, existingGames, ct);
            
            return await ProcessCandidatesBatchAsync(candidates, existingGames, metadataService, ct);
        }

        private async Task ScanDirectoryHierarchicalAsync(DirectoryInfo dir, List<GameCandidate> candidates, List<Game> existingGames, System.Threading.CancellationToken ct)
        {
            ct.ThrowIfCancellationRequested();

            if (!dir.Exists || IsBlacklistedFolder(dir)) return;
            if (IsMetadataSubfolder(dir.Name)) return;

            // 1. Is this folder a game candidate?
            // Criteria: Not generic AND has a "local" executable.
            bool isGeneric = IsGenericFolderName(dir.Name);
            
            if (!isGeneric)
            {
                // We use FindBestExecutable with depth 5 for external libraries, 
                // but we check if the found EXE actually "belongs" to this folder level.
                var (bestExe, isInstaller) = FindBestExecutable(dir.FullName, new[] { ".exe" }, isExternal: true);

                if (!string.IsNullOrEmpty(bestExe))
                {
                    // Logic: If the EXE is found deep inside another NON-GENERIC folder, 
                    // then this current folder is just a container, and we should recurse.
                    if (IsExeBelongingToFolder(dir.FullName, bestExe))
                    {
                        var folderName = dir.Name;
                        var (cleanName, serial) = CleanGameTitle(folderName);

                        if (!IsBlacklistedTitle(cleanName))
                        {
                            bool alreadyInLibrary = existingGames.Any(g => g.Title.Equals(cleanName, StringComparison.OrdinalIgnoreCase));
                            bool alreadyInBatch = candidates.Any(c => c.Title.Equals(cleanName, StringComparison.OrdinalIgnoreCase));

                            if (!alreadyInLibrary && !alreadyInBatch)
                            {
                                candidates.Add(new GameCandidate
                                {
                                    Title = cleanName,
                                    Path = dir.FullName,
                                    ExecutablePath = bestExe,
                                    IsInstaller = isInstaller,
                                    IsExternal = true,
                                    PlatformKey = "pc_windows",
                                    Serial = serial
                                });
                                
                                Log($"[ExternalScan] Identified new game at: {dir.FullName} -> {cleanName}. Following subdirectories skipped.");
                            }
                            else
                            {
                                Log($"[ExternalScan] Folder {dir.FullName} identified as existing game '{cleanName}'. Skipping subdirectories.");
                            }

                            return; // STOP recursion here because this folder IS a game
                        }
                    }
                }
            }

            // 2. Otherwise (generic folder or no local EXE), continue searching subdirectories
            try
            {
                foreach (var subDir in dir.GetDirectories())
                {
                    await ScanDirectoryHierarchicalAsync(subDir, candidates, existingGames, ct);
                }
            }
            catch (Exception ex)
            {
                Log($"Error accessing subdirectories of {dir.FullName}: {ex.Message}");
            }
        }

        private bool IsExeBelongingToFolder(string folderPath, string exePath)
        {
            var relative = Path.GetRelativePath(folderPath, exePath);
            var parts = relative.Split(Path.DirectorySeparatorChar, Path.AltDirectorySeparatorChar);
            
            // If it's more than 3 levels deep, it's likely a sub-game or too nested to be "the" game of this folder.
            if (parts.Length > 4) return false; 
            
            // If any folder in the path between current folder and EXE is NOT generic,
            // then the EXE probably belongs to that sub-folder instead.
            for (int i = 0; i < parts.Length - 1; i++)
            {
                if (!IsGenericFolderName(parts[i])) return false;
            }
            
            return true;
        }

        private bool IsGenericFolderName(string name)
        {
            var generics = new[] 
            { 
                "Ship", "Shipping", "Retail", "Binaries", "x64", "x86", "Win64", "Win32", "Release", 
                "drive_c", "Program Files", "Program Files (x86)", "Users", "Games", "Juegos", "My Games", "Mis Juegos",
                "FitGirl", "FitGirl Repack", "DODI", "DODI Repack", "KaOs", "ElAmigos", "Repack", "Bottles", "drive_c/Games",
                "GOG Games", "Epic Games", "SteamLibrary", "common", "Games_Installed", "Installer",
                "windows", "system32", "syswow64", "Microsoft.NET", "Internet Explorer", "Windows NT"
            };
            return generics.Any(g => name.Equals(g, StringComparison.OrdinalIgnoreCase) || name.Contains(g, StringComparison.OrdinalIgnoreCase));
        }
        
        private bool IsBlacklistedFile(string fileName, bool isExternal)
        {
            if (string.IsNullOrEmpty(fileName)) return false;

            // 1. Exact Filename Blacklist
            if (_filenameBlacklist.Contains(fileName)) return true;

            // 2. Keyword/Substring Blacklist (TAREA 1)
            string lowered = fileName.ToLowerInvariant();
            foreach (var keyword in _keywordBlacklist)
            {
                if (lowered.Contains(keyword)) return true;
            }

            return false;
        }

        private bool IsMetadataSubfolder(string name)
        {
            var metadataFolders = new[] { "artworks", "soundtrack", "avatars", "manual", "wallpapers", "Goodies",            "Common", "Prerequisites", "Support", "Redist", "DirectX", "DotNet", "VCRedist", "PhysX",
            "Windows Media Player", "MD5", "z:"
        };
    return metadataFolders.Any(f => name.EndsWith(f, StringComparison.OrdinalIgnoreCase) || name.Contains(f, StringComparison.OrdinalIgnoreCase));
        }

        private bool IsBlacklistedTitle(string title)
        {
             var block = new HashSet<string>(StringComparer.OrdinalIgnoreCase) { 
                 "Windows", "Program Files", "Program Files (x86)", "Common Files", "Users", 
                 "drive_c", "dosdevices", "Binaries", "Win64", "Win32", "Common", "Engine", "Content",
                 "System32", "syswow64", "Microsoft.NET", "Accessories", "Command",
                 "x64", "x86", "Windows Media Player"
             };
             return block.Contains(title) || _folderBlacklist.Contains(title) || IsMetadataSubfolder(title) || Regex.IsMatch(title, @"^\d+$");
        }

        private bool IsBlacklistedFolder(DirectoryInfo dir)
        {
             return _folderBlacklist.Contains(dir.Name) || dir.Name.StartsWith(".");
        }

        private async Task<int> ProcessCandidatesBatchAsync(List<GameCandidate> candidates, List<Game> existingGames, GameMetadataService metadataService, System.Threading.CancellationToken ct)
        {
            int added = 0;
            const int batchSize = 5;
            const int delayMs = 2500;

            Log($"Processing {candidates.Count} candidates in batches of {batchSize}...");

            for (int i = 0; i < candidates.Count; i += batchSize)
            {
                ct.ThrowIfCancellationRequested();
                var batch = candidates.Skip(i).Take(batchSize).ToList();

                foreach (var candidate in batch)
                {
                    ct.ThrowIfCancellationRequested();
                    
                    // Re-evaluate executable for Folder candidates (Lazy evaluation)
                    string? exePath = candidate.ExecutablePath;
                    bool isInstaller = candidate.IsInstaller;

                    // If it's a folder (no extension) AND no explicit exe path set, try to find it
                    if (string.IsNullOrEmpty(exePath) && Directory.Exists(candidate.Path) && !File.Exists(candidate.Path))
                    {
                         // Folder Mode: Find best exe
                         var result = FindBestExecutable(candidate.Path, null, candidate.IsExternal); // Pass null for exts or assume defaults
                         exePath = result.Path;
                         isInstaller = result.IsInstaller;
                    }
                    else if (string.IsNullOrEmpty(exePath))
                    {
                        // File Mode: Path isExe
                        exePath = candidate.Path;
                        string fileName = Path.GetFileName(exePath);
                        var name = Path.GetFileNameWithoutExtension(exePath);
                        
                        // Check if file is blacklisted in File Mode (re-check with context if needed)
                        if (IsBlacklistedFile(fileName, candidate.IsExternal)) continue;

                        isInstaller = name.Contains("setup", StringComparison.OrdinalIgnoreCase) || 
                                      name.Contains("install", StringComparison.OrdinalIgnoreCase) ||
                                      name.Contains("deploy", StringComparison.OrdinalIgnoreCase);
                    }

                    if (await ProcessPotentialGame(candidate.Title, existingGames, metadataService, candidate.Path, candidate.PlatformKey, candidate.Serial, exePath, isInstaller, candidate.IsExternal))
                    {
                        added++;
                    }
                }

                if (i + batchSize < candidates.Count) await Task.Delay(delayMs, ct);
            }
            
            OnBatchFinished?.Invoke();
            return added;
        }

        // Updated signature to accept exePath and isInstaller and isExternal
        private async Task<bool> ProcessPotentialGame(string gameTitle, List<Game> existingGames, GameMetadataService metadataService, string? localPath = null, string? platformKey = null, string? serial = null, string? executablePath = null, bool isInstaller = false, bool isExternal = false)
        {
            Log($"[Scanner-Trace] Processing Candidate: '{localPath}' (Title: {gameTitle})");
            
            var existingByPath = existingGames.FirstOrDefault(g => g.Path == localPath);
            if (existingByPath != null) return false;

            var existingByTitle = existingGames.FirstOrDefault(g => g.Title.Equals(gameTitle, StringComparison.OrdinalIgnoreCase));
            if (existingByTitle != null)
            {
                if (localPath != null)
                {
                     // SAFETY OVERRIDE: If the filename screams "Installer", force the issue.
                     var fileName = Path.GetFileName(localPath).ToLowerInvariant();
                     if (fileName.Contains("install") || fileName.Contains("setup") || fileName.Contains("update"))
                     {
                          if (!isInstaller) Log($"[ProccessGame] FORCE OVERRIDE: Marking '{fileName}' as Installer due to strong naming match.");
                          isInstaller = true;
                     }
                }

                // VERSION CONTROL: If path is different, treating as Alternate Version (GameFile)
                if (!string.Equals(existingByTitle.Path, localPath, StringComparison.OrdinalIgnoreCase))
                {
                     // Check if we already have this file/version recorded
                     if (existingByTitle.GameFiles == null) existingByTitle.GameFiles = new List<GameFile>();
                     
                     var existingFile = existingByTitle.GameFiles.FirstOrDefault(gf => string.Equals(gf.RelativePath, localPath, StringComparison.OrdinalIgnoreCase));
             
             if (existingFile != null)
             {
                 Log($"[Scanner] Updating existing version: {localPath}");
                 // Update properties in case logic changed (e.g. Quality correction)
                 existingFile.Quality = isInstaller ? "Installer" : "Playable";
                 existingFile.ReleaseGroup = localPath != null ? new DirectoryInfo(localPath).Name : "Unknown";
                 // We don't save individually here because GameFiles are owned by Game? 
                 // Actually they are likely in a separate table. We should save.
                 // But since we don't have UpdateGameFileAsync, we might need to remove and re-add OR use UpdateAsync on the parent if configured to cascade.
                 // For safety/speed in this context, let's assume EF Core tracking might not be active if we didn't fetch with tracking in this scope?
                 // We fetched simple list.
                 
                 // Better approach: Since we injected `_gameRepository`, let's rely on it.
                 // But `IGameRepository` usually doesn't have `UpdateGameFile`.
                 // Let's defer to a specialized update or just skip for now and tell user to delete?
                 // No, automatic fix is better.
                 
                 // Create a direct update via SQL or just re-add? Re-add duplicates.
                 // Let's just create a new one and let EF handle it? No.
                 
                 // For now, let's just Log. The user might need to remove the game to fix it perfectly if we don't implement full update.
                 // WAIT: We can use `_gameRepository.UpdateAsync(existingByTitle.Id, existingByTitle)`? 
                 // If we modified the object in memory, UpdateAsync should persist it if it attaches.
                 
                 // Let's try to update the in-memory object and save via specific internal call.
                 existingFile.Quality = isInstaller ? "Installer" : "Playable";
                 
                 try 
                 {
                     // OPTIMIZATION: Prevent EF Core from attaching the entire Game graph
                     existingFile.Game = null; 
                     
                     // FIX: Use specific method for GameFile to ensure it saves!
                     await _gameRepository.UpdateGameFileAsync(existingFile);
                     // Removed UpdateAsync(parent) to avoid DB locking/redundancy
                 }
                 catch (Exception ex)
                 {
                     Log($"[Scanner] Error updating GameFile persistence: {ex.Message}");
                 }
                 
                 return true;
             }

                     Log($"[Scanner] Found Alternate Version for '{gameTitle}': {localPath}");
                     
                     // Create GameFile (Version)
                     var versionFile = new GameFile
                     {
                          GameId = existingByTitle.Id,
                          RelativePath = localPath ?? "", // Storing Absolute Path
                          DateAdded = DateTime.UtcNow,
                          ReleaseGroup = localPath != null ? new DirectoryInfo(localPath).Name : "Unknown",
                          Quality = isInstaller ? "Installer" : "Playable",
                          Size = (executablePath != null && File.Exists(executablePath)) ? new FileInfo(executablePath).Length : 0
                     };
                     
                     await _gameRepository.AddGameFileAsync(versionFile);
                     existingByTitle.GameFiles.Add(versionFile); 
                     return true;
                }

                // If the existing game has NO metadata (offline fallback), try to search again with the new cleaner title!
                if (!existingByTitle.IgdbId.HasValue || existingByTitle.IgdbId == 0)
                {
                    Log($"[Scanner] Game '{gameTitle}' exists but lacks metadata. Attempting re-resolution...");
                }
                else
                {
                    // If it already has metadata, just update paths
                    Log($"Updating existing game '{gameTitle}' path to: {localPath}");
                    existingByTitle.Path = localPath;
                    existingByTitle.ExecutablePath = executablePath; 
                    existingByTitle.IsExternal = isExternal; 
                    if (isInstaller) existingByTitle.Status = GameStatus.InstallerDetected;
                    
                    await _gameRepository.UpdateAsync(existingByTitle.Id, existingByTitle);
                    return true;
                }
            }

            Game? finalGame = null;

            try
            {
                Log($"Searching metadata for: {gameTitle}");
                var searchResults = await metadataService.SearchGamesAsync(gameTitle, platformKey, null, serial);
                
                if (searchResults != null && searchResults.Any())
                {
                    foreach (var gameData in searchResults)
                    {
                        if (!gameData.IgdbId.HasValue) continue;
                        
                        var match = existingGames.FirstOrDefault(g => g.IgdbId == gameData.IgdbId);
                        if (match != null)
                        {
                            match.Path = localPath;
                            match.ExecutablePath = executablePath;
                            match.IsExternal = isExternal;
                            await _gameRepository.UpdateAsync(match.Id, match);
                            return true;
                        }
                    
                        var fullMetadata = await metadataService.GetGameMetadataAsync(gameData.IgdbId.Value);
                        if (fullMetadata != null)
                        {
                            // ROBUSTNESS CHECK: If mandatory metadata is missing, try next result
                            if (fullMetadata.Year == 0 && string.IsNullOrEmpty(fullMetadata.Images.CoverUrl))
                            {
                                Log($"[Scanner] Metadata for Id {gameData.IgdbId} is empty (No Year/Cover). Trying next search result...");
                                continue;
                            }

                            finalGame = fullMetadata;
                            break; // Found a good one
                        }
                    }
                }
            }
            catch (Exception ex)
            {
                Log($"Error processing metadata for {gameTitle}: {ex.Message}.");
                
                // CRITICAL FIX: If authentication fails, do NOT add the game as "Offline".
                // We want to force the user to fix credentials rather than polluting the library.
                if (ex.Message.Contains("Forbidden") || ex.Message.Contains("authenticate") || ex.Message.Contains("Unauthorized"))
                {
                    Log("Skipping game addition due to authentication failure. Please check IGDB credentials.");
                    return false;
                }

                Log("Proceeding with offline fallback.");
            }

            // Fallback: Enable for Console/ROMs (v0.4.5)
            // If we are here, finalGame is null and metadata search was not successful.
            if (finalGame == null)
            {
                bool isConsole = platformKey != "pc_windows" && platformKey != "macos" && platformKey != "default";
                if (isConsole)
                {
                    Log($"[Scanner] Metadata not found for console game '{gameTitle}'. Creating offline entry.");
                    finalGame = new Game
                    {
                        Title = gameTitle,
                        Path = localPath,
                        ExecutablePath = executablePath,
                        IsExternal = isExternal,
                        PlatformId = await ResolvePlatformIdAsync(platformKey),
                        Status = GameStatus.Released,
                        Year = 0,
                        Overview = "Metadata not found. Added via offline fallback.",
                        Images = new GameImages()
                    };
                }
                else
                {
                    // Fallback to Offline Mode for PC too!
                    // If metadata fails, we still add the game so user can fix it.
                    finalGame = new Game
                    {
                        Title = gameTitle,
                        Path = localPath,
                        ExecutablePath = executablePath,
                        IsExternal = isExternal,
                        PlatformId = await ResolvePlatformIdAsync(platformKey),
                        Status = GameStatus.Released,
                        Year = 0,
                        Overview = "Metadata not found. Added via offline fallback.",
                        Images = new GameImages()
                    };
                    Log($"[Scanner] No metadata found for: '{gameTitle}'. Added as offline game.");
                }
            }

            // Finalize and Add
            if (existingByTitle != null)
            {
                // Updating existing offline game with new metadata
                existingByTitle.Title = finalGame.Title;
                existingByTitle.Overview = finalGame.Overview;
                existingByTitle.Year = finalGame.Year;
                existingByTitle.Images = finalGame.Images;
                existingByTitle.IgdbId = finalGame.IgdbId;
                existingByTitle.PlatformId = finalGame.PlatformId > 0 ? finalGame.PlatformId : await ResolvePlatformIdAsync(platformKey);
                
                await _gameRepository.UpdateAsync(existingByTitle.Id, existingByTitle);
                return true;
            }

            finalGame.Path = localPath;
            finalGame.ExecutablePath = executablePath;
            finalGame.IsExternal = isExternal;
            
            // Ensure PlatformId is set
            if (finalGame.PlatformId == 0)
                finalGame.PlatformId = await ResolvePlatformIdAsync(platformKey);

            if (isInstaller) finalGame.Status = GameStatus.InstallerDetected;

            try 
            {
                var newGame = await _gameRepository.AddAsync(finalGame);
                existingGames.Add(newGame);
                
                Log($"Added new game: {newGame.Title} (Exe: {executablePath ?? "None"}, Ext: {isExternal})");
                LastGameFound = newGame.Title;
                GamesAddedCount++;
                OnGameAdded?.Invoke(newGame);
                return true;
            }
            catch (Exception ex)
            {
                 var innerParam = ex.InnerException != null ? $" Inner: {ex.InnerException.Message}" : "";
                 Log($"Error saving game to DB {gameTitle} (with Metadata: {finalGame.IgdbId.HasValue}): {ex.Message}{innerParam}");
                 return false;
            }
        }
        
        private void Log(string message)
        {
            Console.WriteLine($"[Scanner] {message}");
            try
            {
                // Log to the executable directory
                string logPath = Path.Combine(AppContext.BaseDirectory, "scanner_log.txt");
                
                // Log Rotation: Keep it under 5MB for the scanner (can be noisy)
                var fileInfo = new FileInfo(logPath);
                if (fileInfo.Exists && fileInfo.Length > 5 * 1024 * 1024)
                {
                    var oldLog = logPath + ".old";
                    if (File.Exists(oldLog)) File.Delete(oldLog);
                    File.Move(logPath, oldLog);
                }

                File.AppendAllText(logPath, $"{DateTime.Now}: {message}\n");
                Console.WriteLine(message);
            }
            catch { }
        }

        private bool IsValidFile(string filePath, string[]? validExtensions)
        {
            var fileName = Path.GetFileName(filePath);
            if (fileName.StartsWith("._")) return false; // Skip macOS metadata
            
            // Ignore common tool directories for PS4 exploits to prevent false positives
            if (filePath.Contains("PPPwnGo", StringComparison.OrdinalIgnoreCase) || 
                filePath.Contains("GoldHEN", StringComparison.OrdinalIgnoreCase)) return false;

            // Explicitly Ignore known PS4 tools that are often in the folder
            if (fileName.Contains("PS4.Remote.PKG.Sender", StringComparison.OrdinalIgnoreCase) ||
                fileName.Contains("npcap", StringComparison.OrdinalIgnoreCase)) return false;

            var ext = Path.GetExtension(filePath);

            // TAREA 2: Linux support for extensionless binaries
            bool isLinux = System.Runtime.InteropServices.RuntimeInformation.IsOSPlatform(System.Runtime.InteropServices.OSPlatform.Linux);
            if (isLinux && string.IsNullOrEmpty(ext))
            {
                // On Linux, we allow files without extensions as valid candidates
                // provided they aren't hidden files AND they pass magic byte check.
                if (fileName.StartsWith(".")) return false;
                
                return IsExecutableBinary(filePath);
            }

            if (string.IsNullOrEmpty(ext)) return false;
            
            // Check global blacklist (TAREA 1)
            if (_globalBlacklist.Contains(ext)) return false;

            if (validExtensions == null) return true; // Default mode, any extension not blacklisted

            return validExtensions.Contains(ext, StringComparer.OrdinalIgnoreCase);
        }


        private string GetPlatformFromExtension(string ext)
        {
            if (string.IsNullOrEmpty(ext)) return "default";
            ext = ext.ToLower();

            // TAREA 1: Hardcoded Platform Mapping (v0.4.1 Restore / Granular)
            if (ext == ".nsp" || ext == ".xci" || ext == ".nsz" || ext == ".xcz") return "nintendo_switch";
            if (ext == ".pkg") return "ps4"; 
            if (ext == ".dmg" || ext == ".app") return "macos";
            
            // Retro Mappings (Granular again)
            if (ext == ".z64" || ext == ".n64" || ext == ".v64") return "nintendo_64";
            if (ext == ".sfc" || ext == ".smc") return "snes";
            if (ext == ".nes") return "nes";
            if (ext == ".gb" || ext == ".gbc" || ext == ".gba") return "gameboy_advance";
            if (ext == ".md" || ext == ".gen" || ext == ".smd" || ext == ".sms" || ext == ".gg") return "sega_genesis";
            if (ext == ".pce") return "pc_engine";

            // Default fallback (includes .iso, .exe, .bin, etc.)
            return "default";
        }

        private string? TryGuessPlatform(string folderPath)
        {
            try
            {
                var files = Directory.EnumerateFiles(folderPath, "*.*", SearchOption.AllDirectories).Take(50); // Just a sample
                foreach (var file in files)
                {
                    var ext = Path.GetExtension(file).ToLower();
                    if (ext == ".nsp" || ext == ".xci") return "nintendo_switch";
                    if (ext == ".pkg") return "ps4"; // High probability
                    if (ext == ".z64" || ext == ".n64" || ext == ".sfc") return "retro_emulation";
                }
            }
            catch { }
            return null;
        }

        private (string Title, string? Serial) CleanGameTitle(string originalTitle)
        {
            if (string.IsNullOrWhiteSpace(originalTitle)) return (originalTitle, null);
            
            // 0. Pre-clean URL encoded brackets often found in scene releases (Asterix example)
            // 5B = [, 5D = ]
            string workingTitle = originalTitle.Replace("5B", "[", StringComparison.OrdinalIgnoreCase)
                                               .Replace("5D", "]", StringComparison.OrdinalIgnoreCase);

            // 0. Aggressive Noise Stripping (Content in brackets/parens)
            // This is very common in Switch filenames for TitleIDs, Versions, Scene Tags
            workingTitle = workingTitle.Replace('\u00A0', ' ').Replace('_', ' ').Replace('-', ' ').Replace('+', ' ');
            workingTitle = Regex.Replace(workingTitle, @"[\[［].*?[\]］]", " ");
            workingTitle = Regex.Replace(workingTitle, @"\(.*?\)", " ");
            workingTitle = Regex.Replace(workingTitle, @"\{.*?\}", " ");

            // 1. Size patterns (e.g. 2.90GB, 500MB)
            workingTitle = Regex.Replace(workingTitle, @"\d+(\.\d+)?\s*(gb|mb|kb|gr|mg)", " ", RegexOptions.IgnoreCase);

            string? serial = null;
            
            // 0a. Try to find PlayStation Serial (Global Regions: US, EU, JP, Asia)
            // Covers: PS1 (SLPS, SCCS...), PS2 (SLPM, SLKA...), PS3 (BCAS, BLJM...), PS4 (PLJS, PCJS...), PS5 (ELJS, ELJM...)
            var psSerialMatch = Regex.Match(originalTitle, @"(CUSA|PPSA|BLES|BLUS|BCES|BCUS|NPEB|NPUB|NPEA|NPUA|SLES|SLUS|SCES|SCUS|SLPS|SLPM|SCCS|SLKA|BCAS|BLAS|BCJM|BLJM|BCJS|BLJS|PLJS|PLJM|PCJS|ELJS|ELJM)[-_]?\d{4,5}", RegexOptions.IgnoreCase);
            if (psSerialMatch.Success)
            {
                serial = psSerialMatch.Value.ToUpper().Replace("-", "").Replace("_", "");
                // Remove the serial from working title so it doesn't pollute name
                workingTitle = workingTitle.Replace(psSerialMatch.Value, " ", StringComparison.OrdinalIgnoreCase);
            }

            // 0b. Try to find Switch Serial (16-char hex)
            if (string.IsNullOrEmpty(serial))
            {
                var hexMatch = Regex.Match(originalTitle, @"[0-9a-fA-F]{16}"); 
                if (hexMatch.Success) serial = hexMatch.Value.ToUpper();
            }

            // 0c. Strip common PS4 content ID prefixes and artifacts
            // EP9000-CUSA..., UP9000-CUSA...
            workingTitle = Regex.Replace(workingTitle, @"[EU]P\d{4}-", " ", RegexOptions.IgnoreCase);
            
            // Strip standard version patterns v1.00, v1.0, v1
            // Strip standard version patterns v1.00, v1.0, v1, v05g
            workingTitle = Regex.Replace(workingTitle, @"v\d+([a-zA-Z0-9._-]+)*", " ", RegexOptions.IgnoreCase);
            
            // Strip PS4 specific codes like A0100 (App), V0100 (Version)
            workingTitle = Regex.Replace(workingTitle, @"\b[AV]\d{4}\b", " ", RegexOptions.IgnoreCase);

            // 2. Split and filter words
            var separators = new[] { ' ', '.', '[', ']', '(', ')', '{', '}', '［', '］', '+', ',', '!', '?', '#', '&', ';' };
            var words = workingTitle.Split(separators, StringSplitOptions.RemoveEmptyEntries);
            var cleanWords = new List<string>();

            foreach (var word in words)
            {
                if (_noiseWords.Contains(word)) continue;
                
                // Explicitly kill "00", "01" artifacts mostly left over from versions
                if (word == "00" || word == "01") continue;

                // Explicit check for common 2-letter codes that might have been missed by noise list
                if (word.Length == 2 && (word.Equals("US", StringComparison.OrdinalIgnoreCase) || 
                                       word.Equals("EU", StringComparison.OrdinalIgnoreCase) || 
                                       word.Equals("JP", StringComparison.OrdinalIgnoreCase) ||
                                       word.Equals("UK", StringComparison.OrdinalIgnoreCase))) continue;

                // NUCLEAR RULE: Skip any word that has 4 or more digits (Versions, TitleIDs, Years)
                int digitCount = word.Count(char.IsDigit);
                if (digitCount >= 4) continue;
                
                // Relaxed: Allow short numbers (e.g. "4" for Streets of Rage 4, "2" for Frostpunk 2)
                // if (word.Length <= 2 && int.TryParse(word, out _)) continue;

                cleanWords.Add(word);
            }

            string title = string.Join(" ", cleanWords).Trim();
            
            // Remove lingering noise
            title = Regex.Replace(title, @"\s+", " ");
            
            Log($"[Scanner-Debug] Cleaned: '{originalTitle}' -> '{title}' (Serial: {serial})");
            return (title, serial);
        }

        private string ResolvePlatformFromSerial(string serial)
        {
            if (string.IsNullOrEmpty(serial)) return "default";
            
            // PlayStation 4 / 5
            if (serial.StartsWith("CUSA") || serial.StartsWith("PLAS") || serial.StartsWith("PLJS") || serial.StartsWith("PLJM") || serial.StartsWith("PCJS")) return "ps4";
            if (serial.StartsWith("PPSA") || serial.StartsWith("ELJS") || serial.StartsWith("ELJM")) return "ps5";

            // PlayStation 3
            if (serial.StartsWith("BLES") || serial.StartsWith("BLUS") || 
                serial.StartsWith("BCES") || serial.StartsWith("BCUS") ||
                serial.StartsWith("NPEB") || serial.StartsWith("NPUB") ||
                serial.StartsWith("NPEA") || serial.StartsWith("NPUA") ||
                serial.StartsWith("BCAS") || serial.StartsWith("BLAS") ||
                serial.StartsWith("BCJM") || serial.StartsWith("BLJM") ||
                serial.StartsWith("BCJS") || serial.StartsWith("BLJS")) return "ps3";
            
            // PlayStation 2 / 1 (SLES/SLUS/SCES/SCUS/SLPS/SLPM/SCCS/SLKA)
            if (serial.StartsWith("SLES") || serial.StartsWith("SLUS") || 
                serial.StartsWith("SCES") || serial.StartsWith("SCUS") ||
                serial.StartsWith("SLPS") || serial.StartsWith("SLPM") ||
                serial.StartsWith("SCCS") || serial.StartsWith("SLKA"))
            {
               // Default to PS2 for SLES/SLUS/SCES/SCUS as they are ambiguous without lookup
               // SLPS/SLPM can be PS1 or PS2. 
               // This logic could be improved with an IDDB, but 'ps2' is a safe modern fallback 
               // allowing scanning. ideally we'd differ based on file type if possible.
               return "ps2"; 
            }
            
            // PlayStation Portable (PSP)
            if (serial.StartsWith("ULES") || serial.StartsWith("ULUS") ||
                serial.StartsWith("UCES") || serial.StartsWith("UCUS")) return "psp";

            return "default";
        }

        private async Task<int> ResolvePlatformIdAsync(string? platformKey)
        {
            // 1. Map internal scanner key to DB Slug
            string dbSlug = "pc"; // Default
            int defaultId = 6;    // Default ID (New Schema)

            if (!string.IsNullOrEmpty(platformKey) && !platformKey.Equals("default", StringComparison.OrdinalIgnoreCase))
            {
                switch (platformKey.ToLower())
                {
                    case "pc_windows": dbSlug = "pc"; defaultId = 6; break;
                    case "linux": dbSlug = "linux"; defaultId = 3; break;
                    case "macos": dbSlug = "mac"; defaultId = 14; break;
                    case "ps4": dbSlug = "ps4"; defaultId = 48; break;
                    case "nintendo_switch": dbSlug = "switch"; defaultId = 130; break;
                    case "ps5": dbSlug = "ps5"; defaultId = 167; break;
                    case "xbox_series": dbSlug = "xbox-series-x"; defaultId = 169; break;
                    case "ps3": dbSlug = "ps3"; defaultId = 9; break;
                    case "ps2": dbSlug = "ps2"; defaultId = 8; break;
                    case "ps1": dbSlug = "ps1"; defaultId = 7; break;
                    case "psp": dbSlug = "psp"; defaultId = 38; break;
                    case "nintendo_64": dbSlug = "n64"; defaultId = 4; break;
                    case "snes": dbSlug = "snes"; defaultId = 19; break;
                    case "nes": dbSlug = "nes"; defaultId = 18; break;
                    case "gameboy_advance": dbSlug = "gba"; defaultId = 24; break;
                    case "sega_genesis": dbSlug = "genesis"; defaultId = 29; break;
                    case "pc_engine": dbSlug = "pc-engine"; defaultId = 86; break;
                    case "retro_emulation": dbSlug = "pc"; defaultId = 6; break;
                    default: dbSlug = "pc"; defaultId = 6; break;
                }
            }

            // 2. Try Dynamic Lookup from DB
            try 
            {
                var dbId = await _gameRepository.GetPlatformIdBySlugAsync(dbSlug);
                if (dbId.HasValue) 
                {
                    // Log($"[Platform] Resolved '{platformKey}' -> Slug '{dbSlug}' -> ID {dbId.Value}");
                    return dbId.Value;
                }
                else
                {
                     Log($"[Platform] Slug '{dbSlug}' not found in DB. Attempting fallback to 'pc'.");
                     // Fallback: Try to find "pc" (safe haven)
                     var pcId = await _gameRepository.GetPlatformIdBySlugAsync("pc");
                     if (pcId.HasValue)
                     {
                         return pcId.Value;
                     }
                }
            }
            catch (Exception ex)
            {
                Log($"[Platform] Error looking up slug '{dbSlug}': {ex.Message}");
            }

            // 3. Ultimate Fallback (Desperation)
            // If even "pc" is missing from DB, we are in trouble. 
            // Only strictly safe ID is 1 (Legacy PC) or 6 (New PC). 
            // If we are here, DB is likely empty or broken.
            Log($"[Platform] CRITICAL: Could not find '{dbSlug}' OR 'pc' in DB. Using hardcoded fallback ID {defaultId} (Risk of FK error).");
            return defaultId;
        }
        private bool IsExecutableBinary(string filePath)
        {
            try
            {
                using (var fs = new FileStream(filePath, FileMode.Open, FileAccess.Read, FileShare.Read))
                {
                    if (fs.Length < 4) return false;

                    byte[] buffer = new byte[4];
                    fs.Read(buffer, 0, 4);

                    // Check for ELF Header (0x7F, 'E', 'L', 'F')
                    if (buffer[0] == 0x7F && buffer[1] == 0x45 && buffer[2] == 0x4C && buffer[3] == 0x46)
                        return true;

                    // Check for Shebang '#!' (0x23, 0x21) - Common for shell scripts wrapper
                    if (buffer[0] == 0x23 && buffer[1] == 0x21)
                        return true;
                }
            }
            catch 
            {
                // If we can't read it (permissions, etc.), assume false to remain safe.
                return false;
            }

            return false;
        }
    }
}
