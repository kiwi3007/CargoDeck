namespace Playerr.Core.Prowlarr
{
    public class ProwlarrSettings
    {
        public string Url { get; set; } = "http://localhost:9696";
        public string ApiKey { get; set; } = string.Empty;

        public bool IsConfigured => !string.IsNullOrWhiteSpace(Url) && !string.IsNullOrWhiteSpace(ApiKey);
    }
}
