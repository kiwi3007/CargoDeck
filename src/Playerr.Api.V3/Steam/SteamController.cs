using Microsoft.AspNetCore.Mvc;
using System.Threading.Tasks;
using Playerr.Core.Configuration;
using Playerr.Core.MetadataSource.Steam;

namespace Playerr.Api.V3.Steam
{
    [ApiController]
    [Route("api/v3/steam")]
    public class SteamController : ControllerBase
    {
        private readonly ConfigurationService _configService;
        private readonly SteamClient _steamClient;

        public SteamController(ConfigurationService configService, SteamClient steamClient)
        {
            _configService = configService;
            _steamClient = steamClient;
        }

        [HttpGet("profile")]
        [ResponseCache(Duration = 0, Location = ResponseCacheLocation.None, NoStore = true)]
        public async Task<IActionResult> GetUserProfile()
        {
            try
            {
                var settings = _configService.LoadSteamSettings();
                if (!settings.IsConfigured)
                {
                    return BadRequest(new { success = false, message = "Steam not configured" });
                }

                var profile = await _steamClient.GetPlayerSummariesAsync(settings.SteamId);
                if (profile == null)
                {
                    return NotFound(new { success = false, message = "Steam profile not found or private." });
                }

                return Ok(profile);
            }
            catch (System.Exception ex)
            {
                return StatusCode(500, new { success = false, message = ex.Message });
            }
        }
        [HttpGet("stats")]
        [ResponseCache(Duration = 0, Location = ResponseCacheLocation.None, NoStore = true)]
        public async Task<IActionResult> GetLibraryStats()
        {
            try
            {
                var settings = _configService.LoadSteamSettings();
                if (!settings.IsConfigured)
                {
                    return BadRequest(new { success = false, message = "Steam not configured" });
                }

                var stats = await _steamClient.GetLibraryStatsAsync(settings.SteamId);
                return Ok(stats);
            }
            catch (System.Exception ex)
            {
                return StatusCode(500, new { success = false, message = ex.Message });
            }
        }
        [HttpGet("recent")]
        [ResponseCache(Duration = 0, Location = ResponseCacheLocation.None, NoStore = true)]
        public async Task<IActionResult> GetRecentGames()
        {
            try
            {
                var settings = _configService.LoadSteamSettings();
                if (!settings.IsConfigured)
                {
                    return BadRequest(new { success = false, message = "Steam not configured" });
                }

                // Ensure we have the resolved SteamID (though client methods resolve it too, optimization possible)
                var recentGames = await _steamClient.GetRecentlyPlayedGamesAsync(settings.SteamId);
                return Ok(recentGames);
            }
            catch (System.Exception ex)
            {
                return StatusCode(500, new { success = false, message = ex.Message });
            }
        }
    }
}
