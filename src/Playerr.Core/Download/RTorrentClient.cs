using System;
using System.Collections.Generic;
using System.Net.Http;
using System.Net.Http.Headers;
using System.Text;
using System.Threading.Tasks;
using System.Xml.Linq;
using System.Linq;
using System.Diagnostics.CodeAnalysis;

namespace Playerr.Core.Download
{
    [SuppressMessage("Microsoft.Performance", "CA1822:MarkMembersAsStatic")]
    [SuppressMessage("Microsoft.Design", "CA1031:DoNotCatchGeneralExceptionTypes")]
    public class RTorrentClient : IDownloadClient
    {
        private readonly HttpClient _httpClient;
        private readonly string _baseUrl;
        public Action<string>? OnLog { get; set; }

        public RTorrentClient(string host, int port, string username, string password, string? urlBase = null)
        {
            _httpClient = new HttpClient();
            _httpClient.DefaultRequestHeaders.Add("User-Agent", "Playerr/0.4.7");
            
            if (!string.IsNullOrEmpty(username))
            {
                var authValue = Convert.ToBase64String(Encoding.UTF8.GetBytes($"{username}:{password}"));
                _httpClient.DefaultRequestHeaders.Authorization = new AuthenticationHeaderValue("Basic", authValue);
            }

            string cleanHost = host.Trim();
            if (!cleanHost.StartsWith("http://", StringComparison.OrdinalIgnoreCase) && 
                !cleanHost.StartsWith("https://", StringComparison.OrdinalIgnoreCase))
            {
                cleanHost = $"http://{cleanHost}";
            }
            cleanHost = cleanHost.TrimEnd('/');

            string finalUrlBase = "/RPC2"; // Default rTorrent XML-RPC endpoint
            if (!string.IsNullOrWhiteSpace(urlBase))
            {
                finalUrlBase = urlBase.Trim();
                if (!finalUrlBase.StartsWith("/")) finalUrlBase = "/" + finalUrlBase;
            }

            _baseUrl = $"{cleanHost}:{port}{finalUrlBase}";
        }

        private void Log(string message)
        {
            OnLog?.Invoke($"[rTorrent] {message}");
            Console.WriteLine($"[rTorrent] {message}");
        }

        private async Task<XDocument> CallAsync(string methodName, params object[] parameters)
        {
            try
            {
                var methodCall = new XDocument(
                    new XDeclaration("1.0", "UTF-8", null),
                    new XElement("methodCall",
                        new XElement("methodName", methodName),
                        new XElement("params",
                            parameters.Select(p => new XElement("param", new XElement("value", MapToXmlRpcType(p))))
                        )
                    )
                );

                var xmlRequest = methodCall.Declaration?.ToString() + Environment.NewLine + methodCall.ToString(SaveOptions.DisableFormatting);
                // Log($"XML Request: {xmlRequest}");

                var content = new StringContent(xmlRequest, Encoding.UTF8, "text/xml");
                var response = await _httpClient.PostAsync(_baseUrl, content);
                
                if (!response.IsSuccessStatusCode)
        {
            var errorBody = await response.Content.ReadAsStringAsync();
            var msg = $"[rTorrent] Error: {response.StatusCode} from {_baseUrl}. Body: {new string(errorBody.Take(100).ToArray())}...";
            if (response.StatusCode == System.Net.HttpStatusCode.NotFound)
            {
                msg += " Hint: If you are using ruTorrent, you might need to set URL Base to '/plugins/httprpc/action.php' or '/plugins/rpc/rpc.php'";
            }
            Log(msg);
            response.EnsureSuccessStatusCode();
        }

                var responseString = await response.Content.ReadAsStringAsync();
                // Log($"XML Response: {responseString}");
                return XDocument.Parse(responseString);
            }
            catch (Exception ex)
            {
                Log($"CallAsync Failed: {methodName} at {_baseUrl}. Error: {ex.Message}");
                throw;
            }
        }

        private XElement MapToXmlRpcType(object p)
        {
            if (p is string s) return new XElement("string", s);
            if (p is int i) return new XElement("int", i);
            if (p is long l) return new XElement("i8", l);
            if (p is bool b) return new XElement("boolean", b ? "1" : "0");
            if (p is byte[] bytes) return new XElement("base64", Convert.ToBase64String(bytes));
            if (p is IEnumerable<object> list)
            {
                return new XElement("array",
                    new XElement("data",
                        list.Select(item => new XElement("value", MapToXmlRpcType(item)))
                    )
                );
            }
            return new XElement("string", p.ToString());
        }

        public async Task<bool> TestConnectionAsync()
        {
            try
            {
                var doc = await CallAsync("system.client_version");
                var version = doc.Descendants("string").FirstOrDefault()?.Value;
                Log($"TestConnection result: {version ?? "Empty"}");
                return !string.IsNullOrEmpty(version);
            }
            catch (Exception ex)
            {
                Log($"TestConnection Exception: {ex.Message}");
                return false;
            }
        }

        public async Task<string> GetVersionAsync()
        {
            try
            {
                var doc = await CallAsync("system.client_version");
                return doc.Descendants("string").FirstOrDefault()?.Value ?? "Unknown";
            }
            catch { return "Unknown"; }
        }

        public async Task<bool> AddTorrentAsync(string url, string? category = null)
        {
            try
            {
                Log($"[rTorrent] AddTorrentAsync (RadarrStyle) called for: {url}");
                
                // FINAL APPROACH: Skip ALL URL cleaning/parsing
                // Just use the raw URL with dontEscape to bypass validation
                url = url.Trim();
                
                // SOLUTION: Prowlarr returns HTTP 301 redirects to magnet: links
                // We need to disable auto-redirect and handle magnet links manually
                url = url.Trim();
                
                Log($"[rTorrent] AddTorrentAsync: Downloading from {url}");

                byte[] torrentData;
                try
                {
                    // Create Uri with dontEscape flag - this bypasses validation
                    Uri requestUri;
                    #pragma warning disable CS0618
                    requestUri = new Uri(url, dontEscape: true);
                    #pragma warning restore CS0618
                    
                    Log($"[rTorrent] Created Uri with dontEscape");
                    
                    // Use HttpWebRequest which accepts the Uri object directly
                    var request = (System.Net.HttpWebRequest)System.Net.WebRequest.Create(requestUri);
                    request.Method = "GET";
                    request.UserAgent = "Playerr/1.0";
                    request.Timeout = 30000;
                    request.AllowAutoRedirect = false; // CRITICAL: Don't follow redirects automatically
                    
                    Log($"[rTorrent] Sending HTTP request...");
                    
                    using (var response = (System.Net.HttpWebResponse)await request.GetResponseAsync())
                    {
                        // Check if it's a redirect
                        if (response.StatusCode == System.Net.HttpStatusCode.MovedPermanently ||
                            response.StatusCode == System.Net.HttpStatusCode.Found ||
                            response.StatusCode == System.Net.HttpStatusCode.Redirect)
                        {
                            var location = response.Headers["Location"];
                            Log($"[rTorrent] Got redirect to: {location}");
                            
                            if (location != null && location.StartsWith("magnet:", StringComparison.OrdinalIgnoreCase))
                            {
                                Log($"[rTorrent] Prowlarr returned a magnet link. Using magnet directly.");
                                
                                // Add small delay to prevent overwhelming rTorrent
                                await Task.Delay(500);
                                
                                // Add magnet link directly to rTorrent using load.normal_start
                                // This is safer than load.start as it doesn't immediately start the torrent
                                var magnetParams = new List<object> { "", location };
                                if (!string.IsNullOrEmpty(category))
                                {
                                    magnetParams.Add($"d.custom1.set={category}");
                                }
                                
                                Log("[rTorrent] Calling load.start with magnet link...");
                                var magnetDoc = await CallAsync("load.start", magnetParams.ToArray());
                                
                                if (magnetDoc != null && !magnetDoc.Descendants("fault").Any())
                                {
                                    Log("[rTorrent] Magnet link added successfully!");
                                    return true;
                                }
                                
                                Log($"[rTorrent] Failed to add magnet link.");
                                return false;
                            }
                            
                            Log($"[rTorrent] Unexpected redirect location: {location}");
                            return false;
                        }
                        
                        if (response.StatusCode != System.Net.HttpStatusCode.OK)
                        {
                            Log($"[rTorrent] HTTP error: {response.StatusCode}");
                            return false;
                        }
                        
                        using (var stream = response.GetResponseStream())
                        using (var memoryStream = new System.IO.MemoryStream())
                        {
                            await stream.CopyToAsync(memoryStream);
                            torrentData = memoryStream.ToArray();
                        }
                    }
                    
                    Log($"[rTorrent] Downloaded {torrentData.Length} bytes successfully!");
                }
                catch (Exception ex)
                {
                    Log($"[rTorrent] Download failed: {ex.Message}");
                    if (ex.InnerException != null)
                    {
                        Log($"[rTorrent] Inner Exception: {ex.InnerException.Message}");
                    }
                    return false;
                }
                
                Log($"[rTorrent] Downloaded {torrentData.Length} bytes");
                
                // Build parameters for load.raw_start:
                // 1. Target (empty)
                // 2. Data (base64 automatically handled by MapToXmlRpcType)
                // 3. Optional: custom commands
                var parameters = new List<object> { "", torrentData };
                
                if (!string.IsNullOrEmpty(category))
                {
                    parameters.Add($"d.custom1.set={category}");
                }
                
                Log("[rTorrent] Calling load.raw_start...");
                var doc = await CallAsync("load.raw_start", parameters.ToArray());
                
                if (doc != null && !doc.Descendants("fault").Any())
                {
                    Log("[rTorrent] Torrent added successfully!");
                    return true;
                }

                Log($"[rTorrent] AddTorrentAsync: Failed to add torrent.");
                return false;
            }
            catch (UriFormatException ex)
            {
                Log($"[rTorrent] Invalid URL format: {ex.Message}");
                Log($"[rTorrent] URL was: {url}");
                return false;
            }
            catch (HttpRequestException ex)
            {
                Log($"[rTorrent] HTTP request failed: {ex.Message}");
                return false;
            }
            catch (Exception ex)
            {
                Log($"[rTorrent] AddTorrentAsync Exception: {ex.Message}");
                if (ex.InnerException != null)
                {
                    Log($"[rTorrent] Inner Exception: {ex.InnerException.Message}");
                }
                Log($"[rTorrent] Stack trace: {ex.StackTrace}");
                return false;
            }
        }

        private async Task<XDocument?> TryLoadAsync(string method, params object[] args)
        {
            try
            {
                return await CallAsync(method, args);
            }
            catch (Exception ex)
            {
                Log($"[rTorrent] {method} failed: {ex.Message}");
                return null;
            }
        }

        public Task<bool> AddNzbAsync(string url, string? category = null)
        {
            return Task.FromResult(false);
        }

        public async Task<bool> RemoveDownloadAsync(string id)
        {
            try
            {
                await CallAsync("d.erase", id);
                return true;
            }
            catch { return false; }
        }

        public async Task<bool> PauseDownloadAsync(string id)
        {
            try
            {
                await CallAsync("d.pause", id);
                return true;
            }
            catch { return false; }
        }

        public async Task<bool> ResumeDownloadAsync(string id)
        {
            try
            {
                await CallAsync("d.resume", id);
                return true;
            }
            catch { return false; }
        }

        public async Task<List<DownloadStatus>> GetDownloadsAsync()
        {
            // Retry logic for temporary 502 Bad Gateway errors
            for (int attempt = 0; attempt < 3; attempt++)
            {
                try
                {
                    // d.multicall2 reaches out for torrent details in the 'main' view
                    var doc = await CallAsync("d.multicall2", "", "main", 
                        "d.hash=", 
                        "d.name=", 
                        "d.size_bytes=", 
                        "d.complete=", 
                        "d.base_path=", 
                        "d.is_active=", 
                        "d.bytes_done=",
                        "d.state=");

                    var statusList = new List<DownloadStatus>();
                    
                    // rTorrent XML-RPC response for d.multicall2:
                    // <methodResponse><params><param><value><array><data>
                    //   <value><array><data>  <-- This is one torrent
                    //     <value><string>HASH</string></value>
                    //     <value><string>NAME</string></value>
                    //     ...
                    //   </data></array></value>
                    // </data></array></value></param></params></methodResponse>

                    // Find the main data array
                    var dataRoot = doc.Descendants("param").FirstOrDefault()?.Element("value")?.Element("array")?.Element("data");
                    if (dataRoot == null)
                    {
                        Log($"GetDownloadsAsync: No data found in response. Raw XML: {doc.ToString()}");
                        return statusList;
                    }

                    var torrentValues = dataRoot.Elements("value");
                    Log($"GetDownloadsAsync: Found {torrentValues.Count()} torrents.");

                    foreach (var torrentValue in torrentValues)
                    {
                        var torrentData = torrentValue.Element("array")?.Element("data");
                        if (torrentData == null) continue;

                        var values = torrentData.Elements("value").Select(v => v.Elements().FirstOrDefault()?.Value ?? v.Value).ToList();
                        if (values.Count < 8)
                        {
                            Log($"GetDownloadsAsync: Torrent entry has insufficient fields ({values.Count} < 8). Values: {string.Join(", ", values)}");
                            continue;
                        }

                        var id = values[0];
                        var name = values[1];
                        long.TryParse(values[2], out var size);
                        var isComplete = values[3] == "1";
                        var path = values[4];
                        var isActive = values[5] == "1";
                        long.TryParse(values[6], out var bytesDone);
                        var rawState = values[7];

                        statusList.Add(new DownloadStatus
                        {
                            Id = id,
                            Name = name,
                            Size = size,
                            Progress = size > 0 ? (float)bytesDone / size * 100 : (isComplete ? 100 : 0),
                            State = MapState(isActive, isComplete, rawState),
                            DownloadPath = path,
                            ClientName = "rTorrent"
                        });
                    }

                    return statusList;
                }
                catch (Exception ex)
                {
                    Log($"GetDownloadsAsync Failed (attempt {attempt + 1}/3): {ex.Message}");
                    
                    // If it's a 502 error and we have retries left, wait and retry
                    if (ex.Message.Contains("502") && attempt < 2)
                    {
                        Log($"GetDownloadsAsync: Retrying after 502 error...");
                        await Task.Delay(1000); // Wait 1 second before retry
                        continue;
                    }
                    
                    return new List<DownloadStatus>();
                }
            }
            
            return new List<DownloadStatus>();
        }

        private DownloadState MapState(bool isActive, bool isComplete, string rawState)
        {
            if (isComplete) return DownloadState.Completed;
            if (!isActive) return DownloadState.Paused;
            return DownloadState.Downloading;
        }
    }
}
