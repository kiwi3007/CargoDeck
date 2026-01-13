using System.Collections.Generic;
using System.Linq;
using System.Threading.Tasks;
using Playerr.Core.Games;

namespace Playerr.Core.Launcher
{
    public interface ILauncherService
    {
        Task LaunchGameAsync(Game game);
    }

    public class LauncherService : ILauncherService
    {
        private readonly IEnumerable<ILaunchStrategy> _strategies;

        public LauncherService(IEnumerable<ILaunchStrategy> strategies)
        {
            _strategies = strategies;
        }

        public async Task LaunchGameAsync(Game game)
        {
            var strategy = _strategies.FirstOrDefault(s => s.IsSupported(game));

            if (strategy == null)
            {
                System.Console.WriteLine($"[LauncherService] No suitable launch strategy found for game: {game.Title}");
                throw new System.Exception("No suitable launch strategy found for this game.");
            }

            System.Console.WriteLine($"[LauncherService] Launching game '{game.Title}' using strategy: {strategy.GetType().Name}");
            await strategy.LaunchAsync(game);
        }
    }
}
