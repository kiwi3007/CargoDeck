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
            "www", "app", "to", "com", "net", "org", "iso", "bin", "decepticon", "empress", 
            "tenoke", "rune", "goldberg", "ali213", "p2p", "fairlight"
        };
        public event Action<int>? OnScanFinished;
        public event Action? OnBatchFinished;

        // 1. Valid Extensions (Whitelist)
        private static readonly Dictionary<string, PlatformRule> _platformRules = new(StringComparer.OrdinalIgnoreCase)
        {
            ["nintendo_switch"] = new() { Extensions = new[] { ".nsp", ".xci", ".nsz", ".xcz" } },
            ["ps4"] = new() { Extensions = new[] { ".pkg", ".bin" } },
            ["pc_windows"] = new() { Extensions = new[] { ".iso", ".exe", ".zip", ".rar", ".7z", ".setup" }, IsFolderMode = true },
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
            .Where(ext => !ext.Equals(".bin", StringComparison.OrdinalIgnoreCase)) // Exclude .bin from Auto-Scan (too generic, matches system files)
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
            "steam_api", "crashpad", "unitycrash", "unins000", "uninstall", "update", "config", "dxsetup", "redist", "vcredist", "fna", "mono", "bios", "firmware"
        };

        private static readonly HashSet<string> _filenameBlacklist = new(StringComparer.OrdinalIgnoreCase)
        {
            "crashpad_handler.exe", "unitycrashhandler.exe", "unitycrashhandler64.exe", 
            "dxsetup.exe", "vcredist_x64.exe", "vcredist_x86.exe", "credist.exe", "bsndrpt.exe",
            "socialclub.exe", "epicgameslauncher.exe", "eaapp.exe", "origin.exe", "ubisoftconnect.exe"
        };

        private static readonly HashSet<string> _folderBlacklist = new(StringComparer.OrdinalIgnoreCase)
        {
            "_CommonRedist", "CommonRedist", "Redist", "DirectX", "Support", 
            "Prerequisites", "Launcher", "Ship", "Shipping", 
            "Retail", "x64", "x86", "System", "Binaries", "Engine", "Content", "Asset", "Resource",
            "shadercache", "compatdata", "depotcache", "steamapps", ".steam", ".local", ".cache", "temp", "tmp", "node_modules",
            "windows", "system32", "syswow64", "Microsoft.NET", "Framework", "Framework64", "Internet Explorer", "Accessories", "Windows NT", "INF", "WinSxS", "SysARM32", "Sysnative", "command"
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
                var winePath = settings.WinePrefixPath;
                if (!string.IsNullOrEmpty(winePath) && Directory.Exists(winePath))
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

                    // 4. Installer/Config Penalty (-50) (TAREA 3)
                    if (name.Contains("launch") || name.Contains("settings") || name.Contains("server") || 
                        name.Contains("config") || name.Contains("setup") || name.Contains("install"))
                    {
                        score -= 50;
                        if (name.Contains("setup") || name.Contains("install")) isInstaller = true;
                    }

                    // 5. Folder Location Bonus
                    if (folderName == "binaries" || folderName == "win64" || folderName == "release" || folderName == "shipping" || folderName == "retail") score += 25;
                    
                    // 6. Native Linux executable bonus (on Linux systems)
                    if (System.Runtime.InteropServices.RuntimeInformation.IsOSPlatform(System.Runtime.InteropServices.OSPlatform.Linux) && string.IsNullOrEmpty(file.Extension))
                    {
                        score += 10;
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
                
                // Threshold: If winner score is too low and it looks generic, be careful?
                // For now, trust the score.
                
                return (winner.File.FullName, winner.IsInstaller);
            }
            catch (Exception ex)
            {
                Log($"Error discovering executable in {folderPath}: {ex.Message}");
                return (null, false);
            }
        }

        private List<FileInfo> GetFilesSafe(DirectoryInfo root, int currentDepth, int maxDepth)
        {
            var results = new List<FileInfo>();
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
                DiscoverFilesHierarchical(new DirectoryInfo(rootPath), extensionsToUse, validFilesByFolder, ct);

                Log($"[FileMode] Discovery phase finished. Found {validFilesByFolder.Count} candidate folders items. Applying clustering...");

                foreach (var folderEntry in validFilesByFolder)
                {
                    ct.ThrowIfCancellationRequested();
                    var folderPath = folderEntry.Key;
                    var filePaths = folderEntry.Value;

                    // Choose ONLY THE BEST per folder
                    var (bestExePath, isInstaller) = FindBestExecutableInList(folderPath, filePaths);

                    if (!string.IsNullOrEmpty(bestExePath))
                    {
                        var rawFileName = Path.GetFileNameWithoutExtension(bestExePath);
                        if (string.IsNullOrEmpty(rawFileName)) rawFileName = Path.GetFileName(bestExePath);
                        
                        var rawFolderName = new DirectoryInfo(folderPath).Name;
                        
                        // Clean both to compare their "quality"
                        var (cleanFile, _) = CleanGameTitle(rawFileName);
                        var (cleanFolder, _) = CleanGameTitle(rawFolderName);

                        // Selection Heuristic:
                        // 1. If cleanFile is empty/too short and cleanFolder is better -> Folder
                        // 2. If cleanFolder contains more words or is significantly longer -> Folder
                        // 3. If file is "setup" or "install" -> Folder
                        bool isGenericFile = rawFileName.Equals("setup", StringComparison.OrdinalIgnoreCase) || 
                                             rawFileName.Equals("install", StringComparison.OrdinalIgnoreCase) || 
                                             rawFileName.Equals("game", StringComparison.OrdinalIgnoreCase);

                        string selectedName;
                        string source;

                        // QUALITY HEURISTIC (v0.4.4):
                        // 1. Prefer cleaner strings (fewer surplus words).
                        // 2. Protect sequels (e.g., '4' in 'Game Title 4').
                        // 3. Folder Name is a tie-breaker for installers.

                        if (isGenericFile)
                        {
                            selectedName = cleanFolder;
                            source = "Folder (Generic File)";
                        }
                        else if (cleanFolder.Equals(cleanFile, StringComparison.OrdinalIgnoreCase))
                        {
                            // Perfect match: default to Folder for better path consistency
                            selectedName = cleanFolder;
                            source = "Folder (Exact Match)";
                        }
                        else if (cleanFolder.Contains(cleanFile, StringComparison.OrdinalIgnoreCase) && !_noiseWords.Any(nw => cleanFolder.Contains(nw, StringComparison.OrdinalIgnoreCase)))
                        {
                            // Folder is "Game A Title", File is "Title" -> Pick Folder
                            // Only if folder doesn't contain known noise words
                            selectedName = cleanFolder;
                            source = "Folder (Subset Match)";
                        }
                        else if (cleanFile.Contains(cleanFolder, StringComparison.OrdinalIgnoreCase))
                        {
                            // File is "Game B Special Edition", Folder is "Game B" -> Pick File
                            selectedName = cleanFile;
                            source = "Filename (Subset Match)";
                        }
                        else if (cleanFolder.Length > 3 && cleanFile.StartsWith(cleanFolder, StringComparison.OrdinalIgnoreCase))
                        {
                            // Example: Folder='Game II', File='Game II Build 123'
                            selectedName = cleanFolder;
                            source = "Folder (Cleaner Prefix)";
                        }
                        else if (cleanFile.Length > 3 && cleanFolder.StartsWith(cleanFile, StringComparison.OrdinalIgnoreCase))
                        {
                            // Example: Folder='Game Title Special Edition', File='Game Title'
                            selectedName = cleanFile;
                            source = "Filename (Cleaner Prefix)";
                        }
                        else
                        {
                            // Tie or generic difference: default to Folder as it's usually the "Release Name"
                            selectedName = cleanFolder.Length > 0 ? cleanFolder : cleanFile;
                            source = cleanFolder.Length > 0 ? "Folder (Context Default)" : "Filename (Fallback)";
                        }

                        // Use the cleaned name for the final candidate
                        var finalTitle = selectedName;
                        var (_, serial) = CleanGameTitle(rawFileName); // Extract serial from filename if possible
                        
                        Log($"[Scanner] Title Resolution: File('{cleanFile}') vs Folder('{cleanFolder}') -> Selected: '{finalTitle}' (Source: {source})");

                        string finalPlatformKey = platformKey;
                        if (platformKey == "default") finalPlatformKey = GetPlatformFromExtension(Path.GetExtension(bestExePath));

                        if (!existingGames.Any(g => g.Title.Equals(finalTitle, StringComparison.OrdinalIgnoreCase)))
                        {
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

        private void DiscoverFilesHierarchical(DirectoryInfo root, string[]? allowedExtensions, Dictionary<string, List<string>> results, System.Threading.CancellationToken ct)
        {
            ct.ThrowIfCancellationRequested();

            if (!root.Exists) return;
            if (root.Name.StartsWith(".") || _folderBlacklist.Contains(root.Name) || IsMetadataSubfolder(root.Name)) return;

            try
            {
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
                    DiscoverFilesHierarchical(subDir, allowedExtensions, results, ct);
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

            var bestByScore = candidates.OrderByDescending(c => c.Score).First();
            // Tie-break with size if scores are equal
            var topScorers = candidates.Where(c => c.Score == bestByScore.Score).ToList();
            if (topScorers.Count > 1)
            {
                 bestByScore = topScorers.OrderByDescending(c => new FileInfo(c.FilePath).Length).First();
            }

            return (bestByScore.FilePath, bestByScore.IsInstaller);
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
                "GOG Games", "Epic Games", "SteamLibrary", "SteamApps", "common", "Games_Installed", "Installer",
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
            var metadataFolders = new[] { "artworks", "soundtrack", "avatars", "manual", "wallpapers", "Goodies", "MD5", "Bonus", "Documentation", "Support", "Redist", "DirectX", "DotNet", "VCRedist", "PhysX" };
            return metadataFolders.Any(f => name.EndsWith(f, StringComparison.OrdinalIgnoreCase) || name.Contains(f, StringComparison.OrdinalIgnoreCase));
        }

        private bool IsBlacklistedTitle(string title)
        {
             var block = new HashSet<string>(StringComparer.OrdinalIgnoreCase) { 
                 "Windows", "Program Files", "Program Files (x86)", "Common Files", "Users", 
                 "drive_c", "dosdevices", "Binaries", "Win64", "Win32", "Common", "Engine", "Content",
                 "System32", "syswow64", "Microsoft.NET", "Accessories", "Command"
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

                        isInstaller = name.StartsWith("setup", StringComparison.OrdinalIgnoreCase) || name.StartsWith("install", StringComparison.OrdinalIgnoreCase);
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
            var existingByPath = existingGames.FirstOrDefault(g => g.Path == localPath);
            if (existingByPath != null) return false;

            var existingByTitle = existingGames.FirstOrDefault(g => g.Title.Equals(gameTitle, StringComparison.OrdinalIgnoreCase));
            if (existingByTitle != null)
            {
                // If it's the SAME path, update metadata. If it's different path, maybe we want to keep the existing one or update?
                // For now, update logic:
                Log($"Updating existing game '{gameTitle}' path to: {localPath}");
                existingByTitle.Path = localPath;
                existingByTitle.ExecutablePath = executablePath; 
                existingByTitle.IsExternal = isExternal; // Update Flag
                if (isInstaller) existingByTitle.Status = GameStatus.InstallerDetected;
                
                await _gameRepository.UpdateAsync(existingByTitle.Id, existingByTitle);
                return true;
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

            // Fallback: DISABLED (v0.4.3+)
            // We no longer add games in "Offline" mode if metadata is not found.
            // This prevents the library from being cluttered with messy filenames.
            if (finalGame == null)
            {
                Log($"[Scanner] No metadata found for: '{gameTitle}'. Skipping game addition (Offline mode disabled).");
                return false;
            }

            // Finalize and Add
            finalGame.Path = localPath;
            finalGame.ExecutablePath = executablePath;
            finalGame.IsExternal = isExternal;
            
            // CRITICAL FIX: Ensure PlatformId is ALWAYS set, even when metadata is found.
            // If fullMetadata was fetched, its PlatformId might be 0, which would cause an FK constraint violation.
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
            var ext = Path.GetExtension(filePath);

            // TAREA 2: Linux support for extensionless binaries
            bool isLinux = System.Runtime.InteropServices.RuntimeInformation.IsOSPlatform(System.Runtime.InteropServices.OSPlatform.Linux);
            if (isLinux && string.IsNullOrEmpty(ext))
            {
                // On Linux, we allow files without extensions as valid candidates
                // provided they aren't hidden files.
                return !fileName.StartsWith(".");
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

            // Direct mapping for clear-cut extensions
            if (ext == ".nsp" || ext == ".xci" || ext == ".nsz" || ext == ".xcz") return "nintendo_switch";
            if (ext == ".dmg" || ext == ".app") return "macos";
            
            // Retro Emulation covers most cartridge-based extensions
            string[] retroExts = { ".z64", ".n64", ".v64", ".sfc", ".smc", ".nes", ".gb", ".gba", ".gbc", ".md", ".gen", ".smd", ".sms", ".gg", ".pce" };
            if (retroExts.Contains(ext)) return "retro_emulation";

            // Disk-based or compressed could be many things, default to 'default' or guess
            // For now, let's keep it simple. If it's .pkg, it's mostly PS4/PS3
            if (ext == ".pkg") return "ps4";
            
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
            
            string title = originalTitle;
            string? serial = null;

            // 0. Extract Serial (CUSA12345, etc)
            var serialMatch = Regex.Match(title, @"([A-Z]{4}-?\d{5})", RegexOptions.IgnoreCase);
            if (serialMatch.Success)
            {
                serial = serialMatch.Value.Replace("-", "").ToUpper();
            }

            // 0.1 PRE-SPLIT CLEANING (v0.4.9): Strip complex version markers before tokenization
            // Handles: v1.2.3, v.1.2.3, v 1.2.3, build.123, build 123
            title = Regex.Replace(title, @"[vV][\.\s]?\d+(\.\d+)*\b", "", RegexOptions.IgnoreCase);
            title = Regex.Replace(title, @"build[\.\s]?\d+\b", "", RegexOptions.IgnoreCase);

            // 4. Word-by-Word Filtering (Universal Separators)
            var separators = new[] { ' ', '.', '_', '-', '[', ']', '(', ')' };
            var words = title.Split(separators, StringSplitOptions.RemoveEmptyEntries);
            
            // Refined Filtering:
            // - Stop at common site extensions (com, net, etc)
            // - Skip noise words
            // - Keep numbers that look like sequels (single digits)
            var cleanWords = new List<string>();
            foreach (var word in words)
            {
                if (_noiseWords.Contains(word)) continue;
                
                // version codes: v1, v05g, r10978, (39928)
                // PRESERVE single digits (1-9) as sequels unless they have a 'v' prefix
                if (Regex.IsMatch(word, @"^v\d+[a-z]?$", RegexOptions.IgnoreCase)) continue; 
                if (Regex.IsMatch(word, @"^r\d+$", RegexOptions.IgnoreCase)) continue;
                if (Regex.IsMatch(word, @"^\d{2,6}$")) continue; // Multi-digit versions/builds (10, 2024, 39928)

                cleanWords.Add(word);
            }
            
            title = string.Join(" ", cleanWords);

            // 5. Global Regex Pass (Architectures, years, metadata)
            title = Regex.Replace(title, @"\b(64bit|32bit|x64|x86|build|v|ver|version)\b", "", RegexOptions.IgnoreCase);
            title = Regex.Replace(title, @"(?<=\w\s)(19|20)\d{2}", ""); // Year check

            // Cleanup
            title = Regex.Replace(title, @"\s+", " ").Trim();

            // Final Validation: 
    // If the title is too short (1 char) or just a number (e.g. "0", "1"), it's likely noise
    if (title.Length <= 1 || Regex.IsMatch(title, @"^\d+$"))
    {
        return (string.Empty, serial);
    }

    Log($"[Scanner] Cleaning Title: '{originalTitle}' -> '{title}'");
            
            return (title, serial);
        }

        private string ResolvePlatformFromSerial(string serial)
        {
            if (string.IsNullOrEmpty(serial)) return "default";
            
            // PlayStation 4 / 5
            if (serial.StartsWith("CUSA") || serial.StartsWith("PLAS")) return "ps4";
            if (serial.StartsWith("PPSA")) return "ps5";

            // PlayStation 3
            if (serial.StartsWith("BLES") || serial.StartsWith("BLUS") || 
                serial.StartsWith("BCES") || serial.StartsWith("BCUS") ||
                serial.StartsWith("NPEB") || serial.StartsWith("NPUB") ||
                serial.StartsWith("NPEA") || serial.StartsWith("NPUA")) return "ps3";
            
            // PlayStation 2 / 1 (SLES/SLUS/SCES/SCUS)
            // Typically 5 digits. PS1 also uses these. 
            // Since User asked for "PS1/2", lets look at ID pattern or just map to 'ps2' as default retro target
            if (serial.StartsWith("SLES") || serial.StartsWith("SLUS") || 
                serial.StartsWith("SCES") || serial.StartsWith("SCUS"))
            {
               // Just map to PS2 for now, logic can be improved later
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
                    case "macos": dbSlug = "mac"; defaultId = 14; break; // Note: Scanner uses "macos", DB uses "mac"
                    case "ps4": dbSlug = "ps4"; defaultId = 48; break;
                    case "nintendo_switch": dbSlug = "switch"; defaultId = 130; break;
                    case "ps5": dbSlug = "ps5"; defaultId = 167; break;
                    case "xbox_series": dbSlug = "xbox-series-x"; defaultId = 169; break;
                    case "ps3": dbSlug = "ps3"; defaultId = 9; break;
                    case "ps2": dbSlug = "ps2"; defaultId = 8; break;
                    case "ps1": dbSlug = "ps1"; defaultId = 7; break;
                    case "psp": dbSlug = "psp"; defaultId = 38; break;
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
    }
}
