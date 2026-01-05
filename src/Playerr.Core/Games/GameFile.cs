using System;
using System.Collections.Generic;
using System.Diagnostics.CodeAnalysis;

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
        
        [SuppressMessage("Microsoft.Usage", "CA2227:CollectionPropertiesShouldBeReadOnly")]
        [SuppressMessage("Microsoft.Design", "CA1002:DoNotExposeGenericLists")]
        public List<string> Languages { get; set; } = new();
    }
}
