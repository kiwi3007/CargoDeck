using System.Threading.Tasks;
using Playerr.Core.Games;

namespace Playerr.Core.Launcher
{
    public interface ILaunchStrategy
    {
        bool IsSupported(Game game);
        Task LaunchAsync(Game game, string? overridePath = null);
    }
}
