using System.Diagnostics.CodeAnalysis;

namespace Playerr.Core.Jackett
{
    public class JackettSettings
    {
        [SuppressMessage("Microsoft.Design", "CA1056:UriPropertiesShouldNotBeStrings")]
        public string Url { get; set; } = "http://localhost:9117";
        public string ApiKey { get; set; } = string.Empty;

        public bool IsConfigured => !string.IsNullOrWhiteSpace(Url) && !string.IsNullOrWhiteSpace(ApiKey);
    }
}
