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
                    _logger.LogInformation($"[DownloadMonitor] Loaded {clients.Count} clients. Checking enabled ones...");

                    foreach (var clientConfig in clients.Where(c => c.Enable))
                    {
                        await MonitorClientAsync(clientConfig, stoppingToken);
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

            if (client == null) return;

            try
            {
                var downloads = await client.GetDownloadsAsync();
                _logger.LogInformation($"[DownloadMonitor] Client {config.Name} returned {downloads.Count} downloads.");
                foreach (var download in downloads)
                {
                    _logger.LogInformation($"[DownloadMonitor] ID: {download.Id}, Name: {download.Name}, State: {download.State}, Path: {download.DownloadPath}");
                    if (download.State == DownloadState.Completed)
                    {
                        if (!_processedDownloadIds.Contains(download.Id))
                        {
                            _logger.LogInformation($"[DownloadMonitor] Found completed download: {download.Name} at {download.DownloadPath}");

                            if (!string.IsNullOrEmpty(config.RemotePathMapping) && !string.IsNullOrEmpty(config.LocalPathMapping))
                            {
                                var remote = config.RemotePathMapping;
                                var local = config.LocalPathMapping;

                                _logger.LogInformation($"[DownloadMonitor] Checking mapping: Remote='{remote}', Local='{local}' against Path='{download.DownloadPath}'");

                                if (download.DownloadPath.StartsWith(remote))
                                {
                                    var relative = download.DownloadPath.Substring(remote.Length).TrimStart('/', '\\');
                                    var newPath = System.IO.Path.Combine(local, relative);
                                    _logger.LogInformation($"[DownloadMonitor] Mapping path: {download.DownloadPath} -> {newPath}");
                                    download.DownloadPath = newPath;
                                }
                                else
                                {
                                    _logger.LogWarning($"[DownloadMonitor] Download path '{download.DownloadPath}' does not start with remote mapping '{remote}'");
                                }
                            }
                            else
                            {
                                _logger.LogInformation("[DownloadMonitor] No path mapping configured for this client.");
                            }

                            // Start of user's requested change
                            _logger.LogInformation($"[DownloadMonitor] Detected completed download: {download.Name}");
                            try
                            {
                                _importStatus.MarkImporting(download.Id);
                                await _postDownloadProcessor.ProcessCompletedDownloadAsync(download);
                                _processedDownloadIds.Add(download.Id); // Changed from _processedDownloads.Add(uniqueId) to _processedDownloadIds.Add(download.Id) for correctness
                                _logger.LogInformation($"[DownloadMonitor] Successfully processed download: {download.Name}");
                            }
                            catch (Exception ex)
                            {
                                _logger.LogError(ex, $"[DownloadMonitor] Error processing download {download.Name}: {ex.Message}"); // Changed from Console.WriteLine to _logger.LogError
                            }
                            finally
                            {
                                _importStatus.MarkFinished(download.Id);
                            }
                            // End of user's requested change
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
