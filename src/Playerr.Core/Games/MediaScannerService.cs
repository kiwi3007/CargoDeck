using System;
using System.Collections.Generic;
using System.IO;
using System.Linq;
using System.Threading.Tasks;
using System.Text.RegularExpressions;
using Playerr.Core.Configuration;
using Playerr.Core.MetadataSource;

namespace Playerr.Core.Games
{
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
            ["default"] = new() { Extensions = null, IsFolderMode = false } // Special case: All extensions
        };

        private static string[] _allExtensions = _platformRules.Values
            .Where(r => r.Extensions != null)
            .SelectMany(r => r.Extensions)
            .Distinct()
            .ToArray();

        // 2. Exclusion Rules (Global Blacklist)
        private static readonly HashSet<string> _globalBlacklist = new(StringComparer.OrdinalIgnoreCase)
        {
            ".nfo", ".txt", ".url", ".website", ".html", ".md",
            ".sfv", ".md5", ".sha1",
            ".jpg", ".png", ".jpeg", ".bmp",
            ".ds_store", ".db"
        };

        private class PlatformRule
        {
            public string[] Extensions { get; set; } = Array.Empty<string>();
            public bool IsFolderMode { get; set; }
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
            int added = 0;
            var directories = Directory.GetDirectories(rootPath);

            foreach (var dir in directories)
            {
                ct.ThrowIfCancellationRequested();
                var folderName = Path.GetFileName(dir);
                if (DirectoryContainsValidFile(dir, rule.Extensions))
                {
                    var (cleanName, serial) = CleanGameTitle(folderName);
                    if (await ProcessPotentialGame(cleanName, existingGames, metadataService, dir, platformKey, serial))
                    {
                        added++;
                    }
                }
            }
            return added;
        }

        private bool DirectoryContainsValidFile(string dirPath, string[] extensions)
        {
            try
            {
                var files = Directory.GetFiles(dirPath);
                foreach (var file in files)
                {
                    var ext = Path.GetExtension(file);
                    if (_globalBlacklist.Contains(ext)) continue;
                    
                    if (extensions.Contains(ext, StringComparer.OrdinalIgnoreCase))
                    {
                        return true;
                    }
                    if (Path.GetFileName(file).Equals("eboot.bin", StringComparison.OrdinalIgnoreCase))
                    {
                        return true;
                    }
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
            int added = 0;
            var extensionsToUse = platformKey == "default" ? _allExtensions : rule.Extensions;
            Log($"Scanning (File Mode) Root: {rootPath}. Valid Extensions: {(extensionsToUse != null ? string.Join(", ", extensionsToUse) : "ALL")}");

            try
            {
                // Recursive scan for all files
                var files = Directory.EnumerateFiles(rootPath, "*.*", SearchOption.AllDirectories);
                
                foreach (var file in files)
                {
                    ct.ThrowIfCancellationRequested();
                    
                    if (IsValidFile(file, extensionsToUse))
                    {
                        var name = Path.GetFileNameWithoutExtension(file);
                        Log($"Found valid file: {file}");
                        
                        // Smart platform detection for each file in universal mode
                        string finalPlatformKey = platformKey;
                        if (platformKey == "default")
                        {
                            finalPlatformKey = GetPlatformFromExtension(Path.GetExtension(file));
                        }

                        var (cleanName, serial) = CleanGameTitle(name);
                        // Don't log clean name if it's the same to keep log clean
                        if(cleanName != name) Log($"Cleaned Name: '{cleanName}'" + (serial != null ? $" (Serial: {serial})" : ""));

                        if (await ProcessPotentialGame(cleanName, existingGames, metadataService, file, finalPlatformKey, serial))
                        {
                            added++;
                            Log($"Successfully added: {cleanName} ({finalPlatformKey})");
                        }
                        else
                        {
                            Log($"Skipped/Failed to process: {cleanName}");
                        }
                    }
                }
            }
            catch (Exception ex)
            {
                Log($"Error accessing directories: {ex.Message}");
            }

            return added;
        }
        
        private void Log(string message)
        {
            try
            {
                // Simple debug logging to absolute path to be sure
                string logPath = "/Users/imaik/Documents/Playerr/Proyecto/scanner_log.txt";
                File.AppendAllText(logPath, $"{DateTime.Now}: {message}\n");
                Console.WriteLine(message);
            }
            catch { }
        }

        private bool IsValidFile(string filePath, string[] validExtensions)
        {
            var ext = Path.GetExtension(filePath);
            if (string.IsNullOrEmpty(ext)) return false;
            // Case-insensitive check
            return validExtensions.Contains(ext, StringComparer.OrdinalIgnoreCase) && !_globalBlacklist.Contains(ext);
        }

        private async Task<bool> ProcessPotentialGame(string gameTitle, List<Game> existingGames, GameMetadataService metadataService, string localPath = null, string platformKey = null, string serial = null)
        {
            if (existingGames.Any(g => g.Title.Equals(gameTitle, StringComparison.OrdinalIgnoreCase)))
            {
                Log($"Game already exists in DB: {gameTitle}");
                return false;
            }

            try
            {
                Log($"Searching metadata for: {gameTitle}" + (serial != null ? $" (Serial: {serial})" : "") + (platformKey != null ? $" Platform: {platformKey}" : ""));
                var searchResults = await metadataService.SearchGamesAsync(gameTitle, platformKey, null, serial);
                
                if (searchResults != null && searchResults.Any())
                {
                    var gameData = searchResults.First();
                    
                    // Debug Log
                    Log($"Processing potential match: {gameData.Title} (IGDB: {gameData.IgdbId})");
                    
                    // NEW CHECK: Verify if this IGDB ID is already in our database
                    if (gameData.IgdbId.HasValue)
                    {
                        var match = existingGames.FirstOrDefault(g => g.IgdbId == gameData.IgdbId);
                        if (match != null)
                        {
                            Log($"DUPLICATE DETECTED: {gameData.Title} (ID: {gameData.IgdbId}) matches existing game '{match.Title}' (ID: {match.Id}). SKIPPING.");
                            return false;
                        }
                    }

                    if (gameData.IgdbId.HasValue)
                    {
                        var fullMetadata = await metadataService.GetGameMetadataAsync(gameData.IgdbId.Value);
                        if (fullMetadata != null)
                        {
                            // CRITICAL: Set the local path so the frontend knows where this game is (and can filter by .nsp)
                            fullMetadata.Path = localPath;
                            
                            var newGame = await _gameRepository.AddAsync(fullMetadata);
                            // CRITICAL: Update the local list so subsequent files in this SAME scan don't add it again
                            existingGames.Add(newGame);
                            
                            Log($"Added new game: {newGame.Title} (Local ID: {newGame.Id})");
                            LastGameFound = newGame.Title;
                            GamesAddedCount++;
                            OnGameAdded?.Invoke(newGame);
                            return true;
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

        private (string Title, string Serial) CleanGameTitle(string title)
        {
            if (string.IsNullOrWhiteSpace(title)) return (title, null);
            
            string serial = null;
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
                "FitGirl", "DODI", "EMPRESS"
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

            // 7. Remove extra spaces and trim
            title = Regex.Replace(title, @"\s+", " ").Trim();

            return (title, serial);
        }
    }
}
