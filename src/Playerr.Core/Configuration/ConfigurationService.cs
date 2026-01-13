using System;
using System.Text.Json;
using System.IO;
using Playerr.Core.Prowlarr;
using Playerr.Core.Jackett;
using Playerr.Core.MetadataSource.Igdb;
using Playerr.Core.Download;
using System.Diagnostics.CodeAnalysis;
using System.Collections.Generic;

namespace Playerr.Core.Configuration
{
    [SuppressMessage("Microsoft.Design", "CA1031:DoNotCatchGeneralExceptionTypes")]
    [SuppressMessage("Microsoft.Design", "CA1002:DoNotExposeGenericLists")]
    [SuppressMessage("Microsoft.Performance", "CA1869:CacheAndReuseJsonSerializerOptions")]
    public class ConfigurationService
    {
        private readonly string _configDirectory;
        private readonly string _prowlarrConfigFile;
        private readonly string _jackettConfigFile;
        private readonly string _igdbConfigFile;
        private readonly string _downloadClientsConfigFile;
        private readonly string _mediaConfigFile;
        private readonly string _steamConfigFile;
        private readonly string _postDownloadConfigFile;
        private readonly string _hydraConfigFile;

        public ConfigurationService(string contentRoot)
        {
            var localConfig = Path.Combine(contentRoot, "config");
            
            if (Directory.Exists(localConfig))
            {
                _configDirectory = localConfig;
            }
            else
            {
                var appData = Environment.GetFolderPath(Environment.SpecialFolder.ApplicationData);
                if (!string.IsNullOrEmpty(appData))
                {
                    _configDirectory = Path.Combine(appData, "Playerr", "config");
                }
                else
                {
                    _configDirectory = localConfig;
                }
            }

            _prowlarrConfigFile = Path.Combine(_configDirectory, "prowlarr.json");
            _jackettConfigFile = Path.Combine(_configDirectory, "jackett.json");
            _igdbConfigFile = Path.Combine(_configDirectory, "igdb.json");
            _downloadClientsConfigFile = Path.Combine(_configDirectory, "downloadclients.json");
            _mediaConfigFile = Path.Combine(_configDirectory, "media.json");
            _steamConfigFile = Path.Combine(_configDirectory, "steam.json");
            _postDownloadConfigFile = Path.Combine(_configDirectory, "postdownload.json");
            _hydraConfigFile = Path.Combine(_configDirectory, "hydra.json");
            
            try 
            {
                Directory.CreateDirectory(_configDirectory);
                Console.WriteLine($"[Configuration] Service initialized. Using Config Directory: {_configDirectory}");
            }
            catch (Exception ex)
            {
                Console.WriteLine($"Critical Error: Could not create config directory at {_configDirectory}. Details: {ex.Message}");
            }
        }

        public ProwlarrSettings LoadProwlarrSettings()
        {
            if (File.Exists(_prowlarrConfigFile))
            {
                try
                {
                    var json = File.ReadAllText(_prowlarrConfigFile);
                    return JsonSerializer.Deserialize<ProwlarrSettings>(json, new JsonSerializerOptions { PropertyNameCaseInsensitive = true }) ?? new ProwlarrSettings { Url = string.Empty };
                }
                catch (Exception ex) { Console.WriteLine($"Error loading Prowlarr settings: {ex.Message}"); }
            }
            return new ProwlarrSettings { Url = string.Empty };
        }

        public void SaveProwlarrSettings(ProwlarrSettings settings)
        {
            try { File.WriteAllText(_prowlarrConfigFile, JsonSerializer.Serialize(settings, new JsonSerializerOptions { WriteIndented = true })); }
            catch (Exception ex) { Console.WriteLine($"Error saving Prowlarr settings: {ex.Message}"); }
        }

        public JackettSettings LoadJackettSettings()
        {
            if (File.Exists(_jackettConfigFile))
            {
                try
                {
                    var json = File.ReadAllText(_jackettConfigFile);
                    return JsonSerializer.Deserialize<JackettSettings>(json, new JsonSerializerOptions { PropertyNameCaseInsensitive = true }) ?? new JackettSettings { Url = string.Empty };
                }
                catch (Exception ex) { Console.WriteLine($"Error loading Jackett settings: {ex.Message}"); }
            }
            return new JackettSettings { Url = string.Empty };
        }

        public void SaveJackettSettings(JackettSettings settings)
        {
            try { File.WriteAllText(_jackettConfigFile, JsonSerializer.Serialize(settings, new JsonSerializerOptions { WriteIndented = true })); }
            catch (Exception ex) { Console.WriteLine($"Error saving Jackett settings: {ex.Message}"); }
        }

        public IgdbSettings LoadIgdbSettings()
        {
            if (File.Exists(_igdbConfigFile))
            {
                try
                {
                    var json = File.ReadAllText(_igdbConfigFile);
                    return JsonSerializer.Deserialize<IgdbSettings>(json, new JsonSerializerOptions { PropertyNameCaseInsensitive = true }) ?? new IgdbSettings();
                }
                catch (Exception ex) { Console.WriteLine($"Error loading IGDB settings: {ex.Message}"); }
            }
            return new IgdbSettings { ClientId = Environment.GetEnvironmentVariable("IGDB_CLIENT_ID") ?? "", ClientSecret = Environment.GetEnvironmentVariable("IGDB_CLIENT_SECRET") ?? "" };
        }

        public void SaveIgdbSettings(IgdbSettings settings)
        {
            try { File.WriteAllText(_igdbConfigFile, JsonSerializer.Serialize(settings, new JsonSerializerOptions { WriteIndented = true })); }
            catch (Exception ex) { Console.WriteLine($"Error saving IGDB settings: {ex.Message}"); }
        }

        public List<Playerr.Core.Download.DownloadClient> LoadDownloadClients()
        {
            if (File.Exists(_downloadClientsConfigFile))
            {
                try
                {
                    var json = File.ReadAllText(_downloadClientsConfigFile);
                    return JsonSerializer.Deserialize<List<Playerr.Core.Download.DownloadClient>>(json, new JsonSerializerOptions { PropertyNameCaseInsensitive = true }) ?? new List<Playerr.Core.Download.DownloadClient>();
                }
                catch (Exception ex) { Console.WriteLine($"Error loading download clients: {ex.Message}"); }
            }
            return new List<Playerr.Core.Download.DownloadClient>();
        }

        public void SaveDownloadClients(List<Playerr.Core.Download.DownloadClient> clients)
        {
            try { File.WriteAllText(_downloadClientsConfigFile, JsonSerializer.Serialize(clients, new JsonSerializerOptions { WriteIndented = true })); }
            catch (Exception ex) { Console.WriteLine($"Error saving download clients: {ex.Message}"); }
        }

        public MediaSettings LoadMediaSettings()
        {
            MediaSettings settings = new MediaSettings();
            if (File.Exists(_mediaConfigFile))
            {
                try
                {
                    var json = File.ReadAllText(_mediaConfigFile);
                    settings = JsonSerializer.Deserialize<MediaSettings>(json, new JsonSerializerOptions { PropertyNameCaseInsensitive = true }) ?? new MediaSettings();
                }
                catch (Exception ex) { Console.WriteLine($"Error loading media settings: {ex.Message}"); }
            }

            // Apply Defaults if paths are empty
            var userProfile = Environment.GetFolderPath(Environment.SpecialFolder.UserProfile);
            var documents = Path.Combine(userProfile, "Documents");
            var downloads = Path.Combine(userProfile, "Downloads");

            // Ensure base processing folder exists in Downloads
            var defaultDownloadPath = Path.Combine(downloads, "Playerr");
            
            // Ensure Library folder exists in Documents
            var defaultLibraryPath = Path.Combine(documents, "Playerr", "Library");
            var defaultGamesPath = Path.Combine(documents, "Playerr", "Games");

            if (string.IsNullOrWhiteSpace(settings.DownloadPath)) settings.DownloadPath = defaultDownloadPath;
            if (string.IsNullOrWhiteSpace(settings.DestinationPath)) settings.DestinationPath = defaultLibraryPath;
            if (string.IsNullOrWhiteSpace(settings.FolderPath)) settings.FolderPath = defaultGamesPath;
            
            // Create directories if they don't exist (UX convenience)
            try 
            {
                if (!Directory.Exists(settings.DownloadPath)) Directory.CreateDirectory(settings.DownloadPath);
                if (!Directory.Exists(settings.DestinationPath)) Directory.CreateDirectory(settings.DestinationPath);
                if (!Directory.Exists(settings.FolderPath)) Directory.CreateDirectory(settings.FolderPath);
            }
            catch (Exception ex)
            {
                Console.WriteLine($"[Config] Warning: Could not create default directories: {ex.Message}");
            }

            return settings;
        }

        public void SaveMediaSettings(MediaSettings settings)
        {
            try { File.WriteAllText(_mediaConfigFile, JsonSerializer.Serialize(settings, new JsonSerializerOptions { WriteIndented = true })); }
            catch (Exception ex) { Console.WriteLine($"Error saving media settings: {ex.Message}"); }
        }

        public SteamSettings LoadSteamSettings()
        {
            if (File.Exists(_steamConfigFile))
            {
                try
                {
                    var json = File.ReadAllText(_steamConfigFile);
                    return JsonSerializer.Deserialize<SteamSettings>(json, new JsonSerializerOptions { PropertyNameCaseInsensitive = true }) ?? new SteamSettings();
                }
                catch (Exception ex) { Console.WriteLine($"Error loading Steam settings: {ex.Message}"); }
            }
            return new SteamSettings();
        }

        public void SaveSteamSettings(SteamSettings settings)
        {
            try { File.WriteAllText(_steamConfigFile, JsonSerializer.Serialize(settings, new JsonSerializerOptions { WriteIndented = true })); }
            catch (Exception ex) { Console.WriteLine($"Error saving Steam settings: {ex.Message}"); }
        }

        public PostDownloadSettings LoadPostDownloadSettings()
        {
            if (File.Exists(_postDownloadConfigFile))
            {
                try
                {
                    var json = File.ReadAllText(_postDownloadConfigFile);
                    return JsonSerializer.Deserialize<PostDownloadSettings>(json, new JsonSerializerOptions { PropertyNameCaseInsensitive = true }) ?? new PostDownloadSettings();
                }
                catch { }
            }
            return new PostDownloadSettings();
        }

        public void SavePostDownloadSettings(PostDownloadSettings settings)
        {
            try { File.WriteAllText(_postDownloadConfigFile, JsonSerializer.Serialize(settings, new JsonSerializerOptions { WriteIndented = true })); }
            catch { }
        }

        public List<Playerr.Core.Indexers.HydraConfiguration> LoadHydraIndexers()
        {
            if (File.Exists(_hydraConfigFile))
            {
                try
                {
                    var json = File.ReadAllText(_hydraConfigFile);
                    return JsonSerializer.Deserialize<List<Playerr.Core.Indexers.HydraConfiguration>>(json, new JsonSerializerOptions { PropertyNameCaseInsensitive = true }) ?? new List<Playerr.Core.Indexers.HydraConfiguration>();
                }
                catch (Exception ex) { Console.WriteLine($"Error loading Hydra indexers: {ex.Message}"); }
            }
            return new List<Playerr.Core.Indexers.HydraConfiguration>();
        }

        public void SaveHydraIndexers(List<Playerr.Core.Indexers.HydraConfiguration> indexers)
        {
            try { File.WriteAllText(_hydraConfigFile, JsonSerializer.Serialize(indexers, new JsonSerializerOptions { WriteIndented = true })); }
            catch (Exception ex) { Console.WriteLine($"Error saving Hydra indexers: {ex.Message}"); }
        }
    }

    public class IgdbSettings
    {
        public string ClientId { get; set; } = string.Empty;
        public string ClientSecret { get; set; } = string.Empty;
        public bool IsConfigured => !string.IsNullOrWhiteSpace(ClientId) && !string.IsNullOrWhiteSpace(ClientSecret);
    }

    public class SteamSettings
    {
        public string ApiKey { get; set; } = string.Empty;
        public string SteamId { get; set; } = string.Empty;
        public bool IsConfigured => !string.IsNullOrWhiteSpace(ApiKey) && !string.IsNullOrWhiteSpace(SteamId);
    }

    public class PostDownloadSettings
    {
        public bool EnableAutoMove { get; set; } = true;
        public bool EnableAutoExtract { get; set; } = true;
        public bool EnableDeepClean { get; set; } = true;
        public bool EnableAutoRename { get; set; } = true;
        public int MonitorIntervalSeconds { get; set; } = 60;
        public List<string> UnwantedExtensions { get; set; } = new List<string> { ".txt", ".nfo", ".url" };
    }
}