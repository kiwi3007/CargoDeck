using System.Text.Json.Serialization;

namespace Playerr.Core.Indexers
{
    public class HydraConfiguration
    {
        public int Id { get; set; }
        public string Name { get; set; } = "Hydra Source";
        public string Url { get; set; } = string.Empty;
        public bool Enabled { get; set; } = true;
    }
}
