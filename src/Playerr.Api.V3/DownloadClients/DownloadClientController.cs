using System;
using System.Collections.Generic;
using System.Linq;
using System.Threading.Tasks;
using Microsoft.AspNetCore.Mvc;
using Playerr.Core.Download;

namespace Playerr.Api.V3.DownloadClients
{
    [ApiController]
    [Route("api/v3/downloadclient")]
    public class DownloadClientController : ControllerBase
    {
        private static List<DownloadClient> _clients = new();
        private readonly Core.Configuration.ConfigurationService _configService;

        public DownloadClientController(Core.Configuration.ConfigurationService configService)
        {
            _configService = configService;
            if (!_clients.Any())
            {
                _clients = _configService.LoadDownloadClients();
            }
        }

        [HttpGet]
        public ActionResult<List<DownloadClient>> GetAll()
        {
            return Ok(_clients);
        }

        [HttpGet("{id}")]
        public ActionResult<DownloadClient> GetById(int id)
        {
            var client = _clients.FirstOrDefault(c => c.Id == id);
            if (client == null)
            {
                return NotFound();
            }
            return Ok(client);
        }

        [HttpPost]
        public ActionResult<DownloadClient> Create([FromBody] DownloadClient client)
        {
            client.Id = _clients.Any() ? _clients.Max(c => c.Id) + 1 : 1;
            _clients.Add(client);
            _configService.SaveDownloadClients(_clients);
            return CreatedAtAction(nameof(GetById), new { id = client.Id }, client);
        }

        [HttpPut("{id}")]
        public ActionResult<DownloadClient> Update(int id, [FromBody] DownloadClient client)
        {
            var existingClient = _clients.FirstOrDefault(c => c.Id == id);
            if (existingClient == null)
            {
                return NotFound();
            }

            existingClient.Name = client.Name;
            existingClient.Implementation = client.Implementation;
            existingClient.Host = client.Host;
            existingClient.Port = client.Port;
            existingClient.Username = client.Username;
            existingClient.Password = client.Password;
            existingClient.Category = client.Category;
            existingClient.UrlBase = client.UrlBase;
            existingClient.Enable = client.Enable;
            existingClient.Priority = client.Priority;

            _configService.SaveDownloadClients(_clients);

            return Ok(existingClient);
        }

        [HttpDelete("{id}")]
        public ActionResult Delete(int id)
        {
            var client = _clients.FirstOrDefault(c => c.Id == id);
            if (client == null)
            {
                return NotFound();
            }

            _clients.Remove(client);
            _configService.SaveDownloadClients(_clients);
            return NoContent();
        }

        [HttpPost("test")]
        public async Task<ActionResult> TestConnection([FromBody] TestDownloadClientRequest request)
        {
            try
            {
                bool isConnected = false;
                string version = string.Empty;

                Console.WriteLine($"[DownloadClient] Testing {request.Implementation} at {request.Host}:{request.Port}");

                if (request.Implementation.Equals("qBittorrent", StringComparison.OrdinalIgnoreCase))
                {
                    var qbClient = new QBittorrentClient(
                        request.Host,
                        request.Port,
                        request.Username ?? string.Empty,
                        request.Password ?? string.Empty,
                        request.UrlBase
                    );

                    isConnected = await qbClient.TestConnectionAsync();
                    if (isConnected)
                    {
                        version = await qbClient.GetVersionAsync();
                    }
                }
                else if (request.Implementation.Equals("Transmission", StringComparison.OrdinalIgnoreCase))
                {
                    var transmissionClient = new TransmissionClient(
                        request.Host,
                        request.Port,
                        request.Username ?? string.Empty,
                        request.Password ?? string.Empty
                    );

                    isConnected = await transmissionClient.TestConnectionAsync();
                    if (isConnected)
                    {
                        version = await transmissionClient.GetVersionAsync();
                    }
                }
                else
                {
                    return BadRequest(new { message = $"Unsupported download client: {request.Implementation}" });
                }

                return Ok(new
                {
                    connected = isConnected,
                    version = version,
                    message = isConnected ? "Connection successful" : "Connection failed"
                });
            }
            catch (Exception ex)
            {
                return Ok(new
                {
                    connected = false,
                    message = $"Connection failed: {ex.Message}"
                });
            }
        }

        [HttpPost("add")]
        public async Task<ActionResult> AddTorrent([FromBody] AddTorrentRequest request)
        {
            try
            {
                Console.WriteLine($"[DownloadClient] Attempting to add torrent: {request.Url}");
                
                // Sort by Priority (lower is better, assuming 1 is highest priority)
                // If priorities are equal, use ID (assuming newer clients might be preferred or just stable sort)
                var client = _clients.Where(c => c.Enable).OrderBy(c => c.Priority).ThenBy(c => c.Id).FirstOrDefault();
                
                if (client == null)
                {
                    Console.WriteLine("[DownloadClient] No enabled download client found");
                    return BadRequest(new { message = "No enabled download client found. Please check your settings." });
                }

                if (client.Implementation.Equals("qBittorrent", StringComparison.OrdinalIgnoreCase))
                {
                    var qbClient = new QBittorrentClient(
                        client.Host,
                        client.Port,
                        client.Username ?? string.Empty,
                        client.Password ?? string.Empty,
                        client.UrlBase
                    );

                    bool success = await qbClient.AddTorrentAsync(request.Url, client.Category);
                    if (success)
                    {
                        Console.WriteLine("[DownloadClient] Successfully added torrent to qBittorrent");
                        return Ok(new { message = "Torrent added successfully to qBittorrent" });
                    }
                    else
                    {
                        Console.WriteLine("[DownloadClient] Failed to add torrent to qBittorrent");
                        return StatusCode(500, new { message = "Failed to add torrent to qBittorrent" });
                    }
                }
                else if (client.Implementation.Equals("Transmission", StringComparison.OrdinalIgnoreCase))
                {
                    var transmissionClient = new TransmissionClient(
                        client.Host,
                        client.Port,
                        client.Username ?? string.Empty,
                        client.Password ?? string.Empty
                    );

                    bool success = await transmissionClient.AddTorrentAsync(request.Url, client.Category);
                    if (success)
                    {
                        Console.WriteLine("[DownloadClient] Successfully added torrent to Transmission");
                        return Ok(new { message = "Torrent added successfully to Transmission" });
                    }
                    else
                    {
                        Console.WriteLine("[DownloadClient] Failed to add torrent to Transmission");
                        return StatusCode(500, new { message = "Failed to add torrent to Transmission" });
                    }
                }
                
                return BadRequest(new { message = $"Unsupported download client: {client.Implementation}" });
            }
            catch (Exception ex)
            {
                Console.WriteLine($"[DownloadClient] Error adding torrent: {ex.Message}");
                return StatusCode(500, new { message = $"Error adding torrent: {ex.Message}" });
            }
        }
    }

    public class AddTorrentRequest
    {
        public string Url { get; set; } = string.Empty;
    }

    public class TestDownloadClientRequest
    {
        public string Implementation { get; set; } = string.Empty;
        public string Host { get; set; } = string.Empty;
        public int Port { get; set; }
        public string? Username { get; set; }
        public string? Password { get; set; }
        public string? UrlBase { get; set; }
    }
}
