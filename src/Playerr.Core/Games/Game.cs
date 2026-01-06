using System;
using System.Collections.Generic;
using System.Diagnostics.CodeAnalysis;

namespace Playerr.Core.Games
{
    public class Game
    {
        public int Id { get; set; }
        public string Title { get; set; } = string.Empty;
        public string? AlternativeTitle { get; set; }
        public int Year { get; set; }
        public string? Overview { get; set; }
        public string? Storyline { get; set; }
        public int PlatformId { get; set; }
        public Platform? Platform { get; set; }
        public DateTime Added { get; set; }
        
        // Visual Assets - Similar a Radarr con posters y fanart
        public GameImages Images { get; set; } = new();
        
        [SuppressMessage("Microsoft.Usage", "CA2227:CollectionPropertiesShouldBeReadOnly")]
        [SuppressMessage("Microsoft.Design", "CA1002:DoNotExposeGenericLists")]
        public List<string> Genres { get; set; } = new();
        public string? Developer { get; set; }
        public string? Publisher { get; set; }
        public DateTime? ReleaseDate { get; set; }
        public double? Rating { get; set; } // 0-100 from IGDB
        public int? RatingCount { get; set; }
        
        public GameStatus Status { get; set; }
        public bool Monitored { get; set; }
        public string? Path { get; set; }
        public long? SizeOnDisk { get; set; }
        
        [SuppressMessage("Microsoft.Usage", "CA2227:CollectionPropertiesShouldBeReadOnly")]
        [SuppressMessage("Microsoft.Design", "CA1002:DoNotExposeGenericLists")]
        public List<GameFile> GameFiles { get; set; } = new();
        
        // Metadata IDs
        public int? IgdbId { get; set; }
        public int? SteamId { get; set; }
        public string? GogId { get; set; }
        public string? InstallPath { get; set; }
    }
    
    public class GameImages
    {
        public string? CoverUrl { get; set; }          // Carátula principal
        public string? CoverLargeUrl { get; set; }     // Carátula HD
        public string? BackgroundUrl { get; set; }      // Fondo/Fanart
        [SuppressMessage("Microsoft.Design", "CA1056:UriPropertiesShouldNotBeStrings")]
        public string? BannerUrl { get; set; }         // Banner horizontal
        
        [SuppressMessage("Microsoft.Usage", "CA2227:CollectionPropertiesShouldBeReadOnly")]
        [SuppressMessage("Microsoft.Design", "CA1002:DoNotExposeGenericLists")]
        public List<string> Screenshots { get; set; } = new();  // Screenshots del juego
        
        [SuppressMessage("Microsoft.Usage", "CA2227:CollectionPropertiesShouldBeReadOnly")]
        [SuppressMessage("Microsoft.Design", "CA1002:DoNotExposeGenericLists")]
        public List<string> Artworks { get; set; } = new();     // Arte conceptual
    }

    public enum GameStatus
    {
        TBA,
        Announced,
        Released,
        Downloading,
        Downloaded,
        Missing
    }
}
