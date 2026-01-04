using System;
using System.Collections.Generic;
using System.Linq;
using System.Net.Http;
using System.Threading.Tasks;
using System.Text.Json;
using System.Text.Json.Serialization;
using System.Linq;
using Playerr.Core.Indexers;

namespace Playerr.Core.Prowlarr
{
    public class ProwlarrClient
    {
        private readonly HttpClient _httpClient;
        private readonly string _apiKey;

        public ProwlarrClient(string baseUrl, string apiKey)
        {
            _httpClient = new HttpClient { BaseAddress = new Uri(baseUrl) };
            _apiKey = apiKey;
        }

        public async Task<List<ProwlarrIndexer>> GetIndexersAsync()
        {
            var request = new HttpRequestMessage(HttpMethod.Get, "/api/v1/indexer");
            request.Headers.Add("X-Api-Key", _apiKey);

            var response = await _httpClient.SendAsync(request);
            response.EnsureSuccessStatusCode();

            var content = await response.Content.ReadAsStringAsync();
            return JsonSerializer.Deserialize<List<ProwlarrIndexer>>(content) ?? new List<ProwlarrIndexer>();
        }

        public async Task<List<SearchResult>> SearchAsync(string query, int[]? indexerIds = null, int[]? categories = null)
        {
            // If using development API key, return mock data
            if (_apiKey == "test-api-key-for-development")
            {
                await Task.Delay(1000); // Simulate network delay
                
                return new List<SearchResult>
                {
                    new SearchResult
                    {
                        Title = $"Sample Game Pack - {query}",
                        Size = 5368709120, // 5 GB
                        Seeders = 42,
                        Leechers = 8,
                        PeersFromIndexer = 50,
                        Protocol = "torrent",
                        IndexerId = 1,
                        IndexerName = "MockIndexer",
                        Guid = "mock-guid-1",
                        DownloadUrl = "https://example.com/download/1",
                        MagnetUrl = "magnet:?xt=urn:btih:mock1",
                        PublishDate = DateTime.UtcNow.AddDays(-2),
                        InfoUrl = "https://example.com/info/1",
                        Categories = new List<ProwlarrCategory> { new ProwlarrCategory { Id = 4000, Name = "Games" } } 
                    },
                    new SearchResult
                    {
                        Title = $"Another Game - {query} [Repack]",
                        Size = 2147483648, // 2 GB
                        Seeders = 15,
                        Leechers = 3,
                        PeersFromIndexer = 18,
                        Protocol = "torrent",
                        IndexerId = 2,
                        IndexerName = "MockIndexer2",
                        Guid = "mock-guid-2",
                        DownloadUrl = "https://example.com/download/2",
                        MagnetUrl = "magnet:?xt=urn:btih:mock2",
                        PublishDate = DateTime.UtcNow.AddDays(-5),
                        InfoUrl = "https://example.com/info/2",
                        Categories = new List<ProwlarrCategory> { new ProwlarrCategory { Id = 4000, Name = "Games" } }
                    },
                    new SearchResult
                    {
                        Title = $"Classic Game Collection - {query} Edition",
                        Size = 10737418240, // 10 GB
                        Seeders = 89,
                        Leechers = 15,
                        PeersFromIndexer = 104,
                        Protocol = "torrent",
                        IndexerId = 1,
                        IndexerName = "MockIndexer",
                        Guid = "mock-guid-3",
                        DownloadUrl = "https://example.com/download/3",
                        MagnetUrl = "magnet:?xt=urn:btih:mock3",
                        PublishDate = DateTime.UtcNow.AddDays(-1),
                        InfoUrl = "https://example.com/info/3",
                        Categories = new List<ProwlarrCategory> { new ProwlarrCategory { Id = 4000, Name = "Games" } }
                    }
                };
            }
            
            // Build categories query
            string categoryQuery;
            if (categories != null && categories.Length > 0)
            {
                categoryQuery = string.Join("", categories.Select(c => $"&categories={c}"));
            }
            else
            {
                // Default game categories: 4000 (Games), 1000 (Consoles)
                categoryQuery = "&categories=4000&categories=4010&categories=4020&categories=4030&categories=4040&categories=4050&categories=4060&categories=4070&categories=4080&categories=1000";
            }

            var indexerQuery = indexerIds != null && indexerIds.Length > 0 
                ? "&" + string.Join("&", indexerIds.Select(id => $"indexerIds={id}")) 
                : "";
                
            var fullUrl = $"/api/v1/search?query={Uri.EscapeDataString(query)}{categoryQuery}{indexerQuery}";
            Console.WriteLine($"[Prowlarr] Searching: {fullUrl}");
            
            var request = new HttpRequestMessage(HttpMethod.Get, fullUrl);
            request.Headers.Add("X-Api-Key", _apiKey);

            var response = await _httpClient.SendAsync(request);
            response.EnsureSuccessStatusCode();

            var content = await response.Content.ReadAsStringAsync();
            
            // Debug: Log the raw response and first item to find field names
            Console.WriteLine($"[Prowlarr] Raw Content Length: {content.Length}");
            if (content.StartsWith("[") && content.Length > 2)
            {
                // Try to extract just the first object if the content is large
                int firstBrace = content.IndexOf('{');
                int endBrace = content.IndexOf("},");
                if (firstBrace >= 0 && endBrace > firstBrace)
                {
                    Console.WriteLine($"[Prowlarr] First Object Raw: {content.Substring(firstBrace, endBrace - firstBrace + 1)}");
                }
            }
            var options = new JsonSerializerOptions
            {
                PropertyNameCaseInsensitive = true,
                PropertyNamingPolicy = JsonNamingPolicy.CamelCase,
                DefaultIgnoreCondition = JsonIgnoreCondition.WhenWritingNull
            };
            
            try 
            {
                var results = JsonSerializer.Deserialize<List<SearchResult>>(content, options) ?? new List<SearchResult>();
                
                // Debug: Log deserialization success and first item fields
                Console.WriteLine($"[Prowlarr] Deserialized {results.Count} results.");
                if (results.Count > 0)
                {
                    var first = results[0];
                    Console.WriteLine($"[Prowlarr] First Result Sample: Title='{first.Title}', Size={first.Size}, Seeds={first.Seeders}, Indexer='{first.IndexerName}'");
                }
                
                foreach (var result in results)
                {
                    result.Provider = "Prowlarr";
                }
                
                foreach (var result in results)
                {
                    result.Provider = "Prowlarr";
                }

                return results;
            }
            catch (Exception ex)
            {
                Console.WriteLine($"[Prowlarr] JSON Error: {ex.Message}");
                if (content.Length > 0)
                {
                    Console.WriteLine($"[Prowlarr] Content Start: {content.Substring(0, Math.Min(200, content.Length))}");
                }
                return new List<SearchResult>();
            }
        }

        public async Task<bool> TestConnectionAsync()
        {
            // If using development API key, always return true
            if (_apiKey == "test-api-key-for-development")
            {
                await Task.Delay(500); // Simulate network delay
                return true;
            }
            
            try
            {
                var request = new HttpRequestMessage(HttpMethod.Get, "/api/v1/health");
                request.Headers.Add("X-Api-Key", _apiKey);

                var response = await _httpClient.SendAsync(request);
                return response.IsSuccessStatusCode;
            }
            catch
            {
                return false;
            }
        }
    }

    public class ProwlarrIndexer
    {
        public int Id { get; set; }
        public string Name { get; set; } = string.Empty;
        public bool Enable { get; set; }
        public string Protocol { get; set; } = string.Empty;
    }

    // Enhanced SearchResult based on Radarr's ReleaseResource and actual Prowlarr API
    public class SearchResult
    {
        // Basic info - Prowlarr sends these as top-level fields
        [JsonPropertyName("title")]
        public string Title { get; set; } = string.Empty;
        
        [JsonPropertyName("guid")]
        public string Guid { get; set; } = string.Empty;
        
        // Download info - these are the actual field names from Prowlarr
        [JsonPropertyName("downloadUrl")]
        public string DownloadUrl { get; set; } = string.Empty;
        
        [JsonPropertyName("magnetUrl")] 
        public string MagnetUrl { get; set; } = string.Empty;
        
        [JsonPropertyName("infoUrl")]
        public string InfoUrl { get; set; } = string.Empty;

        
        // Indexer info - these match Prowlarr's actual response
        [JsonPropertyName("indexerId")]
        public int IndexerId { get; set; }
        
        [JsonPropertyName("indexer")]
        public string IndexerName { get; set; } = string.Empty;
        
        [JsonPropertyName("indexerFlags")]
        public string[] IndexerFlags { get; set; } = Array.Empty<string>();
        
        // Size and peers - these are the critical ones that were showing zero
        [JsonPropertyName("size")]
        public long Size { get; set; }
        
        [JsonPropertyName("seeders")]
        public int Seeders { get; set; }
        
        [JsonPropertyName("leechers")] 
        public int Leechers { get; set; }
        
        // Alternative field for total peers from API (some indexers might send this instead)
        [JsonPropertyName("peers")]
        public int? PeersFromIndexer { get; set; }
        
        // Computed property for total peers count (not JSON mapped to avoid collision)
        public int TotalPeers => PeersFromIndexer ?? (Seeders + Leechers);
        
        // Time info - this is likely the issue, Prowlarr might send different field names
        [JsonPropertyName("publishDate")]
        public DateTime PublishDate { get; set; }

        public string Provider { get; set; } = string.Empty;
        
        // Alternative date field names that different indexers might use
        [JsonPropertyName("publishedAt")]
        public DateTime? PublishedAt { get; set; }
        
        [JsonPropertyName("pubDate")]
        public DateTime? PubDate { get; set; }
        
        // Use the first valid date we find
        public DateTime EffectivePublishDate => 
            PublishDate != default(DateTime) ? PublishDate :
            PublishedAt ?? PubDate ?? DateTime.UtcNow.AddDays(-1);
        
        public int Age => CalculateAge(EffectivePublishDate);
        public double AgeHours => (DateTime.UtcNow - EffectivePublishDate).TotalHours;
        public double AgeMinutes => (DateTime.UtcNow - EffectivePublishDate).TotalMinutes;
        
        // Quality and format info (from Prowlarr)
        [JsonPropertyName("categories")]
        public List<ProwlarrCategory> Categories { get; set; } = new List<ProwlarrCategory>();
        
        public string Category => Categories?.Count > 0 ? string.Join(", ", Categories.Select(c => c.Name)) : string.Empty;
        
        [JsonPropertyName("downloadProtocol")]
        public string Protocol { get; set; } = "torrent";
        
        [JsonPropertyName("languages")]
        public string[] Languages { get; set; } = Array.Empty<string>();
        
        // Additional metadata that Prowlarr may provide
        [JsonPropertyName("quality")]
        public string Quality { get; set; } = string.Empty;
        
        [JsonPropertyName("releaseGroup")]
        public string ReleaseGroup { get; set; } = string.Empty;
        
        [JsonPropertyName("source")]
        public string Source { get; set; } = string.Empty;
        
        [JsonPropertyName("container")]
        public string Container { get; set; } = string.Empty;
        
        [JsonPropertyName("codec")]
        public string Codec { get; set; } = string.Empty;
        
        [JsonPropertyName("resolution")]
        public string Resolution { get; set; } = string.Empty;
        
        // Calculated properties for display
        public string FormattedSize => FormatBytes(Size);
        public string FormattedAge => FormatAge();
        
        private static int CalculateAge(DateTime publishDate)
        {
            if (publishDate == DateTime.MinValue || publishDate == default(DateTime))
                return 0;
                
            var age = (int)(DateTime.UtcNow - publishDate).TotalDays;
            return Math.Max(0, age); // Ensure age is not negative
        }
        
        private static string FormatBytes(long bytes)
        {
            if (bytes == 0) return "0 B";
            
            string[] suffixes = { "B", "KB", "MB", "GB", "TB" };
            int counter = 0;
            decimal number = bytes;
            
            while (Math.Round(number / 1024) >= 1)
            {
                number = number / 1024;
                counter++;
            }
            
            return string.Format("{0:n1} {1}", number, suffixes[counter]);
        }
        
        private string FormatAge()
        {
            var publishDate = EffectivePublishDate;
            
            if (publishDate == DateTime.MinValue || publishDate == default(DateTime))
                return "Unknown";
                
            var timeSpan = DateTime.UtcNow.Subtract(publishDate);
            
            // Handle negative or very large time spans
            if (timeSpan.TotalDays < 0 || timeSpan.TotalDays > 365)
                return "Unknown";
            
            if (timeSpan.TotalDays >= 1)
                return $"{(int)timeSpan.TotalDays}d";
            if (timeSpan.TotalHours >= 1)
                return $"{(int)timeSpan.TotalHours}h";
            return $"{(int)timeSpan.TotalMinutes}m";
        }
    }

    public class ProwlarrCategory
    {
        [JsonPropertyName("id")]
        public int Id { get; set; }
        
        [JsonPropertyName("name")]
        public string Name { get; set; } = string.Empty;
    }
}
