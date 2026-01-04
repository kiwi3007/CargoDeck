using System;
using System.IO;
using System.Collections.Generic;
using System.Linq;
using System.Net.Http;
using System.Threading.Tasks;
using System.Text.Json;
using System.Text.Json.Serialization;
using Playerr.Core.Prowlarr; // Reuse SearchResult and ProwlarrCategory if possible, or define common ones

namespace Playerr.Core.Jackett
{
    public class JackettClient
    {
        private readonly HttpClient _httpClient;
        private readonly string _apiKey;

        public JackettClient(string baseUrl, string apiKey)
        {
            if (!baseUrl.EndsWith("/")) baseUrl += "/";
            _httpClient = new HttpClient { BaseAddress = new Uri(baseUrl) };
            _apiKey = apiKey;
        }

        public async Task<List<SearchResult>> SearchAsync(string query, int[]? categories = null)
        {
            var categoryQuery = categories != null && categories.Length > 0 
                ? "&Category[]=" + string.Join("&Category[]=", categories) 
                : "";
                
            var fullUrl = $"api/v2.0/indexers/all/results?apikey={_apiKey}&Query={Uri.EscapeDataString(query)}{categoryQuery}";
            
            try
            {
                Console.WriteLine($"[Jackett] Searching: {_httpClient.BaseAddress}{fullUrl}");
                var response = await _httpClient.GetAsync(fullUrl);
                
                if (!response.IsSuccessStatusCode)
                {
                    var errorContent = await response.Content.ReadAsStringAsync();
                    Console.WriteLine($"[Jackett] Search failed with status {response.StatusCode}: {errorContent}");
                    return new List<SearchResult>();
                }

                var content = await response.Content.ReadAsStringAsync();
                var jackettResponse = JsonSerializer.Deserialize<JackettSearchResponse>(content, new JsonSerializerOptions
                {
                    PropertyNameCaseInsensitive = true
                });

                if (jackettResponse?.Results == null) return new List<SearchResult>();

                return jackettResponse.Results.Select(r => new SearchResult
                {
                    Title = r.Title,
                    Guid = r.Guid,
                    DownloadUrl = r.Link,
                    MagnetUrl = r.MagnetUri,
                    InfoUrl = r.Comments,
                    IndexerName = r.IndexerName ?? "Jackett",
                    Provider = "Jackett",
                    Size = r.Size,
                    Seeders = r.Seeders,
                    Leechers = r.Peers - r.Seeders,
                    PublishDate = r.PublishDate,
                    Protocol = string.IsNullOrEmpty(r.MagnetUri) ? "torrent" : "magnet",
                    Quality = "",
                    ReleaseGroup = "",
                    Categories = r.Category?.Select(c => new ProwlarrCategory { Id = c, Name = "Jackett Category" }).ToList() ?? new List<ProwlarrCategory>()
                }).ToList();
            }
            catch (Exception ex)
            {
                Console.WriteLine($"[Jackett] Search Error: {ex.Message}");
                return new List<SearchResult>();
            }
        }

        public async Task<bool> TestConnectionAsync()
        {
            var testUrl = $"api/v2.0/indexers?apikey={_apiKey}";
            try
            {
                Console.WriteLine($"[Jackett] Testing connection: {_httpClient.BaseAddress}{testUrl}");
                
                var response = await _httpClient.GetAsync(testUrl);
                
                Console.WriteLine($"[Jackett] Response Status: {response.StatusCode}");

                if (!response.IsSuccessStatusCode)
                {
                    var errorContent = await response.Content.ReadAsStringAsync();
                    Console.WriteLine($"[Jackett] Test failed with status {response.StatusCode}: {errorContent}");
                    return false;
                }
                
                Console.WriteLine("[Jackett] Connection successful!");
                return true;
            }
            catch (Exception ex)
            {
                Console.WriteLine($"[Jackett] Test Error: {ex.Message}");
                return false;
            }
        }
    }

    public class JackettSearchResponse
    {
        public List<JackettResult> Results { get; set; } = new();
    }

    public class JackettResult
    {
        public string Title { get; set; } = string.Empty;
        public string Guid { get; set; } = string.Empty;
        public string? IndexerName { get; set; }
        public string Link { get; set; } = string.Empty;
        public string? MagnetUri { get; set; }
        public string? Comments { get; set; }
        public long Size { get; set; }
        public int Seeders { get; set; }
        public int Peers { get; set; }
        public DateTime PublishDate { get; set; }
        public List<int>? Category { get; set; }
    }
}
