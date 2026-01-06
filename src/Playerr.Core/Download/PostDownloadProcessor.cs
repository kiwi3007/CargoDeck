using System;
using System.IO;
using System.IO.Compression;
using System.Linq;
using System.Collections.Generic;
using Playerr.Core.Configuration;
using Playerr.Core.IO;
using Playerr.Core.Games;
using System.Diagnostics.CodeAnalysis;
using SharpCompress.Archives;
using SharpCompress.Archives.Rar;
using SharpCompress.Common;

using Playerr.Core.MetadataSource;

namespace Playerr.Core.Download
{
    [SuppressMessage("Microsoft.Design", "CA1031:DoNotCatchGeneralExceptionTypes")]
    [SuppressMessage("Microsoft.Performance", "CA1822:MarkMembersAsStatic")]
    public class PostDownloadProcessor
    {
        private readonly ConfigurationService _configService;
        private readonly IFileMoverService _fileMover;
        private readonly IGameRepository _gameRepository;

        private readonly IGameMetadataServiceFactory _metadataFactory;

        public PostDownloadProcessor(
            ConfigurationService configService,
            IFileMoverService fileMover,
            IGameRepository gameRepository,
            IGameMetadataServiceFactory metadataFactory)
        {
            _configService = configService;
            _fileMover = fileMover;
            _gameRepository = gameRepository;
            _metadataFactory = metadataFactory;
        }

        public async System.Threading.Tasks.Task ProcessCompletedDownloadAsync(DownloadStatus download)
        {
            if (string.IsNullOrEmpty(download.DownloadPath) || !Directory.Exists(download.DownloadPath))
            {
                if (!File.Exists(download.DownloadPath)) // Check file existence too
                {
                    Console.WriteLine($"[PostDownload] Skip: Path not found or empty for {download.Name}");
                    return;
                }
            }

            var settings = _configService.LoadPostDownloadSettings();
            Console.WriteLine($"[PostDownload] Processing completed download: {download.Name} at {download.DownloadPath}");

            // 1. Auto-Extract
            if (settings.EnableAutoExtract && Directory.Exists(download.DownloadPath))
            {
                ExtractArchives(download.DownloadPath);
                ExtractRarArchives(download.DownloadPath);
            }

            // 2. Deep Clean
            if (settings.EnableDeepClean && Directory.Exists(download.DownloadPath))
            {
                DeepClean(download.DownloadPath, settings.UnwantedExtensions);
            }

            // 3. Auto-Move / Import
            if (settings.EnableAutoMove)
            {
                await AutoMoveToLibrary(download);
            }
        }

        private void ExtractArchives(string path)
        {
            var archives = Directory.EnumerateFiles(path, "*.*", SearchOption.AllDirectories)
                .Where(f => f.EndsWith(".zip", StringComparison.OrdinalIgnoreCase));

            foreach (var archive in archives)
            {
                try
                {
                    Console.WriteLine($"[PostDownload] Extracting: {archive}");
                    ZipFile.ExtractToDirectory(archive, path, overwriteFiles: true);
                }
                catch (Exception ex)
                {
                    Console.WriteLine($"[PostDownload] Extraction failed for {archive}: {ex.Message}");
                }
            }
        }

        private void ExtractRarArchives(string path)
        {
            var archives = Directory.EnumerateFiles(path, "*.*", SearchOption.AllDirectories)
                .Where(f => f.EndsWith(".rar", StringComparison.OrdinalIgnoreCase));

            foreach (var archivePath in archives)
            {
                try
                {
                    // Check if it's a multi-part RAR (part01.rar, etc) and only extract the first one to avoid redundancy
                    if (IsMultiPartNotFirst(archivePath)) continue;

                    Console.WriteLine($"[PostDownload] Extracting RAR: {archivePath}");
                    using (var archive = RarArchive.Open(archivePath))
                    {
                        foreach (var entry in archive.Entries.Where(entry => !entry.IsDirectory))
                        {
                            entry.WriteToDirectory(path, new ExtractionOptions()
                            {
                                ExtractFullPath = true,
                                Overwrite = true
                            });
                        }
                    }
                }
                catch (Exception ex)
                {
                    Console.WriteLine($"[PostDownload] RAR Extraction failed for {archivePath}: {ex.Message}");
                }
            }
        }

        private bool IsMultiPartNotFirst(string path)
        {
            // Simple heuristic for partNN.rar
            // If it ends in part02.rar ... part99.rar, skip it. Only process part01.rar or .rar
            // Or .r00, .r01 legacy style.
            
            var name = Path.GetFileName(path).ToLower();
            if (System.Text.RegularExpressions.Regex.IsMatch(name, @"\.part[0-9]{2,}\.rar$"))
            {
                 // Check if it is part01 or part1
                 if (!name.EndsWith("part01.rar") && !name.EndsWith("part1.rar")) return true;
            }
            return false;
        }

        private void DeepClean(string path, List<string> unwantedExtensions)
        {
            var files = Directory.EnumerateFiles(path, "*.*", SearchOption.AllDirectories);
            foreach (var file in files)
            {
                var ext = Path.GetExtension(file).ToLower();
                if (unwantedExtensions.Contains(ext))
                {
                    try
                    {
                        Console.WriteLine($"[PostDownload] Deleting unwanted file: {file}");
                        File.Delete(file);
                    }
                    catch { }
                }
            }
        }

        private async System.Threading.Tasks.Task AutoMoveToLibrary(DownloadStatus download)
        {
            var mediaSettings = _configService.LoadMediaSettings();
            var libraryRoot = !string.IsNullOrEmpty(mediaSettings.DestinationPath) && Directory.Exists(mediaSettings.DestinationPath)
                ? mediaSettings.DestinationPath 
                : mediaSettings.FolderPath;

            if (string.IsNullOrEmpty(libraryRoot) || !Directory.Exists(libraryRoot))
            {
                Console.WriteLine("[PostDownload] Skip Auto-Move: Library path not configured.");
                return;
            }

            var validExtensions = new[] { ".nsp", ".xci", ".pkg", ".iso", ".exe", ".zip", ".rar", ".7z" };
            bool isDirectory = Directory.Exists(download.DownloadPath);
            
            // Resolve clean name via IGDB
            string containerName = download.Name; // Fallback
            var cleanName = CleanReleaseName(download.Name);
            bool shouldNest = false; // Only nest if we resolve a clean name

            Console.WriteLine($"[PostDownload] Resolving game name for: '{cleanName}' (Original: '{download.Name}')");

            try 
            {
                var metadataService = _metadataFactory.CreateService();
                var searchResults = await metadataService.SearchGamesAsync(cleanName);
                if (searchResults.Any())
                {
                    containerName = SanitizeFileName(searchResults.First().Title);
                    shouldNest = true;
                    Console.WriteLine($"[PostDownload] Resolved game name: '{download.Name}' -> '{containerName}'");
                }
                else
                {
                    Console.WriteLine($"[PostDownload] No match found for '{cleanName}'. Using original name.");
                    // Still sanitize the original name just in case
                    containerName = SanitizeFileName(isDirectory ? new DirectoryInfo(download.DownloadPath!).Name : download.Name);
                }
            }
            catch (Exception ex)
            {
                Console.WriteLine($"[PostDownload] Error resolving game name: {ex.Message}. Using original.");
                containerName = SanitizeFileName(isDirectory ? new DirectoryInfo(download.DownloadPath!).Name : download.Name);
            }
            
            if (isDirectory)
            {
                var files = Directory.GetFiles(download.DownloadPath!, "*.*", SearchOption.AllDirectories);
                bool hasGameFile = files.Any(f => validExtensions.Contains(Path.GetExtension(f).ToLower()));

                if (!hasGameFile)
                {
                    Console.WriteLine($"[PostDownload] No valid game files found in {download.DownloadPath}");
                    return;
                }

                var originalFolderName = new DirectoryInfo(download.DownloadPath!).Name;

                foreach (var file in files)
                {
                    var relativePath = Path.GetRelativePath(download.DownloadPath!, file);
                    string destPath;
                    
                    if (shouldNest)
                    {
                        // Library/CleanName/OriginalReleaseName/File
                        destPath = Path.Combine(libraryRoot, containerName, originalFolderName, relativePath);
                    }
                    else
                    {
                        // Library/OriginalName/File (Fallback)
                        destPath = Path.Combine(libraryRoot, containerName, relativePath);
                    }

                    Console.WriteLine($"[PostDownload] Moving to library: {relativePath} -> {destPath}");
                    Directory.CreateDirectory(Path.GetDirectoryName(destPath)!);
                    
                    if (_fileMover.ImportFile(file, destPath)) { }
                }
            }
            else if (File.Exists(download.DownloadPath))
            {
                var file = download.DownloadPath!;
                if (validExtensions.Contains(Path.GetExtension(file).ToLower()))
                {
                    // For single files, we put them directly in the container or maybe nest?
                    // Usually single files don't have a "Release Name" folder structure, so putting in container is safer.
                    var destPath = Path.Combine(libraryRoot, containerName, Path.GetFileName(file));
                    Console.WriteLine($"[PostDownload] Moving to library: {Path.GetFileName(file)} -> {destPath}");
                    Directory.CreateDirectory(Path.GetDirectoryName(destPath)!); // Ensure container dir exists
                    _fileMover.ImportFile(file, destPath);
                }
            }
        }

        private string CleanReleaseName(string input)
        {
            if (string.IsNullOrEmpty(input)) return string.Empty;
            
            // Remove content in brackets [] and parenthesis ()
            var cleaned = System.Text.RegularExpressions.Regex.Replace(input, @"\[.*?\]|\(.*?\)", "");
            
            // Remove common release tags (simplified list)
            var tags = new[] { "repack", "multi", "goty", "iso", "v1.0", "update" };
            foreach (var tag in tags)
            {
                 cleaned = System.Text.RegularExpressions.Regex.Replace(cleaned, tag, "", System.Text.RegularExpressions.RegexOptions.IgnoreCase);
            }
            
            // Remove extra spaces and dots
            cleaned = cleaned.Replace(".", " ").Trim();
            // Collapse multiple spaces
            cleaned = System.Text.RegularExpressions.Regex.Replace(cleaned, @"\s+", " ");
            
            return cleaned;
        }

        private string SanitizeFileName(string name)
        {
             var invalidChars = Path.GetInvalidFileNameChars();
             return string.Join("_", name.Split(invalidChars, StringSplitOptions.RemoveEmptyEntries)).Trim();
        }
    }
}
