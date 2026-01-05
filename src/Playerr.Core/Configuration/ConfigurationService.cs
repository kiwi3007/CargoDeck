using System;
using System.Text.Json;
using System.IO;
using Playerr.Core.Prowlarr;
using Playerr.Core.Jackett;
using Playerr.Core.MetadataSource.Igdb;
using System.Diagnostics.CodeAnalysis;

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

        public ConfigurationService(string contentRoot)
        {
            // Determine config directory logic:
            // 1. Check for 'config' folder in contentRoot (Portable Mode)
            // 2. If not found, use %AppData%/Playerr/config (Installed Mode - Windows specific)
            
            var localConfig = Path.Combine(contentRoot, "config");
            
            if (Directory.Exists(localConfig))
            {
                _configDirectory = localConfig;
            }
            else
            {
                // Fallback to AppData for installed versions
                var appData = Environment.GetFolderPath(Environment.SpecialFolder.ApplicationData);
                if (!string.IsNullOrEmpty(appData))
                {
                    _configDirectory = Path.Combine(appData, "Playerr", "config");
                }
                else
                {
                    // Fallback to local if AppData is not available (e.g. non-Windows or weird environment)
                    _configDirectory = localConfig;
                }
            }

            _prowlarrConfigFile = Path.Combine(_configDirectory, "prowlarr.json");
            _jackettConfigFile = Path.Combine(_configDirectory, "jackett.json");
            _igdbConfigFile = Path.Combine(_configDirectory, "igdb.json");
            _downloadClientsConfigFile = Path.Combine(_configDirectory, "downloadclients.json");
            _mediaConfigFile = Path.Combine(_configDirectory, "media.json");
            _steamConfigFile = Path.Combine(_configDirectory, "steam.json");
            
            // Ensure config directory exists
            try 
            {
                Directory.CreateDirectory(_configDirectory);
            }
            catch (Exception ex)
            {
                Console.WriteLine($"Critical Error: Could not create config directory at {_configDirectory}. Details: {ex.Message}");
                // If we fail here, the app will likely crash, but at least we logged it.
            }
        }

        public ProwlarrSettings LoadProwlarrSettings()
        {
            if (File.Exists(_prowlarrConfigFile))
            {
                try
                {
                    var json = File.ReadAllText(_prowlarrConfigFile);
                    var settings = JsonSerializer.Deserialize<ProwlarrSettings>(json, new JsonSerializerOptions { PropertyNameCaseInsensitive = true });
                    if (settings != null)
                    {
                        return settings;
                    }
                }
                catch (Exception ex)
                {
                    Console.WriteLine($"Error loading Prowlarr settings: {ex.Message}");
                }
            }
            
            // Return default settings
            return new ProwlarrSettings
            {
                Url = "http://localhost:9696",
                ApiKey = ""
            };
        }

        public void SaveProwlarrSettings(ProwlarrSettings settings)
        {
            try
            {
                var json = JsonSerializer.Serialize(settings, new JsonSerializerOptions { WriteIndented = true });
                File.WriteAllText(_prowlarrConfigFile, json);
            }
            catch (Exception ex)
            {
                Console.WriteLine($"Error saving Prowlarr settings: {ex.Message}");
            }
        }

        public JackettSettings LoadJackettSettings()
        {
            if (File.Exists(_jackettConfigFile))
            {
                try
                {
                    var json = File.ReadAllText(_jackettConfigFile);
                    var settings = JsonSerializer.Deserialize<JackettSettings>(json, new JsonSerializerOptions { PropertyNameCaseInsensitive = true });
                    if (settings != null)
                    {
                        return settings;
                    }
                }
                catch (Exception ex)
                {
                    Console.WriteLine($"Error loading Jackett settings: {ex.Message}");
                }
            }
            
            // Return default settings
            return new JackettSettings
            {
                Url = "http://localhost:9117",
                ApiKey = ""
            };
        }

        public void SaveJackettSettings(JackettSettings settings)
        {
            try
            {
                var json = JsonSerializer.Serialize(settings, new JsonSerializerOptions { WriteIndented = true });
                File.WriteAllText(_jackettConfigFile, json);
            }
            catch (Exception ex)
            {
                Console.WriteLine($"Error saving Jackett settings: {ex.Message}");
            }
        }

        public IgdbSettings LoadIgdbSettings()
        {
            if (File.Exists(_igdbConfigFile))
            {
                try
                {
                    var json = File.ReadAllText(_igdbConfigFile);
                    var settings = JsonSerializer.Deserialize<IgdbSettings>(json, new JsonSerializerOptions { PropertyNameCaseInsensitive = true });
                    if (settings != null)
                    {
                        return settings;
                    }
                }
                catch (Exception ex)
                {
                    Console.WriteLine($"Error loading IGDB settings: {ex.Message}");
                }
            }
            
            // Try environment variables as fallback
            var clientId = Environment.GetEnvironmentVariable("IGDB_CLIENT_ID") ?? string.Empty;
            var clientSecret = Environment.GetEnvironmentVariable("IGDB_CLIENT_SECRET") ?? string.Empty;
            
            return new IgdbSettings
            {
                ClientId = clientId,
                ClientSecret = clientSecret
            };
        }

        public void SaveIgdbSettings(IgdbSettings settings)
        {
            try
            {
                var json = JsonSerializer.Serialize(settings, new JsonSerializerOptions { WriteIndented = true });
                File.WriteAllText(_igdbConfigFile, json);
            }
            catch (Exception ex)
            {
                Console.WriteLine($"Error saving IGDB settings: {ex.Message}");
            }
        }

        public System.Collections.Generic.List<Playerr.Core.Download.DownloadClient> LoadDownloadClients()
        {
            if (File.Exists(_downloadClientsConfigFile))
            {
                try
                {
                    var json = File.ReadAllText(_downloadClientsConfigFile);
                    var clients = JsonSerializer.Deserialize<System.Collections.Generic.List<Playerr.Core.Download.DownloadClient>>(json, new JsonSerializerOptions { PropertyNameCaseInsensitive = true });
                    if (clients != null)
                    {
                        return clients;
                    }
                }
                catch (Exception ex)
                {
                    Console.WriteLine($"Error loading download clients: {ex.Message}");
                }
            }
            
            return new System.Collections.Generic.List<Playerr.Core.Download.DownloadClient>();
        }

        public void SaveDownloadClients(System.Collections.Generic.List<Playerr.Core.Download.DownloadClient> clients)
        {
            try
            {
                var json = JsonSerializer.Serialize(clients, new JsonSerializerOptions { WriteIndented = true });
                File.WriteAllText(_downloadClientsConfigFile, json);
            }
            catch (Exception ex)
            {
                Console.WriteLine($"Error saving download clients: {ex.Message}");
            }
        }

        public MediaSettings LoadMediaSettings()
        {
            if (File.Exists(_mediaConfigFile))
            {
                try
                {
                    var json = File.ReadAllText(_mediaConfigFile);
                    var settings = JsonSerializer.Deserialize<MediaSettings>(json, new JsonSerializerOptions { PropertyNameCaseInsensitive = true });
                    if (settings != null)
                    {
                        return settings;
                    }
                }
                catch (Exception ex)
                {
                    Console.WriteLine($"Error loading media settings: {ex.Message}");
                }
            }
            
            return new MediaSettings();
        }

        public void SaveMediaSettings(MediaSettings settings)
        {
            try
            {
                var json = JsonSerializer.Serialize(settings, new JsonSerializerOptions { WriteIndented = true });
                File.WriteAllText(_mediaConfigFile, json);
            }
            catch (Exception ex)
            {
                Console.WriteLine($"Error saving media settings: {ex.Message}");
            }
        }

        public SteamSettings LoadSteamSettings()
        {
            if (File.Exists(_steamConfigFile))
            {
                try
                {
                    var json = File.ReadAllText(_steamConfigFile);
                    var settings = JsonSerializer.Deserialize<SteamSettings>(json, new JsonSerializerOptions { PropertyNameCaseInsensitive = true });
                    if (settings != null)
                    {
                        return settings;
                    }
                }
                catch (Exception ex)
                {
                    Console.WriteLine($"Error loading Steam settings: {ex.Message}");
                }
            }
            
            return new SteamSettings();
        }

        public void SaveSteamSettings(SteamSettings settings)
        {
            try
            {
                var json = JsonSerializer.Serialize(settings, new JsonSerializerOptions { WriteIndented = true });
                File.WriteAllText(_steamConfigFile, json);
            }
            catch (Exception ex)
            {
                Console.WriteLine($"Error saving Steam settings: {ex.Message}");
            }
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
}