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

        public event Action<Game>? OnGameAdded;
        public event Action? OnScanStarted;
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
            ".ds_store", ".db"
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
            "Retail", "x64", "x86", "System", "Binaries", "Engine", "Content", "Asset", "Resource"
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
                    string folderName = file.Directory?.Name.ToLowerInvariant() ?? "";
                    
                    // --- SCORING RULES ---
                    
                    // New V2 Blacklist Filtering
                    if (IsBlacklistedFile(file.Name, isExternal)) continue;

                    // 1. Installer Trap (Negative or Special State)
                    if (name.StartsWith("setup") || name.StartsWith("install") || name.StartsWith("unins") || name.Contains("redist") || name.StartsWith("config"))
                    {
                        isInstaller = true;
                        score -= 50; // Penalize, but keep as candidate if nothing else found
                    }

                    // 2. Name Match (+50)
                    if (name == root.Name.ToLowerInvariant()) score += 60;
                    if (name.Replace(" ", "").Replace("-", "") == root.Name.ToLowerInvariant().Replace(" ", "").Replace("-", "")) score += 50;

                    // 2.1 Parent Folder Name Match (+40) - Good for deeper structures
                    if (name == folderName) score += 40;

                    // 3. Keywords (+20)
                    if (name.Contains("shipping")) score += 20;
                    if (name.Contains("launcher")) score += 20;
                    if (name.Contains("game")) score += 10;
                    if (name.EndsWith("64")) score += 5;

                    // 4. Folder Location (+10)
                    if (folderName == "binaries" || folderName == "win64" || folderName == "release" || folderName == "shipping" || folderName == "retail") score += 25;
                    
                    // 5. File Size (+30 for largest) - Calculated later relative to others
                    
                    candidates.Add((file, score, isInstaller));
                }

                if (!candidates.Any()) return (null, false);

                // Apply Size Bonus to top 3 largest files
                var largestFiles = candidates.OrderByDescending(x => x.File.Length).Take(3).ToList();
                for (int i = 0; i < candidates.Count; i++)
                {
                    if (largestFiles.Any(l => l.File.FullName == candidates[i].File.FullName))
                    {
                        candidates[i] = (candidates[i].File, candidates[i].Score + 30, candidates[i].IsInstaller);
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
            Log($"Scanning (File Mode) Root: {rootPath}. Valid Extensions: {(extensionsToUse != null ? string.Join(", ", extensionsToUse) : "ALL")}");

            try
            {
                var files = Directory.EnumerateFiles(rootPath, "*.*", SearchOption.AllDirectories);
                
                foreach (var file in files)
                {
                    ct.ThrowIfCancellationRequested();

                    string fileName = Path.GetFileName(file);
                    if (IsBlacklistedFile(fileName, isExternal: false)) continue;
                    
                    if (IsValidFile(file, extensionsToUse))
                    {
                        var name = Path.GetFileNameWithoutExtension(file);
                         
                        // In File Mode, the file itself IS the executable.
                        // We check if it's an installer trap.
                        bool isInstaller = name.StartsWith("setup", StringComparison.OrdinalIgnoreCase) || 
                                           name.StartsWith("install", StringComparison.OrdinalIgnoreCase);

                        if (isInstaller)
                        {
                            // Skip installers in file mode unless we really want them?
                            // For now, let's treat them as valid candidates but flag them.
                        }

                        // Fix for generic filenames
                        if (name.Equals("setup", StringComparison.OrdinalIgnoreCase) || 
                            name.Equals("installer", StringComparison.OrdinalIgnoreCase) || 
                            name.Equals("game", StringComparison.OrdinalIgnoreCase))
                        {
                            var parentDir = Path.GetDirectoryName(file);
                            if (parentDir != null) name = new DirectoryInfo(parentDir).Name;
                        }

                        // Smart detection logic...
                        string finalPlatformKey = platformKey;
                        if (platformKey == "default") finalPlatformKey = GetPlatformFromExtension(Path.GetExtension(file));

                        var (cleanName, serial) = CleanGameTitle(name);
                        
                        if (!existingGames.Any(g => g.Title.Equals(cleanName, StringComparison.OrdinalIgnoreCase)))
                        {
                             candidates.Add(new GameCandidate 
                            { 
                                Title = cleanName, 
                                Path = file, // Executable is the path
                                PlatformKey = finalPlatformKey, 
                                Serial = serial
                                // Note: We need to somehow signal this is an installer or store the ExecutablePath
                                // In file mode, Path == ExecutablePath
                            });
                        }
                    }
                }
            }
            catch (Exception ex)
            {
                Log($"Error accessing directories: {ex.Message}");
            }

            return await ProcessCandidatesBatchAsync(candidates, existingGames, metadataService, ct);

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
                            if (!existingGames.Any(g => g.Title.Equals(cleanName, StringComparison.OrdinalIgnoreCase)) &&
                                !candidates.Any(c => c.Title.Equals(cleanName, StringComparison.OrdinalIgnoreCase)))
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
                                
                                Log($"[ExternalScan] Identified game at: {dir.FullName} -> {cleanName}. Stopping recursion for this path.");
                                return; // Found game! Stop recursion for this branch.
                            }
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
                "GOG Games", "Epic Games", "SteamLibrary", "SteamApps", "common", "Games_Installed", "Installer"
            };
            return generics.Any(g => name.Equals(g, StringComparison.OrdinalIgnoreCase) || name.Contains(g, StringComparison.OrdinalIgnoreCase));
        }
        
        private bool IsBlacklistedFile(string fileName, bool isExternal)
        {
            if (string.IsNullOrEmpty(fileName)) return false;

            // 1. Exact Filename Blacklist
            if (_filenameBlacklist.Contains(fileName)) return true;

            // 2. Pattern Ignore List (Prefixes)
            string lowered = fileName.ToLowerInvariant();
            if (lowered.StartsWith("unins") || 
                lowered.StartsWith("uninstall"))
            {
                return true;
            }

            // Strictly ignore config/setup only if it's EXTERNAL (Wine/Whisky)
            // In the main library, setup.exe might be the only entry point for an uninstalled game.
            if (isExternal && (lowered.StartsWith("config") || lowered.StartsWith("setup")))
            {
                return true;
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
                 "drive_c", "dosdevices", "Binaries", "Win64", "Win32", "Common", "Engine", "Content" 
             };
             return block.Contains(title) || _folderBlacklist.Contains(title) || IsMetadataSubfolder(title);
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

            try
            {
                Log($"Searching metadata for: {gameTitle}");
                var searchResults = await metadataService.SearchGamesAsync(gameTitle, platformKey, null, serial);
                
                if (searchResults != null && searchResults.Any())
                {
                    var gameData = searchResults.First();
                    
                    if (gameData.IgdbId.HasValue)
                    {
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
                            fullMetadata.Path = localPath;
                            fullMetadata.ExecutablePath = executablePath; // Set Exe Path
                            fullMetadata.IsExternal = isExternal; // Set Flag
                            fullMetadata.PlatformId = await ResolvePlatformIdAsync(platformKey);
                            
                            if (isInstaller) fullMetadata.Status = GameStatus.InstallerDetected;

                            var newGame = await _gameRepository.AddAsync(fullMetadata);
                            existingGames.Add(newGame);
                            
                            Log($"Added new game: {newGame.Title} (Exe: {executablePath}, Ext: {isExternal})");
                            LastGameFound = newGame.Title;
                            GamesAddedCount++;
                            OnGameAdded?.Invoke(newGame);
                            return true;
                        }
                    }
                }
            }
            catch (Exception ex)
            {
                Log($"Error processing {gameTitle}: {ex.Message}");
            }
            return false;
        }
        
        private void Log(string message)
        {
            try
            {
                // Log to the executable directory
                string logPath = Path.Combine(AppContext.BaseDirectory, "scanner_log.txt");
                File.AppendAllText(logPath, $"{DateTime.Now}: {message}\n");
                Console.WriteLine(message);
            }
            catch { }
        }

        private bool IsValidFile(string filePath, string[]? validExtensions)
        {
            var ext = Path.GetExtension(filePath);
            if (string.IsNullOrEmpty(ext)) return false;
            // Case-insensitive check
            if (validExtensions == null) // All extensions allowed (default mode)
            {
                 return !_globalBlacklist.Contains(ext);
            }
            return validExtensions.Contains(ext, StringComparer.OrdinalIgnoreCase) && !_globalBlacklist.Contains(ext);
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

        private (string Title, string? Serial) CleanGameTitle(string title)
        {
            if (string.IsNullOrWhiteSpace(title)) return (title, null);
            
            string? serial = null;
            // 0. Extract Platform IDs (CUSA12345, CUSA-12345, BLES12345, BLUS12345, etc) before cleaning
            var serialMatch = Regex.Match(title, @"([A-Z]{4}-?\d{5})", RegexOptions.IgnoreCase);
            if (serialMatch.Success)
            {
                serial = serialMatch.Value.Replace("-", "").ToUpper();
            }

            // 1. Remove common site tags and scene groups (Case Insensitive)
            string[] noise = { 
                "OPOISSO893", "OPOISSO", "CyB1K", "DLPSGAME.COM", "DLPSGAME", 
                "RPGONLY.COM", "RPGONLY", "NSW2U.COM", "NSW2U.IN", "NSW2U",
                "QUARK", "VENOM", "RAZOR1911", "RELOADED", "SKIDROW", "CODEX",
                "FitGirl", "DODI", "EMPRESS", "GOG", "xatab", "gamesfull", "bitsearch", 
                "www", "com", "app", "org", "net"
            };
            
            foreach (var n in noise)
            {
                title = Regex.Replace(title, n, "", RegexOptions.IgnoreCase);
            }

            // 2. Remove Versioning (v1.0, v1.0.4, 1.0.4, 1.00, etc)
            title = Regex.Replace(title, @"v?\d+\.\d+(\.\d+)*", "", RegexOptions.IgnoreCase);

            // 3. Remove Platform IDs (CUSA12345, CUSA-12345, BLES12345, BLUS12345, etc)
            title = Regex.Replace(title, @"[A-Z]{4}-?\d{5}", "", RegexOptions.IgnoreCase);
            
            // 4. Remove Content IDs (EP9000, UP0001, etc)
            title = Regex.Replace(title, @"[A-Z]{2}\d{4}", "", RegexOptions.IgnoreCase);

            // 5. Remove content in brackets [] and parentheses ()
            title = Regex.Replace(title, @"\[.*?\]", "");
            title = Regex.Replace(title, @"\(.*?\)", "");

            // 6. Replace separators (dots, underscores, dashes) with spaces
            title = Regex.Replace(title, @"[._-]", " ");

            title = Regex.Replace(title, @"\s+", " ").Trim();

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
