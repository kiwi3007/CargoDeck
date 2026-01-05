using System;
using System.Net.Http;
using System.Text;
using System.Threading.Tasks;
using System.Text.Json;
using System.Text.Json.Serialization;
using System.Net.Http.Headers;

namespace Playerr.Core.Download
{
    public class NzbgetClient
    {
        private readonly HttpClient _httpClient;
        private readonly string _baseUrl;
        private readonly string _username;
        private readonly string _password;

        public NzbgetClient(string host, int port, string username, string password, string? urlBase = null)
        {
            _httpClient = new HttpClient();
            
            // Basic Auth
            var authValue = Convert.ToBase64String(Encoding.ASCII.GetBytes($"{username}:{password}"));
            _httpClient.DefaultRequestHeaders.Authorization = new AuthenticationHeaderValue("Basic", authValue);
            
            // Handle host formatting
            string cleanHost = host.Trim();
            if (!cleanHost.StartsWith("http://", StringComparison.OrdinalIgnoreCase) && 
                !cleanHost.StartsWith("https://", StringComparison.OrdinalIgnoreCase))
            {
                cleanHost = $"http://{cleanHost}";
            }
            cleanHost = cleanHost.TrimEnd('/');
            
            // Process UrlBase
            string finalUrlBase = "";
            if (!string.IsNullOrWhiteSpace(urlBase))
            {
                finalUrlBase = urlBase.Trim();
                if (!finalUrlBase.StartsWith("/")) finalUrlBase = "/" + finalUrlBase;
                finalUrlBase = finalUrlBase.TrimEnd('/');
            }
            
            // NZBGet JSON-RPC endpoint
            _baseUrl = $"{cleanHost}:{port}{finalUrlBase}/jsonrpc";
            _username = username;
            _password = password;
        }

        public async Task<bool> TestConnectionAsync()
        {
            try
            {
                var version = await GetVersionAsync();
                return !string.IsNullOrEmpty(version);
            }
            catch
            {
                return false;
            }
        }

        public async Task<string> GetVersionAsync()
        {
            try 
            {
                var request = new
                {
                    method = "version",
                    @params = new object[] { },
                    id = 1
                };
                
                var json = JsonSerializer.Serialize(request);
                var content = new StringContent(json, Encoding.UTF8, "application/json");
                
                var response = await _httpClient.PostAsync(_baseUrl, content);
                response.EnsureSuccessStatusCode();
                
                var responseContent = await response.Content.ReadAsStringAsync();
                using var doc = JsonDocument.Parse(responseContent);
                var root = doc.RootElement;
                
                if (root.TryGetProperty("result", out var resultElement))
                {
                    return resultElement.GetString() ?? string.Empty;
                }
                
                return string.Empty;
            }
            catch
            {
                return string.Empty;
            }
        }

        public async Task<bool> AddNzbAsync(string nzbUrl, string category)
        {
            try
            {
                // NZBGet append method: (FileName, Content(Base64), Category, Priority, AddToTop, Paused, DupeKey, DupeScore, DupeMode, Url)
                // When adding by URL, Content should be empty string, and URL passed as last arg.
                // Or simplified 'appendurl' method if available, but 'append' is standard.
                // Let's use 'append' with URL.

                var request = new
                {
                    method = "append",
                    @params = new object[] 
                    { 
                        "",             // FileName (empty to auto-detect from URL)
                        "",             // Content (empty for URL)
                        category ?? "", // Category
                        0,              // Priority (0=Normal)
                        false,          // AddToTop
                        false,          // Paused
                        "",             // DupeKey
                        0,              // DupeScore
                        "SCORE",        // DupeMode
                        new object[] { new { Name = "url", Value = nzbUrl } } // Extra Attributes (URL) - Wait, NZBGet API is tricky with URL.
                        // Actually, NZBGet standard 'append' takes content. 'appendurl' is not a standard RPC method in older versions but often supported via extension scripts.
                        // Official documentation says: append(Filename, Content, Category, Priority, AddToTop, Paused, DupeKey, DupeScore, DupeMode)
                        // But wait, there is no direct 'add by URL' simple method in core RPC without downloading it yourself first?
                        // Let's check documentation logic usually used:
                        // Most apps download the NZB blob then push it.
                        // HOWEVER, NZBGet matches: (filename, content, category, ...).
                        
                        // BUT! Prowlarr/Radarr often send the URL.
                        // Let's try downloading the NZB content ourselves if needed, OR checking if there is an appendurl.
                        // Actually, let's keep it simple: Many clients expect the *content* if using 'append'.
                        // But let's see if we can just pass the URL in the third param if the second is empty? No.
                        
                        // Revised strategy for NZBGet: Playerr (backend) downloads the NZB from Prowlarr URL, then pushes it base64 encoded.
                    },
                    id = 2
                };
                
                // Downloading NZB content first to be safe and compatible
                using var nzbDownloader = new HttpClient();
                var nzbBytes = await nzbDownloader.GetByteArrayAsync(nzbUrl);
                var nzbBase64 = Convert.ToBase64String(nzbBytes);
                
                var appendRequest = new
                {
                    method = "append",
                    @params = new object[] 
                    { 
                        "playerr_download.nzb", // Filename
                        nzbBase64,              // Content (Base64)
                        category ?? "",         // Category
                        0,                      // Priority
                        false,                  // AddToTop
                        false,                  // Paused
                        "",                     // DupeKey
                        0,                      // DupeScore
                        "SCORE"                 // DupeMode
                    },
                    id = 2
                };

                var json = JsonSerializer.Serialize(appendRequest);
                var content = new StringContent(json, Encoding.UTF8, "application/json");
                
                var response = await _httpClient.PostAsync(_baseUrl, content);
                response.EnsureSuccessStatusCode();
                
                var responseContent = await response.Content.ReadAsStringAsync();
                // Check if result > 0 (ID of added file)
                using var doc = JsonDocument.Parse(responseContent);
                var root = doc.RootElement;
                if (root.TryGetProperty("result", out var resultElement))
                {
                   return resultElement.GetInt32() > 0;
                }
                return false;
            }
            catch (Exception ex)
            {
                Console.WriteLine($"[NZBGet] Error adding NZB: {ex.Message}");
                return false;
            }
        }
    }
}
