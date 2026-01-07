using System;
using System.Collections.Generic;
using System.Linq;
using System.Net.Http;
using System.Threading.Tasks;
using System.Text.Json;
using System.Text.Json.Serialization;
using System.Xml.Linq; // Added for XML parsing
using System.Text.Json.Serialization;
using Playerr.Core.Indexers;
using System.Diagnostics.CodeAnalysis;

namespace Playerr.Core.Prowlarr
{
    [SuppressMessage("Microsoft.Globalization", "CA1307:SpecifyStringComparison")]
    [SuppressMessage("Microsoft.Globalization", "CA1310:SpecifyStringComparison")]
    [SuppressMessage("Microsoft.Reliability", "CA2007:DoNotDirectlyAwaitATask")]
    [SuppressMessage("Microsoft.Design", "CA1031:DoNotCatchGeneralExceptionTypes")]
    [SuppressMessage("Microsoft.Design", "CA1001:TypesThatOwnDisposableFieldsShouldBeDisposable")]
    [SuppressMessage("Microsoft.Reliability", "CA2000:Dispose objects before losing scope")]
    [SuppressMessage("Microsoft.Performance", "CA1866:UseCharOverload")]
    [SuppressMessage("Microsoft.Performance", "CA1869:CacheAndReuseJsonSerializerOptions")]
    [SuppressMessage("Microsoft.Design", "CA1054:UriParametersShouldNotBeStrings")]
    [SuppressMessage("Microsoft.Naming", "CA1720:IdentifiersShouldNotContainTypeNames")]
    [SuppressMessage("Microsoft.Design", "CA1056:UriPropertiesShouldNotBeStrings")]
    [SuppressMessage("Microsoft.Performance", "CA1819:PropertiesShouldNotReturnArrays")]
    [SuppressMessage("Microsoft.Usage", "CA2227:CollectionPropertiesShouldBeReadOnly")]
    [SuppressMessage("Microsoft.Design", "CA1002:DoNotExposeGenericLists")]
    public class ProwlarrClient
    {
        private readonly HttpClient _httpClient;
        private readonly string _apiKey;
        private static readonly JsonSerializerOptions _jsonOptions = new()
        {
            PropertyNameCaseInsensitive = true,
            PropertyNamingPolicy = JsonNamingPolicy.CamelCase,
            DefaultIgnoreCondition = JsonIgnoreCondition.WhenWritingNull
        };

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
            return JsonSerializer.Deserialize<List<ProwlarrIndexer>>(content, _jsonOptions) ?? new List<ProwlarrIndexer>();
        }

        public async Task<List<SearchResult>> SearchAsync(string query, int[]? indexerIds = null, int[]? categories = null)
        {
            
            // Build categories query

            // Category Expansion Logic
            var expandedCategories = new HashSet<int>();
            if (categories != null)
            {
                foreach (var cat in categories)
                {
                    expandedCategories.Add(cat);
                    // Expand PC (4000) -> 4000, 4010, 4020, 4030, 4040, 4050
                    if (cat == 4000)
                    {
                        expandedCategories.Add(4010);
                        expandedCategories.Add(4020);
                        expandedCategories.Add(4030); // Mac
                        expandedCategories.Add(4040); // Phone
                        expandedCategories.Add(4050); // Games
                    }
                    // Expand Console (1000) -> 1000, 1010..1090
                    else if (cat == 1000)
                    {
                        expandedCategories.Add(1010); // NDS
                        expandedCategories.Add(1020); // PSP
                        expandedCategories.Add(1030); // Wii
                        expandedCategories.Add(1040); // Xbox
                        expandedCategories.Add(1050); // Xbox 360
                        expandedCategories.Add(1060); // Wiiware
                        expandedCategories.Add(1070); // Xbox 360 DLC
                        expandedCategories.Add(1080); // PS3
                        expandedCategories.Add(1090); // Other
                        expandedCategories.Add(1110); // 3DS
                        expandedCategories.Add(1120); // PS Vita
                        expandedCategories.Add(1130); // WiiU
                        expandedCategories.Add(1140); // Xbox One
                        expandedCategories.Add(1180); // PS4
                    }
                }
            }

            var categoryQuery = "";
            if (expandedCategories.Count > 0)
            {
               categoryQuery = "&categories=" + string.Join("&categories=", expandedCategories);
            }

            var indexerQuery = indexerIds != null && indexerIds.Length > 0 
                ? "&" + string.Join("&", indexerIds.Select(id => $"indexerIds={id}")) 
                : "";
                
            var fullUrl = $"/api/v1/search?query={Uri.EscapeDataString(query)}{categoryQuery}{indexerQuery}";
            
            using var request = new HttpRequestMessage(HttpMethod.Get, fullUrl);
            request.Headers.Add("X-Api-Key", _apiKey);

            var response = await _httpClient.SendAsync(request);
            response.EnsureSuccessStatusCode();

            var content = await response.Content.ReadAsStringAsync();
            
            // Debug: Log the raw response and first item to find field names
            Console.WriteLine($"[Prowlarr] Raw Content Length: {content.Length}");

            try 
            {
                var results = new List<SearchResult>();

                // Detect XML (RSS/Newznab)
                if (content.TrimStart().StartsWith("<"))
                {
                    Console.WriteLine("[Prowlarr] Detected XML response. Parsing as RSS/Newznab...");
                    var doc = XDocument.Parse(content);
                    XNamespace newznab = "http://www.newznab.com/DTD/2010/feeds/attributes/";

                    // Find all 'item' elements in 'channel'
                    var items = doc.Descendants("item");

                    foreach (var item in items)
                    {
                        var result = new SearchResult();
                        result.Title = item.Element("title")?.Value ?? "Unknown Title";
                        result.Guid = item.Element("guid")?.Value ?? Guid.NewGuid().ToString();
                        result.Link = item.Element("link")?.Value ?? string.Empty; // Use Link property for internal mapping
                        result.DownloadUrl = result.Link; // Map <link> to DownloadUrl per user requirement
                        result.InfoUrl = item.Element("comments")?.Value ?? result.Guid;
                        result.PublishDate = DateTime.TryParse(item.Element("pubDate")?.Value, out var date) ? date : DateTime.UtcNow;
                        result.Provider = "Prowlarr"; // Will be overridden by indexer name if found
                        
                        // Parse enclosure for size and protocol
                        var enclosure = item.Element("enclosure");
                        if (enclosure != null)
                        {
                            var lengthStr = enclosure.Attribute("length")?.Value;
                            if (long.TryParse(lengthStr, out var length))
                            {
                                result.Size = length;
                            }
                            
                            var type = enclosure.Attribute("type")?.Value;
                            if (!string.IsNullOrEmpty(type) && type.Equals("application/x-nzb", StringComparison.OrdinalIgnoreCase))
                            {
                                result.Protocol = "nzb";
                            }
                        }

                        // Parse newznab attributes for category and indexer
                        // <newznab:attr name="category" value="4050" />
                        var attrs = item.Elements(newznab + "attr");
                        foreach (var attr in attrs)
                        {
                            var name = attr.Attribute("name")?.Value;
                            var value = attr.Attribute("value")?.Value;

                            if (name == "category" && int.TryParse(value, out var catId))
                            {
                                result.Categories.Add(new ProwlarrCategory { Id = catId, Name = catId.ToString() });
                            }
                            else if (name == "indexer") // Some indexers might provide this?
                            {
                                // result.IndexerName = value; // Typically generic
                            }
                        }
                        
                        // Fallback protocol detection if not set by enclosure
                        if (result.Protocol == "torrent") // Default
                        {
                             if (result.Title.Contains("nzb", StringComparison.OrdinalIgnoreCase) || 
                                 result.DownloadUrl.EndsWith(".nzb", StringComparison.OrdinalIgnoreCase))
                             {
                                 result.Protocol = "nzb";
                             }
                        }

                        results.Add(result);
                    }
                     Console.WriteLine($"[Prowlarr] Parsed {results.Count} items from XML.");
                     return results;
                }
                
                if (content.StartsWith('[') && content.Length > 2)
                {
                    // Try to extract just the first object if the content is large
                    int firstBrace = content.IndexOf('{');
                    int endBrace = content.IndexOf("},", StringComparison.Ordinal);
                    if (firstBrace >= 0 && endBrace > firstBrace)
                    {
                        Console.WriteLine($"[Prowlarr] First Object Raw: {content.Substring(firstBrace, endBrace - firstBrace + 1)}");
                    }
                }
                
                var resultsJson = JsonSerializer.Deserialize<List<SearchResult>>(content, _jsonOptions) ?? new List<SearchResult>();
                
                // Debug: Log deserialization success and first item fields
                Console.WriteLine($"[Prowlarr] Deserialized {resultsJson.Count} results.");
                
                foreach (var result in resultsJson)
                {
                    result.Provider = "Prowlarr";
                    
                    // Improved Protocol Detection
                    if (result.Protocol == "torrent") 
                    {
                        bool isNzb = false;
                        
                        // Check DownloadUrl for .nzb
                        if (!string.IsNullOrEmpty(result.DownloadUrl) && result.DownloadUrl.EndsWith(".nzb", StringComparison.OrdinalIgnoreCase))
                        {
                            isNzb = true;
                        }
                        // Check Indexer Name for "nzb"
                        else if (!string.IsNullOrEmpty(result.IndexerName) && result.IndexerName.Contains("nzb", StringComparison.OrdinalIgnoreCase))
                        {
                            isNzb = true;
                        }
                        // Check GUID for .nzb
                        else if (!string.IsNullOrEmpty(result.Guid) && result.Guid.EndsWith(".nzb", StringComparison.OrdinalIgnoreCase))
                        {
                            isNzb = true;
                        }

                        if (isNzb)
                        {
                            result.Protocol = "nzb";
                        }
                    }
                }

                return resultsJson;
            }
            catch (Exception ex)
            {
                Console.WriteLine($"[Prowlarr] JSON/XML Error: {ex.Message}");
                return new List<SearchResult>();
            }
        }

        public async Task<bool> TestConnectionAsync()
        {
            try
            {
                using var request = new HttpRequestMessage(HttpMethod.Get, "/api/v1/health");
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

    [SuppressMessage("Microsoft.Performance", "CA1852:SealInternalTypes")]
    [SuppressMessage("Microsoft.Performance", "CA1812:AvoidUninstantiatedInternalClasses")]
    public class ProwlarrIndexer
    {
        public int Id { get; set; }
        public string Name { get; set; } = string.Empty;
        public bool Enable { get; set; }
        public string Protocol { get; set; } = string.Empty;
    }

    // Enhanced SearchResult based on Radarr's ReleaseResource and actual Prowlarr API
    [SuppressMessage("Microsoft.Design", "CA1056:UriPropertiesShouldNotBeStrings")]
    [SuppressMessage("Microsoft.Performance", "CA1819:PropertiesShouldNotReturnArrays")]
    [SuppressMessage("Microsoft.Usage", "CA2227:CollectionPropertiesShouldBeReadOnly")]
    [SuppressMessage("Microsoft.Design", "CA1002:DoNotExposeGenericLists")]
    [SuppressMessage("Microsoft.Naming", "CA1720:IdentifiersShouldNotContainTypeNames")]
    public class SearchResult
    {
        // Basic info
        [JsonPropertyName("title")]
        public string Title { get; set; } = string.Empty;
        
        [JsonPropertyName("guid")]
        public string Guid { get; set; } = string.Empty;
        
        [JsonPropertyName("downloadUrl")]
        public string DownloadUrl { get; set; } = string.Empty;
        
        [JsonPropertyName("magnetUrl")] 
        public string MagnetUrl { get; set; } = string.Empty;
        
        // Helper property for XML parsing
        [JsonIgnore]
        public string Link { get; set; } = string.Empty;

        [JsonPropertyName("infoUrl")]
        public string InfoUrl { get; set; } = string.Empty;
        
        [JsonPropertyName("indexerId")]
        public int IndexerId { get; set; }
        
        [JsonPropertyName("indexer")]
        public string IndexerName { get; set; } = string.Empty;
        
        [JsonPropertyName("indexerFlags")]
        public string[] IndexerFlags { get; set; } = Array.Empty<string>();
        
        [JsonPropertyName("size")]
        public long Size { get; set; }
        
        [JsonPropertyName("seeders")]
        public int Seeders { get; set; }
        
        private int _leechers;
        [JsonPropertyName("leechers")] 
        public int Leechers 
        { 
            get => _leechers > 0 ? _leechers : (PeersFromIndexer.HasValue ? Math.Max(0, PeersFromIndexer.Value - Seeders) : 0);
            set => _leechers = value;
        }
        
        [JsonPropertyName("peers")]
        public int? PeersFromIndexer { get; set; }
        
        public int TotalPeers => PeersFromIndexer ?? (Seeders + Leechers);
        
        [JsonPropertyName("publishDate")]
        public DateTime PublishDate { get; set; }

        public string Provider { get; set; } = string.Empty;
        
        [JsonPropertyName("publishedAt")]
        public DateTime? PublishedAt { get; set; }
        
        [JsonPropertyName("pubDate")]
        public DateTime? PubDate { get; set; }
        
        public DateTime EffectivePublishDate => 
            PublishDate != default(DateTime) ? PublishDate :
            PublishedAt ?? PubDate ?? DateTime.UtcNow.AddDays(-1);
        
        public int Age => CalculateAge(EffectivePublishDate);
        public double AgeHours => (DateTime.UtcNow - EffectivePublishDate).TotalHours;
        public double AgeMinutes => (DateTime.UtcNow - EffectivePublishDate).TotalMinutes;
        
        [JsonPropertyName("categories")]
        public List<ProwlarrCategory> Categories { get; set; } = new List<ProwlarrCategory>();
        
        public string Category => Categories?.Count > 0 ? string.Join(", ", Categories.Select(c => c.Name)) : string.Empty;
        
        [JsonPropertyName("protocol")]
        public string Protocol { get; set; } = "torrent";
        
        [JsonPropertyName("languages")]
        public string[] Languages { get; set; } = Array.Empty<string>();
        
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

        [SuppressMessage("Microsoft.Globalization", "CA1305:SpecifyIFormatProvider")]
        public string FormattedSize => FormatBytes(Size);
        public string FormattedAge => FormatAge();
        
        private static int CalculateAge(DateTime publishDate)
        {
            if (publishDate == DateTime.MinValue || publishDate == default(DateTime))
                return 0;
                
            var age = (int)(DateTime.UtcNow - publishDate).TotalDays;
            return Math.Max(0, age);
        }
        
        [SuppressMessage("Microsoft.Globalization", "CA1305:SpecifyIFormatProvider")]
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
            
            if (timeSpan.TotalDays < 0 || timeSpan.TotalDays > 365)
                return "Unknown";
            
            if (timeSpan.TotalDays >= 1)
                return $"{(int)timeSpan.TotalDays}d";
            if (timeSpan.TotalHours >= 1)
                return $"{(int)timeSpan.TotalHours}h";
            return $"{(int)timeSpan.TotalMinutes}m";
        }
    }

    [SuppressMessage("Microsoft.Performance", "CA1852:SealInternalTypes")]
    [SuppressMessage("Microsoft.Performance", "CA1812:AvoidUninstantiatedInternalClasses")]
    public class ProwlarrCategory
    {
        [JsonPropertyName("id")]
        public int Id { get; set; }
        
        [JsonPropertyName("name")]
        public string Name { get; set; } = string.Empty;
    }
}
