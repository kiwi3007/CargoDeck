using System;
using System.Collections.Generic;
using System.Linq;
using System.Threading.Tasks;
using Microsoft.AspNetCore.Mvc;
using Playerr.Core.Prowlarr;
using Playerr.Core.Jackett;
using Playerr.Core.Configuration;
using Playerr.Core.Indexers;
using System.Diagnostics.CodeAnalysis;

namespace Playerr.Api.V3.Search
{
    [ApiController]
    [Route("api/v3/[controller]")]
    [SuppressMessage("Microsoft.Design", "CA1056:UriPropertiesShouldNotBeStrings")]
    [SuppressMessage("Microsoft.Design", "CA1031:DoNotCatchGeneralExceptionTypes")]
    [SuppressMessage("Microsoft.Performance", "CA1860:AvoidUsingAnyWhenUseCount")]
    [SuppressMessage("Microsoft.Performance", "CA1849:CallAsyncMethodsWhenInAnAsyncMethod")]
    [SuppressMessage("Microsoft.Reliability", "CA2008:DoNotCreateTasksWithoutPassingATaskScheduler")]
    public class SearchController : ControllerBase
    {
        private readonly ProwlarrSettings _prowlarrSettings;
        private readonly JackettSettings _jackettSettings;
        private readonly ConfigurationService _configurationService;
        private readonly System.Net.Http.IHttpClientFactory _httpClientFactory;

        public SearchController(ProwlarrSettings prowlarrSettings, JackettSettings jackettSettings, ConfigurationService configurationService, System.Net.Http.IHttpClientFactory httpClientFactory)
        {
            _prowlarrSettings = prowlarrSettings;
            _jackettSettings = jackettSettings;
            _configurationService = configurationService;
            _httpClientFactory = httpClientFactory;
        }

        [HttpGet]
        public async Task<ActionResult<List<SearchResult>>> Search([FromQuery] string query, [FromQuery] string? categories = null)
        {
            if (string.IsNullOrWhiteSpace(query))
            {
                return BadRequest("Query parameter is required");
            }

            var hydraConfigs = _configurationService.LoadHydraIndexers().Where(h => h.Enabled).ToList();

            Console.WriteLine($"[Search] Query: {query}");
            Console.WriteLine($"[Search] Prowlarr: Configured={_prowlarrSettings.IsConfigured}, Enabled={_prowlarrSettings.Enabled}");
            Console.WriteLine($"[Search] Jackett: Configured={_jackettSettings.IsConfigured}, Enabled={_jackettSettings.Enabled}");
            Console.WriteLine($"[Search] Hydra Sources: {hydraConfigs.Count} enabled");

            if (!_prowlarrSettings.IsConfigured && !_jackettSettings.IsConfigured && !hydraConfigs.Any())
            {
                // Return empty list if no providers configured
                return new List<SearchResult>();
            }

            var results = new List<SearchResult>();
            var tasks = new List<Task<List<SearchResult>>>();
            
            // Create a shared HttpClient for this request scope to reuse connections
            // Use a short timeout to prevent slow indexers from blocking the user logic
            var sharedClient = _httpClientFactory.CreateClient("");
            sharedClient.Timeout = TimeSpan.FromSeconds(60);  
            
            int[]? categoryIds = null;
            if (!string.IsNullOrEmpty(categories))
            {
                categoryIds = categories.Split(',')
                                       .Select(c => int.TryParse(c, out var id) ? id : (int?)null)
                                       .Where(id => id.HasValue)
                                       .Select(id => id!.Value)
                                       .ToArray();
            }

            // 1. Search Prowlarr Unified API (Better normalization than individual proxies)
            // 1. Search Prowlarr Unified API (Better normalization than individual proxies)
            if (_prowlarrSettings.IsConfigured && _prowlarrSettings.Enabled)
            {
                var prowlarrClient = new ProwlarrClient(_prowlarrSettings.Url, _prowlarrSettings.ApiKey);
                tasks.Add(prowlarrClient.SearchAsync(query, categoryIds).ContinueWith(t => 
                {
                    if (t.IsFaulted)
                    {
                        Console.WriteLine($"[SearchController] Prowlarr Unified Search Failed: {t.Exception?.InnerException?.Message}");
                        return new List<SearchResult>();
                    }
                    return t.Result;
                }));
            }

            // 2. Search Jackett (Legacy/Direct)
            // 2. Search Jackett (Legacy/Direct)
            if (_jackettSettings.IsConfigured && _jackettSettings.Enabled)
            {
                var jackettClient = new JackettClient(_jackettSettings.Url, _jackettSettings.ApiKey);
                tasks.Add(jackettClient.SearchAsync(query, categoryIds)
                    .ContinueWith(t => 
                    {
                        if (t.IsFaulted) 
                        {
                            Console.WriteLine($"Jackett Search Failed: {t.Exception?.Message}");
                            return new List<SearchResult>();
                        }
                        
                        return t.Result.Select(j => new SearchResult
                        {
                            Title = j.Title,
                            Guid = j.Guid,
                            Size = j.Size,
                            IndexerName = j.Tracker,
                            Seeders = j.Seeders,
                            Leechers = j.Leechers,
                            PeersFromIndexer = j.Peers,
                            PublishDate = j.PublishDate,
                            DownloadUrl = j.DownloadUrl,
                            MagnetUrl = j.MagnetUri,
                            InfoUrl = j.Guid,
                            Protocol = j.Protocol,
                            Provider = "Jackett"
                        }).ToList();
                    }));
            }

            // 3. Search HydraSources (JSON)
            foreach (var hydraConfig in hydraConfigs)
            {
                var hydraClient = new HydraIndexer(sharedClient, hydraConfig.Name, hydraConfig.Url);
                tasks.Add(hydraClient.SearchAsync(query).ContinueWith(t => 
                {
                    if (t.IsFaulted)
                    {
                        Console.WriteLine($"[SearchController] Hydra [{hydraConfig.Name}] Search Failed: {t.Exception?.InnerException?.Message}");
                        return new List<SearchResult>();
                    }
                    return t.Result;
                }));
            }

            try
            {
                await Task.WhenAll(tasks);
                
                foreach (var t in tasks)
                {
                    if (t.Result != null) results.AddRange(t.Result);
                }

                Console.WriteLine($"[SearchController] Total Results: {results.Count}");

                // De-duplicate by title and size
                var uniqueResults = results
                    .GroupBy(r => new { r.Title, r.Size })
                    .Select(g => g.First())
                    .ToList();

                Console.WriteLine($"[SearchController] Returning {uniqueResults.Count} unique results.");

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

    [SuppressMessage("Microsoft.Design", "CA1056:UriPropertiesShouldNotBeStrings")]
    public class TestConnectionRequest
    {
        public string Url { get; set; } = string.Empty;
        public string ApiKey { get; set; } = string.Empty;
        public string Type { get; set; } = "prowlarr"; // "prowlarr" or "jackett"
    }
}
