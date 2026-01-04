using System;
using System.Collections.Generic;
using System.Net.Http;
using System.Net.Http.Headers;
using System.Text;
using System.Text.Json;
using System.Text.Json.Serialization;
using System.Threading.Tasks;

namespace Playerr.Core.Download
{
    public class TransmissionClient
    {
        private readonly HttpClient _httpClient;
        private readonly string _rpcUrl;
        private readonly string _username;
        private readonly string _password;
        private string? _sessionId;

        public TransmissionClient(string host, int port, string username, string password)
        {
            _httpClient = new HttpClient();
            
            string cleanHost = host.Trim();
            if (!cleanHost.StartsWith("http://", StringComparison.OrdinalIgnoreCase) && 
                !cleanHost.StartsWith("https://", StringComparison.OrdinalIgnoreCase))
            {
                cleanHost = $"http://{cleanHost}";
            }
            cleanHost = cleanHost.TrimEnd('/');
            
            _rpcUrl = $"{cleanHost}:{port}/transmission/rpc";
            _username = username;
            _password = password;
        }

        private void SetupHeaders()
        {
            _httpClient.DefaultRequestHeaders.Authorization = new AuthenticationHeaderValue(
                "Basic", 
                Convert.ToBase64String(Encoding.ASCII.GetBytes($"{_username}:{_password}")));

            if (!string.IsNullOrEmpty(_sessionId))
            {
                if (_httpClient.DefaultRequestHeaders.Contains("X-Transmission-Session-Id"))
                {
                    _httpClient.DefaultRequestHeaders.Remove("X-Transmission-Session-Id");
                }
                _httpClient.DefaultRequestHeaders.Add("X-Transmission-Session-Id", _sessionId);
            }
        }

        private async Task<HttpResponseMessage> SendRequestAsync(string method, object? arguments = null)
        {
            SetupHeaders();

            var requestBody = new
            {
                method = method,
                arguments = arguments
            };

            var json = JsonSerializer.Serialize(requestBody);
            var content = new StringContent(json, Encoding.UTF8, "application/json");

            var response = await _httpClient.PostAsync(_rpcUrl, content);

            if (response.StatusCode == System.Net.HttpStatusCode.Conflict) // 409
            {
                if (response.Headers.TryGetValues("X-Transmission-Session-Id", out var values))
                {
                    foreach (var value in values)
                    {
                        _sessionId = value;
                        break;
                    }
                    
                    // Retry with new session ID
                    SetupHeaders();
                    var retryContent = new StringContent(json, Encoding.UTF8, "application/json");
                    response = await _httpClient.PostAsync(_rpcUrl, retryContent);
                }
            }

            return response;
        }

        public async Task<bool> TestConnectionAsync()
        {
            try
            {
                var response = await SendRequestAsync("session-get");
                return response.IsSuccessStatusCode;
            }
            catch (Exception ex)
            {
                Console.WriteLine($"[Transmission] Test connection exception: {ex.Message}");
                return false;
            }
        }

        public async Task<string> GetVersionAsync()
        {
            var response = await SendRequestAsync("session-get");
            if (!response.IsSuccessStatusCode) return string.Empty;

            var json = await response.Content.ReadAsStringAsync();
            using var doc = JsonDocument.Parse(json);
            if (doc.RootElement.TryGetProperty("arguments", out var args))
            {
                if (args.TryGetProperty("version", out var version))
                {
                    return version.GetString() ?? string.Empty;
                }
            }
            return string.Empty;
        }

        public async Task<bool> AddTorrentAsync(string url, string? category = null)
        {
            var args = new Dictionary<string, object>();
            
            if (url.StartsWith("magnet:?", StringComparison.OrdinalIgnoreCase))
            {
                args["filename"] = url;
            }
            else
            {
                // Download the torrent file manually to handle redirects (e.g. from Prowlarr)
                // because Transmission often struggles with 301/302 redirects when fetching by URL.
                try 
                {
                    var cleanUrl = url.Trim();
                    Console.WriteLine($"[Transmission] Manually downloading torrent from: {cleanUrl}");
                    
                    // Use a fresh client to avoid sending Transmission headers (Auth, SessionId) to Prowlarr
                    using var downloadClient = new HttpClient();
                    var torrentBytes = await downloadClient.GetByteArrayAsync(cleanUrl);
                    Console.WriteLine($"[Transmission] Manual download successful. Bytes: {torrentBytes.Length}");
                    
                    var base64 = Convert.ToBase64String(torrentBytes);
                    args["metainfo"] = base64;
                }
                catch (Exception ex)
                {
                    Console.WriteLine($"[Transmission] Failed to download torrent file content: {ex.Message}");
                    Console.WriteLine($"[Transmission] Stack Trace: {ex.StackTrace}");
                    // Fallback to URL if download fails
                    args["filename"] = url; 
                }
            }

            Console.WriteLine($"[Transmission] Sending arguments: {string.Join(", ", args.Keys)}");

            // Transmission doesn't support categories natively in the same way qBittorrent does
            // Usually path is used, but for now we will just add the torrent.
            // If category mapping to download-dir is needed, it would go here.

            var response = await SendRequestAsync("torrent-add", args);
            
            var json = await response.Content.ReadAsStringAsync();
            Console.WriteLine($"[Transmission] AddTorrent Response Code: {response.StatusCode}");
            Console.WriteLine($"[Transmission] AddTorrent Response Body: {json}");

            if (response.IsSuccessStatusCode)
            {
                using var doc = JsonDocument.Parse(json);
                if (doc.RootElement.TryGetProperty("result", out var result))
                {
                    var resultStr = result.GetString();
                    Console.WriteLine($"[Transmission] RPC Result: {resultStr}");
                    return resultStr == "success";
                }
            }
            else
            {
                Console.WriteLine($"[Transmission] Error adding torrent. Status: {response.StatusCode}");
            }

            return false;
        }
    }
}
