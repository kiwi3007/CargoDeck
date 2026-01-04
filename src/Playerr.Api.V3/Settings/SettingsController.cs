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

namespace Playerr.Api.V3.Settings
{
    [ApiController]
    [Route("api/v3/settings")]
    [Route("api/v3/metadata/igdb")]
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
            _prowlarrSettings.Url = request.Url;
            _prowlarrSettings.ApiKey = request.ApiKey;
            
            // Save to persistent storage
            _configService.SaveProwlarrSettings(request);

            return Ok(new { success = true });
        }

        [HttpGet("prowlarr")]
        public ActionResult<ProwlarrSettings> GetProwlarrSettings()
        {
            return Ok(_prowlarrSettings);
        }

        [HttpPost("jackett")]
        public IActionResult SaveJackettSettings([FromBody] JackettSettings request)
        {
            _jackettSettings.Url = request.Url;
            _jackettSettings.ApiKey = request.ApiKey;
            
            // Save to persistent storage
            _configService.SaveJackettSettings(request);

            return Ok(new { success = true });
        }

        [HttpGet("jackett")]
        public ActionResult<JackettSettings> GetJackettSettings()
        {
            return Ok(_jackettSettings);
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
            // Temporarily save to config so client can pick it up? 
            // Better to pass creds to client methods, but Client reads from config.
            // Let's save temporarily or assume user saved first. 
            // The UI flow: User types, hits Test. We should probably NOT save if just testing, 
            // but our SteamClient architecture unfortunately reads from ConfigService.LoadSteamSettings().
            // Ideally we'd pass api key to methods. 
            // Workaround: We will rely on user hitting "Save" first or saving implicitly.
            // Or we can save here if that's acceptable UI behavior (usually "Test" is non-destructive, but saving config is fine).
            // Let's UPDATE the config first so the client sees the values being tested.
            _configService.SaveSteamSettings(request);

            try
            {
                var profile = await _steamClient.GetPlayerSummariesAsync(request.SteamId);
                var userName = profile?.PersonaName ?? "Unknown";
                return Ok(new { success = true, message = $"Connected as {userName}", userName = userName });
            }
            catch (System.Exception ex)
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

                var steamGames = await _steamClient.GetOwnedGamesAsync(settings.SteamId);
                var existingGames = await _gameRepository.GetAllAsync();
                
                int addedCount = 0;
                var metadataService = _metadataServiceFactory.CreateService();

                foreach (var game in steamGames)
                {
                    // Check if exists by SteamId or Title
                    if (!existingGames.Any(g => g.SteamId == game.SteamId || 
                                                (g.Title.Equals(game.Title, System.StringComparison.OrdinalIgnoreCase))))
                    {
                        // Enrich with IGDB Metadata
                        try 
                        {
                            var searchResults = await metadataService.SearchGamesAsync(game.Title);
                            var match = searchResults.FirstOrDefault();
                            
                            if (match != null)
                            {
                                game.IgdbId = match.IgdbId;
                                game.Overview = match.Overview;
                                game.Images = match.Images;
                                game.Genres = match.Genres;
                                game.Developer = match.Developer;
                                game.Publisher = match.Publisher;
                                game.ReleaseDate = match.ReleaseDate;
                                game.Year = match.Year;
                                game.Rating = match.Rating;
                            }
                        }
                        catch (System.Exception ex)
                        {
                            System.Console.WriteLine($"Failed to enrich metadata for {game.Title}: {ex.Message}");
                            // Continue adding the game even if metadata fails
                        }

                        await _gameRepository.AddAsync(game);
                        addedCount++;
                    }
                }

                return Ok(new { success = true, message = $"Synced {steamGames.Count} games. Added {addedCount} new games.", count = addedCount });
            }
            catch (System.Exception ex)
            {
                return BadRequest(new { success = false, message = ex.Message });
            }
        }
    }
}
