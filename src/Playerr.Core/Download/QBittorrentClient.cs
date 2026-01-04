using System;
using System.Collections.Generic;
using System.Net.Http;
using System.Text;
using System.Text.Json;
using System.Threading.Tasks;

namespace Playerr.Core.Download
{
    public class QBittorrentClient
    {
        private readonly HttpClient _httpClient;
        private readonly string _baseUrl;
        private readonly string _username;
        private readonly string _password;
        private string? _cookie;

        public QBittorrentClient(string host, int port, string username, string password, string? urlBase = null)
        {
            _httpClient = new HttpClient();
            
            // Handle case where host might already contain http:// or https://
            string cleanHost = host.Trim();
            if (!cleanHost.StartsWith("http://", StringComparison.OrdinalIgnoreCase) && 
                !cleanHost.StartsWith("https://", StringComparison.OrdinalIgnoreCase))
            {
                cleanHost = $"http://{cleanHost}";
            }
            
            // Remove trailing slash if present
            cleanHost = cleanHost.TrimEnd('/');
            
            // Process UrlBase
            string finalUrlBase = "";
            if (!string.IsNullOrWhiteSpace(urlBase))
            {
                finalUrlBase = urlBase.Trim();
                if (!finalUrlBase.StartsWith("/")) finalUrlBase = "/" + finalUrlBase;
                finalUrlBase = finalUrlBase.TrimEnd('/');
            }
            
            _baseUrl = $"{cleanHost}:{port}{finalUrlBase}/api/v2";
            _username = username;
            _password = password;
        }

        private async Task EnsureAuthenticatedAsync()
        {
            if (!string.IsNullOrEmpty(_cookie))
            {
                return;
            }

            // Set default headers for CSRF and identification
            _httpClient.DefaultRequestHeaders.Remove("Referer");
            _httpClient.DefaultRequestHeaders.Add("Referer", _baseUrl.Replace("/api/v2", "/", StringComparison.OrdinalIgnoreCase));
            _httpClient.DefaultRequestHeaders.Remove("Origin");
            _httpClient.DefaultRequestHeaders.Add("Origin", _baseUrl.Replace("/api/v2", "", StringComparison.OrdinalIgnoreCase));
            _httpClient.DefaultRequestHeaders.Remove("User-Agent");
            _httpClient.DefaultRequestHeaders.Add("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36");

            var data = new Dictionary<string, string>
            {
                { "username", _username },
                { "password", _password }
            };

            var content = new FormUrlEncodedContent(data);
            var response = await _httpClient.PostAsync($"{_baseUrl}/auth/login", content);
            
            if (!response.IsSuccessStatusCode)
            {
                var errorBody = await response.Content.ReadAsStringAsync();
                Console.WriteLine($"[qBittorrent] Login failed. Status: {response.StatusCode}, Body: {errorBody}");
                response.EnsureSuccessStatusCode();
            }

            if (response.Headers.TryGetValues("Set-Cookie", out var cookies))
            {
                foreach (var cookie in cookies)
                {
                    if (cookie.StartsWith("SID=", StringComparison.Ordinal))
                    {
                        _cookie = cookie.Split(';')[0];
                        _httpClient.DefaultRequestHeaders.Remove("Cookie");
                        _httpClient.DefaultRequestHeaders.Add("Cookie", _cookie);
                        break;
                    }
                }
            }
        }

        public async Task<bool> TestConnectionAsync()
        {
            try 
            {
                await EnsureAuthenticatedAsync();
                var response = await _httpClient.GetAsync($"{_baseUrl}/app/version");
                if (!response.IsSuccessStatusCode)
                {
                    var errorBody = await response.Content.ReadAsStringAsync();
                    Console.WriteLine($"[qBittorrent] Test connection failed. Status: {response.StatusCode}, Body: {errorBody}");
                }
                return response.IsSuccessStatusCode;
            }
            catch (Exception ex)
            {
                Console.WriteLine($"[qBittorrent] Test connection exception: {ex.Message}");
                throw;
            }
        }

        public async Task<string> GetVersionAsync()
        {
            await EnsureAuthenticatedAsync();
            var response = await _httpClient.GetAsync($"{_baseUrl}/app/version");
            response.EnsureSuccessStatusCode();
            return await response.Content.ReadAsStringAsync();
        }

        public async Task<bool> AddTorrentAsync(string magnetUrl, string? category = null)
        {
            await EnsureAuthenticatedAsync();

            var content = new MultipartFormDataContent();
            
            // urls field must be strings separated by newlines
            content.Add(new StringContent(magnetUrl), "urls");

            if (!string.IsNullOrEmpty(category))
            {
                content.Add(new StringContent(category), "category");
            }

            var response = await _httpClient.PostAsync($"{_baseUrl}/torrents/add", content);
            
            if (!response.IsSuccessStatusCode)
            {
                var errorBody = await response.Content.ReadAsStringAsync();
                Console.WriteLine($"[qBittorrent] Failed to add torrent. Status: {response.StatusCode}, Body: {errorBody}");
                return false;
            }

            return true;
        }

        public async Task<List<TorrentInfo>> GetTorrentsAsync()
        {
            await EnsureAuthenticatedAsync();
            var response = await _httpClient.GetAsync($"{_baseUrl}/torrents/info");
            response.EnsureSuccessStatusCode();

            var json = await response.Content.ReadAsStringAsync();
            var torrents = JsonSerializer.Deserialize<List<TorrentInfo>>(json, new JsonSerializerOptions
            {
                PropertyNameCaseInsensitive = true
            });

            return torrents ?? new List<TorrentInfo>();
        }
    }

    public class TorrentInfo
    {
        public string Hash { get; set; } = string.Empty;
        public string Name { get; set; } = string.Empty;
        public long Size { get; set; }
        public float Progress { get; set; }
        public string State { get; set; } = string.Empty;
        public int DlSpeed { get; set; }
        public int UpSpeed { get; set; }
        public int Priority { get; set; }
        public int NumSeeds { get; set; }
        public int NumLeechs { get; set; }
        public string Category { get; set; } = string.Empty;
    }
}
