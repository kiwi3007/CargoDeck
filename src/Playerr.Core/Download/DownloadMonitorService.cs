using System;
using System.Collections.Generic;
using System.Linq;
using System.Threading;
using System.Threading.Tasks;
using Microsoft.Extensions.Hosting;
using Microsoft.Extensions.Logging;
using Playerr.Core.Configuration;
using System.Diagnostics.CodeAnalysis;

namespace Playerr.Core.Download
{
    [SuppressMessage("Microsoft.Performance", "CA1812:AvoidUninstantiatedInternalClasses")]
    [SuppressMessage("Microsoft.Design", "CA1031:DoNotCatchGeneralExceptionTypes")]
    public class DownloadMonitorService : BackgroundService
    {
        private readonly ConfigurationService _configService;
        private readonly PostDownloadProcessor _postDownloadProcessor;
        private readonly ImportStatusService _importStatus;
        private readonly ILogger<DownloadMonitorService> _logger;
        private readonly HashSet<string> _processedDownloadIds = new HashSet<string>();
        private readonly TimeSpan _checkInterval = TimeSpan.FromSeconds(30);

        public DownloadMonitorService(
            ConfigurationService configService,
            PostDownloadProcessor postDownloadProcessor,
            ImportStatusService importStatus,
            ILogger<DownloadMonitorService> logger)
        {
            _configService = configService;
            _postDownloadProcessor = postDownloadProcessor;
            _importStatus = importStatus;
            _logger = logger;
        }

        protected override async Task ExecuteAsync(CancellationToken stoppingToken)
        {
            _logger.LogInformation("[DownloadMonitor] Service started.");

            while (!stoppingToken.IsCancellationRequested)
            {
                try
                {
                    var settings = _configService.LoadPostDownloadSettings();
                    var clients = _configService.LoadDownloadClients();
                    var enabledClients = clients.Where(c => c.Enable).ToList();

                    if (enabledClients.Any())
                    {
                        _logger.LogDebug($"[DownloadMonitor] Checking {enabledClients.Count} enabled clients...");
                        foreach (var clientConfig in enabledClients)
                        {
                            await MonitorClientAsync(clientConfig, stoppingToken);
                        }
                    }

                    await Task.Delay(TimeSpan.FromSeconds(settings.MonitorIntervalSeconds), stoppingToken);
                }
                catch (OperationCanceledException) { }
                catch (Exception ex)
                {
                    _logger.LogError(ex, "[DownloadMonitor] Error in monitor loop");
                    await Task.Delay(TimeSpan.FromSeconds(30), stoppingToken); // Back off on error
                }
            }

            _logger.LogInformation("[DownloadMonitor] Service stopping.");
        }

        private async Task MonitorClientAsync(DownloadClient config, CancellationToken ct)
        {
            IDownloadClient? client = null;

            if (config.Implementation.Equals("qBittorrent", StringComparison.OrdinalIgnoreCase))
            {
                client = new QBittorrentClient(config.Host, config.Port, config.Username ?? "", config.Password ?? "", config.UrlBase);
            }
            else if (config.Implementation.Equals("Transmission", StringComparison.OrdinalIgnoreCase))
            {
                client = new TransmissionClient(config.Host, config.Port, config.Username ?? "", config.Password ?? "");
            }
            else if (config.Implementation.Equals("SABnzbd", StringComparison.OrdinalIgnoreCase))
            {
                client = new SabnzbdClient(config.Host, config.Port, config.ApiKey ?? "", config.UrlBase);
            }
            else if (config.Implementation.Equals("NZBGet", StringComparison.OrdinalIgnoreCase))
            {
                client = new NzbgetClient(config.Host, config.Port, config.Username ?? "", config.Password ?? "", config.UrlBase);
            }
            else if (config.Implementation.Equals("Deluge", StringComparison.OrdinalIgnoreCase))
            {
                client = new DelugeClient(config.Host, config.Port, config.Password ?? "", config.UseSsl);
            }
            else if (config.Implementation.Equals("rTorrent", StringComparison.OrdinalIgnoreCase))
            {
                var rtClient = new RTorrentClient(config.Host, config.Port, config.Username ?? "", config.Password ?? "", config.UrlBase);
                rtClient.OnLog = (msg) => _logger.LogInformation(msg);
                client = rtClient;
            }
            else if (config.Implementation.Equals("Flood", StringComparison.OrdinalIgnoreCase))
            {
                var fClient = new FloodClient(config.Host, config.Port, config.Username ?? "", config.Password ?? "", config.UrlBase);
                fClient.OnLog = (msg) => _logger.LogInformation(msg);
                client = fClient;
            }

            if (client == null) return;

            try
            {
                var downloads = await client.GetDownloadsAsync();
                _logger.LogDebug($"[DownloadMonitor] Client {config.Name} returned {downloads.Count} downloads.");

                // Filter by configured category if one is set
                if (!string.IsNullOrWhiteSpace(config.Category))
                {
                    downloads = downloads
                        .Where(d => string.Equals(d.Category, config.Category, StringComparison.OrdinalIgnoreCase))
                        .ToList();
                    _logger.LogDebug($"[DownloadMonitor] After category filter '{config.Category}': {downloads.Count} downloads.");
                }

                foreach (var download in downloads)
                {
                    _logger.LogDebug($"[DownloadMonitor] {download.Name}: {download.State}");
                    if (download.State == DownloadState.Completed && !_processedDownloadIds.Contains(download.Id))
                    {
                        _logger.LogInformation($"[DownloadMonitor] New completed download: {download.Name}");

                        if (!string.IsNullOrEmpty(config.RemotePathMapping) && !string.IsNullOrEmpty(config.LocalPathMapping))
                        {
                            if (download.DownloadPath.StartsWith(config.RemotePathMapping))
                            {
                                var relative = download.DownloadPath.Substring(config.RemotePathMapping.Length).TrimStart('/', '\\');
                                var newPath = System.IO.Path.Combine(config.LocalPathMapping, relative);
                                _logger.LogInformation($"[DownloadMonitor] Path mapped: {download.DownloadPath} -> {newPath}");
                                download.DownloadPath = newPath;
                            }
                            else
                            {
                                _logger.LogWarning($"[DownloadMonitor] Path '{download.DownloadPath}' does not match remote mapping '{config.RemotePathMapping}'");
                            }
                        }

                        try
                        {
                            _importStatus.MarkImporting(download.Id);
                            await _postDownloadProcessor.ProcessCompletedDownloadAsync(download);
                            _processedDownloadIds.Add(download.Id);
                            _logger.LogInformation($"[DownloadMonitor] Processed: {download.Name}");
                        }
                        catch (Exception ex)
                        {
                            _logger.LogError(ex, $"[DownloadMonitor] Error processing {download.Name}: {ex.Message}");
                        }
                        finally
                        {
                            _importStatus.MarkFinished(download.Id);
                        }
                    }
                }
            }
            catch (Exception ex)
            {
                _logger.LogError(ex, $"[DownloadMonitor] Error monitoring {config.Name} ({config.Implementation})");
            }
        }
    }
}
