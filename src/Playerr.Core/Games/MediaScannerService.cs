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
            // Use try-catch for Directory.GetDirectories to handle permission issues
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
                if (DirectoryContainsValidFile(dir, rule.Extensions))
                {
                    var (cleanName, serial) = CleanGameTitle(folderName);
                    if (!existingGames.Any(g => g.Title.Equals(cleanName, StringComparison.OrdinalIgnoreCase)))
                    {
                        candidates.Add(new GameCandidate 
                        { 
                            Title = cleanName, 
                            Path = dir, 
                            PlatformKey = platformKey, 
                            Serial = serial 
                        });
                    }
                    else
                    {
                        Log($"Game already exists in DB (Skipping collect): {cleanName}");
                    }
                }
            }

            return await ProcessCandidatesBatchAsync(candidates, existingGames, metadataService, ct);
        }

        private bool DirectoryContainsValidFile(string dirPath, string[] extensions)
        {
            try
            {
                // New logic: Check Root (Depth 0) and immediate subdirectories (Depth 1)
                // Patterns for fuzzy matching
                var patterns = new[] { "setup*.exe", "install*.exe", "installer.exe", "game.exe" };
                
                // Signatures that indicate a game folder
                var signatureFiles = new HashSet<string>(StringComparer.OrdinalIgnoreCase)
                {
                    "steam_api.dll", "steam_emu.ini", "autorun.inf", "verify.bat", "eboot.bin"
                };

                // Helper to check a specific directory level
                bool CheckDirectory(string path)
                {
                    // 1. Check fuzzy patterns
                    foreach (var pattern in patterns)
                    {
                        if (Directory.GetFiles(path, pattern, SearchOption.TopDirectoryOnly).Any()) return true;
                    }

                    // 2. Check other signatures and extensions
                    var files = Directory.GetFiles(path, "*.*", SearchOption.TopDirectoryOnly);
                    foreach (var file in files)
                    {
                        var fileName = Path.GetFileName(file);
                        var ext = Path.GetExtension(file);

                        if (signatureFiles.Contains(fileName)) return true;
                        if (_globalBlacklist.Contains(ext)) continue;
                        if (extensions != null && extensions.Contains(ext, StringComparer.OrdinalIgnoreCase)) return true;
                    }
                    return false;
                }

                // Check Depth 0: Root
                if (CheckDirectory(dirPath)) return true;

                // Check Depth 1: Immediate subdirectories
                foreach (var subDir in Directory.GetDirectories(dirPath))
                {
                    if (CheckDirectory(subDir)) return true;
                }

                return false;
            }
            catch
            {
                return false;
            }
        }

        private async Task<int> ScanFileModeAsync(string rootPath, PlatformRule rule, List<Game> existingGames, string platformKey, GameMetadataService metadataService, System.Threading.CancellationToken ct)
        {
            var candidates = new List<GameCandidate>();
            var extensionsToUse = platformKey == "default" ? _allExtensions : rule.Extensions;
            Log($"Scanning (File Mode) Root: {rootPath}. Valid Extensions: {(extensionsToUse != null ? string.Join(", ", extensionsToUse) : "ALL")}");

            try
            {
                // Recursive scan for all files
                var files = Directory.EnumerateFiles(rootPath, "*.*", SearchOption.AllDirectories);
                int i = 0;
                
                foreach (var file in files)
                {
                    ct.ThrowIfCancellationRequested();
                    
                    if (IsValidFile(file, extensionsToUse))
                    {
                        var name = Path.GetFileNameWithoutExtension(file);
                        
                        // Fix for generic filenames: use parent folder name instead
                        if (name.Equals("setup", StringComparison.OrdinalIgnoreCase) || 
                            name.Equals("installer", StringComparison.OrdinalIgnoreCase) || 
                            name.Equals("game", StringComparison.OrdinalIgnoreCase) ||
                            name.Equals("autorun", StringComparison.OrdinalIgnoreCase) ||
                            name.StartsWith("setup", StringComparison.OrdinalIgnoreCase))
                        {
                            var parentDir = Path.GetDirectoryName(file);
                            if (parentDir != null)
                            {
                                name = new DirectoryInfo(parentDir).Name;
                            }
                        }
                        Log($"Found valid file: {file}");
                        
                        // Smart platform detection for each file in universal mode
                        string finalPlatformKey = platformKey;
                        if (platformKey == "default")
                        {
                            finalPlatformKey = GetPlatformFromExtension(Path.GetExtension(file));
                        }

                        var (cleanName, serial) = CleanGameTitle(name);
                        
                        if (!existingGames.Any(g => g.Title.Equals(cleanName, StringComparison.OrdinalIgnoreCase)))
                        {
                            candidates.Add(new GameCandidate 
                            { 
                                Title = cleanName, 
                                Path = file, 
                                PlatformKey = finalPlatformKey, 
                                Serial = serial 
                            });
                            
                            // Smart Platform Detection Update:
                            // If we were in default mode but found a serial, checking if we can upgrade the platform key
                            if (platformKey == "default" && !string.IsNullOrEmpty(serial))
                            {
                                var detectedPlatform = ResolvePlatformFromSerial(serial);
                                if (detectedPlatform != "default")
                                {
                                    candidates.Last().PlatformKey = detectedPlatform;
                                    Log($"[SmartDetect] Upgraded '{cleanName}' from default to {detectedPlatform} (Serial: {serial})");
                                }
                            }

                            if(cleanName != name) Log($"Collected Candidate: '{cleanName}'" + (serial != null ? $" (Serial: {serial})" : ""));
                        }
                        else
                        {
                            Log($"Game already exists in DB (Skipping collect): {cleanName}");
                        }
                    }
                    else
                    {
                         // Debug: Log why it was skipped (only for first 50 skipped files to avoid spam)
                         if (i < 50) Log($"Skipped file: {Path.GetFileName(file)} (Ext: {Path.GetExtension(file)}) - Not in whitelist.");
                    }
                    i++;
                }
            }
            catch (Exception ex)
            {
                Log($"Error accessing directories: {ex.Message}");
            }

            return await ProcessCandidatesBatchAsync(candidates, existingGames, metadataService, ct);
        }

        private async Task<int> ProcessCandidatesBatchAsync(List<GameCandidate> candidates, List<Game> existingGames, GameMetadataService metadataService, System.Threading.CancellationToken ct)
        {
            int added = 0;
            const int batchSize = 5; // Reduced for quicker feedback
            const int delayMs = 2500; // Conservative delay to stay well under 4 req/s

            Log($"Processing {candidates.Count} candidates in batches of {batchSize}...");

            for (int i = 0; i < candidates.Count; i += batchSize)
            {
                ct.ThrowIfCancellationRequested();
                
                var batch = candidates.Skip(i).Take(batchSize).ToList();
                Log($"[Scanner] Processing batch {i / batchSize + 1}/{Math.Ceiling((double)candidates.Count / batchSize)}. Size: {batch.Count}");

                foreach (var candidate in batch)
                {
                    ct.ThrowIfCancellationRequested();
                    if (await ProcessPotentialGame(candidate.Title, existingGames, metadataService, candidate.Path, candidate.PlatformKey, candidate.Serial))
                    {
                        added++;
                    }
                }

                if (i + batchSize < candidates.Count)
                {
                    OnBatchFinished?.Invoke();
                    Log($"Waiting {delayMs}ms to respect API rate limit...");
                    await Task.Delay(delayMs, ct);
                }
            }
            
            // Final refresh after last batch
            OnBatchFinished?.Invoke();

            return added;
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

        private async Task<bool> ProcessPotentialGame(string gameTitle, List<Game> existingGames, GameMetadataService metadataService, string? localPath = null, string? platformKey = null, string? serial = null)
        {
            // 1. Check if a game with exactly this path already exists
            var existingByPath = existingGames.FirstOrDefault(g => g.Path == localPath);
            if (existingByPath != null)
            {
                Log($"Game at path '{localPath}' already exists in library: {existingByPath.Title}");
                return false;
            }

            // 2. Check if a game with this title already exists but at a different path
            // (Moved Game or different folder name but same game)
            var existingByTitle = existingGames.FirstOrDefault(g => g.Title.Equals(gameTitle, StringComparison.OrdinalIgnoreCase));
            if (existingByTitle != null)
            {
                Log($"Game '{gameTitle}' already exists with a different path. Updating path to: {localPath}");
                existingByTitle.Path = localPath;
                await _gameRepository.UpdateAsync(existingByTitle.Id, existingByTitle);
                return true;
            }

            try
            {
                Log($"Searching metadata for: {gameTitle}" + (serial != null ? $" (Serial: {serial})" : "") + (platformKey != null ? $" Platform: {platformKey}" : ""));
                var searchResults = await metadataService.SearchGamesAsync(gameTitle, platformKey, null, serial);
                
                if (searchResults != null && searchResults.Any())
                {
                    var gameData = searchResults.First();
                    Log($"Processing potential match: {gameData.Title} (IGDB: {gameData.IgdbId})");
                    
                    // 3. Verify if this IGDB ID is already in our database
                    if (gameData.IgdbId.HasValue)
                    {
                        var match = existingGames.FirstOrDefault(g => g.IgdbId == gameData.IgdbId);
                        if (match != null)
                        {
                            Log($"ID match found: {gameData.Title} (ID: {gameData.IgdbId}) is already in library as '{match.Title}'. Updating its path.");
                            match.Path = localPath;
                            await _gameRepository.UpdateAsync(match.Id, match);
                            return true;
                        }
                    }

                    if (gameData.IgdbId.HasValue)
                    {
                        var fullMetadata = await metadataService.GetGameMetadataAsync(gameData.IgdbId.Value);
                        if (fullMetadata != null)
                        {
                            fullMetadata.Path = localPath;
                            
                            // CRITICAL FIX: Set PlatformId to avoid Foreign Key constraint violation
                            // If platformKey is null or "default", assume PC (Id 1/6) via dynamic lookup
                            fullMetadata.PlatformId = await ResolvePlatformIdAsync(platformKey);

                            var newGame = await _gameRepository.AddAsync(fullMetadata);
                            existingGames.Add(newGame);
                            
                            Log($"Added new game: {newGame.Title} (Local ID: {newGame.Id}, Platform ID: {newGame.PlatformId})");
                            LastGameFound = newGame.Title;
                            GamesAddedCount++;
                            OnGameAdded?.Invoke(newGame);
                            return true;
                        }
                        else
                        {
                            Log($"CRITICAL: Metadata fetch returned null for ID {gameData.IgdbId.Value} (Game: {gameData.Title}). Check IgdbClient logs.");
                        }
                    }
                }
                else
                {
                    Log($"No metadata found for: {gameTitle}");
                }
            }
            catch (Exception ex)
            {
                Log($"Error processing {gameTitle}: {ex.Message}");
            }
            return false;
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
