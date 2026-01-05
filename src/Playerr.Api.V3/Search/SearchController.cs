using System;
using System.Collections.Generic;
using System.Linq;
using System.Threading.Tasks;
using Microsoft.AspNetCore.Mvc;
using Playerr.Core.Prowlarr;
using Playerr.Core.Jackett;

namespace Playerr.Api.V3.Search
{
    [ApiController]
    [Route("api/v3/[controller]")]
    public class SearchController : ControllerBase
    {
        private readonly ProwlarrSettings _prowlarrSettings;
        private readonly JackettSettings _jackettSettings;

        public SearchController(ProwlarrSettings prowlarrSettings, JackettSettings jackettSettings)
        {
            _prowlarrSettings = prowlarrSettings;
            _jackettSettings = jackettSettings;
        }

        [HttpGet]
        public async Task<ActionResult<List<SearchResult>>> Search([FromQuery] string query, [FromQuery] string? categories = null)
        {
            if (string.IsNullOrWhiteSpace(query))
            {
                return BadRequest("Query parameter is required");
            }

            if (!_prowlarrSettings.IsConfigured && !_jackettSettings.IsConfigured)
            {
                return StatusCode(500, new { error = "Neither Prowlarr nor Jackett is configured. Please set URL and API key in Settings." });
            }

            var results = new List<SearchResult>();
            var tasks = new List<Task<List<SearchResult>>>();

            int[]? categoryIds = null;
            if (!string.IsNullOrEmpty(categories))
            {
                categoryIds = categories.Split(',')
                                       .Select(c => int.TryParse(c, out var id) ? id : (int?)null)
                                       .Where(id => id.HasValue)
                                       .Select(id => id!.Value)
                                       .ToArray();
            }

            // Search Prowlarr
            if (_prowlarrSettings.IsConfigured)
            {
                var prowlarrClient = new ProwlarrClient(_prowlarrSettings.Url, _prowlarrSettings.ApiKey);
                tasks.Add(prowlarrClient.SearchAsync(query, null, categoryIds));
            }

            // Search Jackett
            if (_jackettSettings.IsConfigured)
            {
                var jackettClient = new JackettClient(_jackettSettings.Url, _jackettSettings.ApiKey);
                tasks.Add(jackettClient.SearchAsync(query, categoryIds));
            }

            try
            {
                var searchTasks = await Task.WhenAll(tasks);
                foreach (var taskResults in searchTasks)
                {
                    results.AddRange(taskResults);
                    Console.WriteLine($"[SearchController] Added {taskResults.Count} results from provider. Protocols: {string.Join(", ", taskResults.Select(r => r.Protocol).Distinct())}");
                }

                // De-duplicate by title and size (or guid if reliable)
                var uniqueResults = results
                    .GroupBy(r => new { r.Title, r.Size })
                    .Select(g => g.First())
                    .ToList();

                // DEBUG: Inject Mock NZB Result
                uniqueResults.Insert(0, new Playerr.Core.Prowlarr.SearchResult
                {
                    Title = "TEST_NZB_RESULT_FOR_VERIFICATION",
                    Size = 1000000000,
                    Indexer = "Mock NZB Indexer",
                    Protocol = "nzb",
                    Guid = "http://example.com/test.nzb",
                    InfoUrl = "http://example.com"
                });

                Console.WriteLine($"[SearchController] Returning {uniqueResults.Count} results. First Item Protocol: {uniqueResults.FirstOrDefault()?.Protocol}");

                return Ok(uniqueResults);
            }
            catch (Exception ex)
            {
                Console.WriteLine($"[SearchController] Error: {ex.Message}");
                return StatusCode(500, new { error = ex.Message });
            }
        }

        [HttpPost("test")]
        public async Task<IActionResult> TestConnection([FromBody] TestConnectionRequest request)
        {
            try
            {
                if (request.Type == "jackett")
                {
                    var jackettClient = new JackettClient(request.Url, request.ApiKey);
                    var isConnected = await jackettClient.TestConnectionAsync();
                    return Ok(new { 
                        connected = isConnected, 
                        message = isConnected ? "Connection successful" : "Failed to connect. Check URL and API Key." 
                    });
                }
                else
                {
                    var prowlarrClient = new ProwlarrClient(request.Url, request.ApiKey);
                    var isConnected = await prowlarrClient.TestConnectionAsync();
                    return Ok(new { 
                        connected = isConnected, 
                        message = isConnected ? "Connection successful" : "Failed to connect. Check URL and API Key." 
                    });
                }
            }
            catch (Exception ex)
            {
                return Ok(new { 
                    connected = false, 
                    message = $"Connection error: {ex.Message}" 
                });
            }
        }
    }

    public class TestConnectionRequest
    {
        public string Url { get; set; } = string.Empty;
        public string ApiKey { get; set; } = string.Empty;
        public string Type { get; set; } = "prowlarr"; // "prowlarr" or "jackett"
    }
}
