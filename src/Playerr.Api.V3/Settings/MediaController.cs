using Microsoft.AspNetCore.Mvc;
using Playerr.Core.Configuration;
using Playerr.Core.Games;
using System;
using System.Threading.Tasks;
using System.Diagnostics.CodeAnalysis;

namespace Playerr.Api.V3.Settings
{
    [ApiController]
    [Route("api/v3/[controller]")]
    [SuppressMessage("Microsoft.Design", "CA1034:NestedTypesShouldNotBeVisible")]
    public class MediaController : ControllerBase
    {
        private readonly ConfigurationService _configService;
        private readonly MediaScannerService _scannerService;

        public MediaController(ConfigurationService configService, MediaScannerService scannerService)
        {
            _configService = configService;
            _scannerService = scannerService;
        }

        [HttpGet]
        public IActionResult GetSettings()
        {
            return Ok(_configService.LoadMediaSettings());
        }

        [HttpPost]
        public IActionResult SaveSettings([FromBody] MediaSettings settings)
        {
            _configService.SaveMediaSettings(settings);
            return Ok(new { message = "Media settings saved successfully" });
        }

        public class ScanRequest
        {
            [System.Text.Json.Serialization.JsonPropertyName("folderPath")]
            public string? FolderPath { get; set; }

            [System.Text.Json.Serialization.JsonPropertyName("platform")]
            public string? Platform { get; set; }
        }

        [HttpPost("scan")]
        public IActionResult TriggerScan([FromBody] ScanRequest? request = null)
        {
            // Validate IGDB credentials before starting
            var igdbSettings = _configService.LoadIgdbSettings();
            if (!igdbSettings.IsConfigured)
            {
                return BadRequest(new { 
                    success = false, 
                    errorCode = "IGDB_NOT_CONFIGURED",
                    message = "IGDB credentials are required for scanning. Please configure them in the Metadata section." 
                });
            }

            Console.WriteLine($"TriggerScan received. FolderPath: '{request?.FolderPath}', Platform: '{request?.Platform}'");
            // Run scan in background to avoid timeouts
            Task.Run(async () => 
            {
                try
                {
                    await _scannerService.ScanAsync(request?.FolderPath, request?.Platform);
                }
                catch (Exception ex)
                {
                    Console.WriteLine($"Background scan error: {ex}");
                }
            });

            return Ok(new { message = "Scan started in background. Check library in a few minutes." });
        }

        [HttpPost("scan/stop")]
        public IActionResult StopScan()
        {
            _scannerService.StopScan();
            return Ok(new { message = "Scan stopping. Check status bar." });
        }

        [HttpDelete("clean")]
        public async Task<IActionResult> CleanLibrary()
        {
            await _scannerService.CleanLibraryAsync();
            return Ok(new { message = "Library cleaned successfully." });
        }

        [HttpGet("scan/status")]
        public IActionResult GetScanStatus()
        {
            return Ok(new
            {
                isScanning = _scannerService.IsScanning,
                lastGameFound = _scannerService.LastGameFound,
                gamesAddedCount = _scannerService.GamesAddedCount
            });
        }
    }
}
