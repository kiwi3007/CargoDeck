using Microsoft.AspNetCore.Mvc;
using Playerr.Core.Prowlarr;
using System;
using System.Net.Http;
using System.Threading.Tasks;
using System.Diagnostics.CodeAnalysis;

namespace Playerr.Api.V3.Debug
{
    [ApiController]
    [Route("api/v3/debug")]
    [SuppressMessage("Microsoft.Design", "CA1031:DoNotCatchGeneralExceptionTypes")]
    public class DebugController : ControllerBase
    {
        private readonly ProwlarrSettings _prowlarrSettings;

        public DebugController(ProwlarrSettings prowlarrSettings)
        {
            _prowlarrSettings = prowlarrSettings;
        }

        [HttpGet("prowlarr-raw")]
        public async Task<IActionResult> GetProwlarrRaw([FromQuery] string query = "game")
        {
            if (!_prowlarrSettings.IsConfigured)
            {
                return BadRequest("Prowlarr not configured");
            }

            try
            {
                using var httpClient = new HttpClient { BaseAddress = new Uri(_prowlarrSettings.Url) };
                using var request = new HttpRequestMessage(HttpMethod.Get, $"/api/v1/search?query={Uri.EscapeDataString(query)}");
                request.Headers.Add("X-Api-Key", _prowlarrSettings.ApiKey);

                var response = await httpClient.SendAsync(request);
                var content = await response.Content.ReadAsStringAsync();
                
                return Ok(new { 
                    StatusCode = (int)response.StatusCode,
                    ContentType = response.Content.Headers.ContentType?.ToString(),
                    RawContent = content,
                    ContentLength = content.Length
                });
            }
            catch (Exception ex)
            {
                return StatusCode(500, new { error = ex.Message });
            }
        }
    }
}