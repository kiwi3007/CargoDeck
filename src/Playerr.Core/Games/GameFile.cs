using System;
using System.Collections.Generic;

namespace Playerr.Core.Games
{
    public class GameFile
    {
        public int Id { get; set; }
        public int GameId { get; set; }
        public Game? Game { get; set; }
        public string RelativePath { get; set; } = string.Empty;
        public long Size { get; set; }
        public DateTime DateAdded { get; set; }
        public string? Quality { get; set; }
        public string? ReleaseGroup { get; set; }
        public string? Edition { get; set; }
        public List<string> Languages { get; set; } = new();
    }
}
