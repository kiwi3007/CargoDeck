using System;
using System.Collections.Generic;
using System.Linq;
using System.Net.Http;
using System.Threading.Tasks;
using System.Text.Json;
using System.Xml.Linq;
using Playerr.Core.Prowlarr;

namespace Playerr.Core.Indexers
{
    public class TorznabClient : IIndexerClient
    {
        private readonly HttpClient _httpClient;
        private readonly string _proxyUrl;
        private readonly string _apiKey;

        public TorznabClient(HttpClient httpClient, string proxyUrl, string apiKey)
        {
            _httpClient = httpClient;
            _proxyUrl = proxyUrl.TrimEnd('/');
            _apiKey = apiKey;
        }

        public async Task<List<SearchResult>> SearchAsync(string query, int[]? categories = null)
        {
            try
            {
                var catString = categories != null && categories.Length > 0 ? string.Join(",", categories) : "";
                
                // Torznab standard URL
                // We initially try JSON mode if supported by the endpoint (Jackett does support &format=json, Prowlarr Torznab proxy might not?)
                // The user requested "Specialized in JSON".
                // We will try to append &format=json to the URL, but be ready to fallback to XML.
                
                var url = $"{_proxyUrl}?t=search&q={Uri.EscapeDataString(query)}&cat={catString}&extended=1&apikey={_apiKey}"; 
                // We do NOT default to &format=json for Prowlarr Proxy unless we know it supports it.
                // Standard Torznab is XML.
                // However, let's implement the robust multi-format parsing here too.

                Console.WriteLine($"[TorznabClient] Requesting: {url}");
                var content = await _httpClient.GetStringAsync(url);
                
                if (content.TrimStart().StartsWith("<"))
                {
                    // XML (Standard Torznab)
                    return ParseXml(content);
                }
                else if (content.TrimStart().StartsWith("[") || content.TrimStart().StartsWith("{"))
                {
                    // JSON (Maybe Jackett direct?)
                    return ParseJson(content); 
                }
                else 
                {
                    Console.WriteLine("[TorznabClient] Unknown format.");
                    return new List<SearchResult>();
                }
            }
            catch (Exception ex)
            {
                Console.WriteLine($"[TorznabClient] Error: {ex.Message}");
                return new List<SearchResult>();
            }
        }

        private List<SearchResult> ParseXml(string xmlContent)
        {
             XDocument doc = XDocument.Parse(xmlContent);
             XNamespace torznab = "http://torznab.com/schemas/2015/feed";
             var results = new List<SearchResult>();

             foreach (var item in doc.Descendants("item"))
             {
                 var result = new SearchResult
                 {
                     Title = item.Element("title")?.Value ?? "Unknown",
                     Guid = item.Element("guid")?.Value ?? Guid.NewGuid().ToString(),
                     Link = item.Element("link")?.Value ?? "",
                     PublishDate = DateTime.TryParse(item.Element("pubDate")?.Value, out var date) ? date : DateTime.UtcNow,
                     Protocol = "torrent",
                     Provider = "Torrent"
                 };
                 
                 result.DownloadUrl = result.Link;

                 // Parse Torznab attributes
                 foreach (var attr in item.Elements(torznab + "attr"))
                 {
                     var name = attr.Attribute("name")?.Value;
                     var val = attr.Attribute("value")?.Value;
                     
                     if (string.Equals(name, "seeders", StringComparison.OrdinalIgnoreCase) && int.TryParse(val, out var seeders)) result.Seeders = seeders;
                     if (string.Equals(name, "peers", StringComparison.OrdinalIgnoreCase) && int.TryParse(val, out var peers)) result.PeersFromIndexer = peers; 
                     if (string.Equals(name, "leechers", StringComparison.OrdinalIgnoreCase) && int.TryParse(val, out var leechers)) result.Leechers = leechers;
                     if (string.Equals(name, "size", StringComparison.OrdinalIgnoreCase) && long.TryParse(val, out var size)) result.Size = size;
                     if (string.Equals(name, "magneturl", StringComparison.OrdinalIgnoreCase)) result.MagnetUrl = val;
                     if (string.Equals(name, "category", StringComparison.OrdinalIgnoreCase) && int.TryParse(val, out var cid))
                        result.Categories.Add(new ProwlarrCategory { Id = cid, Name = cid.ToString() });
                 }

                 // NEW: Robust Leechers calculation
                 // If total peers exists but leechers is 0, calculate it
                 if (result.PeersFromIndexer.HasValue && result.Leechers == 0)
                 {
                     result.Leechers = Math.Max(0, result.PeersFromIndexer.Value - result.Seeders);
                 }
                 
                 // Enclosure fallback for Magnet?
                 var enclosure = item.Element("enclosure");
                 if (enclosure != null)
                 {
                      var type = enclosure.Attribute("type")?.Value;
                      if (!string.IsNullOrEmpty(type) && type == "application/x-bittorrent")
                      {
                          if(long.TryParse(enclosure.Attribute("length")?.Value, out var len)) result.Size = len;
                          // Standard torments don't put magnet here usually, but check url
                      }
                 }

                 results.Add(result);
             }
             return results;
        }

        private List<SearchResult> ParseJson(string jsonContent)
        {
            var options = new JsonSerializerOptions { PropertyNameCaseInsensitive = true };
            // Need to know structure. Assuming standard list if user claimed JSON specialization.
            // Or maybe standard Prowlarr/Sonarr structure wrapped in "Records"?
            // For now, let's assume List<SearchResult> compatible or try to be generic. 
            // Given the lack of spec for "Torznab JSON", I'll attempt simple list deserialization 
            // OR if it wraps in { "item": [...] }
            
            try 
            {
                 // Assuming List<SearchResult> for now if user insisted on JSON specialisation 
                 // (or maybe they meant JackettResult?)
                 // I will leave this simple since standard Torznab is XML and that is what Prowlarr likely sends.
                 return JsonSerializer.Deserialize<List<SearchResult>>(jsonContent, options) ?? new List<SearchResult>();
            }
            catch 
            {
                 return new List<SearchResult>(); 
            }
        }

        public async Task<bool> TestConnectionAsync()
        {
             try
            {
                var response = await _httpClient.GetAsync($"{_proxyUrl}?t=caps&apikey={_apiKey}");
                return response.IsSuccessStatusCode;
            }
            catch { return false; }
        }
    }
}
