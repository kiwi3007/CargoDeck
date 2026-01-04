using System.Collections.Generic;
using System.Linq;
using System.Threading.Tasks;

namespace Playerr.Core.Games
{
    public class InMemoryGameRepository : IGameRepository
    {
        private readonly List<Game> _games = new();
        private int _nextId = 1;

        public Task<List<Game>> GetAllAsync()
        {
            // Devolver una copia para evitar modificaciones externas
            return Task.FromResult(_games.Select(g => g).ToList());
        }

        public Task<Game?> GetByIdAsync(int id)
        {
            var game = _games.FirstOrDefault(g => g.Id == id);
            return Task.FromResult(game);
        }

        public Task<Game> AddAsync(Game game)
        {
            game.Id = _nextId++;
            _games.Add(game);
            return Task.FromResult(game);
        }

        public Task<Game?> UpdateAsync(int id, Game game)
        {
            var existing = _games.FirstOrDefault(g => g.Id == id);
            if (existing == null)
            {
                return Task.FromResult<Game?>(null);
            }

            game.Id = id;
            var index = _games.IndexOf(existing);
            _games[index] = game;
            return Task.FromResult<Game?>(game);
        }

        public Task<bool> DeleteAsync(int id)
        {
            var existing = _games.FirstOrDefault(g => g.Id == id);
            if (existing == null)
            {
                return Task.FromResult(false);
            }

            _games.Remove(existing);
            return Task.FromResult(true);
        }

        public Task<int> DeleteSteamGamesAsync()
        {
            var steamGames = _games.Where(g => g.SteamId.HasValue && g.SteamId > 0).ToList();
            foreach (var game in steamGames)
            {
                _games.Remove(game);
            }
            return Task.FromResult(steamGames.Count);
        }

        public Task DeleteAllAsync()
        {
            _games.Clear();
            _nextId = 1;
            return Task.CompletedTask;
        }
    }
}
