using System;
using System.Net.Http;
using System.Threading.Tasks;
using System.Text.Json;
using System.Text.Json.Serialization;
using System.Diagnostics.CodeAnalysis;

namespace Playerr.Core.Download
{
    [SuppressMessage("Microsoft.Design", "CA1001:TypesThatOwnDisposableFieldsShouldBeDisposable")]
    [SuppressMessage("Microsoft.Design", "CA1031:DoNotCatchGeneralExceptionTypes")]
    [SuppressMessage("Microsoft.Reliability", "CA2007:DoNotDirectlyAwaitATask")]
    [SuppressMessage("Microsoft.Usage", "CA2234:PassSystemUriObjectsInsteadOfStrings")]
    [SuppressMessage("Microsoft.Design", "CA1054:UriParametersShouldNotBeStrings")]
    [SuppressMessage("Microsoft.Performance", "CA1867:UseCharOverload")]
    [SuppressMessage("Microsoft.Design", "CA1062:ValidateArgumentsOfPublicMethods")]
    public class SabnzbdClient
    {
        private readonly HttpClient _httpClient;
        private readonly string _baseUrl;
        private readonly string _apiKey;

        public SabnzbdClient(string host, int port, string apiKey, string? urlBase = null)
        {
            _httpClient = new HttpClient();
            
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
            
            // SABnzbd API endpoint
            _baseUrl = $"{cleanHost}:{port}{finalUrlBase}/api";
            _apiKey = apiKey;
        }

        public async Task<bool> TestConnectionAsync()
        {
            try
            {
                // Simple version check to test connection
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
                var url = $"{_baseUrl}?mode=version&apikey={_apiKey}&output=json";
                var response = await _httpClient.GetAsync(url);
                response.EnsureSuccessStatusCode();
                
                var content = await response.Content.ReadAsStringAsync();
                var result = JsonSerializer.Deserialize<SabnzbdVersionResponse>(content);
                return result?.Version ?? string.Empty;
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
                // Add NZB by URL
                var url = $"{_baseUrl}?mode=addurl&name={Uri.EscapeDataString(nzbUrl)}&cat={Uri.EscapeDataString(category ?? "default")}&apikey={_apiKey}&output=json";
                var response = await _httpClient.GetAsync(url);
                response.EnsureSuccessStatusCode();
                
                var content = await response.Content.ReadAsStringAsync();
                var result = JsonSerializer.Deserialize<SabnzbdAddResponse>(content);
                
                return result?.Status == true;
            }
            catch (Exception ex)
            {
                Console.WriteLine($"[Sabnzbd] Error adding NZB: {ex.Message}");
                return false;
            }
        }

        [SuppressMessage("Microsoft.Performance", "CA1812:AvoidUninstantiatedInternalClasses")]
        [SuppressMessage("Microsoft.Performance", "CA1852:SealInternalTypes")]
        private class SabnzbdVersionResponse
        {
            [JsonPropertyName("version")]
            public string Version { get; set; }
        }

        [SuppressMessage("Microsoft.Performance", "CA1812:AvoidUninstantiatedInternalClasses")]
        [SuppressMessage("Microsoft.Performance", "CA1852:SealInternalTypes")]
        private class SabnzbdAddResponse
        {
            [JsonPropertyName("status")]
            public bool Status { get; set; }
            
            [JsonPropertyName("nzo_ids")]
            public string[] NzoIds { get; set; }
        }
    }
}
