using System;
using System.IO;
using System.Collections.Generic;
using System.Collections.Concurrent; // Added for session cache
using System.Net.Http;
using System.Net.Http.Headers;
using System.Text;
using System.Text.Json;
using System.Text.Json.Serialization;
using System.Threading.Tasks;
using System.Diagnostics.CodeAnalysis;

namespace Playerr.Core.Download
{
    [SuppressMessage("Microsoft.Design", "CA1001:TypesThatOwnDisposableFieldsShouldBeDisposable")]
    [SuppressMessage("Microsoft.Design", "CA1031:DoNotCatchGeneralExceptionTypes")]
    [SuppressMessage("Microsoft.Reliability", "CA2007:DoNotDirectlyAwaitATask")]
    [SuppressMessage("Microsoft.Design", "CA1054:UriParametersShouldNotBeStrings")]
    [SuppressMessage("Microsoft.Usage", "CA2234:PassSystemUriObjectsInsteadOfStrings")]
    [SuppressMessage("Microsoft.Design", "CA1062:ValidateArgumentsOfPublicMethods")]
    [SuppressMessage("Microsoft.Reliability", "CA2000:Dispose objects before losing scope")]
    public class TransmissionClient : IDownloadClient
    {
        private readonly HttpClient _httpClient;
        private readonly string _rpcUrl;
        private readonly string _username;
        private readonly string _password;
        private static readonly ConcurrentDictionary<string, string> _sessionCache = new ConcurrentDictionary<string, string>();
        private string? _sessionId;

        public TransmissionClient(string host, int port, string username, string password)
        {
            _httpClient = new HttpClient();
            _httpClient.Timeout = TimeSpan.FromSeconds(15); // Set timeout to avoid infinite hangs
            
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

            // Try to get session from cache if not set locally
            if (string.IsNullOrEmpty(_sessionId))
            {
                _sessionCache.TryGetValue(_rpcUrl, out _sessionId);
            }

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

            try 
            {
                var response = await _httpClient.PostAsync(_rpcUrl, content);

                if (response.StatusCode == System.Net.HttpStatusCode.Conflict) // 409
                {
                    if (response.Headers.TryGetValues("X-Transmission-Session-Id", out var values))
                    {
                        foreach (var value in values)
                        {
                            _sessionId = value;
                            _sessionCache[_rpcUrl] = _sessionId; // Update cache
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
            catch (TaskCanceledException)
            {
                throw new TimeoutException($"Transmission RPC {method} timed out.");
            }
            catch (Exception ex)
            {
                throw;
            }
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
                // Prowlarr often redirects to magnet links with HTTP 301
                // We need to follow the redirect and extract the magnet link
                try
                {
                    var cleanUrl = url.Trim();
                    
                    using var httpClient = new HttpClient(new HttpClientHandler 
                    { 
                        AllowAutoRedirect = false // We'll handle redirects manually
                    });
                    httpClient.DefaultRequestHeaders.Add("User-Agent", "Playerr/0.1");
                    httpClient.Timeout = TimeSpan.FromSeconds(10);
                    
                    var httpResponse = await httpClient.GetAsync(cleanUrl);
                    
                    // Check if it's a redirect
                    if (httpResponse.StatusCode == System.Net.HttpStatusCode.MovedPermanently ||
                        httpResponse.StatusCode == System.Net.HttpStatusCode.Found ||
                        httpResponse.StatusCode == System.Net.HttpStatusCode.SeeOther)
                    {
                        var location = httpResponse.Headers.Location?.ToString();
                        if (!string.IsNullOrEmpty(location))
                        {
                            
                            // Check if it's a magnet link
                            if (location.StartsWith("magnet:", StringComparison.OrdinalIgnoreCase))
                            {
                                // Extract filename from original URL (e.g., file=Name+Of+Game)
                                var magnetLink = location;
                                if (magnetLink.Contains("&dn=&") || magnetLink.Contains("&dn=%20&") || magnetLink.EndsWith("&dn="))
                                {
                                    // dn parameter is empty, try to extract from original URL
                                    var uri = new Uri(cleanUrl);
                                    var query = System.Web.HttpUtility.ParseQueryString(uri.Query);
                                    var fileName = query["file"];
                                    
                                    if (!string.IsNullOrEmpty(fileName))
                                    {
                                        // URL encode the filename for the magnet link
                                        var encodedName = Uri.EscapeDataString(fileName);
                                        
                                        // Replace empty dn= with the actual filename
                                        if (magnetLink.Contains("&dn=&"))
                                        {
                                            magnetLink = magnetLink.Replace("&dn=&", $"&dn={encodedName}&");
                                        }
                                        else if (magnetLink.EndsWith("&dn="))
                                        {
                                            magnetLink = magnetLink.Replace("&dn=", $"&dn={encodedName}");
                                        }
                                        
                                    }
                                }
                                
                                args["filename"] = magnetLink;
                                goto skipDownload;
                            }
                            
                            // Otherwise, try to download from the redirect location
                            cleanUrl = location;
                            var redirectResponse = await httpClient.GetAsync(cleanUrl);
                            redirectResponse.EnsureSuccessStatusCode();
                            var torrentBytes = await redirectResponse.Content.ReadAsByteArrayAsync();
                            
                            var base64 = Convert.ToBase64String(torrentBytes);
                            args["metainfo"] = base64;
                        }
                    }
                    
                    skipDownload:;
                }
                catch (Exception ex)
                {
                    
                    // Fallback: send URL directly to Transmission
                    args["filename"] = url;
                }
            }


            // Transmission doesn't support categories natively in the same way qBittorrent does
            // Usually path is used, but for now we will just add the torrent.
            // If category mapping to download-dir is needed, it would go here.

            var response = await SendRequestAsync("torrent-add", args);
            
            var json = await response.Content.ReadAsStringAsync();

            if (response.IsSuccessStatusCode)
            {
                using var doc = JsonDocument.Parse(json);
                if (doc.RootElement.TryGetProperty("result", out var result))
                {
                    var resultStr = result.GetString();
                    if (resultStr == "success") return true;
                    throw new Exception($"Transmission RPC Returned: {resultStr}");
                }
            }
            else
            {
                throw new HttpRequestException($"Transmission returned {response.StatusCode}: {json}");
            }

            return false;
        }



        public Task<bool> AddNzbAsync(string url, string? category = null)
        {
            return Task.FromResult(false); // Not supported
        }

        public async Task<bool> RemoveDownloadAsync(string id)
        {
            
            var args = new Dictionary<string, object>
            {
                { "ids", new[] { int.Parse(id) } },
                { "delete-local-data", true }
            };

            var response = await SendRequestAsync("torrent-remove", args);
            return response.IsSuccessStatusCode;
        }

        public async Task<bool> PauseDownloadAsync(string id)
        {
            
            var args = new Dictionary<string, object> { { "ids", new[] { int.Parse(id) } } };
            var response = await SendRequestAsync("torrent-stop", args);
            
            return response.IsSuccessStatusCode;
        }

        public async Task<bool> ResumeDownloadAsync(string id)
        {
            
            var args = new Dictionary<string, object> { { "ids", new[] { int.Parse(id) } } };
            var response = await SendRequestAsync("torrent-start", args);
            
            return response.IsSuccessStatusCode;
        }

        public async Task<List<DownloadStatus>> GetDownloadsAsync()
        {
            var args = new
            {
                fields = new[] { "id", "name", "totalSize", "percentDone", "status", "downloadDir", "error", "errorString" }
            };

            var response = await SendRequestAsync("torrent-get", args);
            if (!response.IsSuccessStatusCode) 
            {
                 throw new HttpRequestException($"Transmission Queue Error: {response.StatusCode}");
            }

            var json = await response.Content.ReadAsStringAsync();
            var statusList = new List<DownloadStatus>();

            using var doc = JsonDocument.Parse(json);
            if (doc.RootElement.TryGetProperty("arguments", out var arguments) &&
                arguments.TryGetProperty("torrents", out var torrents))
            {
                foreach (var torrent in torrents.EnumerateArray())
                {
                    statusList.Add(new DownloadStatus
                    {
                        Id = torrent.GetProperty("id").GetInt32().ToString(),
                        Name = torrent.GetProperty("name").GetString() ?? string.Empty,
                        Size = torrent.GetProperty("totalSize").GetInt64(),
                        Progress = (float)torrent.GetProperty("percentDone").GetDouble() * 100,
                        State = MapState(torrent.GetProperty("status").GetInt32()),
                        DownloadPath = torrent.GetProperty("downloadDir").GetString()
                    });
                }
            }

            return statusList;
        }

        private DownloadState MapState(int status)
        {
            return status switch
            {
                0 => DownloadState.Paused,     // TR_STATUS_STOPPED
                1 => DownloadState.Checking,   // TR_STATUS_CHECK_WAIT
                2 => DownloadState.Checking,   // TR_STATUS_CHECK
                3 => DownloadState.Queued,     // TR_STATUS_DOWNLOAD_WAIT
                4 => DownloadState.Downloading, // TR_STATUS_DOWNLOAD
                5 => DownloadState.Queued,     // TR_STATUS_SEED_WAIT
                6 => DownloadState.Completed,   // TR_STATUS_SEED
                _ => DownloadState.Unknown
            };
        }
    }
}
