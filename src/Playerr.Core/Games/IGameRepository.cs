using System.Collections.Generic;
using System.Threading.Tasks;

namespace Playerr.Core.Games
{
    public interface IGameRepository
    {
        Task<List<Game>> GetAllAsync();
        Task<Game?> GetByIdAsync(int id);
        Task<Game> AddAsync(Game game);
        Task<Game?> UpdateAsync(int id, Game game);
        Task<bool> DeleteAsync(int id);
        Task<int> DeleteSteamGamesAsync();
        Task DeleteAllAsync();
        Task<int?> GetPlatformIdBySlugAsync(string slug);
        Task<HashSet<int>> GetIgdbIdsAsync();
    }
}
