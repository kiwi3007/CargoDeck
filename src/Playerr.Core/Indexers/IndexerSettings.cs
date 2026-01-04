using System.Collections.Generic;

namespace Playerr.Core.Indexers
{
    public class IndexerSettings
    {
        public int Id { get; set; }
        public string ProwlarrUrl { get; set; } = string.Empty;
        public string ApiKey { get; set; } = string.Empty;
        public bool SyncEnabled { get; set; } = true;
        public List<int> EnabledIndexers { get; set; } = new();
    }
}
