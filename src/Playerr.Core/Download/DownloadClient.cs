namespace Playerr.Core.Download
{
    public class DownloadClient
    {
        public int Id { get; set; }
        public string Name { get; set; } = string.Empty;
        public string Implementation { get; set; } = string.Empty; // qBittorrent, Transmission, etc
        public string Host { get; set; } = string.Empty;
        public int Port { get; set; }
        public string? Username { get; set; }
        public string? Password { get; set; }
        public string? Category { get; set; }
        public string? UrlBase { get; set; }
        public string? ApiKey { get; set; }
        public bool Enable { get; set; } = true;
        public int Priority { get; set; }
    }
}
