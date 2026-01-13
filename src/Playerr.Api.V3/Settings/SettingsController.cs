using Microsoft.AspNetCore.Mvc;
using Playerr.Core.Prowlarr;
using Playerr.Core.Configuration;
using Playerr.Core.MetadataSource;
using Playerr.Core.MetadataSource.Igdb;
using Playerr.Core.Jackett;
using Playerr.Core.MetadataSource.Steam;
using Playerr.Core.Games;
using System.Linq;
using System.Threading.Tasks;
using System.Collections.Generic;
using System;
using System.Diagnostics.CodeAnalysis;

namespace Playerr.Api.V3.Settings
{
    [ApiController]
    [Route("api/v3/settings")]
    [Route("api/v3/metadata/igdb")]
    [SuppressMessage("Microsoft.Design", "CA1031:DoNotCatchGeneralExceptionTypes")]
    [SuppressMessage("Microsoft.Design", "CA1002:DoNotExposeGenericLists")]
    [SuppressMessage("Microsoft.Performance", "CA1869:CacheAndReuseJsonSerializerOptions")]
    public class SettingsController : ControllerBase
    {
        private readonly ProwlarrSettings _prowlarrSettings;
        private readonly JackettSettings _jackettSettings;
        private readonly ConfigurationService _configService;
        private readonly IGameMetadataServiceFactory _metadataServiceFactory;
        private readonly SteamClient _steamClient;
        private readonly IGameRepository _gameRepository;

        public SettingsController(
            ProwlarrSettings prowlarrSettings, 
            JackettSettings jackettSettings, 
            ConfigurationService configService, 
            IGameMetadataServiceFactory metadataServiceFactory,
            SteamClient steamClient,
            IGameRepository gameRepository)
        {
            _prowlarrSettings = prowlarrSettings;
            _jackettSettings = jackettSettings;
            _configService = configService;
            _metadataServiceFactory = metadataServiceFactory;
            _steamClient = steamClient;
            _gameRepository = gameRepository;
        }

        [HttpPost("prowlarr")]
        public IActionResult SaveProwlarrSettings([FromBody] ProwlarrSettings request)
        {
            // Update the injected singleton so other services see the change immediately
            _prowlarrSettings.Url = request.Url;
            _prowlarrSettings.ApiKey = request.ApiKey;
            _prowlarrSettings.Enabled = request.Enabled;
            
            Console.WriteLine($"[Settings] Saving Prowlarr Settings. ENABLED = {request.Enabled}");

            // Save to persistent storage
            _configService.SaveProwlarrSettings(request);

            return Ok(new { success = true });
        }

        [HttpGet("prowlarr")]
        public ActionResult<ProwlarrSettings> GetProwlarrSettings()
        {
            // Load directly from disk to ensure persistence is verified
            return Ok(_configService.LoadProwlarrSettings());
        }

        [HttpPost("jackett")]
        public IActionResult SaveJackettSettings([FromBody] JackettSettings request)
        {
            // Update the injected singleton so other services see the change immediately
            _jackettSettings.Url = request.Url;
            _jackettSettings.ApiKey = request.ApiKey;
            _jackettSettings.Enabled = request.Enabled;
            
            Console.WriteLine($"[Settings] Saving Jackett Settings. ENABLED = {request.Enabled}");

            // Save to persistent storage
            _configService.SaveJackettSettings(request);

            return Ok(new { success = true });
        }

        [HttpGet("jackett")]
        public ActionResult<JackettSettings> GetJackettSettings()
        {
            // Load directly from disk to ensure persistence is verified
            return Ok(_configService.LoadJackettSettings());
        }

        [HttpPost("/api/v3/metadata/igdb")]
        public IActionResult SaveIgdbSettings([FromBody] IgdbSettings request)
        {
            // Save to persistent storage
            _configService.SaveIgdbSettings(request);
            
            // Refresh the IGDB service with new configuration
            _metadataServiceFactory.RefreshConfiguration();

            return Ok(new { success = true, message = "IGDB settings saved and configuration refreshed." });
        }

        [HttpGet("igdb")]
        public ActionResult<IgdbSettings> GetIgdbSettings()
        {
            var settings = _configService.LoadIgdbSettings();
            return Ok(settings);
        }

        [HttpPost("steam")]
        public IActionResult SaveSteamSettings([FromBody] SteamSettings request)
        {
            // Save to persistent storage
            _configService.SaveSteamSettings(request);
            
            return Ok(new { success = true, message = "Steam settings saved." });
        }

        [HttpGet("steam")]
        public ActionResult<SteamSettings> GetSteamSettings()
        {
            var settings = _configService.LoadSteamSettings();
            return Ok(settings);
        }

        [HttpDelete("steam")]
        public async Task<IActionResult> DeleteSteamSettings()
        {
            var emptySettings = new SteamSettings { ApiKey = "", SteamId = "" };
            _configService.SaveSteamSettings(emptySettings);
            
            var deletedCount = await _gameRepository.DeleteSteamGamesAsync();
            return Ok(new { success = true, message = $"Steam settings cleared and {deletedCount} games removed." });
        }

        [HttpDelete("igdb")]
        public IActionResult DeleteIgdbSettings()
        {
            var emptySettings = new IgdbSettings { ClientId = "", ClientSecret = "" };
            _configService.SaveIgdbSettings(emptySettings);
            _metadataServiceFactory.RefreshConfiguration();
            return Ok(new { success = true, message = "IGDB settings cleared." });
        }

        [HttpPost("steam/test")]
        public async Task<IActionResult> TestSteamSettings([FromBody] SteamSettings request)
        {
            _configService.SaveSteamSettings(request);

            try
            {
                // Create a temporary client with the provided credentials to verify them
                var tempClient = new SteamClient(request.ApiKey);
                var profile = await tempClient.GetPlayerProfileAsync(request.SteamId);
                
                if (profile == null)
                {
                    return BadRequest(new { success = false, message = "Connection failed: Invalid API Key or Steam ID. Profile could not be retrieved." });
                }

                var userName = profile.PersonaName;
                return Ok(new { success = true, message = $"Connected as {userName}", userName = userName });
            }
            catch (Exception ex)
            {
                return BadRequest(new { success = false, message = ex.Message });
            }
        }

        [HttpPost("steam/sync")]
        public async Task<IActionResult> SyncSteamLibrary()
        {
            try
            {
                var settings = _configService.LoadSteamSettings();
                if (!settings.IsConfigured)
                    return BadRequest(new { success = false, message = "Steam not configured" });

                // Use a fresh client with the correct key (avoid DI stale key issues)
                var client = new SteamClient(settings.ApiKey);
                var steamGames = await client.GetOwnedGamesAsync(settings.SteamId);
                var existingGames = await _gameRepository.GetAllAsync();
                
                int addedCount = 0;
                var metadataService = _metadataServiceFactory.CreateService();

                foreach (var steamGame in steamGames)
                {
                    // Check if exists by SteamId or Title
                    if (!existingGames.Any(g => g.SteamId == steamGame.AppId || 
                                                (g.Title.Equals(steamGame.Name, StringComparison.OrdinalIgnoreCase))))
                    {
                        var newGame = new Game
                        {
                            Title = steamGame.Name,
                            SteamId = steamGame.AppId,
                            Added = DateTime.UtcNow,
                            Status = GameStatus.Announced, 
                            Monitored = true,
                            PlatformId = 6 // PC
                        };

                        // Enrich with IGDB Metadata
                        try 
                        {
                            var searchResults = await metadataService.SearchGamesAsync(steamGame.Name);
                            var match = searchResults.FirstOrDefault();
                            
                            if (match != null)
                            {
                                newGame.IgdbId = match.IgdbId;
                                newGame.Overview = match.Overview;
                                newGame.Images = match.Images;
                                newGame.Genres = match.Genres;
                                newGame.Developer = match.Developer;
                                newGame.Publisher = match.Publisher;
                                newGame.ReleaseDate = match.ReleaseDate;
                                newGame.Year = match.Year;
                                newGame.Rating = match.Rating;
                            }
                        }
                        catch (Exception)
                        {
                            // Ignore enrichment failures
                        }

                        await _gameRepository.AddAsync(newGame);
                        addedCount++;
                    }
                }

                return Ok(new { success = true, message = $"Synced {steamGames.Count} games. Added {addedCount} new games.", count = addedCount });
            }
            catch (Exception ex)
            {
                return BadRequest(new { success = false, message = ex.Message });
            }
        }
    }
}
