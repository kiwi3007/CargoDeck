using System;
using System.Net.Http;
using System.Text;
using System.Threading.Tasks;
using System.Text.Json;
using System.Text.Json.Serialization;
using System.Net.Http.Headers;
using System.Diagnostics.CodeAnalysis;

namespace Playerr.Core.Download
{
    [SuppressMessage("Microsoft.Design", "CA1001:TypesThatOwnDisposableFieldsShouldBeDisposable")]
    [SuppressMessage("Microsoft.Usage", "CA2234:PassSystemUriObjectsInsteadOfStrings")]
    [SuppressMessage("Microsoft.Reliability", "CA2007:DoNotDirectlyAwaitATask")]
    [SuppressMessage("Microsoft.Design", "CA1031:DoNotCatchGeneralExceptionTypes")]
    [SuppressMessage("Microsoft.Reliability", "CA2000:Dispose objects before losing scope")]
    [SuppressMessage("Microsoft.Performance", "CA1812:AvoidUninstantiatedInternalClasses")]
    [SuppressMessage("Microsoft.Performance", "CA1852:SealInternalTypes")]
    [SuppressMessage("Microsoft.Design", "CA1054:UriParametersShouldNotBeStrings")]
    [SuppressMessage("Microsoft.Performance", "CA1867:UseCharOverload")]
    [SuppressMessage("Microsoft.Design", "CA1062:ValidateArgumentsOfPublicMethods")]
    [SuppressMessage("Microsoft.Performance", "CA1825:AvoidZeroLengthArrayAllocations")]
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
                if (!finalUrlBase.StartsWith("/", StringComparison.OrdinalIgnoreCase)) finalUrlBase = "/" + finalUrlBase;
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
