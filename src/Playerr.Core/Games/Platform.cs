using System.Collections.Generic;
using System.Diagnostics.CodeAnalysis;

namespace Playerr.Core.Games
{
    public class Platform
    {
        public int Id { get; set; }
        public string Name { get; set; } = string.Empty;
        public string Slug { get; set; } = string.Empty;
        public PlatformType Type { get; set; }
        public string? Icon { get; set; }
        
        [SuppressMessage("Microsoft.Usage", "CA2227:CollectionPropertiesShouldBeReadOnly")]
        [SuppressMessage("Microsoft.Design", "CA1002:DoNotExposeGenericLists")]
        public List<Game> Games { get; set; } = new();
    }

    public enum PlatformType
    {
        PC,
        MacOS,
        PlayStation,
        PlayStation2,
        PlayStation3,
        PlayStation4,
        PlayStation5,
        PSP,
        PSVita,
        Xbox,
        Xbox360,
        XboxOne,
        XboxSeriesX,
        Nintendo64,
        GameCube,
        Wii,
        WiiU,
        Switch,
        GameBoy,
        GameBoyAdvance,
        NintendoDS,
        Nintendo3DS,
        SegaGenesis,
        DreamCast,
        Arcade,
        Other
    }
}
